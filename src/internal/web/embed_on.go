//go:build embed

// Package web optionally embeds the compiled React SPA (src/internal/web/dist,
// produced by `npm run build`). It is compiled in only with `go build -tags embed`;
// without the tag the stub in embed_off.go is used so plain builds and tests need
// no frontend build. totemd serves this FS for all non-API routes.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded production build, or ok=false if it can't be read.
func FS() (fs.FS, bool) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, false
	}
	return sub, true
}
