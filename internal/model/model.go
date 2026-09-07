package model

import "time"

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

func ValidRole(role string) bool {
	return role == RoleAdmin || role == RoleUser
}

// User is a management UI account.
type User struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	Username           string    `json:"username" gorm:"uniqueIndex;size:128;not null"`
	PasswordHash       string    `json:"-" gorm:"not null"`
	Role               string    `json:"role" gorm:"size:32;not null;default:user"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (User) TableName() string { return "users" }

// RuntimeConfig is the single-row database-backed configuration.
type RuntimeConfig struct {
	ID                    uint   `json:"id" gorm:"primaryKey"`
	OutputWidth           int    `json:"output_width"`
	OutputHeight          int    `json:"output_height"`
	OutputRotation        int    `json:"output_rotation"`
	DefaultDwellMS        int    `json:"default_dwell_ms"`
	DefaultTransitionJSON string `json:"default_transition" gorm:"type:text"`
	AllowSoftwareFallback bool   `json:"allow_software_fallback"`
	MaxHWDecoders         int    `json:"max_hw_decoders"`
	ReconnectBackoffMS    int    `json:"reconnect_backoff_ms"`
	OfflineGraceMS        int    `json:"offline_grace_ms"`
	PreviewFPS            int    `json:"preview_fps"`
	PreviewWidth          int    `json:"preview_width"`
	PlaceholderColor      string `json:"placeholder_color" gorm:"size:32"`
	GutterPx              int    `json:"gutter_px"`
}

func (RuntimeConfig) TableName() string { return "config" }

// Source is a producer of video frames.
type Source struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:255;not null"`
	Kind        string    `json:"kind" gorm:"size:32;not null"`
	URL         string    `json:"url" gorm:"type:text"`
	Username    string    `json:"username,omitempty" gorm:"size:255"`
	Password    string    `json:"-" gorm:"size:255"`
	OptionsJSON string    `json:"options,omitempty" gorm:"type:text"`
	Enabled     bool      `json:"enabled" gorm:"default:true"`
	ProbeJSON   string    `json:"probe,omitempty" gorm:"type:text"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Source) TableName() string { return "sources" }

// Screen is one complete composition of the output.
type Screen struct {
	ID             uint         `json:"id" gorm:"primaryKey"`
	Name           string       `json:"name" gorm:"size:255;not null"`
	Kind           string       `json:"kind" gorm:"size:32;not null"`
	Layout         string       `json:"layout,omitempty" gorm:"size:32"`
	Loop           bool         `json:"loop"`
	TransitionJSON string       `json:"transition,omitempty" gorm:"type:text"`
	Tiles          []ScreenTile `json:"tiles,omitempty" gorm:"foreignKey:ScreenID"`
	Items          []ScreenItem `json:"items,omitempty" gorm:"foreignKey:ScreenID"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

func (Screen) TableName() string { return "screens" }

// ScreenTile is one cell of a grid screen.
type ScreenTile struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	ScreenID  uint           `json:"screen_id" gorm:"index;not null"`
	CellIndex int            `json:"index" gorm:"not null"`
	SourceID  *uint          `json:"source_id"`
	Fit       string         `json:"fit" gorm:"size:16;not null;default:contain"`
	Sequence  []TileSequence `json:"sequence,omitempty" gorm:"foreignKey:TileID"`
}

func (ScreenTile) TableName() string { return "screen_tiles" }

// TileSequence is an ordered source rotating within a single tile.
type TileSequence struct {
	ID       uint `json:"id" gorm:"primaryKey"`
	TileID   uint `json:"tile_id" gorm:"index;not null"`
	Position int  `json:"position" gorm:"not null"`
	SourceID uint `json:"source_id" gorm:"not null"`
	DwellMS  int  `json:"dwell_ms" gorm:"not null"`
}

func (TileSequence) TableName() string { return "tile_sequences" }

// ScreenItem is a playlist entry on a transition screen.
type ScreenItem struct {
	ID       uint   `json:"id" gorm:"primaryKey"`
	ScreenID uint   `json:"screen_id" gorm:"index;not null"`
	Position int    `json:"position" gorm:"not null"`
	SourceID uint   `json:"source_id" gorm:"not null"`
	DwellMS  int    `json:"dwell_ms" gorm:"not null"`
	Fit      string `json:"fit" gorm:"size:16;not null;default:contain"`
}

func (ScreenItem) TableName() string { return "screen_items" }

// Tour is the top-level display strategy (single row).
type Tour struct {
	ID      uint        `json:"id" gorm:"primaryKey"`
	Enabled bool        `json:"enabled"`
	Loop    bool        `json:"loop"`
	Entries []TourEntry `json:"entries" gorm:"foreignKey:TourID"`
}

func (Tour) TableName() string { return "tours" }

// TourEntry is an ordered screen in the tour.
type TourEntry struct {
	ID             uint   `json:"id" gorm:"primaryKey"`
	TourID         uint   `json:"tour_id" gorm:"index;not null"`
	Position       int    `json:"position" gorm:"not null"`
	ScreenID       uint   `json:"screen_id" gorm:"not null"`
	DwellMS        int    `json:"dwell_ms" gorm:"not null"`
	TransitionJSON string `json:"transition,omitempty" gorm:"type:text"`
}

func (TourEntry) TableName() string { return "tour_entries" }

// Upload is metadata for a file stored in the data directory.
type Upload struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:255;not null"`
	Path      string    `json:"path" gorm:"type:text;not null"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type" gorm:"size:128"`
	CreatedAt time.Time `json:"created_at"`
}

func (Upload) TableName() string { return "uploads" }
