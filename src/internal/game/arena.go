// Package game implements a server-authoritative, WebSocket-multiplayer arena
// game (Bomberman-style) themed on the totem animals.
//
// The package is layered so the rules stay testable without a network:
//
//	arena.go     static map layout + destructible crate layer
//	state.go     match state and the DTOs that go on the wire
//	sim.go       the pure tick function — no I/O, no goroutines
//	protocol.go  client/server message envelopes
//	room.go      one goroutine per room: owns a Match, runs the tick loop
//	registry.go  room codes -> rooms, with idle reaping
//	ws.go        the WebSocket transport
//
// Only room.go/ws.go know about concurrency; sim.go is a plain function of
// (state, inputs) -> state, which is what the unit tests exercise.
package game

import "math/rand/v2"

// Arena dimensions. Both are odd so the classic pillar lattice (walls on
// even/even interior cells) leaves every corridor one tile wide.
const (
	ArenaW = 15
	ArenaH = 13

	// crateDensity is the share of free interior cells seeded with a
	// destructible crate. Spawn pockets are always excluded.
	crateDensity = 0.72

	// spawnClearDepth is how far the crate-free pocket reaches along each axis
	// from a spawn. It must exceed the starting blast radius, or a player's
	// very first bomb could have no survivable tile to retreat to.
	spawnClearDepth = 2
)

// MaxPlayers is the number of spawn corners the arena provides.
const MaxPlayers = 4

// Grid is the map: a static layer of permanent walls and a mutable layer of
// destructible crates, both row-major and indexed by idx(x, y).
type Grid struct {
	solid []bool
	crate []bool

	// spawns are the corner tiles players start on, in slot order.
	spawns [][2]int
}

func idx(x, y int) int { return y*ArenaW + x }

// inBounds reports whether (x, y) is a real cell. The arena is ringed by solid
// border tiles, so simulation never actually needs to range-check — but bomb
// blast walks and client-supplied values do.
func inBounds(x, y int) bool { return x >= 0 && x < ArenaW && y >= 0 && y < ArenaH }

// NewGrid builds a map from seed. The same seed always yields the same layout,
// which is what makes the simulation tests reproducible.
func NewGrid(seed uint64) *Grid {
	g := &Grid{
		solid: make([]bool, ArenaW*ArenaH),
		crate: make([]bool, ArenaW*ArenaH),
		spawns: [][2]int{
			{1, 1}, {ArenaW - 2, 1}, {1, ArenaH - 2}, {ArenaW - 2, ArenaH - 2},
		},
	}

	for y := 0; y < ArenaH; y++ {
		for x := 0; x < ArenaW; x++ {
			border := x == 0 || y == 0 || x == ArenaW-1 || y == ArenaH-1
			pillar := x%2 == 0 && y%2 == 0
			g.solid[idx(x, y)] = border || pillar
		}
	}

	// Clear an L-shaped pocket running inward from each spawn, so nobody starts
	// walled in and everyone can retreat out of their own opening blast.
	keep := map[int]bool{}
	for _, s := range g.spawns {
		sx, sy := s[0], s[1]
		dx, dy := 1, 1
		if sx > ArenaW/2 {
			dx = -1
		}
		if sy > ArenaH/2 {
			dy = -1
		}
		keep[idx(sx, sy)] = true
		for step := 1; step <= spawnClearDepth; step++ {
			keep[idx(sx+dx*step, sy)] = true
			keep[idx(sx, sy+dy*step)] = true
		}
	}

	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	for i, solid := range g.solid {
		if solid || keep[i] {
			continue
		}
		g.crate[i] = rng.Float64() < crateDensity
	}
	return g
}

// Blocked reports whether a tile stops movement and blast: a permanent wall or
// a standing crate.
func (g *Grid) Blocked(x, y int) bool {
	if !inBounds(x, y) {
		return true
	}
	return g.solid[idx(x, y)] || g.crate[idx(x, y)]
}

// Solid reports whether a tile is a permanent wall (crates excluded).
func (g *Grid) Solid(x, y int) bool {
	if !inBounds(x, y) {
		return true
	}
	return g.solid[idx(x, y)]
}

// Crate reports whether a destructible crate stands on the tile.
func (g *Grid) Crate(x, y int) bool {
	return inBounds(x, y) && g.crate[idx(x, y)]
}

// BreakCrate clears a crate and reports whether one was actually there.
func (g *Grid) BreakCrate(x, y int) bool {
	if !g.Crate(x, y) {
		return false
	}
	g.crate[idx(x, y)] = false
	return true
}

// Spawn returns the starting tile for a player slot.
func (g *Grid) Spawn(slot int) (int, int) {
	s := g.spawns[slot%len(g.spawns)]
	return s[0], s[1]
}

// SolidString encodes the permanent walls as row-major '#'/'.' runes. Walls
// never change, so this is sent once when a client joins rather than per tick.
func (g *Grid) SolidString() string {
	b := make([]byte, len(g.solid))
	for i, s := range g.solid {
		if s {
			b[i] = '#'
		} else {
			b[i] = '.'
		}
	}
	return string(b)
}

// CrateString encodes the crate layer the same way. Crates do change, so this
// rides along in snapshots — but only on the ticks where it differs, since
// WebSocket delivery is ordered and the client can hold the last value.
func (g *Grid) CrateString() string {
	b := make([]byte, len(g.crate))
	for i, c := range g.crate {
		if c {
			b[i] = 'x'
		} else {
			b[i] = '.'
		}
	}
	return string(b)
}
