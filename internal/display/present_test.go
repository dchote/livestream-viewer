package display

import (
	"slices"
	"testing"
	"time"

	"github.com/dchote/livestream-viewer/internal/frame"
)

func testFrame(pts time.Duration) *frame.Frame {
	buf := make([]byte, 8*8+8*4)
	return &frame.Frame{
		Width:   8,
		Height:  8,
		Format:  frame.FormatNV12,
		Planes:  [][]byte{buf[:64], buf[64:]},
		Strides: []int{8, 8},
		PTS:     pts,
		Color:   frame.Color{Space: "bt709", Range: "limited"},
	}
}

// cadence runs a source of the given frame period against a display of the
// given refresh period and reports how many refreshes each presented frame
// occupied, in presentation order.
func cadence(t *testing.T, period, vsync time.Duration, jitter []time.Duration, refreshes int) []int {
	t.Helper()
	const origin = 10 * time.Second

	slot := frame.NewSlot()
	var c presentClock
	base := time.Now()

	nextPTS := time.Duration(origin)
	published := 0
	var held []int
	var current uint64

	for i := 0; i < refreshes; i++ {
		now := base.Add(time.Duration(i) * vsync)
		for {
			// The decode thread publishes at media time, give or take the
			// wakeup accuracy of a Go timer.
			arrival := base.Add(nextPTS - origin + jitter[published%len(jitter)])
			if arrival.After(now) {
				break
			}
			slot.Publish(testFrame(nextPTS))
			published++
			nextPTS += period
		}
		if f := c.due(slot, now.Add(vsync), 0); f != nil && f.Seq != current {
			current = f.Seq
			held = append(held, 0)
		}
		if current != 0 {
			held[len(held)-1]++
		}
	}
	return held
}

// A 30fps source on a 60Hz output has exactly one correct answer: every frame
// occupies two refreshes. Anything else is the judder that shows up on moving
// content and hides on a fixed camera.
func TestPresentClockLocksCadence(t *testing.T) {
	t.Parallel()
	jitter := []time.Duration{0, 3 * time.Millisecond, -2 * time.Millisecond, 5 * time.Millisecond, -4 * time.Millisecond}
	held := cadence(t, time.Second/30, time.Second/60, jitter, 600)

	if len(held) < 250 {
		t.Fatalf("presented %d frames over 600 refreshes, want roughly 300", len(held))
	}
	// The first and last entries are truncated by the window.
	for i, n := range held[1 : len(held)-1] {
		if n != 2 {
			t.Fatalf("frame %d occupied %d refreshes, want 2 (cadence %v)", i+1, n, held)
		}
	}
}

// 25fps on 60Hz cannot be even, but it must be the repeating 3:2 pattern
// rather than a random walk.
func TestPresentClockIsStableOnUnevenRatios(t *testing.T) {
	t.Parallel()
	jitter := []time.Duration{0, 2 * time.Millisecond, -3 * time.Millisecond}
	held := cadence(t, time.Second/25, time.Second/60, jitter, 600)

	if len(held) < 200 {
		t.Fatalf("presented %d frames over 600 refreshes, want roughly 250", len(held))
	}
	for i, n := range held[1 : len(held)-1] {
		if n != 2 && n != 3 {
			t.Fatalf("frame %d occupied %d refreshes, want 2 or 3 (cadence %v)", i+1, n, held)
		}
	}
}

// Nothing due means the frame already on screen is still correct. The renderer
// relies on this to skip the upload entirely.
func TestPresentClockHoldsWhenNothingIsDue(t *testing.T) {
	t.Parallel()
	slot := frame.NewSlot()
	var c presentClock
	now := time.Now()

	if c.due(slot, now, 0) != nil {
		t.Fatal("empty slot should present nothing")
	}
	slot.Publish(testFrame(10 * time.Second))
	if f := c.due(slot, now, 0); f != nil {
		t.Fatalf("the first frame is due after the lead, not immediately: %+v", f)
	}
	if f := c.due(slot, now.Add(presentLead), 0); f == nil {
		t.Fatal("the frame should be due once the lead has elapsed")
	}
	if f := c.due(slot, now.Add(presentLead+time.Millisecond), 0); f != nil {
		t.Fatal("nothing new arrived, so nothing should be presented")
	}
}

