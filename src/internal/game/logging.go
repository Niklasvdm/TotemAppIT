package game

import "log/slog"

// vlog is the always-on "verbose" logger: connection lifecycle (drops, reconnects,
// supersedes) go here so they are visible regardless of the quiet default level, and a
// room started in debug traces its gameplay through it (gated by the room's debug flag
// at the call site, so only a debug-activated room is noisy). Until SetVerboseLogger is
// called it is the default logger, so tests and any binary that does not wire logging
// still behave (and stay quiet at the default level).
var vlog = slog.Default()

// SetVerboseLogger installs the always-on logger built by config.SetupLogging. A nil
// argument is ignored so a caller can opt out without silencing everything.
func SetVerboseLogger(l *slog.Logger) {
	if l != nil {
		vlog = l
	}
}
