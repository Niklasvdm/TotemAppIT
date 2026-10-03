package game

import "math"

const (
	// dropChance is the probability a destroyed crate reveals a powerup.
	dropChance = 0.32

	// eps keeps a clamped player a hair off the wall face, so the very next
	// collision test doesn't report them inside it.
	eps = 1e-6
)

// blastDirs is right/left/down/up, the order flames are walked in.
var blastDirs = [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// Step advances the match one tick. It is deterministic: the same state and the
// same inputs always produce the same next state, which is what lets the tests
// assert on exact outcomes.
func (m *Match) Step() {
	m.Tick++

	switch m.Phase {
	case PhasePlay:
		m.expireFlames()
		for _, p := range m.ordered() {
			p.takeInput()
			m.movePlayer(p)
			m.tryBomb(p)
		}
		m.tickBombs()
		m.collect()
		m.burn()
		m.checkRoundEnd()

	case PhaseOver:
		// Let the arena settle so the killing blast plays out on screen, but
		// stop taking input and stop re-deciding the winner.
		m.expireFlames()
		m.tickBombs()
	}
}

// ordered returns seated players by slot. Map iteration order is random in Go,
// and anything that can resolve a tie between two players (who grabs a powerup
// on the same tick) has to be ordered to stay reproducible.
func (m *Match) ordered() []*Player {
	out := make([]*Player, 0, len(m.Players))
	for slot := 0; slot < MaxPlayers; slot++ {
		if p, ok := m.Players[slot]; ok {
			out = append(out, p)
		}
	}
	return out
}

// --- movement ---------------------------------------------------------------

// tileOf returns the tile a point falls in.
func tileOf(cx, cy float64) (int, int) { return int(math.Floor(cx)), int(math.Floor(cy)) }

// overlapsTile reports whether a player's collision box intersects a tile.
func overlapsTile(cx, cy float64, tx, ty int) bool {
	return cx+playerRadius > float64(tx) && cx-playerRadius < float64(tx+1) &&
		cy+playerRadius > float64(ty) && cy-playerRadius < float64(ty+1)
}

// blockedFor reports whether a tile stops this player. Bombs block everyone
// except the players who have not yet stepped clear of them.
func (m *Match) blockedFor(p *Player, tx, ty int) bool {
	if m.Grid.Blocked(tx, ty) {
		return true
	}
	for _, b := range m.Bombs {
		if b.X == tx && b.Y == ty && !b.standing[p.Slot] {
			return true
		}
	}
	return false
}

// collides reports whether the player's box at (cx, cy) overlaps anything solid.
func (m *Match) collides(p *Player, cx, cy float64) bool {
	x0, y0 := tileOf(cx-playerRadius, cy-playerRadius)
	x1, y1 := tileOf(cx+playerRadius, cy+playerRadius)
	for ty := y0; ty <= y1; ty++ {
		for tx := x0; tx <= x1; tx++ {
			if m.blockedFor(p, tx, ty) {
				return true
			}
		}
	}
	return false
}

func (m *Match) movePlayer(p *Player) {
	if !p.Alive {
		return
	}
	in := p.in
	if in.DX == 0 && in.DY == 0 {
		return
	}
	step := p.Speed / TickHz

	// Axes resolve independently: running diagonally into a wall should keep
	// whichever component is still free instead of stopping dead.
	if in.DX != 0 {
		m.slide(p, in.DX, step, true)
	}
	if in.DY != 0 {
		m.slide(p, in.DY, step, false)
	}

	// Moving along one axis pulls the player toward the middle of the corridor.
	// Without this the collision box catches on the pillar lattice at every
	// junction and the arena feels sticky.
	if in.DX != 0 && in.DY == 0 {
		m.recentre(p, step, false)
	}
	if in.DY != 0 && in.DX == 0 {
		m.recentre(p, step, true)
	}

	m.releaseBombs(p)
}

// slide moves one axis by step, clamping to the face of whatever it hits.
func (m *Match) slide(p *Player, dir int, step float64, horizontal bool) {
	nx, ny := p.X, p.Y
	if horizontal {
		nx += float64(dir) * step
	} else {
		ny += float64(dir) * step
	}
	if !m.collides(p, nx, ny) {
		p.X, p.Y = nx, ny
		return
	}

	// Ran into something: sit flush against it rather than losing the frame.
	if horizontal {
		if dir > 0 {
			p.X = math.Floor(nx+playerRadius) - playerRadius - eps
		} else {
			p.X = math.Floor(nx-playerRadius) + 1 + playerRadius + eps
		}
	} else {
		if dir > 0 {
			p.Y = math.Floor(ny+playerRadius) - playerRadius - eps
		} else {
			p.Y = math.Floor(ny-playerRadius) + 1 + playerRadius + eps
		}
	}
}

// recentre eases the off-axis coordinate toward the corridor centre line.
func (m *Match) recentre(p *Player, step float64, horizontal bool) {
	cur, target := p.Y, math.Floor(p.Y)+0.5
	if horizontal {
		cur, target = p.X, math.Floor(p.X)+0.5
	}
	delta := target - cur
	if math.Abs(delta) < eps {
		return
	}
	move := math.Min(math.Abs(delta), step)
	if delta < 0 {
		move = -move
	}

	nx, ny := p.X, p.Y+move
	if horizontal {
		nx, ny = p.X+move, p.Y
	}
	if !m.collides(p, nx, ny) {
		p.X, p.Y = nx, ny
	}
}

// releaseBombs drops a player from a bomb's pass-through set once they are
// fully clear of its tile, which is what makes the bomb solid behind them.
func (m *Match) releaseBombs(p *Player) {
	for _, b := range m.Bombs {
		if b.standing[p.Slot] && !overlapsTile(p.X, p.Y, b.X, b.Y) {
			delete(b.standing, p.Slot)
		}
	}
}

// --- bombs ------------------------------------------------------------------

func (m *Match) bombAt(x, y int) *Bomb {
	for _, b := range m.Bombs {
		if b.X == x && b.Y == y {
			return b
		}
	}
	return nil
}

// takeBomb removes and returns the bomb on a tile, if any.
func (m *Match) takeBomb(x, y int) *Bomb {
	for i, b := range m.Bombs {
		if b.X == x && b.Y == y {
			m.Bombs = append(m.Bombs[:i], m.Bombs[i+1:]...)
			return b
		}
	}
	return nil
}

func (m *Match) liveBombs(slot int) int {
	n := 0
	for _, b := range m.Bombs {
		if b.Owner == slot {
			n++
		}
	}
	return n
}

// tryBomb drops a bomb on the rising edge of the bomb key, so holding it does
// not empty the player's whole stock in four ticks. The request comes from the
// latch rather than the sampled level, so a tap too short to span a tick still
// counts; releasing the key is what re-arms the edge.
func (m *Match) tryBomb(p *Player) {
	requested := p.bombLatch
	p.bombLatch = false

	if !p.Alive {
		return
	}
	if !p.in.Bomb {
		p.bombHeld = false
	}
	if !requested || p.bombHeld {
		return
	}
	p.bombHeld = true

	if m.liveBombs(p.Slot) >= p.Bombs {
		return
	}
	tx, ty := tileOf(p.X, p.Y)
	if m.bombAt(tx, ty) != nil {
		return
	}

	// Everyone already overlapping the tile walks off freely; only re-entry is
	// blocked.
	standing := map[int]bool{}
	for _, q := range m.ordered() {
		if overlapsTile(q.X, q.Y, tx, ty) {
			standing[q.Slot] = true
		}
	}
	m.Bombs = append(m.Bombs, &Bomb{
		X: tx, Y: ty, Owner: p.Slot, Fuse: fuseTicks, Power: p.Power, standing: standing,
	})
}

// tickBombs burns fuses and detonates, draining the chain reaction queue so a
// bomb caught in another's blast goes off on the same tick.
func (m *Match) tickBombs() {
	var queue []*Bomb
	kept := m.Bombs[:0]
	for _, b := range m.Bombs {
		if b.Fuse--; b.Fuse <= 0 {
			queue = append(queue, b)
		} else {
			kept = append(kept, b)
		}
	}
	m.Bombs = kept

	for len(queue) > 0 {
		b := queue[0]
		queue = queue[1:]
		queue = append(queue, m.detonate(b)...)
	}
}

// detonate lays flames from a bomb and returns any bombs it set off.
func (m *Match) detonate(b *Bomb) []*Bomb {
	var chain []*Bomb
	m.addFlame(b.X, b.Y)

	for _, d := range blastDirs {
		for i := 1; i <= b.Power; i++ {
			x, y := b.X+d[0]*i, b.Y+d[1]*i
			if m.Grid.Solid(x, y) {
				break
			}
			if m.Grid.BreakCrate(x, y) {
				// A crate absorbs the blast: it burns, and the flame stops here.
				m.addFlame(x, y)
				m.maybeDrop(x, y)
				break
			}
			m.addFlame(x, y)
			if nb := m.takeBomb(x, y); nb != nil {
				chain = append(chain, nb)
			}
		}
	}
	return chain
}

// addFlame lights a tile, refreshing any flame already there and consuming a
// powerup caught in the blast.
func (m *Match) addFlame(x, y int) {
	for i, u := range m.Powers {
		if u.X == x && u.Y == y {
			m.Powers = append(m.Powers[:i], m.Powers[i+1:]...)
			break
		}
	}
	for _, f := range m.Flames {
		if f.X == x && f.Y == y {
			f.TTL = flameTicks
			return
		}
	}
	m.Flames = append(m.Flames, &Flame{X: x, Y: y, TTL: flameTicks})
}

func (m *Match) expireFlames() {
	kept := m.Flames[:0]
	for _, f := range m.Flames {
		if f.TTL--; f.TTL > 0 {
			kept = append(kept, f)
		}
	}
	m.Flames = kept
}

// --- powerups, deaths, round end --------------------------------------------

func (m *Match) maybeDrop(x, y int) {
	if m.rng.Float64() >= dropChance {
		return
	}
	// Fire is twice as likely as the others: range is the pickup that most
	// changes how the arena plays.
	kinds := [...]PowerKind{PowerFire, PowerFire, PowerBomb, PowerSpeed}
	m.Powers = append(m.Powers, &Powerup{X: x, Y: y, Kind: kinds[m.rng.IntN(len(kinds))]})
}

// collect hands out powerups. Pickup is by the player's centre tile, the same
// rule as taking damage, so what you see under your feet is what you get.
func (m *Match) collect() {
	kept := m.Powers[:0]
	for _, u := range m.Powers {
		taken := false
		for _, p := range m.ordered() {
			if !p.Alive {
				continue
			}
			if tx, ty := tileOf(p.X, p.Y); tx == u.X && ty == u.Y {
				apply(p, u.Kind)
				taken = true
				break
			}
		}
		if !taken {
			kept = append(kept, u)
		}
	}
	m.Powers = kept
}

func apply(p *Player, k PowerKind) {
	switch k {
	case PowerBomb:
		p.Bombs = min(p.Bombs+1, maxBombs)
	case PowerFire:
		p.Power = min(p.Power+1, maxPower)
	case PowerSpeed:
		p.Speed = math.Min(p.Speed+speedStep, maxSpeed)
	}
}

// burn kills players whose centre tile is on fire.
func (m *Match) burn() {
	for _, p := range m.ordered() {
		if !p.Alive {
			continue
		}
		tx, ty := tileOf(p.X, p.Y)
		for _, f := range m.Flames {
			if f.X == tx && f.Y == ty {
				p.Alive = false
				break
			}
		}
	}
}

func (m *Match) alive() []*Player {
	var out []*Player
	for _, p := range m.ordered() {
		if p.Alive {
			out = append(out, p)
		}
	}
	return out
}

// checkRoundEnd closes the round once the field is settled. A lone player is
// practising, so their round ends only when they blow themselves up.
func (m *Match) checkRoundEnd() {
	alive := m.alive()
	if len(m.Players) <= 1 {
		if len(alive) == 0 {
			m.endRound(-1)
		}
		return
	}
	if len(alive) > 1 {
		return
	}
	winner := -1
	if len(alive) == 1 {
		winner = alive[0].Slot
		alive[0].Wins++
	}
	m.endRound(winner)
}

func (m *Match) endRound(winner int) {
	m.Phase = PhaseOver
	m.Winner = winner
	m.overAt = m.Tick
}
