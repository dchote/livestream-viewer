package preview

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// Service holds the latest JPEG of the composited output.
//
// Exactly one encoder goroutine runs at a time and only the newest pending
// frame is kept, matching the newest-frame-wins rule the rest of the pipeline
// follows. Pixel buffers are recycled so the render thread does not allocate a
// full framebuffer on every read-back.
type Service struct {
	subs   atomic.Int32
	mu     sync.Mutex
	jpeg   []byte
	seq    uint64
	wait   chan struct{}
	closed bool

	pendingPix    []byte
	pendingW      int
	pendingH      int
	pendingStride int
	hasPending    bool
	encoding      bool
	free          [][]byte
}

// maxFreeBuffers caps the recycled pixel buffers. Two is enough for one
// in-flight encode plus one staged frame.
const maxFreeBuffers = 2

// New constructs an empty preview mailbox.
func New() *Service {
	return &Service{wait: make(chan struct{})}
}

// Subscribe increments the subscriber count. Unsubscribe with the returned func.
func (s *Service) Subscribe() (unsub func()) {
	s.subs.Add(1)
	return func() { s.subs.Add(-1) }
}

// Subscribers is the number of active MJPEG clients.
func (s *Service) Subscribers() int {
	n := s.subs.Load()
	if n < 0 {
		return 0
	}
	return int(n)
}

// Running reports whether at least one client is waiting for frames.
func (s *Service) Wanted() bool {
	return s.Subscribers() > 0
}

// PushRGBA stages a frame for JPEG encoding. Called from the render thread, so
// it never blocks on the encoder: if an encode is already in flight the staged
// frame is simply replaced.
func (s *Service) PushRGBA(pix []byte, w, h, stride int) {
	if w <= 0 || h <= 0 || len(pix) == 0 {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	buf := s.takeBufLocked(len(pix))
	copy(buf, pix)
	if s.hasPending {
		s.recycleLocked(s.pendingPix)
	}
	s.pendingPix, s.pendingW, s.pendingH, s.pendingStride = buf, w, h, stride
	s.hasPending = true
	start := !s.encoding
	s.encoding = true
	s.mu.Unlock()
	if start {
		go s.encodeLoop()
	}
}

func (s *Service) takeBufLocked(n int) []byte {
	for i, b := range s.free {
		if cap(b) >= n {
			s.free = append(s.free[:i], s.free[i+1:]...)
			return b[:n]
		}
	}
	return make([]byte, n)
}

func (s *Service) recycleLocked(b []byte) {
	if b == nil || len(s.free) >= maxFreeBuffers {
		return
	}
	s.free = append(s.free, b)
}

// encodeLoop drains staged frames until none is left. Only one runs at a time.
func (s *Service) encodeLoop() {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("preview encoder panicked", "panic", rec, "stack", string(debug.Stack()))
			s.mu.Lock()
			s.encoding = false
			s.mu.Unlock()
		}
	}()
	var scratch bytes.Buffer
	for {
		s.mu.Lock()
		if s.closed || !s.hasPending {
			s.encoding = false
			s.mu.Unlock()
			return
		}
		pix, w, h, stride := s.pendingPix, s.pendingW, s.pendingH, s.pendingStride
		s.pendingPix, s.hasPending = nil, false
		s.mu.Unlock()

		s.encodeOne(&scratch, pix, w, h, stride)
	}
}

func (s *Service) encodeOne(scratch *bytes.Buffer, pix []byte, w, h, stride int) {
	if stride <= 0 {
		stride = w * 4
	}
	scratch.Reset()
	img := &image.RGBA{Pix: pix, Stride: stride, Rect: image.Rect(0, 0, w, h)}
	err := jpeg.Encode(scratch, img, &jpeg.Options{Quality: 70})

	s.mu.Lock()
	s.recycleLocked(pix)
	if err != nil || s.closed {
		s.mu.Unlock()
		return
	}
	// s.jpeg is only ever read under this mutex (readers copy it out), so the
	// backing array can be reused instead of reallocated every frame.
	s.jpeg = append(s.jpeg[:0], scratch.Bytes()...)
	s.seq++
	ch := s.wait
	s.wait = make(chan struct{})
	s.mu.Unlock()
	close(ch)
}

// Close unblocks every WaitNext waiter. Further waits return immediately.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.wait)
}

// Closed reports that the mailbox will not publish again.
func (s *Service) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// WaitNextCtx blocks until a newer JPEG is published, or until ctx is
// cancelled or timeout elapses. It returns nil if nothing new arrived: a
// caller must not fall back on the previous frame, or a stalled encoder would
// look like a live stream.
func (s *Service) WaitNextCtx(ctx context.Context, timeout time.Duration) []byte {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	ch := s.wait
	seq := s.seq
	s.mu.Unlock()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
	case <-ctx.Done():
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.jpeg == nil || s.seq == seq {
		return nil
	}
	return append([]byte(nil), s.jpeg...)
}
