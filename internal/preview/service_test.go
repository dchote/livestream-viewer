package preview

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

func seqOf(s *Service) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seq
}

// latest copies out the stored JPEG. Production callers always wait for a
// fresh frame, so the service exposes no accessor for the current one.
func latest(s *Service) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jpeg == nil {
		return nil
	}
	return append([]byte(nil), s.jpeg...)
}

func waitNext(s *Service, timeout time.Duration) []byte {
	return s.WaitNextCtx(context.Background(), timeout)
}

// pushAndWait publishes one frame and blocks until the encoder has stored it.
func pushAndWait(t *testing.T, s *Service, pix []byte, w, h, stride int) {
	t.Helper()
	before := seqOf(s)
	s.PushRGBA(pix, w, h, stride)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if seqOf(s) != before {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("encoder never published the frame")
}

func TestWaitNextIgnoresStale(t *testing.T) {
	s := New()
	pushAndWait(t, s, []byte{1, 2, 3, 4}, 1, 1, 4)
	first := latest(s)
	if first == nil {
		t.Fatal("expected stale jpeg")
	}
	unsub := s.Subscribe()
	defer unsub()
	got := waitNext(s, 50*time.Millisecond)
	if got != nil {
		t.Fatal("an already-published frame must not satisfy a wait")
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		s.PushRGBA([]byte{9, 9, 9, 9}, 1, 1, 4)
	}()
	got = waitNext(s, 500*time.Millisecond)
	if got == nil {
		t.Fatal("expected new frame")
	}
	if len(got) == len(first) && string(got) == string(first) {
		t.Fatal("wait returned the stale jpeg")
	}
}

func TestWaitNextCtxCancel(t *testing.T) {
	s := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	got := s.WaitNextCtx(ctx, 5*time.Second)
	if got != nil {
		t.Fatal("cancelled wait should not return a frame")
	}
	if time.Since(start) > time.Second {
		t.Fatal("WaitNextCtx ignored cancel")
	}
}

func TestWaitNextCloseUnblocks(t *testing.T) {
	s := New()
	start := time.Now()
	done := make(chan []byte, 1)
	go func() {
		done <- s.WaitNextCtx(context.Background(), 5*time.Second)
	}()
	time.Sleep(20 * time.Millisecond)
	s.Close()
	got := <-done
	if got != nil {
		t.Fatal("close should not return a frame")
	}
	if time.Since(start) > time.Second {
		t.Fatal("WaitNextCtx ignored Close")
	}
	if !s.Closed() {
		t.Fatal("expected Closed")
	}
	if waitNext(s, time.Second) != nil {
		t.Fatal("a wait after Close must return immediately")
	}
}

// The render thread pushes read-backs for the life of the process. Encoding
// must not spawn a goroutine per frame, or a slow encoder would pile them up
// without bound.
func TestPushDoesNotLeakGoroutines(t *testing.T) {
	s := New()
	pushAndWait(t, s, make([]byte, 64*64*4), 64, 64, 64*4)

	before := runtime.NumGoroutine()
	for i := 0; i < 500; i++ {
		pix := make([]byte, 64*64*4)
		pix[0] = byte(i)
		s.PushRGBA(pix, 64, 64, 64*4)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		idle := !s.encoding && !s.hasPending
		s.mu.Unlock()
		if idle {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	// Allow the last encoder goroutine to unwind.
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Fatalf("goroutines grew from %d to %d across 500 pushes", before, after)
	}

	s.mu.Lock()
	free := len(s.free)
	s.mu.Unlock()
	if free > maxFreeBuffers {
		t.Fatalf("free buffer list grew to %d", free)
	}
}

// Newest frame wins: pushes that arrive while an encode is in flight replace
// each other rather than queueing.
func TestPushKeepsOnlyNewestPending(t *testing.T) {
	s := New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pix := make([]byte, 32*32*4)
			pix[0] = byte(i)
			s.PushRGBA(pix, 32, 32, 32*4)
		}(i)
	}
	wg.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		idle := !s.encoding && !s.hasPending
		s.mu.Unlock()
		if idle {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	s.mu.Lock()
	pending, encoding, seq := s.hasPending, s.encoding, s.seq
	s.mu.Unlock()
	if pending || encoding {
		t.Fatal("encoder did not settle")
	}
	if seq == 0 || seq > 50 {
		t.Fatalf("published %d frames for 50 pushes", seq)
	}
	if latest(s) == nil {
		t.Fatal("expected a published frame")
	}
}

func TestPushAfterCloseIsDropped(t *testing.T) {
	s := New()
	s.Close()
	s.PushRGBA(make([]byte, 16*16*4), 16, 16, 16*4)
	time.Sleep(20 * time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hasPending || s.encoding {
		t.Fatal("close must drop staged frames")
	}
	if s.seq != 0 {
		t.Fatal("nothing should publish after close")
	}
}
