package game

// Wire message discriminators, carried in every frame's "t" field.
const (
	// client -> server
	MsgInput   = "input"   // movement/bomb intent for the next tick
	MsgStart   = "start"   // host begins the first round
	MsgRestart = "restart" // host begins another round after the scoreboard

	// server -> client
	MsgWelcome = "welcome"
	MsgRoster  = "roster"
	MsgState   = "state"
	MsgError   = "error"
)

// ClientMsg is every frame a client may send. Inputs dominate the traffic, so
// they share one flat struct rather than a tagged union.
type ClientMsg struct {
	T    string `json:"t"`
	DX   int    `json:"dx"`
	DY   int    `json:"dy"`
	Bomb bool   `json:"bomb"`

	// Seq is the client's own tick counter. The server consumes one input per
	// tick and echoes the last one it used, which is how a predicting client
	// knows what it still has in flight.
	Seq uint32 `json:"seq"`
}

// ArenaDTO is the immutable map, sent once per client in the welcome frame.
type ArenaDTO struct {
	W     int    `json:"w"`
	H     int    `json:"h"`
	Solid string `json:"solid"`
}

// ArenaDTO describes the grid's permanent walls for the wire.
func (g *Grid) ArenaDTO() ArenaDTO {
	return ArenaDTO{W: ArenaW, H: ArenaH, Solid: g.SolidString()}
}

// RosterEntry is the slow-changing identity of a seated player.
type RosterEntry struct {
	S int    `json:"s"`
	N string `json:"n"`
	M string `json:"m"`
	W int    `json:"w"`
}

// WelcomeMsg is the first frame a client receives: who it is, the map, and the
// tick rate it should interpolate against.
type WelcomeMsg struct {
	T      string        `json:"t"`
	You    int           `json:"you"`
	Code   string        `json:"code"`
	Host   int           `json:"host"`
	Hz     int           `json:"hz"`
	Fuse   int           `json:"fuse"`  // bomb fuse in ticks, for the client's countdown
	Build  string        `json:"build"` // server build stamp, shown next to the client's
	Arena  ArenaDTO      `json:"arena"`
	Roster []RosterEntry `json:"roster"`
}

// RosterMsg is broadcast whenever the player list, host or win tally changes.
type RosterMsg struct {
	T      string        `json:"t"`
	Host   int           `json:"host"`
	Roster []RosterEntry `json:"roster"`
}

// Snapshot is the per-tick authoritative state. Field names are single letters
// because this goes out 30 times a second to every client.
type Snapshot struct {
	T      string      `json:"t"`
	K      uint64      `json:"k"`
	Ph     Phase       `json:"ph"`
	Winner int         `json:"win"`
	P      []PlayerDTO `json:"p,omitempty"`
	B      []BombDTO   `json:"b,omitempty"`
	F      []FlameDTO  `json:"f,omitempty"`
	U      []PowerDTO  `json:"u,omitempty"`
	C      string      `json:"c,omitempty"` // crate layer, only when it changed
}

// ErrorMsg reports a refusal (room full, unknown code) before the socket closes.
type ErrorMsg struct {
	T   string `json:"t"`
	Err string `json:"err"`
}
