package game

import (
	"encoding/json"
	"testing"
)

func wlLast(t *testing.T, f *fakeOutbox, seat int) wlView {
	t.Helper()
	v, ok := f.last[seat].(wlView)
	if !ok {
		t.Fatalf("seat %d has no wlView", seat)
	}
	return v
}

func wlCmd(t *testing.T, g *wavelengthGame, seat int, host bool, m map[string]any) {
	t.Helper()
	b, _ := json.Marshal(m)
	g.Command(seat, host, b)
}

// 4 players: 0,1 on team Left; 2,3 on team Right. Started.
func startedWavelength(t *testing.T, seed uint64) (*wavelengthGame, *fakeOutbox) {
	t.Helper()
	fo := newFakeOutbox(0, 1, 2, 3)
	g := newWavelengthGame(fo, seed).(*wavelengthGame)
	for i := 0; i < 4; i++ {
		g.AddPlayer("p", "")
	}
	wlCmd(t, g, 0, false, map[string]any{"t": wlMsgSetup, "team": 0})
	wlCmd(t, g, 1, false, map[string]any{"t": wlMsgSetup, "team": 0})
	wlCmd(t, g, 2, false, map[string]any{"t": wlMsgSetup, "team": 1})
	wlCmd(t, g, 3, false, map[string]any{"t": wlMsgSetup, "team": 1})
	wlCmd(t, g, 0, true, map[string]any{"t": wlMsgStart})
	if g.phase != wlClue {
		t.Fatalf("after start: phase %q", g.phase)
	}
	return g, fo
}

func TestWavelengthGetScore(t *testing.T) {
	cases := []struct{ target, guess, want int }{
		{10, 10, 4}, {10, 11, 3}, {10, 9, 3}, {10, 12, 2}, {10, 8, 2}, {10, 13, 0}, {10, 0, 0},
	}
	for _, c := range cases {
		if got := wlGetScore(c.target, c.guess); got != c.want {
			t.Errorf("wlGetScore(%d,%d)=%d want %d", c.target, c.guess, got, c.want)
		}
	}
}

func TestWavelengthStartSetsUpRound(t *testing.T) {
	g, _ := startedWavelength(t, 1)
	if g.spectrum[0] == "" || g.spectrum[1] == "" {
		t.Fatalf("no spectrum card dealt: %v", g.spectrum)
	}
	if g.target < 0 || g.target > wlScale {
		t.Fatalf("target %d out of range", g.target)
	}
	// The psychic must be on the active team.
	if p := g.players[g.psychic]; p == nil || p.team != g.active {
		t.Fatalf("psychic %d not on active team %d", g.psychic, g.active)
	}
}

func TestWavelengthPrivateTargetOnlyToPsychic(t *testing.T) {
	g, fo := startedWavelength(t, 2)
	if wlLast(t, fo, g.psychic).Target == nil {
		t.Fatal("psychic did not receive the target")
	}
	// a non-psychic seat
	other := g.teamSeats(g.active)
	var guesser int
	for _, s := range other {
		if s != g.psychic {
			guesser = s
		}
	}
	if wlLast(t, fo, guesser).Target != nil {
		t.Fatal("a non-psychic was sent the target")
	}
}

func TestWavelengthFullRoundScoring(t *testing.T) {
	g, _ := startedWavelength(t, 3)
	active := g.active
	g.target = 10 // pin for a deterministic score
	// psychic gives a clue
	wlCmd(t, g, g.psychic, false, map[string]any{"t": wlMsgClue, "clue": "middle"})
	if g.phase != wlGuess {
		t.Fatalf("after clue: phase %q", g.phase)
	}
	// an active-team guesser dials to 11 and locks (score should be 3)
	var guesser int
	for _, s := range g.teamSeats(active) {
		if s != g.psychic {
			guesser = s
		}
	}
	wlCmd(t, g, guesser, false, map[string]any{"t": wlMsgDial, "value": 11})
	wlCmd(t, g, guesser, false, map[string]any{"t": wlMsgLock})
	if g.phase != wlBet || g.guess != 11 {
		t.Fatalf("after lock: phase %q guess %d", g.phase, g.guess)
	}
	// the OTHER team bets "left" (target 10 < guess 11 → correct → +1)
	better := g.teamSeats(1 - active)[0]
	wlCmd(t, g, better, false, map[string]any{"t": wlMsgBet, "side": "left"})
	if g.phase != wlReveal {
		t.Fatalf("after bet: phase %q", g.phase)
	}
	if g.scores[active] != 3 {
		t.Fatalf("active team score = %d, want 3", g.scores[active])
	}
	if g.scores[1-active] != 1 {
		t.Fatalf("other team bet bonus = %d, want 1", g.scores[1-active])
	}
	// at reveal, everyone sees the target
	fo2 := g.out.(*fakeOutbox)
	if wlLast(t, fo2, better).Target == nil {
		t.Fatal("target not revealed to everyone at reveal")
	}
}

func TestWavelengthRejectsWrongRoles(t *testing.T) {
	g, _ := startedWavelength(t, 4)
	active := g.active
	// a non-psychic cannot give the clue
	var guesser int
	for _, s := range g.teamSeats(active) {
		if s != g.psychic {
			guesser = s
		}
	}
	wlCmd(t, g, guesser, false, map[string]any{"t": wlMsgClue, "clue": "x"})
	if g.clue != "" || g.phase != wlClue {
		t.Fatal("a non-psychic set the clue")
	}
	// the active team cannot bet on itself (bet is for the other team)
	wlCmd(t, g, g.psychic, false, map[string]any{"t": wlMsgClue, "clue": "x"}) // advance to guess
	wlCmd(t, g, guesser, false, map[string]any{"t": wlMsgLock})               // to bet
	wlCmd(t, g, guesser, false, map[string]any{"t": wlMsgBet, "side": "left"})
	if g.phase != wlBet {
		t.Fatalf("active team was allowed to bet: phase %q", g.phase)
	}
}
