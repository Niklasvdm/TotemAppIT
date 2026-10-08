package game

import (
	"encoding/json"
	"testing"
)

func tgLastView(t *testing.T, f *fakeOutbox, seat int) theGameView {
	t.Helper()
	v, ok := f.last[seat].(theGameView)
	if !ok {
		t.Fatalf("seat %d has no theGameView", seat)
	}
	return v
}

func tgCmd(t *testing.T, g *theGameGame, seat int, host bool, m map[string]any) {
	t.Helper()
	b, _ := json.Marshal(m)
	g.Command(seat, host, b)
}

// two seated players, started.
func startedTheGame(t *testing.T, seed uint64) (*theGameGame, *fakeOutbox) {
	t.Helper()
	fo := newFakeOutbox(0, 1)
	g := newTheGameGame(fo, seed).(*theGameGame)
	g.AddPlayer("a", "")
	g.AddPlayer("b", "")
	tgCmd(t, g, 0, true, map[string]any{"t": tgMsgStart})
	if g.phase != tgPlaying {
		t.Fatalf("phase after start: %q", g.phase)
	}
	return g, fo
}

func TestTheGameDealAndPiles(t *testing.T) {
	g, fo := startedTheGame(t, 1)
	// 2 players -> 7 cards each; piles start 1,1,100,100.
	if len(g.hands[0]) != 7 || len(g.hands[1]) != 7 {
		t.Fatalf("hand sizes: %d, %d (want 7)", len(g.hands[0]), len(g.hands[1]))
	}
	if g.piles != [tgPiles]int{1, 1, 100, 100} {
		t.Fatalf("piles start = %v", g.piles)
	}
	// 98 cards total (2..99), 14 dealt, so 84 in the draw deck.
	if len(g.deck) != 84 {
		t.Fatalf("deck = %d, want 84", len(g.deck))
	}
	// Private hand view: seat 0 sees its own cards, seat 1 only a count.
	v0 := tgLastView(t, fo, 0)
	if len(v0.Hand) != 7 {
		t.Fatalf("seat 0 hand view %d, want 7", len(v0.Hand))
	}
	for _, r := range v0.Roster {
		if r.S == 1 && r.Cards != 7 {
			t.Fatalf("seat 0 should see seat 1's count (7), got %d", r.Cards)
		}
	}
}

func TestTheGameCanPlayRules(t *testing.T) {
	g := newTheGameGame(newFakeOutbox(), 1).(*theGameGame)
	g.piles = [tgPiles]int{30, 30, 70, 70} // 0,1 ascending; 2,3 descending

	cases := []struct {
		card, pile int
		want       bool
	}{
		{31, 0, true},  // ascending: higher
		{30, 0, false}, // equal: no
		{29, 0, false}, // lower: no
		{20, 0, true},  // ascending backwards trick: exactly 10 lower
		{69, 2, true},  // descending: lower
		{71, 2, false}, // higher: no
		{80, 2, true},  // descending backwards trick: exactly 10 higher
	}
	for _, c := range cases {
		if got := g.canPlay(c.card, c.pile); got != c.want {
			t.Errorf("canPlay(%d, pile %d) = %v, want %v", c.card, c.pile, got, c.want)
		}
	}
}

func TestTheGamePlayAdvancesPileAndTurn(t *testing.T) {
	g, _ := startedTheGame(t, 2)
	g.hands = map[int][]int{0: {40, 41, 42}, 1: {50, 51}}
	g.deck = []int{60, 61, 62, 63} // non-empty -> minimum is 2
	g.piles = [tgPiles]int{1, 1, 100, 100}
	g.turn = 0
	g.played = 0

	tgCmd(t, g, 0, false, map[string]any{"t": tgMsgPlay, "card": 40, "pile": 0})
	tgCmd(t, g, 0, false, map[string]any{"t": tgMsgPlay, "card": 41, "pile": 1})
	if g.piles[0] != 40 || g.piles[1] != 41 || g.played != 2 {
		t.Fatalf("after two plays: piles=%v played=%d", g.piles, g.played)
	}
	// Ending the turn with the minimum met refills and passes to seat 1.
	tgCmd(t, g, 0, false, map[string]any{"t": tgMsgEndTurn})
	if g.turn != 1 || g.played != 0 {
		t.Fatalf("turn did not pass: turn=%d played=%d", g.turn, g.played)
	}
	// Seat 0 had 1 card left and drew the 4-card deck back up (hand size 7 caps it,
	// but only 4 were available): 5 cards, deck now empty.
	if len(g.hands[0]) != 5 || len(g.deck) != 0 {
		t.Fatalf("refill: hand=%d deck=%d, want 5 and 0", len(g.hands[0]), len(g.deck))
	}
}

func TestTheGameEndTurnBelowMinimumIsRejectedWhenMovesRemain(t *testing.T) {
	g, _ := startedTheGame(t, 3)
	g.hands = map[int][]int{0: {40, 41}, 1: {50}}
	g.deck = []int{60, 61}
	g.piles = [tgPiles]int{1, 1, 100, 100}
	g.turn = 0
	g.played = 0
	tgCmd(t, g, 0, false, map[string]any{"t": tgMsgPlay, "card": 40, "pile": 0})
	// Only one card played (min is 2) but a legal move (41) remains: end-turn is a no-op.
	tgCmd(t, g, 0, false, map[string]any{"t": tgMsgEndTurn})
	if g.turn != 0 || g.phase != tgPlaying {
		t.Fatalf("end-turn below minimum with moves left should do nothing: turn=%d phase=%q", g.turn, g.phase)
	}
}

func TestTheGameStuckBelowMinimumLoses(t *testing.T) {
	g, _ := startedTheGame(t, 4)
	// Seat 0 has one card and no second move; deck non-empty so min is 2.
	g.hands = map[int][]int{0: {40}, 1: {50}}
	g.deck = []int{60, 61}
	g.piles = [tgPiles]int{1, 1, 100, 100}
	g.turn = 0
	g.played = 0
	tgCmd(t, g, 0, false, map[string]any{"t": tgMsgPlay, "card": 40, "pile": 0})
	tgCmd(t, g, 0, false, map[string]any{"t": tgMsgEndTurn}) // can't reach 2 -> lose
	if g.phase != tgLost {
		t.Fatalf("stuck below the minimum should lose: phase=%q", g.phase)
	}
}

func TestTheGameEmptyingEverythingWins(t *testing.T) {
	g, _ := startedTheGame(t, 5)
	g.hands = map[int][]int{0: {40, 41}, 1: {}}
	g.deck = nil // deck empty -> min is 1
	g.piles = [tgPiles]int{1, 1, 100, 100}
	g.turn = 0
	g.played = 0
	tgCmd(t, g, 0, false, map[string]any{"t": tgMsgPlay, "card": 40, "pile": 0})
	tgCmd(t, g, 0, false, map[string]any{"t": tgMsgPlay, "card": 41, "pile": 1})
	if g.phase != tgWon {
		t.Fatalf("emptying the last hand with an empty deck should win: phase=%q", g.phase)
	}
}
