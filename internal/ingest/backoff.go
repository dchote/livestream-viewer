package ingest

import (
	"time"

	"github.com/dchote/livestream-viewer/internal/backoff"
)

// Backoff is the reconnect delay used by decode workers.
func Backoff(attempt int, baseMS int) time.Duration {
	return backoff.Delay(attempt, baseMS)
}
