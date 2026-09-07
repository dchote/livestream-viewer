package database

import (
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

	if err := seed(db); err != nil {
		return nil, err
	}
	return db, nil
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
			OutputRotation:        0,
			DefaultDwellMS:        30000,
			DefaultTransition:     transition.DefaultCut(),
			AllowSoftwareFallback: true,
			MaxHWDecoders:         4,
			ReconnectBackoffMS:    2000,
			OfflineGraceMS:        5000,
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
