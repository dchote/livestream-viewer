package ingest

import (
	"context"
	"testing"
	"time"
)

func TestRealtimePacerFirstFrameIsImmediate(t *testing.T) {
	t.Parallel()
	var p realtimePacer
	start := time.Now()
	if !p.wait(context.Background(), 2*time.Second) {
		t.Fatal("first wait")
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("first frame waited %v", time.Since(start))
	}
}

func TestRealtimePacerHoldsToMediaTime(t *testing.T) {
	t.Parallel()
	var p realtimePacer
	if !p.wait(context.Background(), time.Second) {
		t.Fatal("origin")
	}
	start := time.Now()
	if !p.wait(context.Background(), time.Second+80*time.Millisecond) {
		t.Fatal("paced")
	}
	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Fatalf("expected a hold, waited %v", elapsed)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("held too long: %v", elapsed)
	}
}

func TestRealtimePacerSnapsWhenBehind(t *testing.T) {
	t.Parallel()
	p := newPacer(0)
	p.origin = time.Now().Add(-time.Second)
	p.first = time.Second
	p.set = true
	start := time.Now()
	if !p.wait(context.Background(), time.Second+50*time.Millisecond) {
		t.Fatal("snap")
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("behind frame must not catch up by waiting, took %v", time.Since(start))
	}
	if !p.set || p.first != time.Second+50*time.Millisecond {
		t.Fatalf("expected origin snap, first=%v set=%v", p.first, p.set)
	}
}

func TestRealtimePacerAbsorbsJitterInsideBuffer(t *testing.T) {
	t.Parallel()
	p := newPacer(4000)
	p.origin = time.Now().Add(-200 * time.Millisecond)
	p.first = time.Second
	p.set = true
	if !p.wait(context.Background(), time.Second+50*time.Millisecond) {
		t.Fatal("absorb")
	}
	if p.first != time.Second {
		t.Fatalf("must not snap inside the buffer, first=%v", p.first)
	}
}

func TestRealtimePacerResetOnBackwardPTS(t *testing.T) {
	t.Parallel()
	var p realtimePacer
	if !p.wait(context.Background(), 5*time.Second) {
		t.Fatal("origin")
	}
	start := time.Now()
	if !p.wait(context.Background(), time.Second) {
		t.Fatal("loop")
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("backward PTS waited %v", time.Since(start))
	}
}

func TestRealtimePacerCancel(t *testing.T) {
	t.Parallel()
	var p realtimePacer
	if !p.wait(context.Background(), time.Second) {
		t.Fatal("origin")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p.wait(ctx, time.Second+time.Second) {
		t.Fatal("expected cancel")
	}
}

func TestRealtimePacerMissingPTS(t *testing.T) {
	t.Parallel()
	var p realtimePacer
	if !p.wait(context.Background(), -1) {
		t.Fatal("nopts")
	}
	if p.set {
		t.Fatal("missing timestamps must not start a clock")
	}
	if !p.wait(context.Background(), 0) {
		t.Fatal("zero pts is valid")
	}
	if !p.set {
		t.Fatal("zero PTS must start a clock")
	}
}
