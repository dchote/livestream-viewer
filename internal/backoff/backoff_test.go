package backoff

import (
	"math"
	"testing"
	"time"
)

func TestDelayCaps(t *testing.T) {
	small := Delay(0, 1000)
	if small < time.Second || small > 2*time.Second {
		t.Fatalf("attempt 0 = %v", small)
	}
	capped := Delay(20, 1000)
	if capped < 30*time.Second || capped > 40*time.Second {
		t.Fatalf("capped = %v", capped)
	}
}

// A source that is unreachable for hours drives attempt arbitrarily high. The
// delay must stay in range instead of overflowing to a panic or a zero delay.
func TestDelayStaysBoundedForHighAttempts(t *testing.T) {
	for _, attempt := range []int{5, 30, 52, 53, 54, 64, 1000, math.MaxInt} {
		for _, base := range []int{0, -1, 1000, 2000, 60_000} {
			d := Delay(attempt, base)
			if d <= 0 || d > 40*time.Second {
				t.Fatalf("attempt=%d base=%d gave %v", attempt, base, d)
			}
		}
	}
}

func TestDelayGrowsThenPlateaus(t *testing.T) {
	prev := time.Duration(0)
	for attempt := 0; attempt < 6; attempt++ {
		d := Delay(attempt, 1000)
		if d < prev {
			t.Fatalf("attempt %d (%v) shorter than previous (%v)", attempt, d, prev)
		}
		prev = d
	}
}
