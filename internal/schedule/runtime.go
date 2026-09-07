package schedule

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dchote/livestream-viewer/internal/display/strategy"
	"github.com/dchote/livestream-viewer/internal/model"
)

// Clock is injectable so tests can drive wall time.
type Clock interface {
	Now() time.Time
}

// RealClock uses the system clock.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// ManualClock is a test clock.
type ManualClock struct {
	mu sync.Mutex
	t  time.Time
}

func NewManualClock(t time.Time) *ManualClock { return &ManualClock{t: t} }

func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *ManualClock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

func (c *ManualClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// State is an immutable published snapshot of the scheduler.
type State struct {
	At               time.Time   `json:"at"`
	DisplayRunning   bool        `json:"display_running"`
	DisplayEnabled   bool        `json:"display_enabled"`
	Paused           bool        `json:"paused"`
	Pinned           bool        `json:"pinned"`
	ActiveScreen     *uint       `json:"active_screen"`
	ActiveScreenName string      `json:"active_screen_name,omitempty"`
	NextScreen       *uint       `json:"next_screen"`
	DwellRemainingMS int         `json:"dwell_remaining_ms"`
	Transition       *TransState `json:"transition,omitempty"`
	FPS              float64     `json:"fps"`
	Tiles            []TileState `json:"tiles"`
}

// TransState is an in-progress transition.
type TransState struct {
	Type     string  `json:"type"`
	Subtype  string  `json:"subtype,omitempty"`
	Progress float64 `json:"progress"`
}

// TileState is one visible tile.
type TileState struct {
	Index         int    `json:"index"`
	SourceID      *uint  `json:"source_id"`
	SourceName    string `json:"source_name,omitempty"`
	Fit           string `json:"fit,omitempty"`
	Decoder       string `json:"decoder"`
	SequenceIndex int    `json:"sequence_index,omitempty"`
}

type seqKey struct {
	ScreenID uint
	Index    int
}

type seqPos struct {
	Item  int
	Start time.Time
}

const (
	phaseDwell      = "dwell"
	phaseTransition = "transition"
)

// Runtime is the headless wall-clock scheduler.
type Runtime struct {
	clock          Clock
	displayEnabled bool
	mu             sync.Mutex
	snap           *strategy.Snapshot
	paused         bool
	pinned         bool
	tourIndex      int
	overrideScreen *uint
	playlistIndex  int
	playlistPhase  string
	playlistStart  time.Time
	seq            map[seqKey]seqPos
	phase          string
	phaseStart     time.Time
	published      atomic.Pointer[State]
	onChange       func(*State)
}

// New constructs a runtime. clock may be nil (RealClock). Starts paused until a snapshot arrives.
func New(clock Clock, displayEnabled bool) *Runtime {
	if clock == nil {
		clock = RealClock{}
	}
	r := &Runtime{
		clock:          clock,
		displayEnabled: displayEnabled,
		paused:         true,
		phase:          phaseDwell,
		playlistPhase:  phaseDwell,
		seq:            map[seqKey]seqPos{},
	}
	st := r.emptyState(clock.Now())
	r.published.Store(st)
	return r
}

// SetOnChange registers a listener invoked whenever published state changes.
func (r *Runtime) SetOnChange(fn func(*State)) {
	r.mu.Lock()
	r.onChange = fn
	r.mu.Unlock()
}

// State returns the last published snapshot.
func (r *Runtime) State() *State {
	st := r.published.Load()
	if st == nil {
		return r.emptyState(time.Now())
	}
	return st
}

// ApplyStrategy replaces the resolved configuration.
func (r *Runtime) ApplyStrategy(snap *strategy.Snapshot) {
	r.mu.Lock()
	r.applyLocked(snap, r.clock.Now())
	st := r.buildStateLocked(r.clock.Now())
	r.published.Store(st)
	onChange := r.onChange
	r.mu.Unlock()
	// A strategy change always publishes: an edit can alter fields that
	// stateChangedForSSE does not compare (tile fit, dwell, layout).
	notify(onChange, st)
}

func (r *Runtime) applyLocked(snap *strategy.Snapshot, now time.Time) {
	prev := r.activeScreenIDLocked()
	wasEnabled := r.snap != nil && r.snap.Tour.Enabled
	r.snap = snap
	if r.overrideScreen != nil && (snap == nil || snap.Screens[*r.overrideScreen] == nil) {
		r.overrideScreen = nil
		r.pinned = false
	}
	if snap == nil || (len(snap.Tour.Entries) == 0 && r.overrideScreen == nil) {
		r.tourIndex = 0
		r.playlistIndex = 0
		r.phase = phaseDwell
		r.phaseStart = now
		r.paused = true
		return
	}
	if prev != nil {
		found := false
		for i, e := range snap.Tour.Entries {
			if e.ScreenID == *prev {
				r.tourIndex = i
				found = true
				break
			}
		}
		if !found {
			r.tourIndex = 0
		}
	} else {
		r.tourIndex = 0
	}
	if r.tourIndex >= len(snap.Tour.Entries) {
		r.tourIndex = 0
	}
	r.phase = phaseDwell
	r.phaseStart = now
	r.playlistIndex = 0
	r.playlistPhase = phaseDwell
	r.playlistStart = now
	r.resetSequencesLocked(now)
	if !snap.Tour.Enabled {
		r.paused = true
	} else if !wasEnabled {
		r.paused = false
		r.pinned = false
		r.overrideScreen = nil
	}
}

// Next advances one tour step.
func (r *Runtime) Next() {
	r.mu.Lock()
	r.stepTourLocked(1, r.clock.Now())
	st, onChange := r.publishLocked()
	r.mu.Unlock()
	notify(onChange, st)
}

// Previous steps back one tour step.
func (r *Runtime) Previous() {
	r.mu.Lock()
	r.stepTourLocked(-1, r.clock.Now())
	st, onChange := r.publishLocked()
	r.mu.Unlock()
	notify(onChange, st)
}

// Goto pins a screen and pauses the tour.
func (r *Runtime) Goto(screenID uint) bool {
	r.mu.Lock()
	if r.snap == nil {
		r.mu.Unlock()
		return false
	}
	if _, ok := r.snap.Screens[screenID]; !ok {
		r.mu.Unlock()
		return false
	}
	id := screenID
	r.overrideScreen = &id
	for i, e := range r.snap.Tour.Entries {
		if e.ScreenID == screenID {
			r.tourIndex = i
			break
		}
	}
	r.pinned = true
	r.paused = true
	r.phase = phaseDwell
	r.phaseStart = r.clock.Now()
	r.playlistIndex = 0
	r.playlistPhase = phaseDwell
	r.playlistStart = r.clock.Now()
	r.resetSequencesLocked(r.clock.Now())
	st, onChange := r.publishLocked()
	r.mu.Unlock()
	notify(onChange, st)
	return true
}

// Pause holds the tour.
func (r *Runtime) Pause() {
	r.mu.Lock()
	r.paused = true
	st, onChange := r.publishLocked()
	r.mu.Unlock()
	notify(onChange, st)
}

// Resume releases a pin and the tour hold.
func (r *Runtime) Resume() {
	r.mu.Lock()
	r.pinned = false
	r.paused = false
	r.overrideScreen = nil
	r.phase = phaseDwell
	r.phaseStart = r.clock.Now()
	st, onChange := r.publishLocked()
	r.mu.Unlock()
	notify(onChange, st)
}

func (r *Runtime) stepTourLocked(delta int, now time.Time) {
	if r.snap == nil || len(r.snap.Tour.Entries) == 0 {
		return
	}
	r.overrideScreen = nil
	r.pinned = false
	n := len(r.snap.Tour.Entries)
	r.tourIndex = ((r.tourIndex+delta)%n + n) % n
	r.phase = phaseDwell
	r.phaseStart = now
	r.playlistIndex = 0
	r.playlistPhase = phaseDwell
	r.playlistStart = now
	r.resetSequencesLocked(now)
}

func (r *Runtime) resetSequencesLocked(now time.Time) {
	r.seq = map[seqKey]seqPos{}
	id := r.activeScreenIDLocked()
	if id == nil || r.snap == nil {
		return
	}
	sc := r.snap.Screens[*id]
	if sc == nil {
		return
	}
	for _, t := range sc.Tiles {
		if len(t.Sequence) == 0 {
			continue
		}
		r.seq[seqKey{ScreenID: sc.ID, Index: t.Index}] = seqPos{Item: 0, Start: now}
	}
}

func (r *Runtime) activeScreenIDLocked() *uint {
	if r.overrideScreen != nil {
		if r.snap != nil {
			if _, ok := r.snap.Screens[*r.overrideScreen]; ok {
				return r.overrideScreen
			}
		}
		r.overrideScreen = nil
		r.pinned = false
	}
	if r.snap == nil || len(r.snap.Tour.Entries) == 0 {
		return nil
	}
	if r.tourIndex < 0 || r.tourIndex >= len(r.snap.Tour.Entries) {
		return nil
	}
	id := r.snap.Tour.Entries[r.tourIndex].ScreenID
	return &id
}

func (r *Runtime) nextScreenIDLocked() *uint {
	if r.snap == nil || len(r.snap.Tour.Entries) < 2 {
		return nil
	}
	if r.paused || r.pinned || !r.snap.Tour.Enabled {
		return nil
	}
	n := len(r.snap.Tour.Entries)
	next := r.tourIndex + 1
	if next >= n {
		if !r.snap.Tour.Loop {
			return nil
		}
		next = 0
	}
	id := r.snap.Tour.Entries[next].ScreenID
	return &id
}

// Tick advances timers using the clock.
func (r *Runtime) Tick() {
	r.mu.Lock()
	r.advanceLocked(r.clock.Now())
	st, onChange := r.publishLocked()
	r.mu.Unlock()
	notify(onChange, st)
}

// Run ticks until ctx is cancelled.
func (r *Runtime) Run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Tick()
		}
	}
}

