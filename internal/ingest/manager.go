package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dchote/livestream-viewer/internal/frame"
	"github.com/dchote/livestream-viewer/internal/ingest/capability"
	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/schedule"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
	"gorm.io/gorm"
)

// Binder applies slot bind/unbind on the render thread.
type Binder interface {
	Bind(id uint, slot *frame.Slot) bool
	Unbind(id uint)
}

// Manager owns decode workers. It never runs on the render thread.
type Manager struct {
	db     *gorm.DB
	rt     *schedule.Runtime
	caps   capability.Info
	tools  resolver.Tools
	binder Binder

	mu      sync.Mutex
	workers map[uint]*worker
	hwInUse int
	applied []uint // sorted; the set the current workers were built from

	dirty    atomic.Bool
	stopOnce sync.Once
	needed   chan []uint
	authGen  atomic.Uint64
}

type worker struct {
	cancel  context.CancelFunc
	slot    *frame.Slot
	usingHW bool
	fp      string
	done    chan struct{}
}

// NewManager constructs an ingest manager.
func NewManager(db *gorm.DB, rt *schedule.Runtime, caps capability.Info, tools resolver.Tools, binder Binder) *Manager {
	return &Manager{
		db:      db,
		rt:      rt,
		caps:    caps,
		tools:   tools,
		binder:  binder,
		workers: map[uint]*worker{},
		needed:  make(chan []uint, 1),
	}
}

// Run applies Sync requests until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) {
	defer m.StopAll()
	for {
		select {
		case <-ctx.Done():
			return
		case ids := <-m.needed:
			m.syncGuarded(ctx, ids)
		}
	}
}

// syncGuarded contains panics so one bad sync cannot kill the manager. If this
// loop dies, sources are never started or stopped again and the wall silently
// freezes on whatever it was showing.
func (m *Manager) syncGuarded(ctx context.Context, ids []uint) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("ingest sync panicked", "panic", rec, "stack", string(debug.Stack()))
			// Force a full pass next time; this one may have half-applied.
			m.dirty.Store(true)
		}
	}()
	m.sync(ctx, ids)
}

// RequestSync asks the manager to keep exactly these source IDs decoded.
//
// The render loop calls this every frame. When the set is unchanged this
// returns without allocating or waking the manager, which otherwise re-queried
// SQLite at frame rate for the entire life of the process.
func (m *Manager) RequestSync(ids []uint) {
	if m.settled(ids) {
		return
	}
	select {
	case m.needed <- append([]uint(nil), ids...):
	default:
		select {
		case <-m.needed:
		default:
		}
		select {
		case m.needed <- append([]uint(nil), ids...):
		default:
		}
	}
}

// Resync forces the next sync to re-read the database. Callers use it after
// writing configuration or source rows, where the needed set may be identical
// but the underlying records have changed.
func (m *Manager) Resync(ids []uint) {
	m.dirty.Store(true)
	m.RequestSync(ids)
}

