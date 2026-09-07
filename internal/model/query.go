package model

import "gorm.io/gorm"

// PreloadScreenAssociations applies the ordered preloads that every full screen
// read needs. Tiles, tile sequences, and playlist items must come back in their
// stored order or the resolved display strategy is silently reordered, so the
// ordering lives here rather than being restated at each call site.
func PreloadScreenAssociations(db *gorm.DB) *gorm.DB {
	return db.
		Preload("Tiles", orderBy("cell_index asc")).
		Preload("Tiles.Sequence", orderBy("position asc")).
		Preload("Items", orderBy("position asc"))
}

// PreloadTourEntries loads tour entries in playback order.
func PreloadTourEntries(db *gorm.DB) *gorm.DB {
	return db.Preload("Entries", orderBy("position asc"))
}

func orderBy(clause string) func(*gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB { return tx.Order(clause) }
}
