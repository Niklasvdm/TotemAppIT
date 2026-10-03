package game

import (
	"math"
	"math/rand/v2"
)

// Simulation constants. Durations are expressed in ticks so the simulation is
// fully integer-driven and reproducible; TickHz is the only bridge to wall time.
const (
	TickHz = 30

	fuseTicks  = TickHz * 2        // bomb detonates 2s after it is dropped
	flameTicks = TickHz * 45 / 100 // flames linger ~0.45s
	overTicks  = TickHz * 3        // scoreboard dwell before a new round can start

	baseSpeed = 4.2 // tiles per second
	speedStep = 0.9 // added per speed pickup
	maxSpeed  = 8.0

	playerRadius = 0.34 // half-extent of the player's collision box, in tiles

	baseBombs = 1
	basePower = 1 // classic opening reach; fire pickups grow it
	maxBombs  = 6
	maxPower  = 8
)

// Phase is the match lifecycle. Clients render different screens per phase.
type Phase string

const (
	PhaseLobby Phase = "lobby"
	PhasePlay  Phase = "play"
	PhaseOver  Phase = "over"
)

// PowerKind is a crate drop.
type PowerKind string

const (
	PowerBomb  PowerKind = "bomb"  // +1 concurrent bomb
	PowerFire  PowerKind = "fire"  // +1 blast radius
	PowerSpeed PowerKind = "speed" // faster movement
)

// Input is one client's intent for a tick. The server never trusts positions
// from clients — only a direction and whether the bomb key is down.
type Input struct {
	DX   int  `json:"dx"`
	DY   int  `json:"dy"`
	Bomb bool `json:"bomb"`
}

// clamp normalises a client-supplied direction to -1, 0 or 1.
func (in Input) clamp() Input {
	sign := func(v int) int {
		switch {
		case v > 0:
			return 1
		case v < 0:
			return -1
		}
		return 0
	}
	return Input{DX: sign(in.DX), DY: sign(in.DY), Bomb: in.Bomb}
}

// Player is one participant. Position is in tile units: the centre of tile
// (tx, ty) is (tx+0.5, ty+0.5).
type Player struct {
	Slot   int
	Name   string
	Animal string // totem animal slug, for the client's sprite/emoji
	X, Y   float64
	Alive  bool
	Bombs  int // max concurrent bombs
	Power  int // blast radius in tiles
	Speed  float64
	Wins   int

	in       Input
	bombHeld bool // edge detection: holding the key must not spam bombs

	// bombLatch remembers a bomb request that arrived between two ticks. The
	// key is otherwise sampled once per tick, so a tap shorter than 1/TickHz
	// would land and clear before the tick ever saw it.
	bombLatch bool
}

// Bomb is a placed bomb occupying exactly one tile.
type Bomb struct {
	X, Y  int
	Owner int
	Fuse  int
	Power int

	// standing are the slots still overlapping this tile. A player may walk off
	// the bomb they just dropped, but not back onto it once clear.
	standing map[int]bool
}

// Flame is one tile of blast, lethal while TTL > 0.
type Flame struct {
	X, Y int
	TTL  int
}

// Powerup is a pickup revealed by destroying a crate.
type Powerup struct {
	X, Y int
	Kind PowerKind
}

// Match is the whole authoritative state of one room's game.
type Match struct {
	Grid    *Grid
	Phase   Phase
	Tick    uint64
	Players map[int]*Player
	Bombs   []*Bomb
	Flames  []*Flame
	Powers  []*Powerup

	// Winner is the slot that won the last round, or -1 for a draw. Only
	// meaningful in PhaseOver.
	Winner int

	seed   uint64
	overAt uint64

	// rng drives crate drops. It belongs to the match (not the global source)
	// so a seed reproduces a whole game, drops included.
	rng *rand.Rand
}

// NewMatch creates a lobby-phase match. Players join before Start is called.
func NewMatch(seed uint64) *Match {
	return &Match{
		Grid:    NewGrid(seed),
		Phase:   PhaseLobby,
		Players: map[int]*Player{},
		Winner:  -1,
		seed:    seed,
		rng:     rand.New(rand.NewPCG(seed, 0x243f6a8885a308d3)),
	}
}

// AddPlayer seats a player in the lowest free slot, returning it. It reports
// false when the room is full.
func (m *Match) AddPlayer(name, animal string) (int, bool) {
	for slot := 0; slot < MaxPlayers; slot++ {
		if _, taken := m.Players[slot]; taken {
			continue
		}
		p := &Player{Slot: slot, Name: name, Animal: animal}
		m.Players[slot] = p
		m.resetPlayer(p)
		p.Alive = m.Phase != PhasePlay // joining mid-round waits for the next one
		return slot, true
	}
	return 0, false
}

