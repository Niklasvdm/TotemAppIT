package game

import "encoding/json"

// decodeCmd unmarshals one client command frame into T, returning ok=false on malformed
// JSON. It centralises the per-game "var c struct{...}; if json.Unmarshal(raw,&c)!=nil"
// boilerplate so every Command handler decodes the same way, and gives one place to
// harden decoding later if needed. The frame size is already bounded upstream
// (maxCommandBytes in Room.Command), so this only has to parse, not police length.
//
// Decoding stays lenient (unknown fields are ignored) on purpose: clients send only the
// fields a given message needs, a subset of each game's command struct, so rejecting
// unknown fields would break valid frames. T is always a plain data struct of scalar
// JSON fields, never anything that executes, so a decoded command is inert data.
func decodeCmd[T any](raw []byte) (T, bool) {
	var c T
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, false
	}
	return c, true
}
