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

// testSeq hands out input sequence numbers. Only the ack tests read them back.
var testSeq uint32

// pushInput queues one input the way a client would, with its own sequence.
func pushInput(m *Match, slot int, in Input) uint32 {
	testSeq++
	m.QueueInput(slot, testSeq, in)
	return testSeq
}

// stepN advances n ticks with one player holding the same input throughout,
// queueing one input per tick exactly as a predicting client does.
func stepN(m *Match, n, slot int, in Input) {
	for i := 0; i < n; i++ {
		pushInput(m, slot, in)
		m.Step()
	}
}

// walkTicks is how many ticks a player needs to cover d tiles at base speed.
// Tests say "walk a tile and a half", not "step 11 times", so they keep working
// when the tick rate changes.
func walkTicks(d float64) int {
	return int(math.Ceil(d*TickHz/baseSpeed)) + 1
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

func TestBombPressAndReleaseInTheSameFrameStillCounts(t *testing.T) {
	m := openMatch(t, 1)

	// Press and release both arrive before the next tick. Inputs are consumed
	// one per tick, so the release cannot overwrite the press and lose it.
	pushInput(m, 0, Input{Bomb: true})
	pushInput(m, 0, Input{Bomb: false})
	m.Step()

	if len(m.Bombs) != 1 {
		t.Fatalf("a quick tap dropped %d bombs, want 1", len(m.Bombs))
	}

	// Next tick consumes the queued release, which re-arms the edge.
	m.Players[0].Bombs = 2
	m.Step()
	stepN(m, walkTicks(1), 0, Input{DX: 1}) // clear of the first bomb's tile

	pushInput(m, 0, Input{DX: 1, Bomb: true})
	m.Step()
	if len(m.Bombs) != 2 {
		t.Fatalf("second press left %d bombs, want 2", len(m.Bombs))
	}
}

// A client that stops talking must not keep walking. Holding the last input
// smooths over a dropped frame, but holding it indefinitely marches the player
// across the arena and lands as a teleport when their client catches up.
func TestStarvedPlayerStopsInsteadOfRunningOn(t *testing.T) {
	m := openMatch(t, 1)
	p := m.Players[0]

	stepN(m, 5, 0, Input{DX: 1}) // moving, with input arriving every tick

	// The client goes quiet. The hold carries the player a little further...
	before := p.X
	for i := 0; i < holdTicks; i++ {
		m.Step()
	}
	if p.X <= before {
		t.Fatal("player stopped dead the instant input stopped, losing the stride")
	}

	// ...and then they stand still, however long the silence lasts.
	held := p.X
	for i := 0; i < TickHz*2; i++ {
		m.Step()
	}
	if p.X != held {
		t.Fatalf("player kept walking without input: %v -> %v", held, p.X)
	}
}

func TestAckReportsTheLastConsumedInput(t *testing.T) {
	m := openMatch(t, 1)

	first := pushInput(m, 0, Input{DX: 1})
	second := pushInput(m, 0, Input{DX: 1})

	m.Step()
	if got := m.Players[0].ack; got != first {
		t.Fatalf("ack = %d after one tick, want %d — one input per tick", got, first)
	}
	m.Step()
	if got := m.Players[0].ack; got != second {
		t.Fatalf("ack = %d after two ticks, want %d", got, second)
	}

	// Queue run dry: the player holds their last input and the ack stands, so
	// the client knows that input is still its own to replay.
	m.Step()
	if got := m.Players[0].ack; got != second {
		t.Fatalf("ack advanced to %d with nothing queued, want %d", got, second)
	}
}

func TestInputQueueIsBounded(t *testing.T) {
	m := openMatch(t, 1)

	// A client running far ahead must lose its stalest intent, not its freshest.
	for i := 0; i < maxPending*3; i++ {
		pushInput(m, 0, Input{DX: 1})
	}
	newest := pushInput(m, 0, Input{DX: -1})

	if n := len(m.Players[0].pending); n > maxPending {
		t.Fatalf("queued %d inputs, want at most %d", n, maxPending)
	}
	last := m.Players[0].pending[len(m.Players[0].pending)-1]
	if last.seq != newest {
		t.Fatalf("freshest input was dropped: tail seq = %d, want %d", last.seq, newest)
	}
}

func TestBombStockLimitsConcurrentBombs(t *testing.T) {
	m := openMatch(t, 1)
	m.Players[0].Bombs = 2

	stepN(m, 1, 0, Input{Bomb: true})
	stepN(m, walkTicks(1), 0, Input{DX: 1})  // step clear of the first bomb
	stepN(m, 1, 0, Input{DX: 1, Bomb: true}) // and drop the second
	if len(m.Bombs) != 2 {
		t.Fatalf("got %d bombs, want 2", len(m.Bombs))
	}

	stepN(m, walkTicks(1), 0, Input{DX: 1})
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
	stepN(m, walkTicks(1), 0, Input{DX: 1})
	if p.X < 2.4 {
		t.Fatalf("X = %v: player could not step off their own bomb", p.X)
	}

	// Now the bomb is solid behind them: the box's left edge stops at x = 2.
	stepN(m, walkTicks(2), 0, Input{DX: -1})
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

// Snapshot is a pure read: every call describes the whole match. Taking one for
// a joining player must not rob the next broadcast of its crate layer, which is
// exactly what a snapshot that quietly tracked "what changed" used to do.
func TestSnapshotIsCompleteAndRepeatable(t *testing.T) {
	m := openMatch(t, 1)
	m.Grid.crate[idx(3, 1)] = true

	first, second := m.Snapshot(), m.Snapshot()
	if first.C == "" {
		t.Fatal("snapshot omitted the crate layer")
	}
	if first.C != second.C {
		t.Fatal("two reads of an unchanged match disagreed")
	}

	m.Grid.BreakCrate(3, 1)
	if third := m.Snapshot(); third.C == first.C {
		t.Fatal("snapshot did not reflect the destroyed crate")
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
