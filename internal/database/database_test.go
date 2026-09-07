package database

import (
	"path/filepath"
	"testing"

	"github.com/dchote/livestream-viewer/internal/config"
	"github.com/dchote/livestream-viewer/internal/display/layout"
	"github.com/dchote/livestream-viewer/internal/model"
)

func TestOpenSeedsAdminAndTour(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DatabasePath: filepath.Join(dir, "test.sqlite"),
		DataDir:      dir,
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var user model.User
	if err := db.Where("username = ?", defaultAdminUser).First(&user).Error; err != nil {
		t.Fatalf("admin: %v", err)
	}
	if user.Role != model.RoleAdmin {
		t.Fatalf("role = %s", user.Role)
	}
	if !user.MustChangePassword {
		t.Fatal("seeded admin must change password")
	}

	var n int64
	if err := db.Model(&model.RuntimeConfig{}).Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("config rows = %d err=%v", n, err)
	}
	if err := db.Model(&model.Tour{}).Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("tour rows = %d err=%v", n, err)
	}

	db2, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := db2.Model(&model.User{}).Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("users after re-open = %d", n)
	}
}

// Screens created before "1x1" was retired must be rewritten to the full-bleed
// layout, or ByID misses and the screen renders with no geometry.
func TestOpenMigratesRetiredLayouts(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DatabasePath: filepath.Join(dir, "test.sqlite"),
		DataDir:      dir,
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	legacy := model.Screen{Name: "legacy", Kind: model.ScreenKindGrid, Layout: "1x1"}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	keep := model.Screen{Name: "quad", Kind: model.ScreenKindGrid, Layout: "2x2"}
	if err := db.Create(&keep).Error; err != nil {
		t.Fatal(err)
	}

	// Reopen: migration runs on every start, not just first run.
	if _, err := Open(cfg); err != nil {
		t.Fatal(err)
	}

	var migrated model.Screen
	if err := db.First(&migrated, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if migrated.Layout != layout.FullBleedID {
		t.Fatalf("layout = %q, want %q", migrated.Layout, layout.FullBleedID)
	}

	var untouched model.Screen
	if err := db.First(&untouched, keep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if untouched.Layout != "2x2" {
		t.Fatalf("untouched layout = %q, want 2x2", untouched.Layout)
	}
}