func (r *Runtime) advanceLocked(now time.Time) {
	if r.snap == nil {
		return
	}
	r.advanceSequencesLocked(now)
	r.advancePlaylistLocked(now)
	if r.paused || r.pinned || !r.snap.Tour.Enabled || len(r.snap.Tour.Entries) == 0 {
		return
	}
	entry := r.snap.Tour.Entries[r.tourIndex]
	switch r.phase {
	case phaseDwell:
		if now.Sub(r.phaseStart) >= time.Duration(entry.DwellMS)*time.Millisecond {
			next, ok := r.peekNextTourEntryLocked()
			if !ok {
				r.paused = true
				r.phase = phaseDwell
				r.phaseStart = now
				return
			}
			// Incoming transition: play the *next* entry's transition before landing on it.
			if next.Transition.Type == "cut" || next.Transition.DurationMS <= 0 {
				r.advanceTourIndexLocked(now)
			} else {
				r.phase = phaseTransition
				r.phaseStart = now
			}
		}
	case phaseTransition:
		next, ok := r.peekNextTourEntryLocked()
		if !ok {
			r.advanceTourIndexLocked(now)
			return
		}
		dur := time.Duration(next.Transition.DurationMS) * time.Millisecond
		if now.Sub(r.phaseStart) >= dur {
			r.advanceTourIndexLocked(now)
		}
	}
}

