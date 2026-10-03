// Package buildinfo identifies which build is running.
//
// "Is the thing I just deployed actually the thing that is running?" is a
// question that costs real debugging time, especially with a browser that may
// be serving a cached bundle. Both halves of the app therefore carry a stamp,
// and the game HUD shows them side by side.
package buildinfo

import "runtime/debug"

// Version is the human-readable release, mirrored from the repo's VERSION file
// by the deploy's -ldflags. It stays "dev" for a plain `go build`, which is
// itself useful information.
var Version = "dev"

// revision is resolved once: Go stamps the VCS commit into every binary built
// inside a git tree, so the exact source is identifiable with no build flags.
var revision = func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			if len(s.Value) > 7 {
				return s.Value[:7]
			}
			return s.Value
		}
	}
	return ""
}()

// String is the full build stamp, e.g. "0.4.0+a1b2c3d".
func String() string {
	if revision == "" {
		return Version
	}
	return Version + "+" + revision
}
