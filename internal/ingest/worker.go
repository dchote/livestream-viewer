package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/dchote/livestream-viewer/internal/frame"
	"github.com/dchote/livestream-viewer/internal/ingest/capability"
	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/schedule"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
)

// errHWUnusable means the hardware decoder opened but never produced a frame.
var errHWUnusable = errors.New("hardware decode produced no frames")

// errResolve wraps a URL-resolution failure so the worker can tell "the stream
// broke" apart from "we could not even work out what to open".
var errResolve = errors.New("source resolution failed")

const hwGiveUp = 2500 * time.Millisecond

// maxResolveFailures is how many consecutive resolution failures trip the
// circuit breaker. Past that the source is reported failed and retried at the
// backoff ceiling instead of hammering yt-dlp.
const maxResolveFailures = 5

// resolvedURLTTL bounds how long one decode session may run on a resolved URL.
// YouTube manifest URLs carry expire/ip parameters, so a session is recycled
// well inside the expiry window to pick up a fresh URL.
const resolvedURLTTL = 30 * time.Minute

// WorkerConfig is per-source decode policy.
type WorkerConfig struct {
	Source       model.Source
	Slot         *frame.Slot
	Caps         capability.Info
	AllowSW      bool
	UseHW        bool
	BackoffMS    int
	Tools        resolver.Tools
	OnHealth     func(status, code, message string)
	OnHWFallback func()
	ResolveURL   func(ctx context.Context, s *model.Source) (string, error)
	StartDelay   time.Duration
}

// RunWorker demuxes and decodes until ctx is cancelled. LockOSThread is required.
//
// A worker owns exactly one source. Every failure mode here is contained to
// that source: a dead camera must never disturb the other tiles or the render
// loop, and must never end the process.
func RunWorker(ctx context.Context, cfg WorkerConfig) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func() {
		// Backstop only: decodeSession already converts its own panics into
		// errors. Reaching here means the retry loop itself failed, so the
		// source stays down rather than taking the wall with it.
		if rec := recover(); rec != nil {
			slog.Error("decode worker panicked; source is offline until reconfigured",
				"source", cfg.Source.Name, "id", cfg.Source.ID,
				"panic", rec, "stack", string(debug.Stack()))
			setHealth(cfg, schedule.DecoderFailed, "", "")
		}
	}()
	if cfg.Slot == nil {
		return
	}
	if cfg.StartDelay > 0 {
		timer := time.NewTimer(cfg.StartDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			setHealth(cfg, schedule.DecoderOffline, "", "")
			return
		case <-timer.C:
		}
	}
	useHW := cfg.UseHW
	attempt := 0
	resolveFails := 0
	var lastCode, lastMsg string
	for {
		if ctx.Err() != nil {
			setHealth(cfg, schedule.DecoderOffline, "", "")
			return
		}
		switch {
		case resolveFails >= maxResolveFailures:
			setHealth(cfg, schedule.DecoderFailed, lastCode, lastMsg)
		case attempt == 0:
			setHealth(cfg, schedule.DecoderConnecting, lastCode, lastMsg)
		default:
			setHealth(cfg, schedule.DecoderReconnecting, lastCode, lastMsg)
		}
		published, err := runSession(ctx, cfg, useHW)
		if ctx.Err() != nil {
			setHealth(cfg, schedule.DecoderOffline, "", "")
			return
		}
		if published > 0 {
			attempt = 0
			lastCode, lastMsg = "", ""
		}
		if errors.Is(err, errResolve) {
			resolveFails++
			lastCode, lastMsg = resolver.Classify(err.Error())
			if lastMsg == "" {
				lastMsg = err.Error()
			}
			if resolveFails == maxResolveFailures {
				// Circuit break: keep retrying at the ceiling so the source
				// recovers on its own once the operator fixes the cause, but
				// stop reporting it as merely "reconnecting".
				slog.Error("source resolution keeps failing; marking source failed",
					"source", cfg.Source.Name, "id", cfg.Source.ID,
					"failures", resolveFails, "err", err)
			}
		} else if err == nil || published > 0 {
			resolveFails = 0
			if err == nil {
				lastCode, lastMsg = "", ""
			}
		}
		if errors.Is(err, errHWUnusable) && cfg.AllowSW && useHW {
			slog.Warn("hardware decode produced no frames; falling back to software", "source", cfg.Source.Name, "id", cfg.Source.ID)
			useHW = false
			if cfg.OnHWFallback != nil {
				cfg.OnHWFallback()
			}
			continue
		}
		if err != nil {
			slog.Info("decode session ended", "source", cfg.Source.Name, "id", cfg.Source.ID, "err", err)
		}
		delay := Backoff(attempt, cfg.BackoffMS)
		if resolveFails >= maxResolveFailures {
			delay = Backoff(maxResolveFailures*2, cfg.BackoffMS)
		}
		timer := time.NewTimer(delay)
		attempt++
		select {
		case <-ctx.Done():
			timer.Stop()
			setHealth(cfg, schedule.DecoderOffline, "", "")
			return
		case <-timer.C:
		}
	}
}

