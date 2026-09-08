package database

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/dchote/livestream-viewer/internal/config"
	"github.com/dchote/livestream-viewer/internal/display/layout"
	"github.com/dchote/livestream-viewer/internal/display/transition"
	"github.com/dchote/livestream-viewer/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const defaultAdminUser = "admin"
const defaultAdminPass = "admin"

// Open opens SQLite, migrates the schema, and seeds first-run rows.
func Open(cfg *config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(cfg.DatabasePath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := tune(db); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.RuntimeConfig{},
		&model.Source{},
		&model.Screen{},
		&model.ScreenTile{},
		&model.TileSequence{},
		&model.ScreenItem{},
		&model.Tour{},
		&model.TourEntry{},
		&model.Upload{},
	); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := migrateRetiredLayouts(db); err != nil {
		return nil, err
	}

	if err := migrateRetiredTransitions(db); err != nil {
		return nil, err
	}

	if err := seed(db); err != nil {
		return nil, err
	}
	return db, nil
}

// tune configures SQLite for a long-running single-process service.
//
// The dataset is tiny but writes arrive concurrently from API handlers, probe
// results, and the scheduler. With the default rollback journal and no busy
// timeout those collide as "database is locked" errors that surface to the UI.
// A single writer connection removes the contention entirely; WAL keeps
// readers from blocking behind it.
func tune(db *gorm.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
	}
	for _, p := range pragmas {
		if err := db.Exec(p).Error; err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("sql db handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	return nil
}

// retiredLayouts maps layout ids that have been removed from the catalogue to
// their replacement. Screens created before the removal keep working.
var retiredLayouts = map[string]string{
	// "1x1" was geometrically identical to the full-bleed layout.
	"1x1": layout.FullBleedID,
}

func migrateRetiredLayouts(db *gorm.DB) error {
	for old, replacement := range retiredLayouts {
		res := db.Model(&model.Screen{}).Where("layout = ?", old).Update("layout", replacement)
		if res.Error != nil {
			return fmt.Errorf("migrate layout %q: %w", old, res.Error)
		}
		if res.RowsAffected > 0 {
			slog.Info("migrated retired layout", "from", old, "to", replacement, "screens", res.RowsAffected)
		}
	}
	return nil
}

// migrateRetiredTransitions rewrites stored specs whose type is no longer in
// the catalogue. Transitions are JSON columns, so this is done in Go rather
// than SQL. Without it a screen saved with a withdrawn type would fail
// validation on the next strategy build and take the whole display with it.
func migrateRetiredTransitions(db *gorm.DB) error {
	migrated := 0

	var cfg model.RuntimeConfig
	if err := db.First(&cfg).Error; err == nil {
		if spec, changed := transition.Retire(cfg.DefaultTransition); changed {
			cfg.DefaultTransition = spec
			if err := db.Save(&cfg).Error; err != nil {
				return fmt.Errorf("migrate default transition: %w", err)
			}
			migrated++
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("load config for transition migration: %w", err)
	}

	var screens []model.Screen
	if err := db.Find(&screens).Error; err != nil {
		return fmt.Errorf("load screens for transition migration: %w", err)
	}
	for i := range screens {
		spec, changed := transition.Retire(screens[i].Transition)
		if !changed {
			continue
		}
		if err := db.Model(&screens[i]).Update("transition", spec).Error; err != nil {
			return fmt.Errorf("migrate screen %d transition: %w", screens[i].ID, err)
		}
		migrated++
	}

	var entries []model.TourEntry
	if err := db.Find(&entries).Error; err != nil {
		return fmt.Errorf("load tour entries for transition migration: %w", err)
	}
	for i := range entries {
		spec, changed := transition.Retire(entries[i].Transition)
		if !changed {
			continue
		}
		if err := db.Model(&entries[i]).Update("transition", spec).Error; err != nil {
			return fmt.Errorf("migrate tour entry %d transition: %w", entries[i].ID, err)
		}
		migrated++
	}

	if migrated > 0 {
		slog.Info("migrated retired transitions to fade", "count", migrated)
	}
	return nil
}

func seed(db *gorm.DB) error {
	var users int64
	if err := db.Model(&model.User{}).Count(&users).Error; err != nil {
		return err
	}
	if users == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte(defaultAdminPass), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash admin password: %w", err)
		}
		u := model.User{
			Username:           defaultAdminUser,
			PasswordHash:       string(hash),
			Role:               model.RoleAdmin,
			MustChangePassword: true,
		}
		if err := db.Create(&u).Error; err != nil {
			return fmt.Errorf("seed admin: %w", err)
		}
		slog.Info("seeded default admin user", "username", defaultAdminUser)
	} else if err := flagDefaultAdminPassword(db); err != nil {
		return err
	}

	var configs int64
	if err := db.Model(&model.RuntimeConfig{}).Count(&configs).Error; err != nil {
		return err
	}
	if configs == 0 {
		rc := model.RuntimeConfig{
			OutputWidth:           1920,
			OutputHeight:          1080,
			DefaultDwellMS:        30000,
			DefaultTransition:     transition.DefaultCut(),
			AllowSoftwareFallback: true,
			MaxHWDecoders:         4,
			ReconnectBackoffMS:    2000,
			PreviewFPS:            2,
			PreviewWidth:          640,
			PlaceholderColor:      "#111111",
			GutterPx:              4,
		}
		if err := db.Create(&rc).Error; err != nil {
			return fmt.Errorf("seed config: %w", err)
		}
	}

	var tours int64
	if err := db.Model(&model.Tour{}).Count(&tours).Error; err != nil {
		return err
	}
	if tours == 0 {
		t := model.Tour{Enabled: false, Loop: false}
		if err := db.Create(&t).Error; err != nil {
			return fmt.Errorf("seed tour: %w", err)
		}
	}
	return nil
}

// flagDefaultAdminPassword marks the seeded admin for a password change when the
// password is still the published default. Existing databases from before this
// flag existed would otherwise skip the login prompt.
func flagDefaultAdminPassword(db *gorm.DB) error {
	var u model.User
	if err := db.Where("username = ?", defaultAdminUser).First(&u).Error; err != nil {
		return nil
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(defaultAdminPass)) != nil {
		return nil
	}
	if u.MustChangePassword {
		return nil
	}
	return db.Model(&u).Update("must_change_password", true).Error
}
