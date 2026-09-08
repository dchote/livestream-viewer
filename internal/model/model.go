package model

import (
	"encoding/json"
	"time"

	"github.com/dchote/livestream-viewer/internal/display/transition"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"

	KindYouTube = "youtube"
	KindRTSP    = "rtsp"
	KindHLS     = "hls"
	KindDASH    = "dash"
	KindHTTP    = "http"
	KindSRT     = "srt"
	KindRTMP    = "rtmp"
	KindFile    = "file"

	FitContain = "contain"
	FitCover   = "cover"
	FitFill    = "fill"

	ScreenKindGrid       = "grid"
	ScreenKindTransition = "transition"

	MinDwellMS  = 1000
	MinBufferMS = 0
	MaxBufferMS = 30000
	// DefaultBufferMSSegmented is the jitter buffer for YouTube, HLS, and DASH
	// when options.buffer_ms is omitted. About two to three HLS segments.
	DefaultBufferMSSegmented = 4000

	// MinPasswordLength is mirrored by frontend/src/utils/passwords.js.
	MinPasswordLength = 8

	ProbeOK          = "ok"
	ProbeUnavailable = "unavailable"
	ProbeError       = "error"
)

func ValidRole(role string) bool {
	return role == RoleAdmin || role == RoleUser
}

func ValidKind(kind string) bool {
	switch kind {
	case KindYouTube, KindRTSP, KindHLS, KindDASH, KindHTTP, KindSRT, KindRTMP, KindFile:
		return true
	}
	return false
}

func ValidFit(fit string) bool {
	switch fit {
	case FitContain, FitCover, FitFill:
		return true
	}
	return false
}

func ValidScreenKind(kind string) bool {
	return kind == ScreenKindGrid || kind == ScreenKindTransition
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
	ID                    uint            `json:"id" gorm:"primaryKey"`
	OutputWidth           int             `json:"output_width"`
	OutputHeight          int             `json:"output_height"`
	DefaultDwellMS        int             `json:"default_dwell_ms"`
	DefaultTransition     transition.Spec `json:"default_transition" gorm:"serializer:json"`
	AllowSoftwareFallback bool            `json:"allow_software_fallback"`
	MaxHWDecoders         int             `json:"max_hw_decoders"`
	ReconnectBackoffMS    int             `json:"reconnect_backoff_ms"`
	PreviewFPS            int             `json:"preview_fps"`
	PreviewWidth          int             `json:"preview_width"`
	PlaceholderColor      string          `json:"placeholder_color" gorm:"size:32"`
	GutterPx              int             `json:"gutter_px"`
}

func (RuntimeConfig) TableName() string { return "config" }

// SourceOptions is kind-specific configuration stored as JSON.
type SourceOptions struct {
	Transport string `json:"transport,omitempty"`
	Loop      *bool  `json:"loop,omitempty"`
	UploadID  *uint  `json:"upload_id,omitempty"`
	// TLSVerify applies to RTSPS (RTSP over TLS). Nil means do not verify
	// (typical for LAN NVR certificates). Set true to require a valid cert.
	TLSVerify *bool `json:"tls_verify,omitempty"`
	// BufferMS is the jitter buffer in milliseconds. Nil means the kind
	// default (4s for YouTube/HLS/DASH, 0 for RTSP and the rest). 0 is an
	// explicit live-edge / low-latency choice.
	BufferMS *int `json:"buffer_ms,omitempty"`
	// ForceSoftware skips hardware decode even when the probe and host
	// would otherwise use it.
	ForceSoftware bool `json:"force_software,omitempty"`
}

// EffectiveBufferMS is the jitter buffer used at open. Segmented live
// defaults to DefaultBufferMSSegmented so playback is smooth unless the
// operator opts into live-edge latency.
func (s Source) EffectiveBufferMS() int {
	if s.Options.BufferMS != nil {
		return *s.Options.BufferMS
	}
	switch s.Kind {
	case KindYouTube, KindHLS, KindDASH:
		return DefaultBufferMSSegmented
	default:
		return 0
	}
}

// ProbeResult is the cached outcome of probing a source.
type ProbeResult struct {
	Status   string    `json:"status,omitempty"`
	Code     string    `json:"code,omitempty"`
	Message  string    `json:"message,omitempty"`
	Codec    string    `json:"codec,omitempty"`
	Width    int       `json:"width,omitempty"`
	Height   int       `json:"height,omitempty"`
	FPS      float64   `json:"fps,omitempty"`
	HWDecode *bool     `json:"hw_decode"`
	ProbedAt time.Time `json:"probed_at,omitempty"`
}

// Source is a producer of video frames.
type Source struct {
	ID        uint          `json:"id" gorm:"primaryKey"`
	Name      string        `json:"name" gorm:"size:255;not null"`
	Kind      string        `json:"kind" gorm:"size:32;not null"`
	URL       string        `json:"url" gorm:"type:text"`
	Username  string        `json:"username,omitempty" gorm:"size:255"`
	Password  string        `json:"-" gorm:"size:255"`
	Options   SourceOptions `json:"options" gorm:"serializer:json"`
	Enabled   bool          `json:"enabled" gorm:"default:true"`
	Probe     ProbeResult   `json:"probe" gorm:"serializer:json"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`

	// Live ingest, filled by the API from the scheduler. Not persisted.
	Decoder   string `json:"decoder,omitempty" gorm:"-"`
	ErrorCode string `json:"error_code,omitempty" gorm:"-"`
	Error     string `json:"error,omitempty" gorm:"-"`
}

func (Source) TableName() string { return "sources" }

// MarshalJSON adds has_password without exposing the secret.
func (s Source) MarshalJSON() ([]byte, error) {
	type Alias Source
	return json.Marshal(struct {
		Alias
		HasPassword bool `json:"has_password"`
	}{Alias: Alias(s), HasPassword: s.Password != ""})
}

// Screen is one complete composition of the output.
type Screen struct {
	ID         uint            `json:"id" gorm:"primaryKey"`
	Name       string          `json:"name" gorm:"size:255;not null"`
	Kind       string          `json:"kind" gorm:"size:32;not null"`
	Layout     string          `json:"layout,omitempty" gorm:"size:32"`
	Loop       bool            `json:"loop"`
	Transition transition.Spec `json:"transition,omitempty" gorm:"serializer:json"`
	Tiles      []ScreenTile    `json:"tiles,omitempty" gorm:"foreignKey:ScreenID"`
	Items      []ScreenItem    `json:"items,omitempty" gorm:"foreignKey:ScreenID"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
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
	ID         uint            `json:"id" gorm:"primaryKey"`
	TourID     uint            `json:"tour_id" gorm:"index;not null"`
	Position   int             `json:"position" gorm:"not null"`
	ScreenID   uint            `json:"screen_id" gorm:"not null"`
	DwellMS    int             `json:"dwell_ms" gorm:"not null"`
	Transition transition.Spec `json:"transition" gorm:"serializer:json"`
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

// ScreenRef is a compact reference used in 409 conflict details.
type ScreenRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// SourceRef is a compact reference used in 409 conflict details.
type SourceRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}