func (r *Runtime) peekNextTourEntryLocked() (strategy.TourEntrySnap, bool) {
	n := len(r.snap.Tour.Entries)
	if n == 0 {
		return strategy.TourEntrySnap{}, false
	}
	next := r.tourIndex + 1
	if next >= n {
		if !r.snap.Tour.Loop {
			return strategy.TourEntrySnap{}, false
		}
		next = 0
	}
	return r.snap.Tour.Entries[next], true
}

func (r *Runtime) advanceTourIndexLocked(now time.Time) {
	n := len(r.snap.Tour.Entries)
	next := r.tourIndex + 1
	if next >= n {
		if !r.snap.Tour.Loop {
			r.paused = true
			r.phase = phaseDwell
			r.phaseStart = now
			return
		}
		next = 0
	}
	r.tourIndex = next
	r.phase = phaseDwell
	r.phaseStart = now
	r.playlistIndex = 0
	r.playlistPhase = phaseDwell
	r.playlistStart = now
	r.resetSequencesLocked(now)
}

func (r *Runtime) advancePlaylistLocked(now time.Time) {
	id := r.activeScreenIDLocked()
	if id == nil {
		return
	}
	sc := r.snap.Screens[*id]
	if sc == nil || sc.Kind != model.ScreenKindTransition || len(sc.Items) == 0 {
		return
	}
	item := sc.Items[r.playlistIndex%len(sc.Items)]
	switch r.playlistPhase {
	case phaseDwell:
		if now.Sub(r.playlistStart) >= time.Duration(item.DwellMS)*time.Millisecond {
			dur := sc.Transition.DurationMS
			if sc.Transition.Type == "cut" || dur <= 0 {
				r.stepPlaylistLocked(now, sc)
			} else {
				r.playlistPhase = phaseTransition
				r.playlistStart = now
			}
		}
	case phaseTransition:
		dur := time.Duration(sc.Transition.DurationMS) * time.Millisecond
		if now.Sub(r.playlistStart) >= dur {
			r.stepPlaylistLocked(now, sc)
		}
	}
}

