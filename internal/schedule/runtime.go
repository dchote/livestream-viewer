package schedule

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dchote/livestream-viewer/internal/display/layout"
	"github.com/dchote/livestream-viewer/internal/display/strategy"
	"github.com/dchote/livestream-viewer/internal/display/transition"
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
	At               time.Time      `json:"at"`
	DisplayRunning   bool           `json:"display_running"`
	DisplayEnabled   bool           `json:"display_enabled"`
	Paused           bool           `json:"paused"`
	Pinned           bool           `json:"pinned"`
	ActiveScreen     *uint          `json:"active_screen"`
	ActiveScreenName string         `json:"active_screen_name,omitempty"`
	NextScreen       *uint          `json:"next_screen"`
	DwellRemainingMS int            `json:"dwell_remaining_ms"`
	Transition       *TransState    `json:"transition,omitempty"`
	FPS              float64        `json:"fps"`
	DroppedFrames    uint64         `json:"dropped_frames"`
	DisplayError     string         `json:"display_error,omitempty"`
	Degradations     []string       `json:"degradations,omitempty"`
	Tiles            []TileState    `json:"tiles"`
	Decoders         []DecoderState `json:"decoders"`
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
	ErrorCode     string `json:"error_code,omitempty"`
	Error         string `json:"error,omitempty"`
	SequenceIndex int    `json:"sequence_index,omitempty"`
}

