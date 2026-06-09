// Package web embeds the compiled frontend assets for serving.
package web

import "embed"

// Dist holds the embedded compiled frontend assets.
//go:embed dist
var Dist embed.FS
