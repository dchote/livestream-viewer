package backoff

import (
	"math/rand"
	"time"
)

// CapMS is the maximum reconnect delay. A source that has been dead for hours
// should retry at a steady slow rate, not drift towards never retrying.
const CapMS = 30_000

// Delay returns a jittered delay for attempt (0-based).
//
// attempt is unbounded: a permanently unreachable source increments it forever.
// Doubling is therefore done by bounded iteration rather than a shift, which
// would overflow int (32-bit on armv7) and produce a negative delay.
func Delay(attempt int, baseMS int) time.Duration {
	if baseMS <= 0 {
		baseMS = 1000
	}
	if attempt < 0 {
		attempt = 0
	}
	ms := baseMS
	for i := 0; i < attempt && ms < CapMS && ms > 0; i++ {
		ms *= 2
	}
	if ms > CapMS || ms <= 0 {
		ms = CapMS
	}
	jitter := rand.Intn(ms/4 + 1)
	return time.Duration(ms+jitter) * time.Millisecond
}
