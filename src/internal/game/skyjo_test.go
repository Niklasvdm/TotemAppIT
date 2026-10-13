package game

import (
	"encoding/json"
	"testing"
)

func skCmd(t *testing.T, g *skyjoGame, seat int, host bool, m map[string]any) {
	t.Helper()
	b, _ := json.Marshal(m)
	g.Command(seat, host, b)
}

func skLast(t *testing.T, f *fakeOutbox, seat int) skyjoView {
	t.Helper()
	v, ok := f.last[seat].(skyjoView)
	if !ok {
		t.Fatalf("seat %d has no skyjoView", seat)
	}
	return v
}

func startedSkyjo(t *testing.T, n int, seed uint64) (*skyjoGame, *fakeOutbox) {
	t.Helper()
	seats := make([]int, n)
	for i := range seats {
		seats[i] = i
	}
	fo := newFakeOutbox(seats...)
	g := newSkyjoGame(fo, seed).(*skyjoGame)
	for i := 0; i < n; i++ {
		g.AddPlayer("p", "")
	}
	skCmd(t, g, 0, true, map[string]any{"t": skMsgStart})
	if g.phase != skSetup {
		t.Fatalf("after start: phase %q, want setup", g.phase)
	}
	return g, fo
}

func TestSkyjoDeckDistribution(t *testing.T) {
	d := buildSkyjoDeck()
	if len(d) != 150 {
		t.Fatalf("deck has %d cards, want 150", len(d))
	}
	count := map[int]int{}
	for _, v := range d {
		count[v]++
	}
	if count[-2] != 5 || count[-1] != 10 || count[0] != 15 {
		t.Fatalf("low-card counts wrong: -2=%d -1=%d 0=%d", count[-2], count[-1], count[0])
	}
	for v := 1; v <= 12; v++ {
		if count[v] != 10 {
			t.Fatalf("value %d has %d cards, want 10", v, count[v])
		}
	}
	if count[13] != 0 {
		t.Fatal("deck should hold no value above 12")
	}
}

func TestSkyjoSetupFlipsStartHighestSum(t *testing.T) {
	g, _ := startedSkyjo(t, 2, 3)
	// Rig known opening cards so the start player is deterministic.
	g.players[0].grid[0].value, g.players[0].grid[1].value = 2, 3 // sum 5
	g.players[1].grid[0].value, g.players[1].grid[1].value = 9, 9 // sum 18 -> starts
	skCmd(t, g, 0, false, map[string]any{"t": skMsgFlip, "cell": 0})
	skCmd(t, g, 0, false, map[string]any{"t": skMsgFlip, "cell": 1})
	skCmd(t, g, 1, false, map[string]any{"t": skMsgFlip, "cell": 0})
	if g.phase != skSetup {
		t.Fatal("play should not begin until everyone has flipped 2")
	}
	skCmd(t, g, 1, false, map[string]any{"t": skMsgFlip, "cell": 1})
	if g.phase != skPlay {
		t.Fatalf("play should begin once all flipped 2, phase=%q", g.phase)
	}
	if g.turn != 1 {
		t.Fatalf("highest opening sum should start: turn=%d, want 1", g.turn)
	}
}

func TestSkyjoDrawSwapFlow(t *testing.T) {
	g, _ := startedSkyjo(t, 2, 7)
	for s := 0; s < 2; s++ {
		skCmd(t, g, s, false, map[string]any{"t": skMsgFlip, "cell": 0})
		skCmd(t, g, s, false, map[string]any{"t": skMsgFlip, "cell": 1})
	}
	cur := g.turn
	skCmd(t, g, cur, false, map[string]any{"t": skMsgDraw})
	if g.drawn == skNone {
		t.Fatal("draw should give the active seat a held card")
	}
	// Only the active seat sees the drawn card.
	if v := skLast(t, g.out.(*fakeOutbox), cur); v.Drawn == skNone {
		t.Fatal("active seat should see its drawn card")
	}
	other := 1 - cur
	if v := skLast(t, g.out.(*fakeOutbox), other); v.Drawn != skNone {
		t.Fatal("a non-active seat must not see the drawn card")
	}
	skCmd(t, g, cur, false, map[string]any{"t": skMsgSwap, "cell": 5})
	if g.drawn != skNone {
		t.Fatal("swap should clear the held card")
	}
	if !g.players[cur].grid[5].up {
		t.Fatal("swapped cell should be face-up")
	}
	if g.turn == cur {
		t.Fatal("the turn should pass after a swap")
	}
}

func TestSkyjoColumnClears(t *testing.T) {
	g, _ := startedSkyjo(t, 2, 1)
	p := g.players[0]
	for _, i := range []int{0, skCols, 2 * skCols} { // column 0: cells 0,4,8
		p.grid[i] = skyjoCell{value: 7, up: true}
	}
	g.clearColumns(0)
	for _, i := range []int{0, skCols, 2 * skCols} {
		if !p.grid[i].gone {
			t.Fatalf("cell %d should be cleared", i)
		}
	}
}

func skAllUp(p *skyjoPlayer, vals [skCells]int) {
	for i := range vals {
		p.grid[i] = skyjoCell{value: vals[i], up: true}
	}
}

func TestSkyjoCloserDoubledWhenNotLowest(t *testing.T) {
	fo := newFakeOutbox(0, 1)
	g := newSkyjoGame(fo, 1).(*skyjoGame)
	g.AddPlayer("a", "")
	g.AddPlayer("b", "")
	skAllUp(g.players[0], [skCells]int{10}) // closer sum 10
	skAllUp(g.players[1], [skCells]int{5})  // other sum 5 -> closer not lowest
	g.closer = 0
	g.endRound()
	if g.players[0].score != 20 {
		t.Fatalf("closer who is not lowest should be doubled: %d, want 20", g.players[0].score)
	}
	if g.players[1].score != 5 {
		t.Fatalf("non-closer scores normally: %d, want 5", g.players[1].score)
	}
}

func TestSkyjoCloserNotDoubledWhenLowest(t *testing.T) {
	fo := newFakeOutbox(0, 1)
	g := newSkyjoGame(fo, 1).(*skyjoGame)
	g.AddPlayer("a", "")
	g.AddPlayer("b", "")
	skAllUp(g.players[0], [skCells]int{3}) // closer sum 3 (lowest)
	skAllUp(g.players[1], [skCells]int{8})
	g.closer = 0
	g.endRound()
	if g.players[0].score != 3 {
		t.Fatalf("a closer who IS lowest is not doubled: %d, want 3", g.players[0].score)
	}
}

func TestSkyjoGameEndsPastHundred(t *testing.T) {
	fo := newFakeOutbox(0, 1)
	g := newSkyjoGame(fo, 1).(*skyjoGame)
	g.AddPlayer("a", "")
	g.AddPlayer("b", "")
	g.players[0].score = 95
	g.players[1].score = 40
	skAllUp(g.players[0], [skCells]int{10}) // -> 105
	skAllUp(g.players[1], [skCells]int{2})  // -> 42
	g.closer = -1
	g.endRound()
	if g.phase != skOver {
		t.Fatalf("passing %d should end the game, phase=%q", skEndScore, g.phase)
	}
	if g.gameWinner != 1 {
		t.Fatalf("lowest total should win: winner=%d, want 1", g.gameWinner)
	}
}
