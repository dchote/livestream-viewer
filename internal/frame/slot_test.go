package frame

import (
	"sync"
	"testing"
	"time"
)

func TestSlotNewestWins(t *testing.T) {
	s := NewSlot()
	if s.Take() != nil {
		t.Fatal("empty slot")
	}
	a := newNV12(8, 8)
	a.PTS = time.Millisecond
	s.Publish(a)
	got := s.Take()
	if got == nil || got.PTS != time.Millisecond {
		t.Fatalf("got %+v", got)
	}
	b := newNV12(8, 8)
	b.PTS = 2 * time.Millisecond
	s.Publish(b)
	c := newNV12(8, 8)
	c.PTS = 3 * time.Millisecond
	s.Publish(c)
	got = s.Take()
	if got.PTS != 3*time.Millisecond {
		t.Fatalf("want newest PTS, got %v", got.PTS)
	}
}

// Take re-returns the newest frame until something newer is published, so the
// renderer identifies a repeat by Seq. Without that it re-uploads the same
// image to the GPU on every vsync.
func TestSlotSeqIdentifiesRepeats(t *testing.T) {
	s := NewSlot()
	a := newNV12(8, 8)
	s.Publish(a)
	first := s.Take()
	if first.Seq == 0 {
		t.Fatal("published frames must carry a sequence")
	}
	if again := s.Take(); again.Seq != first.Seq {
		t.Fatalf("repeat Take changed Seq: %d -> %d", first.Seq, again.Seq)
	}
	s.Publish(newNV12(8, 8))
	if next := s.Take(); next.Seq <= first.Seq {
		t.Fatalf("new frame Seq %d did not advance past %d", next.Seq, first.Seq)
	}
}

func TestSlotRace(t *testing.T) {
	s := NewSlot()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			f := newNV12(16, 16)
			f.PTS = time.Duration(i+1) * time.Millisecond
			s.Publish(f)
		}
	}()
	var last time.Duration
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			f := s.Take()
			if f == nil {
				continue
			}
			if f.PTS < last {
				t.Errorf("PTS went backwards %v -> %v", last, f.PTS)
			}
			last = f.PTS
			if f.Width != 16 || len(f.Planes) != 2 {
				t.Errorf("torn frame %+v", f)
			}
		}
	}()
	wg.Wait()
}

// The slot allocates its buffers once and resizes only when the stream's
// geometry changes, so a long-running source must not grow the heap.
func TestSlotReusesBuffers(t *testing.T) {
	s := NewSlot()
	src := newNV12(32, 32)
	s.Publish(src)
	first := s.Take()

	for i := 0; i < 100; i++ {
		s.Publish(src)
		if got := s.Take(); got == nil {
			t.Fatal("expected a frame")
		}
	}
	seen := map[*Frame]bool{}
	for _, b := range s.bufs {
		if b != nil {
			seen[b] = true
		}
	}
	if len(seen) > poolSize {
		t.Fatalf("slot holds %d buffers, want at most %d", len(seen), poolSize)
	}
	if !seen[first] {
		t.Fatal("the slot should have reused its original buffers")
	}

	// A geometry change must resize in place rather than accumulate.
	s.Publish(newNV12(64, 64))
	got := s.Take()
	if got == nil || got.Width != 64 || got.Height != 64 {
		t.Fatalf("resize failed: %+v", got)
	}
	if len(s.bufs) != poolSize {
		t.Fatalf("slot grew to %d buffers", len(s.bufs))
	}
}

// The renderer schedules against the display clock, so it has to be able to
// look at what is due without consuming it.
func TestSlotPeekAndAdvanceInOrder(t *testing.T) {
	s := NewSlot()
	for i := 1; i <= 3; i++ {
		f := newNV12(8, 8)
		f.PTS = time.Duration(i) * time.Millisecond
		s.Publish(f)
	}
	if s.Depth() != 3 {
		t.Fatalf("depth %d, want 3", s.Depth())
	}
	pts, ok := s.Peek()
	if !ok || pts != time.Millisecond {
		t.Fatalf("peek = %v %v, want the oldest frame", pts, ok)
	}
	if again, _ := s.Peek(); again != pts {
		t.Fatal("Peek consumed a frame")
	}
	for i := 1; i <= 3; i++ {
		f := s.Advance()
		if f == nil || f.PTS != time.Duration(i)*time.Millisecond {
			t.Fatalf("advance %d returned %+v", i, f)
		}
	}
	if s.Advance() != nil {
		t.Fatal("expected a drained queue")
	}
}

