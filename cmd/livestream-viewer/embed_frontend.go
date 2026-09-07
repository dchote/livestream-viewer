//go:build embed_frontend

package main

import (
	"embed"
	"io/fs"
)

//go:embed frontend-dist
var frontendDist embed.FS

func getFrontendFS() (fs.FS, error) {
	return fs.Sub(frontendDist, "frontend-dist")
}
