package pot

import "embed"

// Bundled bgutil yt-dlp plugin (GPL-3, separate from the MIT Go binary).
// Copied into data/yt-dlp-plugins at startup so a packaged binary does not
// need the source tree or GitHub.
//
//go:embed all:pluginfs
var bundledPluginFS embed.FS
