//go:build !embed

package web

import "io/fs"

// FS reports that no SPA is embedded. This is the default build (dev uses the
// Vite dev server); build with `-tags embed` after `npm run build` to serve the
// SPA from totemd itself.
func FS() (fs.FS, bool) { return nil, false }