// runSession contains a decode session's panics. libav is cgo, drivers are
// third-party, and camera bitstreams are hostile input; a panic here becomes an
// ordinary session error so the reconnect loop handles it.
func runSession(ctx context.Context, cfg WorkerConfig, useHW bool) (published int, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("decode session panicked: %v", rec)
			slog.Error("decode session panicked",
				"source", cfg.Source.Name, "id", cfg.Source.ID,
				"panic", rec, "stack", string(debug.Stack()))
		}
	}()
	return decodeSession(ctx, cfg, useHW)
}

func setHealth(cfg WorkerConfig, status, code, message string) {
	if cfg.OnHealth != nil {
		cfg.OnHealth(status, code, message)
	}
}

func resolveOpen(ctx context.Context, cfg WorkerConfig) (OpenOptions, error) {
	s := cfg.Source
	openURL := s.URL
	if cfg.ResolveURL != nil {
		u, err := cfg.ResolveURL(ctx, &s)
		if err != nil {
			return OpenOptions{}, fmt.Errorf("%w: %w", errResolve, err)
		}
		openURL = u
	} else if s.Kind == model.KindYouTube {
		u, err := cfg.Tools.ResolveYouTube(ctx, s.URL)
		if err != nil {
			return OpenOptions{}, fmt.Errorf("%w: %w", errResolve, err)
		}
		openURL = u
	}
	return OptionsFromSource(&s, openURL), nil
}

// needsPeriodicResolve reports whether the source's playable URL expires and
// must be re-resolved by recycling the session.
func needsPeriodicResolve(s model.Source) bool {
	return s.Kind == model.KindYouTube
}

func decodeSession(ctx context.Context, cfg WorkerConfig, useHW bool) (int, error) {
	opts, err := resolveOpen(ctx, cfg)
	if err != nil {
		return 0, err
	}
	sess, err := OpenVideo(ctx, opts, cfg.Caps, useHW, cfg.AllowSW)
	if err != nil {
		return 0, err
	}
	defer sess.Close()

	pkt := astiav.AllocPacket()
	defer pkt.Free()
	decFrame := astiav.AllocFrame()
	defer decFrame.Free()
	swFrame := astiav.AllocFrame()
	defer swFrame.Free()
	nvFrame := astiav.AllocFrame()
	defer nvFrame.Free()

	scaler := nvScaler{}
	defer scaler.close()

	tb := sess.Stream.TimeBase()
	fileLoop := ShouldLoop(cfg.Source)
	started := time.Now()
	expires := needsPeriodicResolve(cfg.Source)
	published := 0
	videoPkts := 0
	recvErrs := 0
	reported := false
	gotKey := false
	pace := shouldPace(cfg.Source)
	pacer := newPacer(opts.BufferMS)

	for ctx.Err() == nil {
		if sess.UsingHW && published == 0 && (recvErrs >= 8 || (videoPkts >= 8 && time.Since(started) > hwGiveUp)) {
			return published, errHWUnusable
		}
		if expires && published > 0 && time.Since(started) > resolvedURLTTL {
			// End cleanly so the retry loop re-resolves before the manifest
			// URL expires. The tile holds its last frame across the reopen.
			slog.Info("recycling session to refresh resolved URL", "source", cfg.Source.Name, "id", cfg.Source.ID)
			return published, nil
		}
		if err := sess.Format.ReadFrame(pkt); err != nil {
			if errors.Is(err, astiav.ErrEof) && fileLoop {
				pkt.Unref()
				_ = sess.Format.SeekFrame(sess.Stream.Index(), 0, astiav.NewSeekFlags().Add(astiav.SeekFlagBackward))
				pacer.reset()
				continue
			}
			pkt.Unref()
			if sess.UsingHW && published == 0 {
				return published, errHWUnusable
			}
			return published, err
		}
		if pkt.StreamIndex() != sess.Stream.Index() {
			pkt.Unref()
			continue
		}
		videoPkts++
		if drop := discardUntilKeyframe(sess.UsingHW, &gotKey, pkt); drop {
			pkt.Unref()
			continue
		}
		sent := false
		for ctx.Err() == nil {
			if err := sess.Codec.SendPacket(pkt); err != nil {
				if errors.Is(err, astiav.ErrEagain) {
					n, recverr, pubErr := drainFrames(ctx, cfg, sess, decFrame, swFrame, nvFrame, &scaler, tb, pace, &pacer, &reported)
					published += n
					recvErrs += recverr
					if pubErr != nil {
						pkt.Unref()
						return published, pubErr
					}
					continue
				}
				pkt.Unref()
				sent = true
				if sess.UsingHW && published == 0 {
					break
				}
				return published, err
			}
			pkt.Unref()
			sent = true
			n, recverr, pubErr := drainFrames(ctx, cfg, sess, decFrame, swFrame, nvFrame, &scaler, tb, pace, &pacer, &reported)
			published += n
			recvErrs += recverr
			if pubErr != nil {
				return published, pubErr
			}
			break
		}
		if !sent {
			pkt.Unref()
		}
	}
	return published, ctx.Err()
}

