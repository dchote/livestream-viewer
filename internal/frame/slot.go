package frame

import (
	"sync"
	"sync/atomic"
	"time"
)

// QueueDepth is how many decoded frames a slot holds for presentation. The
// renderer needs enough lookahead to see which frame is due next; past a few
// frames a deeper queue only buys latency.
const QueueDepth = 6

// poolSize covers the queue plus the buffer the producer is filling and the
// one the consumer still holds, so a free buffer always exists.
const poolSize = QueueDepth + 2

const noIndex = -1

// Slot is a bounded single-producer/single-consumer presentation queue.
//
// It holds a few frames rather than only the newest because the renderer
// schedules presentation against the display clock and has to know which
// frame is due. A newest-only handoff cannot express "not yet": a frame that
// arrives a millisecond early is overwritten by its successor before the
// refresh it belonged to, which is what turns a moving picture juddery.
//
// The queue is bounded and evicts its oldest entry when full, so a renderer
// that falls behind loses frames instead of accruing latency. Freshness still
// beats completeness; the queue only decides *when*, never *whether*.
//
// The mutex covers index bookkeeping. Frame copies happen outside it, so
// neither side ever waits on the other for longer than a few comparisons.
type Slot struct {
	mu    sync.Mutex
	bufs  [poolSize]*Frame
	queue []int // buffer indices, oldest first
	busy  int   // index handed to the consumer, or noIndex
	fill  int   // index the producer is writing, or noIndex
	seq   atomic.Uint64
	drops atomic.Uint64
}

// NewSlot allocates an empty slot. Buffers are created on the first Publish.
func NewSlot() *Slot {
	return &Slot{
		queue: make([]int, 0, QueueDepth),
		busy:  noIndex,
		fill:  noIndex,
	}
}

// Publish copies src into a free buffer and queues it. When the queue is full
// the oldest frame is dropped.
func (s *Slot) Publish(src *Frame) {
	if src == nil {
		return
	}
	s.mu.Lock()
	idx := s.reserveLocked()
	s.fill = idx
	dst := s.bufs[idx]
	s.mu.Unlock()

	// The reserved buffer is neither queued nor held by the consumer, so it
	// can be filled without the lock.
	if dst == nil || dst.Width != src.Width || dst.Height != src.Height || dst.Format != src.Format {
		dst = cloneAlloc(src)
	}
	copyFrame(dst, src)
	dst.Seq = s.seq.Add(1)
	if dst.Received.IsZero() {
		dst.Received = time.Now()
	}

	s.mu.Lock()
	s.bufs[idx] = dst
	s.fill = noIndex
	if len(s.queue) == QueueDepth {
		copy(s.queue, s.queue[1:])
		s.queue = s.queue[:QueueDepth-1]
		s.drops.Add(1)
	}
	s.queue = append(s.queue, idx)
	s.mu.Unlock()
}

// reserveLocked picks a buffer the consumer cannot be reading.
func (s *Slot) reserveLocked() int {
	for i := 0; i < poolSize; i++ {
		if i == s.busy || i == s.fill || s.queuedLocked(i) {
			continue
		}
		return i
	}
	return 0 // unreachable: the pool outsizes the queue plus busy plus fill
}

func (s *Slot) queuedLocked(idx int) bool {
	for _, q := range s.queue {
		if q == idx {
			return true
		}
	}
	return false
}

// Peek reports the PTS of the oldest queued frame without consuming it.
func (s *Slot) Peek() (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return 0, false
	}
	return s.bufs[s.queue[0]].PTS, true
}

// Advance pops the oldest queued frame. The returned pointer is owned by the
// slot; the consumer must not retain it past the next Advance or Take.
func (s *Slot) Advance() *Frame {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.popLocked()
}

func (s *Slot) popLocked() *Frame {
	if len(s.queue) == 0 {
		return nil
	}
	idx := s.queue[0]
	copy(s.queue, s.queue[1:])
	s.queue = s.queue[:len(s.queue)-1]
	s.busy = idx
	return s.bufs[idx]
}

// Take drains the queue and returns the newest frame. It is the untimed path,
// for callers with no display clock to schedule against.
//
// Take is not "consume": with the queue empty it keeps returning the frame the
// consumer already holds. Compare Frame.Seq to tell a fresh frame from a repeat.
func (s *Slot) Take() *Frame {
	s.mu.Lock()
	defer s.mu.Unlock()
	var newest *Frame
	for {
		f := s.popLocked()
		if f == nil {
			break
		}
		newest = f
	}
	if newest != nil {
		return newest
	}
	if s.busy == noIndex {
		return nil
	}
	return s.bufs[s.busy]
}

