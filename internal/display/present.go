package display

import (
	"time"

	"github.com/dchote/livestream-viewer/internal/frame"
)

// defaultVsync is the assumed refresh interval until the loop has measured one.
const defaultVsync = time.Second / 60

// presentLead is how far behind the arriving stream the clock anchors itself.
// It is the whole reason the queue has anything in it: with no lead the
// renderer shows each frame the instant it is decoded, so the decode thread's
// timer jitter lands directly on the screen and the queue can never absorb it.
// 50ms covers ordinary scheduling jitter and stays inside QueueDepth for any
// frame rate up to 120fps.
const presentLead = 50 * time.Millisecond

// resyncThreshold is how far a source's queued media time may diverge from its
// presentation clock before the clock is re-anchored rather than chased. A
// stream that stalls resumes at the media time it left off, and chasing that
// would drain the queue in a single pass — the burst the clock exists to stop.
const resyncThreshold = time.Second

// slew is the per-refresh correction applied when the queue drifts away from
// its working depth. The encoder's clock and the panel's are never quite the
// same rate, and left alone that difference accumulates until a frame is
// dropped or the queue starves. Nudging the anchor spreads the correction out
// below the threshold of visibility.
const slew = time.Millisecond

// staggerSpread is how many refreshes apart consecutive sources are anchored,
// cycling so the offset stays bounded no matter how many tiles are on the wall.
const staggerSpread = 4

// presentClock maps one source's media timeline onto the display's timeline.
//
// Showing whatever is newest at each refresh looks correct on a fixed camera
// and judders on a moving one. The decoder's clock and the display's clock are
// independent, so a 30fps stream on a 60Hz output lands two refreshes apart,
// then three, then two, and a frame is occasionally skipped outright when two
// arrive inside one refresh. Anchoring media time to wall time makes the
// mapping linear, so the same stream resolves to a fixed cadence and only
// corrects at the rate the two clocks actually drift.
type presentClock struct {
	wall time.Time
	pts  time.Duration
	set  bool
}

func (c *presentClock) anchor(at time.Time, pts time.Duration) {
	c.wall, c.pts, c.set = at, pts, true
}

// dueAt is the media time that belongs on screen at wall time t.
func (c *presentClock) dueAt(t time.Time) time.Duration {
	return c.pts + t.Sub(c.wall)
}

// due picks the frame to show at present, discarding any it passes over. It
// returns nil when the frame already on screen is still the right one, which
// is the ordinary case for any source slower than the refresh rate.
//
// stagger offsets this source's anchor so that sources sharing a frame rate do
// not all fall due on the same refresh. Their clocks are independent — there is
// no common timebase to sync a YouTube stream to an RTSP camera — but their
// uploads land in one shared render pass, and a grid of 1080p tiles converging
// on the same refresh is what turns per-tile timing into a whole-wall hitch.
func (c *presentClock) due(slot *frame.Slot, present time.Time, stagger time.Duration) *frame.Frame {
	head, ok := slot.Peek()
	if !ok {
		return nil
	}
	if head < 0 {
		// Nothing to schedule against. Fall back to newest-wins and let the
		// clock re-anchor once real timestamps arrive.
		c.set = false
		return slot.Take()
	}
	if !c.set {
		c.anchor(present.Add(presentLead+stagger), head)
	}
	target := c.dueAt(present)
	if head > target+resyncThreshold || head < target-resyncThreshold {
		// A reconnect, a loop, or a stall long enough that the old anchor no
		// longer describes this stream. Restart the mapping at the newest
		// frame rather than replaying the backlog.
		f := slot.Take()
		if f != nil {
			c.anchor(present.Add(presentLead+stagger), f.PTS)
		}
		return f
	}
	var due *frame.Frame
	for {
		head, ok := slot.Peek()
		if !ok || head > target {
			break
		}
		due = slot.Advance()
	}
	switch depth := slot.Depth(); {
	case depth >= frame.QueueDepth-1:
		c.wall = c.wall.Add(-slew) // running late; bring frames forward
	case depth == 0 && due != nil:
		c.wall = c.wall.Add(slew) // running early; hold the next one back
	}
	return due
}
