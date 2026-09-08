package ingest

import (
	"math"
	"testing"
	"time"
)

func TestBackoffCaps(t *testing.T) {
	small := Backoff(0, 1000)
	if small < time.Second || small > 2*time.Second {
		t.Fatalf("attempt 0 = %v", small)
	}
	capped := Backoff(20, 1000)
	if capped < 30*time.Second || capped > 40*time.Second {
		t.Fatalf("capped = %v", capped)
	}
}

func TestBackoffStaysBoundedForHighAttempts(t *testing.T) {
	for _, attempt := range []int{5, 30, 52, 53, 54, 64, 1000, math.MaxInt} {
		for _, base := range []int{0, -1, 1000, 2000, 60_000} {
			d := Backoff(attempt, base)
			if d <= 0 || d > 40*time.Second {
				t.Fatalf("attempt=%d base=%d gave %v", attempt, base, d)
			}
		}
	}
}

func TestBackoffGrowsThenPlateaus(t *testing.T) {
	prev := time.Duration(0)
	for attempt := 0; attempt < 6; attempt++ {
		d := Backoff(attempt, 1000)
		if d < prev {
			t.Fatalf("attempt %d (%v) shorter than previous (%v)", attempt, d, prev)
		}
		prev = d
	}
}