// RemovePlayer frees a slot.
func (m *Match) RemovePlayer(slot int) {
	delete(m.Players, slot)
}

// SetInput records a client's intent for the next tick. Direction is a level
// (whatever is held when the tick runs), but a bomb press latches, so a quick
// tap between two ticks still drops a bomb.
func (m *Match) SetInput(slot int, in Input) {
	p, ok := m.Players[slot]
	if !ok {
		return
	}
	p.in = in.clamp()
	if p.in.Bomb {
		p.bombLatch = true
	}
}

func (m *Match) resetPlayer(p *Player) {
	sx, sy := m.Grid.Spawn(p.Slot)
	p.X, p.Y = float64(sx)+0.5, float64(sy)+0.5
	p.Alive = true
	p.Bombs = baseBombs
	p.Power = basePower
	p.Speed = baseSpeed
	p.in = Input{}
	p.bombHeld = false
	p.bombLatch = false
}

// Start begins a round: a fresh map, everyone back to their corner with base
// stats. Round number is folded into the seed so each round differs.
func (m *Match) Start(round uint64) {
	roundSeed := m.seed + round*0x2545f4914f6cdd1d
	m.Grid = NewGrid(roundSeed)
	m.rng = rand.New(rand.NewPCG(roundSeed, 0x243f6a8885a308d3))
	m.Bombs = nil
	m.Flames = nil
	m.Powers = nil
	m.Winner = -1
	for _, p := range m.Players {
		m.resetPlayer(p)
	}
	m.Phase = PhasePlay
}

// AliveCount reports how many players are still in the round.
func (m *Match) AliveCount() int {
	n := 0
	for _, p := range m.Players {
		if p.Alive {
			n++
		}
	}
	return n
}

// CanRestart reports whether the scoreboard dwell after a round has elapsed.
func (m *Match) CanRestart() bool {
	return m.Phase == PhaseOver && m.Tick >= m.overAt+overTicks
}

// --- wire DTOs --------------------------------------------------------------

// PlayerDTO is the per-tick slice of a player. Name, animal and wins change
// rarely and ride in RosterMsg instead of in every snapshot.
type PlayerDTO struct {
	S int     `json:"s"`
	X float64 `json:"x"`
	Y float64 `json:"y"`
	A bool    `json:"a"`
	B int     `json:"b"`
	P int     `json:"p"`
}

// BombDTO carries the fuse so the client can pulse the sprite in sync.
type BombDTO struct {
	X int `json:"x"`
	Y int `json:"y"`
	F int `json:"f"`
}

type FlameDTO struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type PowerDTO struct {
	X int       `json:"x"`
	Y int       `json:"y"`
	K PowerKind `json:"k"`
}

// r2 trims coordinates to two decimals: at 30 snapshots a second the extra
// mantissa digits are pure bandwidth.
func r2(v float64) float64 { return math.Round(v*100) / 100 }

// Snapshot renders the complete current state for the wire. It is a pure read:
// call it as often as you like and every call describes the whole match. Thinning
// it for a client that has already seen most of it is the Room's job, because
// only the Room knows what each client has received.
func (m *Match) Snapshot() Snapshot {
	s := Snapshot{T: MsgState, K: m.Tick, Ph: m.Phase, Winner: m.Winner}

	for slot := 0; slot < MaxPlayers; slot++ {
		p, ok := m.Players[slot]
		if !ok {
			continue
		}
		s.P = append(s.P, PlayerDTO{S: p.Slot, X: r2(p.X), Y: r2(p.Y), A: p.Alive, B: p.Bombs, P: p.Power})
	}
	for _, b := range m.Bombs {
		s.B = append(s.B, BombDTO{X: b.X, Y: b.Y, F: b.Fuse})
	}
	for _, f := range m.Flames {
		s.F = append(s.F, FlameDTO{X: f.X, Y: f.Y})
	}
	for _, u := range m.Powers {
		s.U = append(s.U, PowerDTO{X: u.X, Y: u.Y, K: u.Kind})
	}
	s.C = m.Grid.CrateString()
	return s
}

// Roster is the slowly-changing player list for the lobby and the scoreboard.
func (m *Match) Roster() []RosterEntry {
	out := make([]RosterEntry, 0, len(m.Players))
	for slot := 0; slot < MaxPlayers; slot++ {
		p, ok := m.Players[slot]
		if !ok {
			continue
		}
		out = append(out, RosterEntry{S: p.Slot, N: p.Name, M: p.Animal, W: p.Wins})
	}
	return out
}
