package ingest

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dchote/livestream-viewer/internal/config"
	"github.com/dchote/livestream-viewer/internal/database"
	"github.com/dchote/livestream-viewer/internal/frame"
	"github.com/dchote/livestream-viewer/internal/ingest/capability"
	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(&config.Config{
		DatabasePath: filepath.Join(dir, "test.sqlite"),
		DataDir:      dir,
		JWTSecret:    "test-secret",
		HTTPPort:     8099,
	})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

type recordingBinder struct {
	bound   []uint
	unbound []uint
}

func (b *recordingBinder) Bind(id uint, _ *frame.Slot) bool {
	b.bound = append(b.bound, id)
	return true
}
func (b *recordingBinder) Unbind(id uint) { b.unbound = append(b.unbound, id) }

// sync loads every needed source in one query keyed by primary key. If that
// query does not behave as expected no worker ever starts and the wall stays
// black, so it is worth asserting against a real database.
func TestSyncStartsWorkersForEnabledSources(t *testing.T) {
	db := testDB(t)
	enabled := model.Source{Name: "cam-a", Kind: model.KindFile, URL: "a.mp4", Enabled: true}
	disabled := model.Source{Name: "cam-b", Kind: model.KindFile, URL: "b.mp4", Enabled: false}
	other := model.Source{Name: "cam-c", Kind: model.KindFile, URL: "c.mp4", Enabled: true}
	for _, s := range []*model.Source{&enabled, &disabled, &other} {
		if err := db.Create(s).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Source.Enabled carries gorm:"default:true", so Create ignores an
	// explicit false and stores the default. Disable it with an update.
	if err := db.Model(&disabled).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}

	binder := &recordingBinder{}
	m := NewManager(db, nil, capability.Info{}, resolver.Tools{}, binder)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// A large start delay keeps the workers parked so the test never touches
	// libav; only the bookkeeping is under test.
	m.sync(ctx, []uint{enabled.ID, disabled.ID, other.ID})

	m.mu.Lock()
	workers := len(m.workers)
	_, hasEnabled := m.workers[enabled.ID]
	_, hasDisabled := m.workers[disabled.ID]
	applied := append([]uint(nil), m.applied...)
	m.mu.Unlock()

	if workers != 2 {
		t.Fatalf("started %d workers, want 2", workers)
	}
	if !hasEnabled {
		t.Error("enabled source did not start")
	}
	if hasDisabled {
		t.Error("disabled source must not start")
	}
	if len(applied) != 2 {
		t.Fatalf("applied = %v", applied)
	}

	// The same request must now be a no-op.
	if !m.settled([]uint{other.ID, enabled.ID}) {
		t.Fatal("manager should be settled on the applied set")
	}
	if m.settled([]uint{enabled.ID}) {
		t.Fatal("a smaller set must not be settled")
	}

	// Dropping a source stops and unbinds exactly that worker.
	m.sync(ctx, []uint{enabled.ID})
	m.mu.Lock()
	workers = len(m.workers)
	m.mu.Unlock()
	if workers != 1 {
		t.Fatalf("after narrowing, %d workers remain", workers)
	}
	if len(binder.unbound) != 1 || binder.unbound[0] != other.ID {
		t.Fatalf("unbound = %v, want [%d]", binder.unbound, other.ID)
	}

	m.StopAll()
	m.mu.Lock()
	workers, applied = len(m.workers), m.applied
	m.mu.Unlock()
	if workers != 0 || applied != nil {
		t.Fatalf("StopAll left %d workers, applied=%v", workers, applied)
	}
}

// A missing row must not start a worker or wedge the manager.
func TestSyncSkipsUnknownSourceIDs(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, nil, capability.Info{}, resolver.Tools{}, &recordingBinder{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.sync(ctx, []uint{999, 1000})

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.workers) != 0 {
		t.Fatalf("started %d workers for unknown ids", len(m.workers))
	}
}

// decodePolicy must fall back to the stored config when no snapshot exists yet.
func TestDecodePolicyFallsBackToDatabase(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, nil, capability.Info{}, resolver.Tools{}, nil)
	maxHW, allowSW, backoff := m.decodePolicy()
	if backoff <= 0 {
		t.Fatalf("backoff = %d", backoff)
	}
	if maxHW < 0 {
		t.Fatalf("maxHW = %d", maxHW)
	}
	_ = allowSW

	// Sanity: the seeded defaults are what the worker would use.
	var cfg model.RuntimeConfig
	if err := db.First(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	if backoff != cfg.ReconnectBackoffMS {
		t.Fatalf("backoff %d does not match stored %d", backoff, cfg.ReconnectBackoffMS)
	}
	_ = time.Second
}