// A stall resumes at the media time it left off. Chasing the old anchor would
// dump the backlog in one pass, which is the burst this clock exists to stop.
func TestPresentClockResyncsAfterStall(t *testing.T) {
	t.Parallel()
	slot := frame.NewSlot()
	var c presentClock
	base := time.Now()

	slot.Publish(testFrame(10 * time.Second))
	c.due(slot, base, 0) // anchors the clock
	if c.due(slot, base.Add(presentLead), 0) == nil {
		t.Fatal("first frame")
	}

	// Ten seconds of wall time pass with no frames, then the stream resumes
	// where it stopped.
	resume := base.Add(10 * time.Second)
	for i := 1; i <= 4; i++ {
		slot.Publish(testFrame(10*time.Second + time.Duration(i)*time.Second/30))
	}
	f := c.due(slot, resume, 0)
	if f == nil {
		t.Fatal("resync should present a frame")
	}
	if slot.Depth() != 0 {
		t.Fatalf("resync should drain the backlog, %d frames left", slot.Depth())
	}
	// The clock now describes the resumed stream, so the next frame waits
	// its turn instead of being dumped with the rest.
	slot.Publish(testFrame(10*time.Second + 5*time.Second/30))
	if got := c.due(slot, resume.Add(time.Millisecond), 0); got != nil {
		t.Fatalf("clock did not re-anchor: presented %+v immediately after resync", got)
	}
}

// A source with no usable timestamps has nothing to schedule against and must
// degrade to newest-wins rather than stalling.
func TestPresentClockFallsBackWithoutTimestamps(t *testing.T) {
	t.Parallel()
	slot := frame.NewSlot()
	var c presentClock
	now := time.Now()

	slot.Publish(testFrame(-1))
	slot.Publish(testFrame(-1))
	f := c.due(slot, now, 0)
	if f == nil {
		t.Fatal("untimed frames should still be presented")
	}
	if c.set {
		t.Fatal("a clock must not anchor on absent timestamps")
	}
}

// simSource is one tile in a simulated grid: its own stream, its own clock.
type simSource struct {
	slot    *frame.Slot
	clock   presentClock
	period  time.Duration
	stagger time.Duration
	nextPTS time.Duration
	pubs    int
	held    []int
	current uint64
}

// runGrid steps a set of sources through refreshes of the given interval and
// reports, per refresh, how many of them presented a new frame.
func runGrid(srcs []*simSource, vsync time.Duration, refreshes int) []int {
	const origin = 10 * time.Second
	jitter := []time.Duration{0, 2 * time.Millisecond, -time.Millisecond, 4 * time.Millisecond}
	base := time.Now()
	for _, s := range srcs {
		s.nextPTS = origin
	}
	updates := make([]int, refreshes)
	for i := 0; i < refreshes; i++ {
		now := base.Add(time.Duration(i) * vsync)
		for _, s := range srcs {
			for {
				arrival := base.Add(s.nextPTS - origin + jitter[s.pubs%len(jitter)])
				if arrival.After(now) {
					break
				}
				s.slot.Publish(testFrame(s.nextPTS))
				s.pubs++
				s.nextPTS += s.period
			}
			if f := s.clock.due(s.slot, now.Add(vsync), s.stagger); f != nil && f.Seq != s.current {
				s.current = f.Seq
				s.held = append(s.held, 0)
				updates[i]++
			}
			if s.current != 0 {
				s.held[len(s.held)-1]++
			}
		}
	}
	return updates
}

// A grid mixes frame rates and every tile has to hold its own cadence without
// the others perturbing it. The clocks are independent by design: there is no
// common timebase between a YouTube stream and an RTSP camera, and inventing
// one would mean dropping or repeating frames on all but one source.
func TestGridKeepsPerSourceCadence(t *testing.T) {
	t.Parallel()
	const (
		vsync     = time.Second / 60
		refreshes = 900
	)
	cases := []struct {
		name   string
		period time.Duration
		want   []int
	}{
		{"60fps", time.Second / 60, []int{1}},
		{"30fps", time.Second / 30, []int{2}},
		{"15fps", time.Second / 15, []int{4}},
		{"25fps", time.Second / 25, []int{2, 3}},
	}
	srcs := make([]*simSource, len(cases))
	for i, c := range cases {
		srcs[i] = &simSource{
			slot:    frame.NewSlot(),
			period:  c.period,
			stagger: time.Duration(i%staggerSpread) * vsync,
		}
	}
	runGrid(srcs, vsync, refreshes)

	for i, c := range cases {
		held := srcs[i].held
		if len(held) < 3 {
			t.Fatalf("%s presented %d frames", c.name, len(held))
		}
		for j, got := range held[1 : len(held)-1] {
			if !slices.Contains(c.want, got) {
				t.Fatalf("%s frame %d occupied %d refreshes, want one of %v", c.name, j+1, got, c.want)
			}
		}
		// A stable cadence is not enough on its own: a frame quietly skipped
		// would never appear in held at all.
		ideal := float64(refreshes) * float64(vsync) / float64(c.period)
		if diff := float64(len(held)) - ideal; diff > ideal*0.05 || diff < -ideal*0.05 {
			t.Fatalf("%s presented %d frames, want about %.0f", c.name, len(held), ideal)
		}
	}
}

