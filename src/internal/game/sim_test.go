package game

import (
	"fmt"
	"math"
	"testing"
)

// openMatch starts a match on a crate-free arena. Movement and blast tests then
// depend only on the wall lattice, not on which crates a seed happened to place.
func openMatch(t *testing.T, players int) *Match {
	t.Helper()
	m := NewMatch(42)
	for i := 0; i < players; i++ {
		if _, ok := m.AddPlayer(fmt.Sprintf("p%d", i), "vos"); !ok {
			t.Fatalf("could not seat player %d", i)
		}
	}
	m.Start(1)
	for i := range m.Grid.crate {
		m.Grid.crate[i] = false
	}
	return m
}

// stepN advances n ticks with one player holding the same input throughout.
func stepN(m *Match, n, slot int, in Input) {
	for i := 0; i < n; i++ {
		m.SetInput(slot, in)
		m.Step()
	}
}

func flameSet(m *Match) map[[2]int]bool {
	out := map[[2]int]bool{}
	for _, f := range m.Flames {
		out[[2]int{f.X, f.Y}] = true
	}
	return out
}

func TestPlayerWalksAtSpeed(t *testing.T) {
	m := openMatch(t, 1)
	p := m.Players[0]
	startX, startY := p.X, p.Y

	stepN(m, 10, 0, Input{DX: 1})

	want := startX + 10*baseSpeed/TickHz
	if math.Abs(p.X-want) > 1e-6 {
		t.Fatalf("X = %v after 10 ticks, want %v", p.X, want)
	}
	if math.Abs(p.Y-startY) > 1e-6 {
		t.Fatalf("Y drifted to %v, want %v", p.Y, startY)
	}
}

func TestPlayerStopsAtWall(t *testing.T) {
	m := openMatch(t, 1)
	p := m.Players[0] // spawns at tile (1,1); tile (0,1) is the border wall

	stepN(m, 60, 0, Input{DX: -1})

	// Flush against the wall face: the box's left edge sits on x = 1.
	wantX := 1 + playerRadius
	if math.Abs(p.X-wantX) > 1e-3 {
		t.Fatalf("X = %v against the wall, want ~%v", p.X, wantX)
	}

	// And it stays put rather than creeping through.
	before := p.X
	stepN(m, 30, 0, Input{DX: -1})
	if math.Abs(p.X-before) > 1e-9 {
		t.Fatalf("player crept from %v to %v while pressed into a wall", before, p.X)
	}
}

func TestDiagonalIntoWallKeepsTheFreeAxis(t *testing.T) {
	m := openMatch(t, 1)
	p := m.Players[0]

	// Up runs into the border wall; right is open. The blocked axis should
	// settle against the wall face while the free axis keeps travelling.
	stepN(m, 30, 0, Input{DX: 1, DY: -1})

	wantY := 1 + playerRadius
	if math.Abs(p.Y-wantY) > 1e-3 {
		t.Fatalf("Y = %v, want it flush against the wall at ~%v", p.Y, wantY)
	}
	wantX := 1.5 + 30*baseSpeed/TickHz
	if math.Abs(p.X-wantX) > 1e-6 {
		t.Fatalf("X = %v, want the free axis unimpeded at %v", p.X, wantX)
	}
}

func TestBombIsEdgeTriggered(t *testing.T) {
	m := openMatch(t, 1)

	// Holding the key for many ticks must still drop exactly one bomb.
	stepN(m, 20, 0, Input{Bomb: true})
	if len(m.Bombs) != 1 {
		t.Fatalf("holding the bomb key placed %d bombs, want 1", len(m.Bombs))
	}

	// Releasing and pressing again, with the stock at 1, must not add another.
	stepN(m, 2, 0, Input{})
	stepN(m, 2, 0, Input{Bomb: true})
	if len(m.Bombs) != 1 {
		t.Fatalf("exceeded the bomb stock: %d bombs live, want 1", len(m.Bombs))
	}
}

func TestBombTapShorterThanATickStillCounts(t *testing.T) {
	m := openMatch(t, 1)

	// Press and release between two ticks. Sampling the key level at tick time
	// would see Bomb=false and lose the press entirely.
	m.SetInput(0, Input{Bomb: true})
	m.SetInput(0, Input{Bomb: false})
	m.Step()

	if len(m.Bombs) != 1 {
		t.Fatalf("a quick tap dropped %d bombs, want 1", len(m.Bombs))
	}

	// The release already happened, so the next press must drop another.
	m.Players[0].Bombs = 2
	stepN(m, 3, 0, Input{DX: 1})
	m.SetInput(0, Input{DX: 1, Bomb: true})
	m.SetInput(0, Input{DX: 1, Bomb: false})
	m.Step()

	if len(m.Bombs) != 2 {
		t.Fatalf("second tap left %d bombs, want 2", len(m.Bombs))
	}
}

