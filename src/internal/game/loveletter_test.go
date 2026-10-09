package game

import (
	"encoding/json"
	"testing"
)

func llLast(t *testing.T, f *fakeOutbox, seat int) loveLetterView {
	t.Helper()
	v, ok := f.last[seat].(loveLetterView)
	if !ok {
		t.Fatalf("seat %d has no loveLetterView", seat)
	}
	return v
}

func llCmd(t *testing.T, g *loveLetterGame, seat int, host bool, m map[string]any) {
	t.Helper()
	b, _ := json.Marshal(m)
	g.Command(seat, host, b)
}

func startedLove(t *testing.T, n int, seed uint64) (*loveLetterGame, *fakeOutbox) {
	t.Helper()
	seats := make([]int, n)
	for i := range seats {
		seats[i] = i
	}
	fo := newFakeOutbox(seats...)
	g := newLoveLetterGame(fo, seed).(*loveLetterGame)
	for i := 0; i < n; i++ {
		g.AddPlayer("p", "")
	}
	llCmd(t, g, 0, true, map[string]any{"t": llMsgStart})
	if g.phase != llPlay {
		t.Fatalf("after start: phase %q", g.phase)
	}
	return g, fo
}

func TestLovePrivateHandOnlyToOwner(t *testing.T) {
	g, fo := startedLove(t, 3, 1)
	// Every seat's own hand is populated; a view carries only that seat's hand,
	// and the active seat holds two cards while others hold one.
	for s := 0; s < 3; s++ {
		v := llLast(t, fo, s)
		if s == g.active {
			if len(v.Hand) != 2 {
				t.Fatalf("active seat %d hand=%v, want 2", s, v.Hand)
			}
		} else if len(v.Hand) != 1 {
			t.Fatalf("seat %d hand=%v, want 1", s, v.Hand)
		}
		// Other players appear only as a face-down count, never their values.
		for _, pv := range v.Players {
			if pv.S != s && pv.Reveal != nil {
				t.Fatalf("seat %d saw seat %d's hand mid-round", s, pv.S)
			}
		}
	}
}

func TestLoveGuardCorrectEliminates(t *testing.T) {
	g, _ := startedLove(t, 2, 2)
	a := g.active
	o := g.nextAlive(a)
	// Rig the opponent's hand and give the active a Guard to play.
	g.players[o].hand = []int{llPriest}
	g.players[a].hand = []int{llGuard, llBaron}
	llCmd(t, g, a, false, map[string]any{"t": llMsgPlay, "card": llGuard, "target": o, "guess": llPriest})
	if !g.players[o].out {
		t.Fatal("a correct Guard guess should eliminate the target")
	}
	if g.phase != llRoundEnd && g.phase != llOver {
		t.Fatalf("round should end when one player remains, phase=%q", g.phase)
	}
}

func TestLoveHandmaidProtects(t *testing.T) {
	g, _ := startedLove(t, 3, 3)
	a := g.active
	g.players[a].hand = []int{llHandmaid, llGuard}
	llCmd(t, g, a, false, map[string]any{"t": llMsgPlay, "card": llHandmaid})
	if !g.players[a].protected {
		t.Fatal("Handmaid should protect its player")
	}
	if g.canTarget(a) {
		t.Fatal("a protected player must not be targetable")
	}
}

func TestLoveCountessForcedWithRoyalty(t *testing.T) {
	g, _ := startedLove(t, 3, 4)
	a := g.active
	g.players[a].hand = []int{llCountess, llKing}
	// Trying to play the King while holding the Countess is rejected…
	llCmd(t, g, a, false, map[string]any{"t": llMsgPlay, "card": llKing, "target": g.nextAlive(a)})
	if g.active != a || len(g.players[a].hand) != 2 {
		t.Fatal("playing King while holding Countess must be rejected")
	}
	// …but playing the Countess itself is fine.
	llCmd(t, g, a, false, map[string]any{"t": llMsgPlay, "card": llCountess})
	if g.active == a {
		t.Fatal("playing the Countess should pass the turn")
	}
}

func TestLovePrincessSelfDiscardIsOut(t *testing.T) {
	g, _ := startedLove(t, 3, 5)
	a := g.active
	g.players[a].hand = []int{llPrincess, llGuard}
	llCmd(t, g, a, false, map[string]any{"t": llMsgPlay, "card": llPrincess})
	if !g.players[a].out {
		t.Fatal("discarding the Princess eliminates you")
	}
}