// Independent clocks are right, but their texture uploads land in one shared
// render pass. Sources at the same rate must not all fall due on the same
// refresh, or per-tile timing becomes a whole-wall overrun.
func TestStaggerSpreadsUploadsAcrossRefreshes(t *testing.T) {
	t.Parallel()
	const (
		vsync   = time.Second / 60
		sources = 4
	)
	peak := func(stagger bool) int {
		srcs := make([]*simSource, sources)
		for i := range srcs {
			srcs[i] = &simSource{slot: frame.NewSlot(), period: time.Second / 30}
			if stagger {
				srcs[i].stagger = time.Duration(i%staggerSpread) * vsync
			}
		}
		updates := runGrid(srcs, vsync, 300)
		worst := 0
		for _, n := range updates[60:] { // past the anchoring transient
			if n > worst {
				worst = n
			}
		}
		return worst
	}

	if got := peak(false); got != sources {
		t.Fatalf("unstaggered peak %d, expected all %d sources to converge", got, sources)
	}
	if got := peak(true); got > sources/2 {
		t.Fatalf("staggered peak %d uploads in one refresh, want at most %d", got, sources/2)
	}
}

func TestRefreshPrefersTheDisplayMode(t *testing.T) {
	t.Parallel()
	e := NewEngine(Config{}, nil, nil, nil)
	if e.vsync != defaultVsync {
		t.Fatalf("vsync starts at %v, want %v", e.vsync, defaultVsync)
	}
	e.setRefresh(time.Second / 120)
	if e.vsync != time.Second/120 {
		t.Fatalf("vsync %v, want the reported mode", e.vsync)
	}
	// Our own overruns must not be mistaken for the panel slowing down.
	for i := 0; i < 100; i++ {
		e.observeVsync(time.Second / 20)
	}
	if e.vsync != time.Second/120 {
		t.Fatalf("loop timing overrode the display mode: %v", e.vsync)
	}
	// SDL declining to report keeps the last good value rather than guessing.
	e.setRefresh(0)
	if e.vsync != time.Second/120 {
		t.Fatalf("vsync %v", e.vsync)
	}
}

func TestObserveVsyncIsTheFallback(t *testing.T) {
	t.Parallel()
	e := NewEngine(Config{}, nil, nil, nil)
	for i := 0; i < 200; i++ {
		e.observeVsync(time.Second / 120)
	}
	if d := e.vsync - time.Second/120; d > time.Microsecond || d < -time.Microsecond {
		t.Fatalf("vsync settled at %v, want %v", e.vsync, time.Second/120)
	}
	// A stall is not evidence about the refresh rate.
	before := e.vsync
	e.observeVsync(3 * time.Second)
	e.observeVsync(0)
	if e.vsync != before {
		t.Fatalf("vsync moved to %v on an outlier, want %v", e.vsync, before)
	}
}

func TestMissedRefresh(t *testing.T) {
	t.Parallel()
	e := NewEngine(Config{}, nil, nil, nil)
	e.setRefresh(time.Second / 60)
	if e.missedRefresh(time.Second / 60) {
		t.Fatal("holding the refresh is not a miss")
	}
	if e.missedRefresh(time.Second/60 + time.Millisecond) {
		t.Fatal("ordinary scheduling noise is not a miss")
	}
	if !e.missedRefresh(2 * time.Second / 60) {
		t.Fatal("a doubled loop period is a missed vblank")
	}
}

// Each source is anchored a refresh further along than the last, and a clock
// that is already running keeps the offset it was given.
func TestStaggerCyclesAndIsStable(t *testing.T) {
	t.Parallel()
	e := NewEngine(Config{}, nil, nil, nil)
	e.setRefresh(time.Second / 60)
	var got []time.Duration
	for i := 0; i < staggerSpread+2; i++ {
		got = append(got, e.stagger(&presentClock{}))
	}
	for i, d := range got {
		if want := time.Duration(i%staggerSpread) * e.vsync; d != want {
			t.Fatalf("source %d staggered by %v, want %v", i, d, want)
		}
	}
	if d := e.stagger(&presentClock{set: true}); d != 0 {
		t.Fatalf("an anchored clock must not be re-staggered, got %v", d)
	}
}
