package ingest

import (
	"context"
	"time"
)

const defaultSnapBehind = 80 * time.Millisecond

// maxAhead caps a single wait. A PTS discontinuity must not sleep for seconds.
const maxAhead = 500 * time.Millisecond

// realtimePacer stops a decode worker running faster than media time.
//
// Segmented live (YouTube HLS) delivers a whole segment's packets at once.
// Publishing them immediately would overflow the presentation queue, so a
// wall clock on the stream stalls for a segment and then races through it.
// The render loop stays display-clock driven; this only keeps the producer
// from running ahead. Wait, then publish.
type realtimePacer struct {
	origin     time.Time
	first      time.Duration
	set        bool
	snapBehind time.Duration
}

func newPacer(bufferMS int) realtimePacer {
	snap := defaultSnapBehind
	if d := time.Duration(bufferMS) * time.Millisecond; d > snap {
		snap = d
	}
	return realtimePacer{snapBehind: snap}
}

func (p *realtimePacer) reset() {
	p.set = false
}

func (p *realtimePacer) snap() time.Duration {
	if p.snapBehind > 0 {
		return p.snapBehind
	}
	return defaultSnapBehind
}

// wait blocks until pts is due relative to the first frame. A cancelled
// context returns false. Negative timestamps (NOPTS) are a no-op; zero is
// a real media time and starts the clock.
func (p *realtimePacer) wait(ctx context.Context, pts time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	if pts < 0 {
		return true
	}
	now := time.Now()
	if !p.set {
		p.origin = now
		p.first = pts
		p.set = true
		return true
	}
	media := pts - p.first
	if media < 0 {
		p.origin = now
		p.first = pts
		return true
	}
	delay := p.origin.Add(media).Sub(now)
	switch {
	case delay <= 0:
		if delay < -p.snap() {
			p.origin = now
			p.first = pts
		}
		return true
	case delay > maxAhead:
		p.origin = now
		p.first = pts
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