// Depth is the number of frames queued for presentation.
func (s *Slot) Depth() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// Dropped counts frames evicted because the consumer fell behind.
func (s *Slot) Dropped() uint64 {
	return s.drops.Load()
}

func cloneAlloc(src *Frame) *Frame {
	return allocYUV(src.Width, src.Height, src.Format, 0)
}

// Prepare reserves a pool buffer the producer can write into. The mutex is
// not held across the copy: fill in Packed() then Commit. Abort if the copy
// fails. One Prepare at a time.
func (s *Slot) Prepare(w, h int, format Format, packedMin int) *Frame {
	s.mu.Lock()
	idx := s.reserveLocked()
	s.fill = idx
	dst := s.bufs[idx]
	s.mu.Unlock()
	if dst == nil || dst.Width != w || dst.Height != h || dst.Format != format || packedCap(dst) < packedMin {
		dst = allocYUV(w, h, format, packedMin)
		s.mu.Lock()
		s.bufs[idx] = dst
		s.mu.Unlock()
	}
	return dst
}

// Commit queues the buffer reserved by Prepare.
func (s *Slot) Commit(pts time.Duration, color Color) {
	s.mu.Lock()
	idx := s.fill
	s.fill = noIndex
	if idx == noIndex {
		s.mu.Unlock()
		return
	}
	dst := s.bufs[idx]
	dst.PTS = pts
	dst.Color = color
	dst.Received = time.Now()
	dst.Seq = s.seq.Add(1)
	if len(s.queue) == QueueDepth {
		copy(s.queue, s.queue[1:])
		s.queue = s.queue[:QueueDepth-1]
		s.drops.Add(1)
	}
	s.queue = append(s.queue, idx)
	s.mu.Unlock()
}

// Abort releases a Prepare without queueing.
func (s *Slot) Abort() {
	s.mu.Lock()
	s.fill = noIndex
	s.mu.Unlock()
}

func packedCap(f *Frame) int {
	if f == nil || len(f.Planes) == 0 {
		return 0
	}
	return cap(f.Planes[0])
}

// Packed is the contiguous backing store for a pool frame (Y then chroma).
func (f *Frame) Packed() []byte {
	if f == nil || len(f.Planes) == 0 {
		return nil
	}
	return f.Planes[0][:cap(f.Planes[0])]
}

func allocYUV(w, h int, format Format, packedMin int) *Frame {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	ySize := w * h
	if format == FormatI420 {
		cw, ch := (w+1)/2, (h+1)/2
		uSize := cw * ch
		need := ySize + 2*uSize
		if packedMin > need {
			need = packedMin
		}
		buf := make([]byte, need)
		return &Frame{
			Width:   w,
			Height:  h,
			Format:  FormatI420,
			Planes:  [][]byte{buf[:ySize], buf[ySize : ySize+uSize], buf[ySize+uSize : ySize+2*uSize]},
			Strides: []int{w, cw, cw},
			Color:   Color{Space: "bt709", Range: "limited"},
		}
	}
	uvSize := w * ((h + 1) / 2)
	need := ySize + uvSize
	if packedMin > need {
		need = packedMin
	}
	buf := make([]byte, need)
	return &Frame{
		Width:   w,
		Height:  h,
		Format:  FormatNV12,
		Planes:  [][]byte{buf[:ySize], buf[ySize : ySize+uvSize]},
		Strides: []int{w, w},
		Color:   Color{Space: "bt709", Range: "limited"},
	}
}

// newNV12 allocates one contiguous buffer and slices the Y and UV planes out
// of it, so a frame is a single allocation rather than two.
func newNV12(w, h int) *Frame {
	return allocYUV(w, h, FormatNV12, 0)
}

func copyFrame(dst, src *Frame) {
	dst.Width = src.Width
	dst.Height = src.Height
	dst.Format = src.Format
	dst.PTS = src.PTS
	dst.Received = src.Received
	dst.Color = src.Color
	if len(dst.Planes) != len(src.Planes) {
		dst.Planes = make([][]byte, len(src.Planes))
		dst.Strides = make([]int, len(src.Strides))
	}
	for i := range src.Planes {
		need := len(src.Planes[i])
		if len(dst.Planes[i]) < need {
			dst.Planes[i] = make([]byte, need)
		}
		copy(dst.Planes[i][:need], src.Planes[i])
		if i < len(src.Strides) {
			if len(dst.Strides) < i+1 {
				dst.Strides = append(dst.Strides, src.Strides[i])
			} else {
				dst.Strides[i] = src.Strides[i]
			}
		}
	}
}
