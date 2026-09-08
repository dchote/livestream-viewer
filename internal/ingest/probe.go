package ingest

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/dchote/livestream-viewer/internal/ingest/capability"
	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
)

const probeTimeout = 8 * time.Second

// maxProbeMessage bounds what a probe stores. libav and yt-dlp can produce
// long diagnostics and the result is persisted on every source row.
const maxProbeMessage = 512

// userinfoRE matches the credential section of a URL.
var userinfoRE = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://)[^/\s@]*@`)

// probeMessage prepares an error for storage in the database and return over
// the API. RTSP credentials are injected into the open URL, and libav or
// yt-dlp diagnostics sometimes echo that URL back, so userinfo is stripped
// before the string leaves this package.
func probeMessage(msg string) string {
	msg = userinfoRE.ReplaceAllString(msg, "$1***@")
	if len(msg) > maxProbeMessage {
		msg = msg[:maxProbeMessage] + "..."
	}
	return msg
}

// ProbeInput is everything needed to inspect a source with libav.
type ProbeInput struct {
	Source   *model.Source
	Tools    resolver.Tools
	Caps     capability.Info
	ThumbDir string
}

// Inspect probes codec/geometry with libav, writes a JPEG thumbnail, and sets hw_decode from the capability matrix.
func Inspect(ctx context.Context, in ProbeInput) model.ProbeResult {
	now := time.Now().UTC()
	s := in.Source
	if s == nil {
		return model.ProbeResult{Status: model.ProbeError, Message: "source is required", ProbedAt: now}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	openURL := s.URL
	if s.Kind == model.KindYouTube {
		yt, _, _ := in.Tools.Available()
		if !yt {
			return model.ProbeResult{
				Status:   model.ProbeUnavailable,
				Code:     resolver.CodeToolMissing,
				Message:  resolver.ToolMissingMessage,
				ProbedAt: now,
			}
		}
		// ResolveYouTube applies its own deadline. Do not share the libav probe
		// timeout or a slow extract would starve OpenVideo.
		resolved, err := in.Tools.ResolveYouTube(ctx, s.URL)
		if err != nil {
			code, user := resolver.Classify(err.Error())
			msg := user
			if msg == "" {
				msg = probeMessage(err.Error())
			}
			return model.ProbeResult{Status: model.ProbeError, Code: code, Message: msg, ProbedAt: now}
		}
		openURL = resolved
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	opts := OptionsFromSource(s, openURL)
	opts.BufferMS = 0
	sess, err := OpenVideo(ctx, opts, in.Caps, false, true)
	if err != nil {
		return model.ProbeResult{Status: model.ProbeError, Message: probeMessage(err.Error()), ProbedAt: now}
	}
	defer sess.Close()

	cp := sess.Stream.CodecParameters()
	codec := cp.CodecID().Name()
	w, h := cp.Width(), cp.Height()
	fps := sess.Stream.AvgFrameRate().Float64()
	if fps == 0 {
		fps = sess.Stream.RFrameRate().Float64()
	}
	hw := false
	if !s.Options.ForceSoftware {
		hw = hardwareDecodable(ctx, opts, in.Caps, codec)
	}
	hwDecode := &hw

	if err := decodeThumbnail(sess, in.ThumbDir, s.ID); err != nil {
		return model.ProbeResult{
			Status:   model.ProbeOK,
			Codec:    codec,
			Width:    w,
			Height:   h,
			FPS:      fps,
			HWDecode: hwDecode,
			Message:  probeMessage("thumbnail: " + err.Error()),
			ProbedAt: now,
		}
	}

	return model.ProbeResult{
		Status:   model.ProbeOK,
		Codec:    codec,
		Width:    w,
		Height:   h,
		FPS:      fps,
		HWDecode: hwDecode,
		ProbedAt: now,
	}
}

func decodeThumbnail(sess *Session, dir string, id uint) error {
	if dir == "" || id == 0 {
		return nil
	}
	pkt := astiav.AllocPacket()
	defer pkt.Free()
	fr := astiav.AllocFrame()
	defer fr.Free()
	sw := astiav.AllocFrame()
	defer sw.Free()
	rgb := astiav.AllocFrame()
	defer rgb.Free()

	deadline := time.Now().Add(4 * time.Second)
	_ = sess.Format.SeekFrame(sess.Stream.Index(), 0, astiav.NewSeekFlags().Add(astiav.SeekFlagBackward))
	for time.Now().Before(deadline) {
		if err := sess.Format.ReadFrame(pkt); err != nil {
			return err
		}
		if pkt.StreamIndex() != sess.Stream.Index() {
			pkt.Unref()
			continue
		}
		if err := sess.Codec.SendPacket(pkt); err != nil {
			pkt.Unref()
			continue
		}
		pkt.Unref()
		if err := sess.Codec.ReceiveFrame(fr); err != nil {
			continue
		}
		src := fr
		if fr.HardwareFramesContext() != nil {
			if err := fr.TransferHardwareData(sw); err != nil {
				fr.Unref()
				return err
			}
			src = sw
		}
		tw := src.Width()
		if tw > 320 {
			tw = 320
		}
		th := src.Height() * tw / src.Width()
		if th < 1 {
			th = 1
		}
		sws, err := astiav.CreateSoftwareScaleContext(
			src.Width(), src.Height(), src.PixelFormat(),
			tw, th, astiav.PixelFormatRgba,
			astiav.NewSoftwareScaleContextFlags(astiav.SoftwareScaleContextFlagBilinear),
		)
		if err != nil {
			fr.Unref()
			sw.Unref()
			return err
		}
		if err := sws.ScaleFrame(src, rgb); err != nil {
			sws.Free()
			fr.Unref()
			sw.Unref()
			return err
		}
		sws.Free()
		raw, err := packedBytes(rgb)
		if err != nil {
			fr.Unref()
			sw.Unref()
			rgb.Unref()
			return err
		}
		img := &image.NRGBA{
			Pix:    raw,
			Stride: tw * 4,
			Rect:   image.Rect(0, 0, tw, th),
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(dir, fmt.Sprintf("%d.jpg", id))
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
			return err
		}
		fr.Unref()
		sw.Unref()
		rgb.Unref()
		return os.WriteFile(path, buf.Bytes(), 0o644)
	}
	return fmt.Errorf("no video frame")
}

func packedBytes(f *astiav.Frame) ([]byte, error) {
	size, err := f.ImageBufferSize(1)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, size)
	n, err := f.ImageCopyToBuffer(buf, 1)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// hardwareDecodable is true only when this input actually yields a hardware frame.
func hardwareDecodable(ctx context.Context, opts OpenOptions, caps capability.Info, codec string) bool {
	if !caps.Supports(codec) {
		return false
	}
	hwCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	sess, err := OpenVideo(hwCtx, opts, caps, true, false)
	if err != nil {
		return false
	}
	defer sess.Close()
	if !sess.UsingHW {
		return false
	}
	return decodeAtLeastOne(hwCtx, sess)
}

func decodeAtLeastOne(ctx context.Context, sess *Session) bool {
	pkt := astiav.AllocPacket()
	defer pkt.Free()
	fr := astiav.AllocFrame()
	defer fr.Free()
	deadline := time.Now().Add(4 * time.Second)
	gotKey := false
	for ctx.Err() == nil && time.Now().Before(deadline) {
		if err := sess.Format.ReadFrame(pkt); err != nil {
			return false
		}
		if pkt.StreamIndex() != sess.Stream.Index() {
			pkt.Unref()
			continue
		}
		if discardUntilKeyframe(sess.UsingHW, &gotKey, pkt) {
			pkt.Unref()
			continue
		}
		if err := sess.Codec.SendPacket(pkt); err != nil {
			pkt.Unref()
			continue
		}
		pkt.Unref()
		if err := sess.Codec.ReceiveFrame(fr); err != nil {
			continue
		}
		fr.Unref()
		return true
	}
	return false
}