func drainFrames(ctx context.Context, cfg WorkerConfig, sess *Session, decFrame, swFrame, nvFrame *astiav.Frame, scaler *nvScaler, tb astiav.Rational, pace bool, pacer *realtimePacer, reported *bool) (published, recvErrs int, err error) {
	for ctx.Err() == nil {
		if err := sess.Codec.ReceiveFrame(decFrame); err != nil {
			if errors.Is(err, astiav.ErrEagain) || errors.Is(err, astiav.ErrEof) {
				return published, recvErrs, nil
			}
			recvErrs++
			decFrame.Unref()
			return published, recvErrs, nil
		}
		pts := ptsDuration(decFrame.Pts(), tb)
		if pace {
			if !pacer.wait(ctx, pts) {
				decFrame.Unref()
				return published, recvErrs, ctx.Err()
			}
		}
		if err := publishFrame(cfg.Slot, decFrame, swFrame, nvFrame, scaler, tb); err != nil {
			decFrame.Unref()
			if sess.UsingHW && published == 0 && !*reported {
				return published, recvErrs, nil
			}
			return published, recvErrs, err
		}
		decFrame.Unref()
		published++
		if !*reported {
			*reported = true
			if sess.UsingHW {
				setHealth(cfg, schedule.DecoderHardware, "", "")
			} else {
				setHealth(cfg, schedule.DecoderSoftware, "", "")
			}
			slog.Info("decoded first frame", "source", cfg.Source.Name, "id", cfg.Source.ID, "hw", sess.UsingHW)
		}
	}
	return published, recvErrs, ctx.Err()
}

func publishFrame(slot *frame.Slot, dec, sw, nv *astiav.Frame, scaler *nvScaler, tb astiav.Rational) error {
	src := dec
	if dec.HardwareFramesContext() != nil {
		// Unref unconditionally: a failed transfer can still leave buffers
		// attached, and sw is reused for every frame of the session.
		defer sw.Unref()
		if err := dec.TransferHardwareData(sw); err != nil {
			return fmt.Errorf("hwdownload: %w", err)
		}
		src = sw
	}
	if _, ok := nativeYUV(src.PixelFormat()); !ok {
		defer nv.Unref()
		if err := scaler.scale(src, nv); err != nil {
			return err
		}
		src = nv
	}
	return publishToSlot(slot, src, tb)
}

type nvScaler struct {
	ctx    *astiav.SoftwareScaleContext
	w, h   int
	srcFmt astiav.PixelFormat
}

func (s *nvScaler) close() {
	if s != nil && s.ctx != nil {
		s.ctx.Free()
		s.ctx = nil
	}
}

func (s *nvScaler) scale(src, dst *astiav.Frame) error {
	w, h := src.Width(), src.Height()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("invalid frame size")
	}
	pf := src.PixelFormat()
	if s.ctx == nil || s.w != w || s.h != h || s.srcFmt != pf {
		s.close()
		ctx, err := astiav.CreateSoftwareScaleContext(
			w, h, pf,
			w, h, astiav.PixelFormatNv12,
			astiav.NewSoftwareScaleContextFlags(astiav.SoftwareScaleContextFlagBilinear),
		)
		if err != nil {
			return err
		}
		s.ctx = ctx
		s.w, s.h, s.srcFmt = w, h, pf
	}
	return s.ctx.ScaleFrame(src, dst)
}

// discardUntilKeyframe drops packets a hardware decoder cannot start on.
// VideoToolbox (and most hwaccels) cannot conceal a missing IDR; feeding
// mid-GOP P-frames at RTSP join prints
// "hardware accelerator failed to decode picture" for every picture until
// the next keyframe. Software decode can start anywhere.
func discardUntilKeyframe(usingHW bool, gotKey *bool, pkt *astiav.Packet) bool {
	if pkt == nil || gotKey == nil {
		return true
	}
	if pkt.Flags().Has(astiav.PacketFlagCorrupt) {
		return true
	}
	if !usingHW || *gotKey {
		return false
	}
	if pkt.Flags().Has(astiav.PacketFlagKey) {
		*gotKey = true
		return false
	}
	return true
}