// settled reports whether ids already match the applied worker set.
func (m *Manager) settled(ids []uint) bool {
	if m.dirty.Load() {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return sameSet(m.applied, ids)
}

// sameSet compares a sorted set against an unordered, duplicate-free set.
func sameSet(sorted, ids []uint) bool {
	if len(sorted) != len(ids) {
		return false
	}
	for _, id := range ids {
		if _, ok := slices.BinarySearch(sorted, id); !ok {
			return false
		}
	}
	return true
}

// StopAll unbinds then cancels every worker. Safe to call more than once.
func (m *Manager) StopAll() {
	m.stopOnce.Do(func() {
		m.mu.Lock()
		ids := make([]uint, 0, len(m.workers))
		for id := range m.workers {
			ids = append(ids, id)
		}
		m.mu.Unlock()
		for _, id := range ids {
			m.stopOne(id)
		}
		m.mu.Lock()
		m.applied = nil
		m.mu.Unlock()
	})
}

func (m *Manager) sync(ctx context.Context, needed []uint) {
	if m.settled(needed) {
		return
	}
	m.dirty.Store(false)

	// One query for every source referenced, rather than one query per id.
	want := map[uint]struct{}{}
	rows := map[uint]model.Source{}
	if len(needed) > 0 {
		var sources []model.Source
		if err := m.db.Find(&sources, needed).Error; err != nil {
			slog.Error("ingest sync could not load sources", "error", err)
			m.dirty.Store(true)
			return
		}
		for _, src := range sources {
			rows[src.ID] = src
		}
	}
	for _, id := range needed {
		src, ok := rows[id]
		if !ok {
			continue
		}
		if !src.Enabled {
			if m.rt != nil {
				m.rt.SetDecoderHealth(id, schedule.DecoderDisabled, "", "")
			}
			continue
		}
		want[id] = struct{}{}
	}
	m.mu.Lock()
	var extra, missing []uint
	for id, w := range m.workers {
		if _, ok := want[id]; !ok {
			extra = append(extra, id)
			continue
		}
		if w.fp != m.fingerprint(rows[id]) {
			extra = append(extra, id)
			missing = append(missing, id)
		}
	}
	for id := range want {
		if _, ok := m.workers[id]; !ok {
			missing = append(missing, id)
		}
	}
	m.mu.Unlock()

	for _, id := range extra {
		m.stopOne(id)
	}

	maxHW, allowSW, backoff := m.decodePolicy()

	slices.Sort(missing)
	started := 0
	for _, id := range missing {
		if ctx.Err() != nil {
			return
		}
		src := rows[id]
		slot := frame.NewSlot()
		srcAllowSW := allowSW || src.Options.ForceSoftware
		useHW := false
		m.mu.Lock()
		if ShouldUseHW(src, m.caps) && (maxHW <= 0 || m.hwInUse < maxHW) {
			useHW = true
			m.hwInUse++
		}
		m.mu.Unlock()

		if m.binder != nil && !m.binder.Bind(id, slot) {
			m.mu.Lock()
			if useHW && m.hwInUse > 0 {
				m.hwInUse--
			}
			m.mu.Unlock()
			slog.Warn("ingest bind timed out; will retry", "source", src.Name, "id", id)
			m.dirty.Store(true)
			continue
		}

		wctx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		m.mu.Lock()
		m.workers[id] = &worker{cancel: cancel, slot: slot, usingHW: useHW, fp: m.fingerprint(src), done: done}
		m.mu.Unlock()

		sid := id
		srcCopy := src
		delay := time.Duration(started) * 150 * time.Millisecond
		started++
		go func() {
			defer close(done)
			RunWorker(wctx, WorkerConfig{
				Source:     srcCopy,
				Slot:       slot,
				Caps:       m.caps,
				AllowSW:    srcAllowSW,
				UseHW:      useHW,
				BackoffMS:  backoff,
				Tools:      m.tools,
				StartDelay: delay,
				OnHealth: func(status, code, message string) {
					if m.rt != nil {
						m.rt.SetDecoderHealth(sid, status, code, message)
					}
				},
				OnHWFallback: func() {
					m.mu.Lock()
					defer m.mu.Unlock()
					if w := m.workers[sid]; w != nil && w.usingHW {
						w.usingHW = false
						if m.hwInUse > 0 {
							m.hwInUse--
						}
					}
				},
			})
		}()
	}

	m.mu.Lock()
	m.applied = slices.Sorted(maps.Keys(m.workers))
	m.mu.Unlock()
}

// decodePolicy reads the knobs that govern worker startup. The scheduler
// snapshot already carries them, so the steady state costs no query; the
// database is only consulted before the first snapshot arrives.
func (m *Manager) decodePolicy() (maxHW int, allowSW bool, backoffMS int) {
	if m.rt != nil {
		if snap := m.rt.Snapshot(); snap != nil {
			return snap.MaxHWDecoders, snap.AllowSoftwareFallback, snap.ReconnectBackoffMS
		}
	}
	var cfg model.RuntimeConfig
	if err := m.db.First(&cfg).Error; err != nil {
		slog.Warn("ingest sync could not load config; using defaults", "error", err)
		return 0, true, 2000
	}
	return cfg.MaxHWDecoders, cfg.AllowSoftwareFallback, cfg.ReconnectBackoffMS
}

func (m *Manager) stopOne(id uint) {
	if m.binder != nil {
		m.binder.Unbind(id)
	}
	m.mu.Lock()
	w := m.workers[id]
	if w != nil {
		if w.usingHW && m.hwInUse > 0 {
			m.hwInUse--
		}
		delete(m.workers, id)
	}
	m.mu.Unlock()
	if w != nil {
		w.cancel()
		waitWorker(w.done)
	}
	if m.rt != nil {
		m.rt.SetDecoderHealth(id, schedule.DecoderOffline, "", "")
	}
}

const workerStopWait = 5 * time.Second

func waitWorker(done <-chan struct{}) {
	if done == nil {
		return
	}
	timer := time.NewTimer(workerStopWait)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		slog.Warn("decode worker did not exit in time")
	}
}

// ShouldUseHW reports whether this source should open a hardware decoder.
// Probe.hw_decode false is always honored. VideoToolbox cannot decode many
// IP-camera RTSP bitstreams (including UniFi Protect); those stay on software
// unless a probe actually produced a hardware frame.
func ShouldUseHW(src model.Source, caps capability.Info) bool {
	if src.Options.ForceSoftware {
		return false
	}
	if src.Probe.HWDecode != nil && !*src.Probe.HWDecode {
		return false
	}
	if src.Probe.Codec != "" && !caps.Supports(src.Probe.Codec) {
		return false
	}
	if !caps.H264HW && !caps.HEVCHW && caps.HWTypeName == "" {
		return false
	}
	if caps.VideoToolbox && src.Kind == model.KindRTSP {
		if src.Probe.HWDecode == nil || !*src.Probe.HWDecode {
			return false
		}
	}
	return true
}

// ReloadYouTube bumps the auth generation so every YouTube worker restarts
// and clears a tripped resolve circuit breaker.
func (m *Manager) ReloadYouTube() {
	if m == nil {
		return
	}
	m.authGen.Add(1)
	m.dirty.Store(true)
}

func (m *Manager) fingerprint(s model.Source) string {
	fp := ingestFingerprint(s)
	if s.Kind == model.KindYouTube {
		return fmt.Sprintf("%s|yt:%d", fp, m.authGen.Load())
	}
	return fp
}

// ingestFingerprint captures the fields that require a worker restart.
func ingestFingerprint(s model.Source) string {
	loop, tls, up := "-", "-", uint(0)
	if s.Options.Loop != nil {
		if *s.Options.Loop {
			loop = "1"
		} else {
			loop = "0"
		}
	}
	if s.Options.TLSVerify != nil && *s.Options.TLSVerify {
		tls = "1"
	}
	if s.Options.UploadID != nil {
		up = *s.Options.UploadID
	}
	sw := 0
	if s.Options.ForceSoftware {
		sw = 1
	}
	hw := "-"
	if s.Probe.HWDecode != nil {
		if *s.Probe.HWDecode {
			hw = "1"
		} else {
			hw = "0"
		}
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d|%s|%s|%d|%s|%s",
		s.Kind, s.URL, s.Username, s.Password, s.Options.Transport,
		s.EffectiveBufferMS(), sw, loop, tls, up, s.Probe.Codec, hw)
}
