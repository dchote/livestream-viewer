package source

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/dchote/livestream-viewer/internal/model"
	"gorm.io/gorm"
)

// ValidateCreate checks a new source record.
func ValidateCreate(s *model.Source) error {
	if s == nil {
		return fmt.Errorf("source is required")
	}
	s.Name = strings.TrimSpace(s.Name)
	s.Kind = strings.TrimSpace(s.Kind)
	s.URL = strings.TrimSpace(s.URL)
	s.Username = strings.TrimSpace(s.Username)
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	if !model.ValidKind(s.Kind) {
		return fmt.Errorf("invalid kind %q", s.Kind)
	}
	return validateKind(s, true)
}

// ValidateUpdate checks a source after PATCH fields have been applied.
func ValidateUpdate(s *model.Source) error {
	if s == nil {
		return fmt.Errorf("source is required")
	}
	s.Name = strings.TrimSpace(s.Name)
	s.Kind = strings.TrimSpace(s.Kind)
	s.URL = strings.TrimSpace(s.URL)
	s.Username = strings.TrimSpace(s.Username)
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	if !model.ValidKind(s.Kind) {
		return fmt.Errorf("invalid kind %q", s.Kind)
	}
	return validateKind(s, false)
}

func validateKind(s *model.Source, creating bool) error {
	switch s.Kind {
	case model.KindFile:
		if s.Options.UploadID == nil && s.URL == "" {
			return fmt.Errorf("file sources require upload_id or a stored path")
		}
		if s.Options.Transport != "" {
			return fmt.Errorf("transport is only valid for RTSP sources")
		}
	case model.KindRTSP:
		if s.URL == "" {
			return fmt.Errorf("url is required")
		}
		if _, err := url.Parse(s.URL); err != nil {
			return fmt.Errorf("url is not valid")
		}
		t := strings.ToLower(strings.TrimSpace(s.Options.Transport))
		if t == "" {
			s.Options.Transport = "tcp"
		} else if t != "tcp" && t != "udp" {
			return fmt.Errorf("rtsp transport must be tcp or udp")
		} else {
			s.Options.Transport = t
		}
	default:
		if s.URL == "" {
			return fmt.Errorf("url is required")
		}
		if creating || s.URL != "" {
			if _, err := url.Parse(s.URL); err != nil {
				return fmt.Errorf("url is not valid")
			}
		}
		if s.Options.Transport != "" {
			return fmt.Errorf("transport is only valid for RTSP sources")
		}
	}
	return nil
}

// ScreenRef is an alias for conflict payloads.
type ScreenRef = model.ScreenRef

// ReferencingScreens returns screens that use sourceID in a tile, sequence, or playlist item.
func ReferencingScreens(db *gorm.DB, sourceID uint) ([]model.ScreenRef, error) {
	ids := map[uint]struct{}{}

	var tiles []model.ScreenTile
	if err := db.Where("source_id = ?", sourceID).Find(&tiles).Error; err != nil {
		return nil, err
	}
	for _, t := range tiles {
		ids[t.ScreenID] = struct{}{}
	}

	var seqs []model.TileSequence
	if err := db.Where("source_id = ?", sourceID).Find(&seqs).Error; err != nil {
		return nil, err
	}
	if len(seqs) > 0 {
		tileIDs := make([]uint, 0, len(seqs))
		for _, s := range seqs {
			tileIDs = append(tileIDs, s.TileID)
		}
		var seqTiles []model.ScreenTile
		if err := db.Where("id IN ?", tileIDs).Find(&seqTiles).Error; err != nil {
			return nil, err
		}
		for _, t := range seqTiles {
			ids[t.ScreenID] = struct{}{}
		}
	}

	var items []model.ScreenItem
	if err := db.Where("source_id = ?", sourceID).Find(&items).Error; err != nil {
		return nil, err
	}
	for _, it := range items {
		ids[it.ScreenID] = struct{}{}
	}

	if len(ids) == 0 {
		return []model.ScreenRef{}, nil
	}
	idList := make([]uint, 0, len(ids))
	for id := range ids {
		idList = append(idList, id)
	}
	var screens []model.Screen
	if err := db.Select("id", "name").Where("id IN ?", idList).Order("id asc").Find(&screens).Error; err != nil {
		return nil, err
	}
	out := make([]model.ScreenRef, 0, len(screens))
	for _, s := range screens {
		out = append(out, model.ScreenRef{ID: s.ID, Name: s.Name})
	}
	return out, nil
}

// SourcesUsingUpload returns file sources that reference the upload.
func SourcesUsingUpload(db *gorm.DB, uploadID uint, path string) ([]model.SourceRef, error) {
	var sources []model.Source
	if err := db.Where("kind = ?", model.KindFile).Find(&sources).Error; err != nil {
		return nil, err
	}
	out := make([]model.SourceRef, 0)
	for _, s := range sources {
		if s.Options.UploadID != nil && *s.Options.UploadID == uploadID {
			out = append(out, model.SourceRef{ID: s.ID, Name: s.Name})
			continue
		}
		if path != "" && s.URL == path {
			out = append(out, model.SourceRef{ID: s.ID, Name: s.Name})
		}
	}
	return out, nil
}
