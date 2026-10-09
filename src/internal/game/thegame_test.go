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

func TestTheGameHandSizesScaleWithPlayers(t *testing.T) {
	// 1→8, 2→7, 3/4/5→6; deck is 98 minus what's dealt.
	for _, tc := range []struct{ players, hand int }{{1, 8}, {2, 7}, {3, 6}, {4, 6}, {5, 6}} {
		fo := newFakeOutbox()
		g := newTheGameGame(fo, 11).(*theGameGame)
		seats := make([]int, tc.players)
		for i := 0; i < tc.players; i++ {
			s, ok := g.AddPlayer("p", "")
			if !ok {
				t.Fatalf("%d players: AddPlayer %d failed", tc.players, i)
			}
			seats[i] = s
		}
		fo.seats = seats
		tgCmd(t, g, seats[0], true, map[string]any{"t": tgMsgStart})
		seen := map[int]bool{}
		for _, s := range seats {
			if len(g.hands[s]) != tc.hand {
				t.Fatalf("%d players: seat %d has %d cards, want %d", tc.players, s, len(g.hands[s]), tc.hand)
			}
			for _, c := range g.hands[s] {
				if seen[c] {
					t.Fatalf("%d players: duplicate card %d dealt", tc.players, c)
				}
				seen[c] = true
			}
		}
		if want := 98 - tc.players*tc.hand; len(g.deck) != want {
			t.Fatalf("%d players: deck=%d want %d", tc.players, len(g.deck), want)
		}
	}
}

func TestTheGameEmptyHandIsSkippedAndSerialisesAsArray(t *testing.T) {
	g, fo := startedTheGame(t, 7) // seats 0,1
	g.deck = nil
	g.hands = map[int][]int{0: {}, 1: {40}}
	fo.seats = []int{0, 1}
	// nextSeat from 1 must skip the empty seat 0 (it stays on 1, the only player left).
	if nx := g.nextSeat(1); nx != 1 {
		t.Fatalf("nextSeat skipping empty: got %d, want 1", nx)
	}
	g.hands[0] = []int{30}
	if nx := g.nextSeat(1); nx != 0 {
		t.Fatalf("nextSeat with cards: got %d, want 0", nx)
	}
	// An empty hand must serialise as [] (not nil → JSON null, which crashed the client).
	g.hands[0] = []int{}
	g.broadcastViews()
	if v := tgLastView(t, fo, 0); v.Hand == nil {
		t.Fatal("empty hand serialised as nil")
	}
}

func TestTheGameReserveOnlyByNonCurrentAndClears(t *testing.T) {
	g, _ := startedTheGame(t, 9) // seats 0,1; turn is random
	cur := g.turn
	other := 1 - cur

	tgCmd(t, g, other, false, map[string]any{"t": tgMsgReserve, "pile": 2})
	if g.reserved[2] != other {
		t.Fatalf("non-current reserve: reserved[2]=%d want %d", g.reserved[2], other)
	}
	tgCmd(t, g, other, false, map[string]any{"t": tgMsgReserve, "pile": 2}) // toggle off
	if g.reserved[2] != -1 {
		t.Fatalf("reserve toggle off: reserved[2]=%d want -1", g.reserved[2])
	}
	tgCmd(t, g, cur, false, map[string]any{"t": tgMsgReserve, "pile": 0}) // current may not reserve
	if g.reserved[0] != -1 {
		t.Fatal("the active player was allowed to reserve")
	}
	// A reservation is cleared when the turn ends.
	g.reserved[1] = other
	g.hands = map[int][]int{cur: {40, 41}, other: {50}}
	g.deck = []int{60, 61}
	g.piles = [tgPiles]int{1, 1, 100, 100}
	g.played = 0
	tgCmd(t, g, cur, false, map[string]any{"t": tgMsgPlay, "card": 40, "pile": 0})
	tgCmd(t, g, cur, false, map[string]any{"t": tgMsgPlay, "card": 41, "pile": 1})
	tgCmd(t, g, cur, false, map[string]any{"t": tgMsgEndTurn})
	if g.reserved != ([tgPiles]int{-1, -1, -1, -1}) {
		t.Fatalf("reservations not cleared on end-turn: %v", g.reserved)
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