// A renderer that falls behind must lose frames rather than accrue latency.
func TestSlotDropsOldestWhenFull(t *testing.T) {
	s := NewSlot()
	over := 3
	for i := 1; i <= QueueDepth+over; i++ {
		f := newNV12(8, 8)
		f.PTS = time.Duration(i) * time.Millisecond
		s.Publish(f)
	}
	if s.Depth() != QueueDepth {
		t.Fatalf("depth %d, want %d", s.Depth(), QueueDepth)
	}
	if got := s.Dropped(); got != uint64(over) {
		t.Fatalf("dropped %d, want %d", got, over)
	}
	pts, _ := s.Peek()
	if want := time.Duration(over+1) * time.Millisecond; pts != want {
		t.Fatalf("oldest surviving PTS %v, want %v", pts, want)
	}
	if got := s.Take(); got.PTS != time.Duration(QueueDepth+over)*time.Millisecond {
		t.Fatalf("Take should drain to newest, got %v", got.PTS)
	}
}

// The consumer holds a pointer into the pool between calls, so the producer
// must never hand back the buffer it is still reading.
func TestSlotDoesNotReuseHeldBuffer(t *testing.T) {
	s := NewSlot()
	s.Publish(newNV12(8, 8))
	held := s.Advance()
	held.Planes[0][0] = 0xAB
	for i := 0; i < poolSize*4; i++ {
		f := newNV12(8, 8)
		f.Planes[0][0] = 0x11
		s.Publish(f)
	}
	if held.Planes[0][0] != 0xAB {
		t.Fatal("the producer overwrote the buffer the consumer was holding")
	}
}

// Ingest writes packed pixels into the reserved pool buffer. Commit must
// queue that buffer, not copy it; a second copy is the bandwidth we cannot
// afford on the boards this runs on.
func TestSlotPrepareCommitsWithoutCopy(t *testing.T) {
	s := NewSlot()
	dst := s.Prepare(8, 8, FormatNV12, 8*8+8*4)
	buf := dst.Packed()
	buf[0] = 0x42
	s.Commit(time.Millisecond, Color{Space: "bt709", Range: "limited"})
	got := s.Take()
	if got == nil || got.Planes[0][0] != 0x42 {
		t.Fatal("producer write did not land in the queued buffer")
	}
	if &got.Planes[0][0] != &buf[0] {
		t.Fatal("Commit copied instead of queueing the prepared buffer")
	}
}

func TestSlotPrepareAbortDoesNotQueue(t *testing.T) {
	s := NewSlot()
	s.Prepare(8, 8, FormatNV12, 8*8+8*4)
	s.Abort()
	if s.Take() != nil {
		t.Fatal("Abort queued a frame")
	}
}

func TestSlotPrepareI420Layout(t *testing.T) {
	s := NewSlot()
	dst := s.Prepare(8, 8, FormatI420, 8*8+2*4*4)
	if dst.Format != FormatI420 || len(dst.Planes) != 3 || len(dst.Strides) != 3 {
		t.Fatalf("i420 layout %+v", dst)
	}
	if dst.Strides[0] != 8 || dst.Strides[1] != 4 || dst.Strides[2] != 4 {
		t.Fatalf("i420 strides %v", dst.Strides)
	}
	s.Commit(0, Color{Space: "bt709", Range: "limited"})
	got := s.Take()
	if got.Format != FormatI420 || len(got.Planes) != 3 {
		t.Fatalf("queued %+v", got)
	}
}

func TestSlotPrepareDoesNotReuseHeldBuffer(t *testing.T) {
	s := NewSlot()
	dst := s.Prepare(8, 8, FormatNV12, 8*8+8*4)
	dst.Planes[0][0] = 0xAB
	s.Commit(0, Color{})
	held := s.Advance()
	for i := 0; i < poolSize*4; i++ {
		p := s.Prepare(8, 8, FormatNV12, 8*8+8*4)
		p.Packed()[0] = 0x11
		s.Commit(time.Duration(i+1)*time.Millisecond, Color{})
	}
	if held.Planes[0][0] != 0xAB {
		t.Fatal("Prepare reused the buffer the consumer was holding")
	}
}