func TestBombStockLimitsConcurrentBombs(t *testing.T) {
	m := openMatch(t, 1)
	m.Players[0].Bombs = 2

	stepN(m, 1, 0, Input{Bomb: true})
	stepN(m, 4, 0, Input{DX: 1})             // step clear of the first bomb
	stepN(m, 1, 0, Input{DX: 1, Bomb: true}) // and drop the second
	if len(m.Bombs) != 2 {
		t.Fatalf("got %d bombs, want 2", len(m.Bombs))
	}

	stepN(m, 4, 0, Input{DX: 1})
	stepN(m, 1, 0, Input{DX: 1, Bomb: true})
	if len(m.Bombs) != 2 {
		t.Fatalf("got %d bombs, want the stock of 2 to hold", len(m.Bombs))
	}
}

func TestBlastShapeStopsAtWalls(t *testing.T) {
	m := openMatch(t, 1)
	// Bomb at (1,1) with range 2: the border blocks left and up.
	m.Bombs = []*Bomb{{X: 1, Y: 1, Owner: 0, Fuse: 1, Power: 2, standing: map[int]bool{}}}
	m.Players[0].Alive = false // keep the blast from ending the round mid-test

	m.Step()

	got := flameSet(m)
	want := map[[2]int]bool{{1, 1}: true, {2, 1}: true, {3, 1}: true, {1, 2}: true, {1, 3}: true}
	if len(got) != len(want) {
		t.Fatalf("flames = %v, want %v", got, want)
	}
	for tile := range want {
		if !got[tile] {
			t.Fatalf("missing flame at %v (got %v)", tile, got)
		}
	}
}

func TestBlastStopsAtTheFirstCrate(t *testing.T) {
	m := openMatch(t, 1)
	m.Players[0].Alive = false
	m.Grid.crate[idx(3, 1)] = true // range-2 bomb at (1,1) reaches it

	m.Bombs = []*Bomb{{X: 1, Y: 1, Owner: 0, Fuse: 1, Power: 4, standing: map[int]bool{}}}
	m.Step()

	if m.Grid.Crate(3, 1) {
		t.Fatal("crate in the blast line survived")
	}
	got := flameSet(m)
	if !got[[2]int{3, 1}] {
		t.Fatal("the destroyed crate's tile did not catch fire")
	}
	if got[[2]int{5, 1}] {
		t.Fatal("blast continued past the crate it destroyed")
	}
}

func TestChainDetonationIsSimultaneous(t *testing.T) {
	m := openMatch(t, 1)
	m.Players[0].Alive = false

	// The first bomb's blast reaches the second, whose own fuse is far from done.
	m.Bombs = []*Bomb{
		{X: 1, Y: 1, Owner: 0, Fuse: 1, Power: 2, standing: map[int]bool{}},
		{X: 3, Y: 1, Owner: 0, Fuse: fuseTicks, Power: 2, standing: map[int]bool{}},
	}
	m.Step()

	if len(m.Bombs) != 0 {
		t.Fatalf("%d bombs left, want the chain to clear both", len(m.Bombs))
	}
	// The second bomb's own reach proves it actually detonated.
	if !flameSet(m)[[2]int{5, 1}] {
		t.Fatalf("chained bomb did not lay its own flames: %v", flameSet(m))
	}
}

func TestFlamesExpire(t *testing.T) {
	m := openMatch(t, 1)
	m.Players[0].Alive = false
	m.Bombs = []*Bomb{{X: 1, Y: 1, Owner: 0, Fuse: 1, Power: 1, standing: map[int]bool{}}}

	m.Step()
	if len(m.Flames) == 0 {
		t.Fatal("no flames right after detonation")
	}
	for i := 0; i < flameTicks; i++ {
		m.Step()
	}
	if len(m.Flames) != 0 {
		t.Fatalf("%d flames still burning after %d ticks", len(m.Flames), flameTicks)
	}
}

func TestFlameKillsPlayerOnItsTile(t *testing.T) {
	m := openMatch(t, 2)
	m.Bombs = []*Bomb{{X: 1, Y: 1, Owner: 0, Fuse: 1, Power: 1, standing: map[int]bool{}}}

	m.Step() // player 0 stands on (1,1)

	if m.Players[0].Alive {
		t.Fatal("player standing in the blast survived")
	}
	if !m.Players[1].Alive {
		t.Fatal("player in the far corner was killed")
	}
}

func TestLastPlayerStandingWinsTheRound(t *testing.T) {
	m := openMatch(t, 2)
	m.Bombs = []*Bomb{{X: 1, Y: 1, Owner: 1, Fuse: 1, Power: 1, standing: map[int]bool{}}}

	m.Step()

	if m.Phase != PhaseOver {
		t.Fatalf("phase = %q, want %q", m.Phase, PhaseOver)
	}
	if m.Winner != 1 {
		t.Fatalf("winner = %d, want 1", m.Winner)
	}
	if m.Players[1].Wins != 1 {
		t.Fatalf("winner's tally = %d, want 1", m.Players[1].Wins)
	}
}