func (r *Runtime) stepPlaylistLocked(now time.Time, sc *strategy.ScreenSnap) {
	next := r.playlistIndex + 1
	if next >= len(sc.Items) {
		if !sc.Loop {
			r.playlistPhase = phaseDwell
			return
		}
		next = 0
	}
	r.playlistIndex = next
	r.playlistPhase = phaseDwell
	r.playlistStart = now
}

func (r *Runtime) advanceSequencesLocked(now time.Time) {
	id := r.activeScreenIDLocked()
	if id == nil {
		return
	}
	sc := r.snap.Screens[*id]
	if sc == nil {
		return
	}
	for _, t := range sc.Tiles {
		if len(t.Sequence) == 0 {
			continue
		}
		key := seqKey{ScreenID: sc.ID, Index: t.Index}
		pos, ok := r.seq[key]
		if !ok {
			pos = seqPos{Item: 0, Start: now}
		}
		item := t.Sequence[pos.Item%len(t.Sequence)]
		if now.Sub(pos.Start) >= time.Duration(item.DwellMS)*time.Millisecond {
			pos.Item = (pos.Item + 1) % len(t.Sequence)
			pos.Start = now
		}
		r.seq[key] = pos
	}
}

// publishLocked stores the new snapshot and returns the listener to invoke,
// or nil when nothing SSE-visible changed. Fan-out deliberately happens after
// the caller releases mu — notifying under the lock would let a slow
// subscriber stall display commands, and it gives the callback a second lock
// ordering to worry about.
func (r *Runtime) publishLocked() (*State, func(*State)) {
	st := r.buildStateLocked(r.clock.Now())
	prev := r.published.Load()
	r.published.Store(st)
	if r.onChange == nil || !stateChangedForSSE(prev, st) {
		return st, nil
	}
	return st, r.onChange
}

func notify(fn func(*State), st *State) {
	if fn != nil {
		fn(st)
	}
}

func stateChangedForSSE(prev, next *State) bool {
	if prev == nil {
		return true
	}
	if prev.Paused != next.Paused || prev.Pinned != next.Pinned {
		return true
	}
	if ptrUint(prev.ActiveScreen) != ptrUint(next.ActiveScreen) {
		return true
	}
	if ptrUint(prev.NextScreen) != ptrUint(next.NextScreen) {
		return true
	}
	if prev.ActiveScreenName != next.ActiveScreenName {
		return true
	}
	// Throttle dwell countdown to whole seconds for SSE.
	if prev.DwellRemainingMS/1000 != next.DwellRemainingMS/1000 {
		return true
	}
	if (prev.Transition == nil) != (next.Transition == nil) {
		return true
	}
	if prev.Transition != nil && next.Transition != nil {
		if prev.Transition.Type != next.Transition.Type || prev.Transition.Subtype != next.Transition.Subtype {
			return true
		}
		if int(prev.Transition.Progress*10) != int(next.Transition.Progress*10) {
			return true
		}
	}
	if len(prev.Tiles) != len(next.Tiles) {
		return true
	}
	for i := range next.Tiles {
		a, b := prev.Tiles[i], next.Tiles[i]
		if ptrUint(a.SourceID) != ptrUint(b.SourceID) || a.Decoder != b.Decoder || a.SequenceIndex != b.SequenceIndex {
			return true
		}
	}
	return false
}