// DecoderState is live ingest health for one source, including sources that
// are not on the current screen.
type DecoderState struct {
	SourceID  uint   `json:"source_id"`
	Decoder   string `json:"decoder"`
	ErrorCode string `json:"error_code,omitempty"`
	Error     string `json:"error,omitempty"`
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
	health         map[uint]decoderHealth
	fps            float64
	dropped        uint64
	displayRunning bool
	displayError   string
	degradations   []string
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
		health:         map[uint]decoderHealth{},
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

// Snapshot returns the current strategy snapshot (may be nil).
func (r *Runtime) Snapshot() *strategy.Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snap
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
	r.pruneHealthLocked()
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

// pruneHealthLocked drops decoder health for sources that no longer exist.
// Without it the map keeps an entry for every source ever configured, which
// only matters over a long lifetime of adding and deleting cameras.
func (r *Runtime) pruneHealthLocked() {
	if len(r.health) == 0 {
		return
	}
	if r.snap == nil {
		clear(r.health)
		return
	}
	for id := range r.health {
		if _, ok := r.snap.Sources[id]; !ok {
			delete(r.health, id)
		}
	}
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
	if prev.DisplayRunning != next.DisplayRunning || prev.DisplayError != next.DisplayError {
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
		if ptrUint(a.SourceID) != ptrUint(b.SourceID) || a.Decoder != b.Decoder || a.ErrorCode != b.ErrorCode || a.SequenceIndex != b.SequenceIndex {
			return true
		}
	}
	return decodersChanged(prev.Decoders, next.Decoders)
}

func decodersChanged(a, b []DecoderState) bool {
	if len(a) != len(b) {
		return true
	}
	prev := map[uint]DecoderState{}
	for _, d := range a {
		prev[d.SourceID] = d
	}
	for _, d := range b {
		p, ok := prev[d.SourceID]
		if !ok || p.Decoder != d.Decoder || p.ErrorCode != d.ErrorCode {
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
		DisplayRunning: r.displayRunning,
		DisplayEnabled: r.displayEnabled,
		DisplayError:   r.displayError,
		Degradations:   append([]string(nil), r.degradations...),
		FPS:            r.fps,
		DroppedFrames:  r.dropped,
		Paused:         true,
		Tiles:          []TileState{},
		Decoders:       []DecoderState{},
	}
}

func (r *Runtime) buildStateLocked(now time.Time) *State {
	st := r.emptyState(now)
	if r.snap == nil {
		st.Decoders = r.decodersLocked()
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
	st.Decoders = r.decodersLocked()
	return st
}

func (r *Runtime) decodersLocked() []DecoderState {
	seen := map[uint]struct{}{}
	out := make([]DecoderState, 0, len(r.health))
	if r.snap != nil {
		ids := make([]uint, 0, len(r.snap.Sources))
		for id := range r.snap.Sources {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			_, decoder, code, msg := r.sourceMeta(id)
			out = append(out, DecoderState{SourceID: id, Decoder: decoder, ErrorCode: code, Error: msg})
			seen[id] = struct{}{}
		}
	}
	extra := make([]uint, 0, len(r.health))
	for id := range r.health {
		if _, ok := seen[id]; !ok {
			extra = append(extra, id)
		}
	}
	slices.Sort(extra)
	for _, id := range extra {
		h := r.health[id]
		status := h.Status
		if status == "" {
			status = DecoderOffline
		}
		out = append(out, DecoderState{SourceID: id, Decoder: status, ErrorCode: h.Code, Error: h.Message})
	}
	return out
}

func (r *Runtime) tilesLocked(sc *strategy.ScreenSnap) []TileState {
	if sc.Kind == model.ScreenKindTransition {
		if len(sc.Items) == 0 {
			return []TileState{}
		}
		idx := r.playlistIndex % len(sc.Items)
		it := sc.Items[idx]
		sid := it.SourceID
		name, decoder, code, msg := r.sourceMeta(sid)
		return []TileState{{
			Index:      0,
			SourceID:   &sid,
			SourceName: name,
			Fit:        it.Fit,
			Decoder:    decoder,
			ErrorCode:  code,
			Error:      msg,
		}}
	}
	out := make([]TileState, 0, len(sc.Tiles))
	for _, t := range sc.Tiles {
		ts := TileState{Index: t.Index, Fit: t.Fit, Decoder: "offline"}
		if len(t.Sequence) > 0 {
			pos := r.seq[seqKey{ScreenID: sc.ID, Index: t.Index}]
			item := t.Sequence[pos.Item%len(t.Sequence)]
			sid := item.SourceID
			name, decoder, code, msg := r.sourceMeta(sid)
			ts.SourceID = &sid
			ts.SourceName = name
			ts.Decoder = decoder
			ts.ErrorCode = code
			ts.Error = msg
			ts.SequenceIndex = pos.Item
		} else if t.SourceID != nil {
			name, decoder, code, msg := r.sourceMeta(*t.SourceID)
			ts.SourceID = t.SourceID
			ts.SourceName = name
			ts.Decoder = decoder
			ts.ErrorCode = code
			ts.Error = msg
		} else {
			ts.Decoder = "offline"
		}
		out = append(out, ts)
	}
	return out
}

func (r *Runtime) sourceMeta(id uint) (name, decoder, code, message string) {
	if r.snap == nil {
		return "", "offline", "", ""
	}
	ref := r.snap.Sources[id]
	if ref == nil {
		return "", "offline", "", ""
	}
	if !ref.Enabled {
		return ref.Name, "disabled", "", ""
	}
	if st, ok := r.health[id]; ok && st.Status != "" {
		return ref.Name, st.Status, st.Code, st.Message
	}
	return ref.Name, "offline", "", ""
}

type decoderHealth struct {
	Status  string
	Code    string
	Message string
}

const (
	DecoderConnecting   = "connecting"
	DecoderHardware     = "hardware"
	DecoderSoftware     = "software"
	DecoderReconnecting = "reconnecting"
	DecoderOffline      = "offline"
	DecoderDisabled     = "disabled"
	// DecoderFailed means the worker gave up on the normal reconnect path:
	// resolution keeps failing, or the worker itself panicked. It still
	// retries at the backoff ceiling, but the UI should show it as broken.
	DecoderFailed = "failed"
)

// SetDecoderHealth records per-source decoder status for the next published snapshot.
func (r *Runtime) SetDecoderHealth(id uint, status, code, message string) {
	r.mu.Lock()
	if r.health == nil {
		r.health = map[uint]decoderHealth{}
	}
	r.health[id] = decoderHealth{Status: status, Code: code, Message: message}
	st, onChange := r.publishLocked()
	r.mu.Unlock()
	notify(onChange, st)
}

// SourceHealth is the live decoder status for one source, including sources
// that are not on the current screen.
func (r *Runtime) SourceHealth(id uint) (status, code, message string) {
	if r == nil {
		return "", "", ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.health[id]
	return h.Status, h.Code, h.Message
}

// YouTubeAuthIssue is the first live youtube_bot_check or youtube_auth error.
func (r *Runtime) YouTubeAuthIssue() (code, message string) {
	if r == nil {
		return "", ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var authCode, authMsg string
	for _, h := range r.health {
		if h.Code == "youtube_bot_check" {
			return h.Code, h.Message
		}
		if h.Code == "youtube_auth" && authCode == "" {
			authCode, authMsg = h.Code, h.Message
		}
	}
	return authCode, authMsg
}

// SetDisplayMetrics is called by the engine each frame.
func (r *Runtime) SetDisplayMetrics(running bool, fps float64, dropped uint64, displayErr string, degradations []string) {
	r.mu.Lock()
	r.displayRunning = running
	r.fps = fps
	r.dropped = dropped
	r.displayError = displayErr
	r.degradations = append([]string(nil), degradations...)
	st := r.buildStateLocked(r.clock.Now())
	prev := r.published.Load()
	changed := prev == nil || prev.DisplayRunning != st.DisplayRunning || prev.DisplayError != st.DisplayError
	r.published.Store(st)
	onChange := r.onChange
	r.mu.Unlock()
	if changed {
		notify(onChange, st)
	}
}

// Layer is one composed screen for the renderer.
type Layer struct {
	ScreenID uint
	Name     string
	Kind     string
	Layout   string
	Rects    []layout.Rect
	Tiles    []TileState
}

// ComposeView is the outgoing (and optional incoming) layer plus raw transition progress.
type ComposeView struct {
	Outgoing   Layer
	Incoming   *Layer
	Transition transition.Spec
	Progress   float64 // 0..1, uneased
	Needed     []uint  // source IDs the manager should keep decoded
}

// ComposeView builds what the engine should draw right now.
func (r *Runtime) ComposeView() ComposeView {
	r.mu.Lock()
	defer r.mu.Unlock()
	var v ComposeView
	if r.snap == nil {
		return v
	}
	id := r.activeScreenIDLocked()
	if id != nil {
		if sc := r.snap.Screens[*id]; sc != nil {
			v.Outgoing = r.layerLocked(sc)
			v.Needed = sourceIDs(v.Outgoing)

			if r.phase == phaseTransition {
				if next, ok := r.peekNextTourEntryLocked(); ok {
					if nsc := r.snap.Screens[next.ScreenID]; nsc != nil {
						inc := r.layerLocked(nsc)
						v.Incoming = &inc
						v.Transition = next.Transition
						dur := float64(next.Transition.DurationMS)
						if dur > 0 {
							v.Progress = r.clock.Now().Sub(r.phaseStart).Seconds() * 1000 / dur
							if v.Progress < 0 {
								v.Progress = 0
							}
							if v.Progress > 1 {
								v.Progress = 1
							}
						} else {
							v.Progress = 1
						}
						v.Needed = append(v.Needed, sourceIDs(inc)...)
					}
				}
			} else if sc.Kind == model.ScreenKindTransition && r.playlistPhase == phaseTransition && len(sc.Items) > 0 {
				cur := r.playlistIndex % len(sc.Items)
				next := (cur + 1) % len(sc.Items)
				if sc.Loop || cur+1 < len(sc.Items) {
					outgoingItem := sc.Items[cur]
					incomingItem := sc.Items[next]
					v.Outgoing.Tiles = []TileState{{
						Index: 0, SourceID: ptr(outgoingItem.SourceID), Fit: outgoingItem.Fit,
					}}
					inc := v.Outgoing
					inc.Tiles = []TileState{{
						Index: 0, SourceID: ptr(incomingItem.SourceID), Fit: incomingItem.Fit,
					}}
					v.Incoming = &inc
					v.Transition = sc.Transition
					dur := float64(sc.Transition.DurationMS)
					if dur > 0 {
						v.Progress = r.clock.Now().Sub(r.playlistStart).Seconds() * 1000 / dur
						if v.Progress < 0 {
							v.Progress = 0
						}
						if v.Progress > 1 {
							v.Progress = 1
						}
					} else {
						// A zero-duration transition (a cut) is already
						// complete; leaving progress at 0 held the outgoing
						// item on screen for the whole transition phase.
						v.Progress = 1
					}
					v.Needed = append(v.Needed, outgoingItem.SourceID, incomingItem.SourceID)
				}
			}
		}
	}
	// Pre-roll every source on any screen so Goto/tour/playlist cuts already have frames.
	v.Needed = uniqueIDs(append(v.Needed, r.strategySourceIDsLocked()...))
	return v
}

func ptr(id uint) *uint { return &id }

func (r *Runtime) layerLocked(sc *strategy.ScreenSnap) Layer {
	return Layer{
		ScreenID: sc.ID,
		Name:     sc.Name,
		Kind:     sc.Kind,
		Layout:   sc.Layout,
		Rects:    sc.Rects,
		Tiles:    r.tilesLocked(sc),
	}
}

func (r *Runtime) screenSourceIDsLocked(sc *strategy.ScreenSnap) []uint {
	var ids []uint
	for _, t := range sc.Tiles {
		if t.SourceID != nil {
			ids = append(ids, *t.SourceID)
		}
		for _, s := range t.Sequence {
			ids = append(ids, s.SourceID)
		}
	}
	for _, it := range sc.Items {
		ids = append(ids, it.SourceID)
	}
	return ids
}

func (r *Runtime) strategySourceIDsLocked() []uint {
	var ids []uint
	if r.snap == nil {
		return ids
	}
	for _, sc := range r.snap.Screens {
		ids = append(ids, r.screenSourceIDsLocked(sc)...)
	}
	out := make([]uint, 0, len(ids))
	for _, id := range uniqueIDs(ids) {
		if ref := r.snap.Sources[id]; ref != nil && !ref.Enabled {
			continue
		}
		out = append(out, id)
	}
	return out
}

func sourceIDs(l Layer) []uint {
	var ids []uint
	for _, t := range l.Tiles {
		if t.SourceID != nil {
			ids = append(ids, *t.SourceID)
		}
	}
	return ids
}

func uniqueIDs(in []uint) []uint {
	seen := map[uint]struct{}{}
	out := make([]uint, 0, len(in))
	for _, id := range in {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
