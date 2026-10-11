package game

import (
	"encoding/json"
	"testing"
)

func hanLast(t *testing.T, f *fakeOutbox, seat int) hanabiView {
	t.Helper()
	v, ok := f.last[seat].(hanabiView)
	if !ok {
		t.Fatalf("seat %d has no hanabiView", seat)
	}
	return v
}

func hanCmd(t *testing.T, g *hanabiGame, seat int, host bool, m map[string]any) {
	t.Helper()
	b, _ := json.Marshal(m)
	g.Command(seat, host, b)
}

func startedHanabi(t *testing.T, n int, seed uint64) (*hanabiGame, *fakeOutbox) {
	t.Helper()
	seats := make([]int, n)
	for i := range seats {
		seats[i] = i
	}
	fo := newFakeOutbox(seats...)
	g := newHanabiGame(fo, seed).(*hanabiGame)
	for i := 0; i < n; i++ {
		g.AddPlayer("p", "")
	}
	hanCmd(t, g, 0, true, map[string]any{"t": hanMsgStart})
	if g.phase != hanPlay {
		t.Fatalf("after start: phase %q", g.phase)
	}
	return g, fo
}

func TestHanabiOwnHandHiddenOthersVisible(t *testing.T) {
	_, fo := startedHanabi(t, 3, 1)
	v := hanLast(t, fo, 0)
	for _, h := range v.Hands {
		for _, c := range h.Cards {
			if h.S == 0 {
				if c.Color != -1 || c.Value != 0 {
					t.Fatal("seat 0 should not see the face of its own card")
				}
			} else if c.Color < 0 || c.Value < 1 {
				t.Fatalf("seat 0 should see teammate seat %d's card faces", h.S)
			}
		}
	}
}

func TestHanabiDeckAndTokens(t *testing.T) {
	_, fo := startedHanabi(t, 3, 2)
	// 50 cards, 5 each to 3 players = 15 dealt, 35 left.
	if v := hanLast(t, fo, 0); v.Deck != 35 || v.Hints != 8 || v.Fuses != 3 {
		t.Fatalf("deck=%d hints=%d fuses=%d", v.Deck, v.Hints, v.Fuses)
	}
}

func TestHanabiHintMarksMatchingCards(t *testing.T) {
	g, _ := startedHanabi(t, 2, 3)
	cur := g.turn
	other := g.order[0]
	if other == cur {
		other = g.order[1]
	}
	// Force a known card in the other hand and hint its value.
	g.players[other].hand[0].color = 2
	g.players[other].hand[0].value = 3
	before := g.hints
	hanCmd(t, g, cur, false, map[string]any{"t": hanMsgHint, "target": other, "kind": "value", "value": 3})
	if !g.players[other].hand[0].knownValue {
		t.Fatal("a value hint should mark the matching card as value-known")
	}
	if g.hints != before-1 {
		t.Fatalf("a hint should spend a token: %d -> %d", before, g.hints)
	}
}

func TestHanabiMisfireCostsFuse(t *testing.T) {
	g, _ := startedHanabi(t, 2, 4)
	cur := g.turn
	// Make the first card impossible to play now (value 3 on an empty stack).
	g.players[cur].hand[0].color = 0
	g.players[cur].hand[0].value = 3
	before := g.fuses
	hanCmd(t, g, cur, false, map[string]any{"t": hanMsgPlay, "card": 0})
	if g.fuses != before-1 {
		t.Fatalf("a misfire should cost a fuse: %d -> %d", before, g.fuses)
	}
}

func TestHanabiSuccessfulPlayBuildsStack(t *testing.T) {
	g, _ := startedHanabi(t, 2, 5)
	cur := g.turn
	g.players[cur].hand[0].color = 1
	g.players[cur].hand[0].value = 1
	hanCmd(t, g, cur, false, map[string]any{"t": hanMsgPlay, "card": 0})
	if g.stacks[1] != 1 {
		t.Fatalf("playing a 1 on an empty stack should set it to 1, got %d", g.stacks[1])
	}
}

// The deck emptying is not an instant end: it starts a final round in which every
// player, including the one who drew the last card, takes one more turn, then the game
// ends and is scored (standard rule). Regression for an off-by-one that skipped the
// drawer's final turn.
func TestHanabiFinalRoundIncludesDrawer(t *testing.T) {
	g, _ := startedHanabi(t, 3, 7)
	n := len(g.order)
	// Shrink the deck to a single card so the current player's discard empties it.
	// Start with hint tokens spent so discarding is allowed (you may not discard at the
	// full 8 tokens); each discard regains one but never reaches the cap within this round.
	g.deck = g.deck[:1]
	g.hints = 0
	g.lastRound = false
	drawer := g.turn
	hanCmd(t, g, drawer, false, map[string]any{"t": hanMsgDiscard, "card": 0})
	if !g.lastRound || g.phase != hanPlay {
		t.Fatalf("emptying the deck should start a final round, not end the game: lastRound=%v phase=%q", g.lastRound, g.phase)
	}
	// Each player (including the drawer) takes exactly one final turn; discarding is
	// always a legal turn and never ends the game another way.
	seen := map[int]bool{}
	turns := 0
	for g.phase == hanPlay {
		cur := g.turn
		seen[cur] = true
		turns++
		if turns > n+2 {
			t.Fatal("final round ran too long")
		}
		hanCmd(t, g, cur, false, map[string]any{"t": hanMsgDiscard, "card": 0})
	}
	if turns != n {
		t.Fatalf("final round should be exactly %d turns (one per player, drawer included), got %d", n, turns)
	}
	if !seen[drawer] {
		t.Fatal("the player who drew the last card must still get a final turn")
	}
}