func ptrUint(p *uint) uint {
	if p == nil {
		return 0
	}
	return *p
}

func (r *Runtime) emptyState(now time.Time) *State {
	return &State{
		At:             now,
		DisplayRunning: false,
		DisplayEnabled: r.displayEnabled,
		Paused:         true,
		Tiles:          []TileState{},
	}
}

func (r *Runtime) buildStateLocked(now time.Time) *State {
	st := r.emptyState(now)
	if r.snap == nil {
		return st
	}
	st.Paused = r.paused
	st.Pinned = r.pinned
	id := r.activeScreenIDLocked()
	st.ActiveScreen = id
	st.NextScreen = r.nextScreenIDLocked()
	if id != nil {
		if sc := r.snap.Screens[*id]; sc != nil {
			st.ActiveScreenName = sc.Name
			st.Tiles = r.tilesLocked(sc)
		}
		if r.tourIndex < len(r.snap.Tour.Entries) {
			entry := r.snap.Tour.Entries[r.tourIndex]
			if r.phase == phaseDwell {
				elapsed := now.Sub(r.phaseStart)
				remain := time.Duration(entry.DwellMS)*time.Millisecond - elapsed
				if remain < 0 {
					remain = 0
				}
				st.DwellRemainingMS = int(remain / time.Millisecond)
			} else if next, ok := r.peekNextTourEntryLocked(); ok {
				st.DwellRemainingMS = 0
				dur := float64(next.Transition.DurationMS)
				prog := 1.0
				if dur > 0 {
					prog = now.Sub(r.phaseStart).Seconds() * 1000 / dur
					if prog < 0 {
						prog = 0
					}
					if prog > 1 {
						prog = 1
					}
				}
				st.Transition = &TransState{Type: next.Transition.Type, Subtype: next.Transition.Subtype, Progress: prog}
			}
		}
	}
	return st
}

func (r *Runtime) tilesLocked(sc *strategy.ScreenSnap) []TileState {
	if sc.Kind == model.ScreenKindTransition {
		if len(sc.Items) == 0 {
			return []TileState{}
		}
		idx := r.playlistIndex % len(sc.Items)
		it := sc.Items[idx]
		sid := it.SourceID
		name, decoder := r.sourceMeta(sid)
		return []TileState{{
			Index:      0,
			SourceID:   &sid,
			SourceName: name,
			Fit:        it.Fit,
			Decoder:    decoder,
		}}
	}
	out := make([]TileState, 0, len(sc.Tiles))
	for _, t := range sc.Tiles {
		ts := TileState{Index: t.Index, Fit: t.Fit, Decoder: "offline"}
		if len(t.Sequence) > 0 {
			pos := r.seq[seqKey{ScreenID: sc.ID, Index: t.Index}]
			item := t.Sequence[pos.Item%len(t.Sequence)]
			sid := item.SourceID
			name, decoder := r.sourceMeta(sid)
			ts.SourceID = &sid
			ts.SourceName = name
			ts.Decoder = decoder
			ts.SequenceIndex = pos.Item
		} else if t.SourceID != nil {
			name, decoder := r.sourceMeta(*t.SourceID)
			ts.SourceID = t.SourceID
			ts.SourceName = name
			ts.Decoder = decoder
		} else {
			ts.Decoder = "offline"
		}
		out = append(out, ts)
	}
	return out
}

func (r *Runtime) sourceMeta(id uint) (name, decoder string) {
	if r.snap == nil {
		return "", "offline"
	}
	ref := r.snap.Sources[id]
	if ref == nil {
		return "", "offline"
	}
	if !ref.Enabled {
		return ref.Name, "disabled"
	}
	return ref.Name, "offline"
}
