package database

import (
	"path/filepath"
	"testing"

	"github.com/dchote/livestream-viewer/internal/config"
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
