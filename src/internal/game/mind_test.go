package game

import (
	"encoding/json"
	"testing"
)

func mindLast(t *testing.T, f *fakeOutbox, seat int) mindView {
	t.Helper()
	v, ok := f.last[seat].(mindView)
	if !ok {
		t.Fatalf("seat %d has no mindView", seat)
	}
	return v
}

func mindCmd(t *testing.T, g *theMindGame, seat int, host bool, typ string) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"t": typ})
	g.Command(seat, host, b)
}

// three seated players, started (level 1).
func startedMind(t *testing.T, seed uint64) (*theMindGame, *fakeOutbox) {
	t.Helper()
	fo := newFakeOutbox(0, 1, 2)
	g := newTheMindGame(fo, seed).(*theMindGame)
	for i := 0; i < 3; i++ {
		if _, ok := g.AddPlayer("p", ""); !ok {
			t.Fatalf("AddPlayer %d", i)
		}
	}
	mindCmd(t, g, 0, true, mindMsgStart)
	if g.phase != mindPlaying {
		t.Fatalf("after start: phase %q", g.phase)
	}
	return g, fo
}

func TestMindDealAndPrivateHands(t *testing.T) {
	g, fo := startedMind(t, 1)
	if g.lives != 3 || g.stars != 1 || g.level != 1 {
		t.Fatalf("start resources: lives=%d stars=%d level=%d", g.lives, g.stars, g.level)
	}
	// Level 1 -> one card each; hands disjoint.
	seen := map[int]bool{}
	for s := 0; s < 3; s++ {
		if len(g.hands[s]) != 1 {
			t.Fatalf("seat %d has %d cards, want 1", s, len(g.hands[s]))
		}
		c := g.hands[s][0]
		if c < 1 || c > 100 || seen[c] {
			t.Fatalf("bad/duplicate card %d", c)
		}
		seen[c] = true
	}
	// Private view: you see your hand; others only a count.
	v0 := mindLast(t, fo, 0)
	if len(v0.Hand) != 1 || v0.Hand[0] != g.hands[0][0] {
		t.Fatalf("seat 0 hand view %v, want %v", v0.Hand, g.hands[0])
	}
	for _, r := range v0.Roster {
		if r.S != 0 && r.Cards != 1 {
			t.Fatalf("seat 0 should see others' counts, not cards: %+v", r)
		}
	}
}

func TestMindAscendingPlayIsSafe(t *testing.T) {
	g, _ := startedMind(t, 2)
	// Force a known ascending layout: 0 holds the lowest.
	g.hands = map[int][]int{0: {10}, 1: {20}, 2: {30}}
	g.lives = 3
	mindCmd(t, g, 0, false, mindMsgPlay) // the global lowest — no mistake
	if g.lives != 3 {
		t.Fatalf("a correct play cost a life: lives=%d", g.lives)
	}
	if g.pile != 10 || len(g.hands[0]) != 0 {
		t.Fatalf("pile=%d hand0=%v, want pile 10 and empty hand", g.pile, g.hands[0])
	}
}

func TestMindMistakeLosesLifeAndBurnsLowerCards(t *testing.T) {
	g, _ := startedMind(t, 3)
	// Seat 1 jumps the gun at 20 while seat 0 still holds 10 (and 15).
	g.hands = map[int][]int{0: {10, 15}, 1: {20}, 2: {30}}
	g.lives = 3
	mindCmd(t, g, 1, false, mindMsgPlay) // plays 20 -> mistake
	if g.lives != 2 {
		t.Fatalf("mistake did not cost a life: lives=%d", g.lives)
	}
	if len(g.hands[0]) != 0 { // 10 and 15 are both < 20, burned
		t.Fatalf("lower cards not burned: seat0 still holds %v", g.hands[0])
	}
	if g.pile != 20 {
		t.Fatalf("pile=%d, want 20", g.pile)
	}
}

func TestMindLosingLastLifeEndsGame(t *testing.T) {
	g, _ := startedMind(t, 4)
	g.hands = map[int][]int{0: {5}, 1: {9}, 2: {1}} // seat 0 plays 5 while seat 2 holds 1
	g.lives = 1
	mindCmd(t, g, 0, false, mindMsgPlay)
	if g.phase != mindLost {
		t.Fatalf("game should be lost at 0 lives, phase=%q lives=%d", g.phase, g.lives)
	}
}

func TestMindEmptyHandSerialisesAsArray(t *testing.T) {
	g, fo := startedMind(t, 1)
	g.hands = map[int][]int{0: {}, 1: {5}, 2: {6}} // seat 0 has played out
	g.broadcastViews()
	if v := mindLast(t, fo, 0); v.Hand == nil {
		t.Fatal("empty hand serialised as nil (would be JSON null → client crash)")
	}
}

func TestMindThrowingStarDiscardsEachLowest(t *testing.T) {
	g, _ := startedMind(t, 5)
	g.hands = map[int][]int{0: {10, 40}, 1: {20, 50}, 2: {30, 60}}
	g.stars = 1
	// Unanimous vote fires the star: each drops their lowest (10,20,30).
	mindCmd(t, g, 0, false, mindMsgStar)
	mindCmd(t, g, 1, false, mindMsgStar)
	mindCmd(t, g, 2, false, mindMsgStar)
	if g.stars != 0 {
		t.Fatalf("star not consumed: stars=%d", g.stars)
	}
	for s, want := range map[int][]int{0: {40}, 1: {50}, 2: {60}} {
		if len(g.hands[s]) != 1 || g.hands[s][0] != want[0] {
			t.Fatalf("seat %d hand %v, want %v (lowest discarded)", s, g.hands[s], want)
		}
	}
	if g.pile != 30 {
		t.Fatalf("pile=%d, want the highest discarded lowest (30)", g.pile)
	}
}
