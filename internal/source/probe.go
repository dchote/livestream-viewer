package source

import (
	"context"
	"time"

	"github.com/dchote/livestream-viewer/internal/ingest"
	"github.com/dchote/livestream-viewer/internal/ingest/capability"
	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
)

// Prober inspects a source with libav (yt-dlp still required for YouTube URLs).
type Prober struct {
	Tools    resolver.Tools
	Caps     capability.Info
	ThumbDir string
}

// Probe inspects a source. YouTube without yt-dlp yields status unavailable.
func (p Prober) Probe(ctx context.Context, s *model.Source) model.ProbeResult {
	if s == nil {
		return model.ProbeResult{Status: model.ProbeError, Message: "source is required", ProbedAt: time.Now().UTC()}
	}
	return ingest.Inspect(ctx, ingest.ProbeInput{
		Source:   s,
		Tools:    p.Tools,
		Caps:     p.Caps,
		ThumbDir: p.ThumbDir,
	})
}