func TestSoloRoundEndsOnlyOnSelfDestruction(t *testing.T) {
	m := openMatch(t, 1)

	stepN(m, 30, 0, Input{DX: 1})
	if m.Phase != PhasePlay {
		t.Fatalf("phase = %q while practising alone, want %q", m.Phase, PhasePlay)
	}

	m.Bombs = []*Bomb{{X: tileX(m.Players[0]), Y: tileY(m.Players[0]), Fuse: 1, Power: 1, standing: map[int]bool{}}}
	m.Step()

	if m.Phase != PhaseOver {
		t.Fatalf("phase = %q after blowing myself up, want %q", m.Phase, PhaseOver)
	}
	if m.Winner != -1 {
		t.Fatalf("winner = %d, want -1 (nobody)", m.Winner)
	}
}

func TestPlayerWalksOffOwnBombButNotBackOn(t *testing.T) {
	m := openMatch(t, 1)
	p := m.Players[0]

	stepN(m, 1, 0, Input{Bomb: true})
	if len(m.Bombs) != 1 {
		t.Fatalf("expected a bomb underfoot, got %d", len(m.Bombs))
	}

	// Walking off must work even though the bomb tile is otherwise solid.
	stepN(m, 10, 0, Input{DX: 1})
	if p.X < 2.4 {
		t.Fatalf("X = %v: player could not step off their own bomb", p.X)
	}

	// Now the bomb is solid behind them: the box's left edge stops at x = 2.
	stepN(m, 20, 0, Input{DX: -1})
	wantX := 2 + playerRadius
	if p.X < wantX-1e-3 {
		t.Fatalf("X = %v, want >= ~%v — player walked back onto the bomb", p.X, wantX)
	}
}

func TestPowerupsApplyAndCap(t *testing.T) {
	m := openMatch(t, 1)
	p := m.Players[0]
	tx, ty := tileOf(p.X, p.Y)

	m.Powers = []*Powerup{{X: tx, Y: ty, Kind: PowerFire}}
	wantPower := p.Power + 1
	m.Step()

	if p.Power != wantPower {
		t.Fatalf("power = %d, want %d", p.Power, wantPower)
	}
	if len(m.Powers) != 0 {
		t.Fatal("collected powerup was not removed")
	}

	p.Power = maxPower
	m.Powers = []*Powerup{{X: tx, Y: ty, Kind: PowerFire}}
	m.Step()
	if p.Power != maxPower {
		t.Fatalf("power = %d, want it capped at %d", p.Power, maxPower)
	}
}

func TestBlastDestroysPowerup(t *testing.T) {
	m := openMatch(t, 1)
	m.Players[0].Alive = false
	m.Powers = []*Powerup{{X: 3, Y: 1, Kind: PowerBomb}}
	m.Bombs = []*Bomb{{X: 1, Y: 1, Owner: 0, Fuse: 1, Power: 2, standing: map[int]bool{}}}

	m.Step()

	if len(m.Powers) != 0 {
		t.Fatal("powerup caught in the blast survived")
	}
}

func TestSnapshotSendsCrateLayerOnlyOnChange(t *testing.T) {
	m := openMatch(t, 1)
	m.Grid.crate[idx(3, 1)] = true

	if first := m.Snapshot(); first.C == "" {
		t.Fatal("first snapshot omitted the crate layer")
	}
	if second := m.Snapshot(); second.C != "" {
		t.Fatal("unchanged crate layer was resent")
	}

	m.Grid.BreakCrate(3, 1)
	if third := m.Snapshot(); third.C == "" {
		t.Fatal("changed crate layer was not sent")
	}
}

func TestRoomIsCappedAtMaxPlayers(t *testing.T) {
	m := NewMatch(5)
	for i := 0; i < MaxPlayers; i++ {
		if _, ok := m.AddPlayer("p", "vos"); !ok {
			t.Fatalf("seat %d refused below the cap", i)
		}
	}
	if _, ok := m.AddPlayer("extra", "vos"); ok {
		t.Fatal("seated a player beyond MaxPlayers")
	}

	m.RemovePlayer(1)
	slot, ok := m.AddPlayer("rejoin", "vos")
	if !ok {
		t.Fatal("freed slot was not reused")
	}
	if slot != 1 {
		t.Fatalf("rejoined into slot %d, want the freed slot 1", slot)
	}
}

func tileX(p *Player) int { x, _ := tileOf(p.X, p.Y); return x }
func tileY(p *Player) int { _, y := tileOf(p.X, p.Y); return y }
