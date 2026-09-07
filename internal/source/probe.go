package source

import (
	"context"
	"net/url"
	"time"

	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
)

// Prober runs kind-specific probe operations.
type Prober struct {
	Tools resolver.Tools
}

// Probe inspects a source. Missing tools yield status unavailable, not an error.
func (p Prober) Probe(ctx context.Context, s *model.Source) model.ProbeResult {
	now := time.Now().UTC()
	yt, ffprobe, _ := p.Tools.Available()

	openURL := s.URL
	switch s.Kind {
	case model.KindYouTube:
		if !yt {
			return model.ProbeResult{
				Status:   model.ProbeUnavailable,
				Message:  "yt-dlp is not installed; install it on PATH to probe YouTube sources",
				ProbedAt: now,
			}
		}
		resolved, err := p.Tools.ResolveYouTube(ctx, s.URL)
		if err != nil {
			return model.ProbeResult{Status: model.ProbeError, Message: err.Error(), ProbedAt: now}
		}
		openURL = resolved
	case model.KindRTSP:
		openURL = injectRTSPCredentials(s.URL, s.Username, s.Password)
	}

	if !ffprobe {
		return model.ProbeResult{
			Status:   model.ProbeUnavailable,
			Message:  "ffprobe is not installed; install FFmpeg on PATH to probe codec and resolution",
			ProbedAt: now,
		}
	}
	codec, w, h, fps, err := p.Tools.ProbeOpenURL(ctx, openURL)
	if err != nil {
		return model.ProbeResult{Status: model.ProbeError, Message: err.Error(), ProbedAt: now}
	}
	return model.ProbeResult{
		Status:   model.ProbeOK,
		Codec:    codec,
		Width:    w,
		Height:   h,
		FPS:      fps,
		HWDecode: nil,
		ProbedAt: now,
	}
}

func injectRTSPCredentials(raw, username, password string) string {
	if username == "" && password == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.User = url.UserPassword(username, password)
	return u.String()
}
