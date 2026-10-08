package game

import (
	"encoding/json"
	"testing"
)

// fakeOutbox captures what a Game emits, so a game can be tested without a Room.
// It stores the last view per seat as `any`; each game's tests type-assert it.
type fakeOutbox struct {
	seats []int
	last  map[int]any // last view sent to each seat
}

func newFakeOutbox(seats ...int) *fakeOutbox {
	return &fakeOutbox{seats: seats, last: map[int]any{}}
}
func (f *fakeOutbox) Send(seat int, v any) { f.last[seat] = v }
func (f *fakeOutbox) Broadcast(any)        {}
func (f *fakeOutbox) BroadcastRoster()     {}
func (f *fakeOutbox) Seats() []int         { return f.seats }
func (f *fakeOutbox) Host() int            { return 0 }

func cnLast(t *testing.T, f *fakeOutbox, seat int) cnView {
	t.Helper()
	v, ok := f.last[seat].(cnView)
	if !ok {
		t.Fatalf("seat %d has no cnView", seat)
	}
	return v
}

func cnCmd(t *testing.T, g *codenamesGame, seat int, host bool, m map[string]any) {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal cmd: %v", err)
	}
	g.Command(seat, host, b)
}

// a standard four-seat game: 0=red spymaster, 1=red guesser, 2=blue spymaster,
// 3=blue guesser — then started by the host (seat 0).
func startedGame(t *testing.T, seed uint64) (*codenamesGame, *fakeOutbox) {
	t.Helper()
	fo := newFakeOutbox(0, 1, 2, 3)
	g := newCodenamesGame(fo, seed).(*codenamesGame)
	for seat := 0; seat < 4; seat++ {
		if _, ok := g.AddPlayer("p", ""); !ok {
			t.Fatalf("AddPlayer %d failed", seat)
		}
	}
	cnCmd(t, g, 0, false, map[string]any{"t": cnMsgSetup, "team": 0, "spymaster": true})
	cnCmd(t, g, 1, false, map[string]any{"t": cnMsgSetup, "team": 0, "spymaster": false})
	cnCmd(t, g, 2, false, map[string]any{"t": cnMsgSetup, "team": 1, "spymaster": true})
	cnCmd(t, g, 3, false, map[string]any{"t": cnMsgSetup, "team": 1, "spymaster": false})
	cnCmd(t, g, 0, true, map[string]any{"t": cnMsgStart})
	if g.phase != cnClue {
		t.Fatalf("after start: phase %q, want %q", g.phase, cnClue)
	}
	return g, fo
}

// spymaster / guesser seats for the team whose turn it is in this fixture.
func (g *codenamesGame) curSpymaster() int {
	if g.turn == 0 {
		return 0
	}
	return 2
}
func (g *codenamesGame) curGuesser() int {
	if g.turn == 0 {
		return 1
	}
	return 3
}

func firstTile(g *codenamesGame, colour string) int {
	for i, c := range g.key {
		if c == colour && !g.revealed[i] {
			return i
		}
	}
	return -1
}

func TestCodenamesKeyDistribution(t *testing.T) {
	g, _ := startedGame(t, 42)
	if len(g.words) != cnTiles || len(g.key) != cnTiles {
		t.Fatalf("board sizes: %d words, %d key", len(g.words), len(g.key))
	}
	seen := map[string]bool{}
	for _, w := range g.words {
		if seen[w] {
			t.Fatalf("duplicate word on the board: %q", w)
		}
		seen[w] = true
	}
	var red, blue, neutral, assassin int
	for _, c := range g.key {
		switch c {
		case cnRedCol:
			red++
		case cnBlueCol:
			blue++
		case cnNeutralCol:
			neutral++
		case cnAssassin:
			assassin++
		}
	}
	if neutral != cnNeutrals || assassin != 1 {
		t.Fatalf("neutral=%d assassin=%d, want %d and 1", neutral, assassin, cnNeutrals)
	}
	// One team has 9 (the starter), the other 8.
	if !((red == 9 && blue == 8) || (red == 8 && blue == 9)) {
		t.Fatalf("agent split red=%d blue=%d, want 9/8", red, blue)
	}
}

func TestCodenamesPrivateViewKeyOnlyForSpymasters(t *testing.T) {
	_, fo := startedGame(t, 7)
	if len(cnLast(t, fo, 0).Key) != cnTiles { // seat 0 is a spymaster
		t.Fatalf("spymaster saw key of len %d, want %d", len(cnLast(t, fo, 0).Key), cnTiles)
	}
	if cnLast(t, fo, 1).Key != nil { // seat 1 is a guesser
		t.Fatalf("a guesser was sent the key: %v", cnLast(t, fo, 1).Key)
	}
	if len(cnLast(t, fo, 1).Words) != cnTiles {
		t.Fatalf("guesser did not get the words: %d", len(cnLast(t, fo, 1).Words))
	}
}

func TestCodenamesGuessOwnAgentContinues(t *testing.T) {
	g, _ := startedGame(t, 1)
	turn := g.turn
	cnCmd(t, g, g.curSpymaster(), false, map[string]any{"t": cnMsgClue, "word": "tree", "count": 2})
	if g.phase != cnGuess || g.guesses != 3 {
		t.Fatalf("after clue: phase %q guesses %d, want guess/3", g.phase, g.guesses)
	}
	cnCmd(t, g, g.curGuesser(), false, map[string]any{"t": cnMsgGuess, "tile": firstTile(g, teamColour(turn))})
	if g.turn != turn || g.phase != cnGuess {
		t.Fatalf("own-agent guess ended the turn: turn %d phase %q", g.turn, g.phase)
	}
	if g.guesses != 2 {
		t.Fatalf("guesses = %d after one own-agent guess, want 2", g.guesses)
	}
}

func TestCodenamesNeutralEndsTurn(t *testing.T) {
	g, _ := startedGame(t, 2)
	turn := g.turn
	cnCmd(t, g, g.curSpymaster(), false, map[string]any{"t": cnMsgClue, "word": "x", "count": 1})
	cnCmd(t, g, g.curGuesser(), false, map[string]any{"t": cnMsgGuess, "tile": firstTile(g, cnNeutralCol)})
	if g.turn != 1-turn || g.phase != cnClue {
		t.Fatalf("neutral did not pass the turn: turn %d (was %d), phase %q", g.turn, turn, g.phase)
	}
}

func TestCodenamesAssassinLosesInstantly(t *testing.T) {
	g, _ := startedGame(t, 3)
	turn := g.turn
	cnCmd(t, g, g.curSpymaster(), false, map[string]any{"t": cnMsgClue, "word": "x", "count": 1})
	cnCmd(t, g, g.curGuesser(), false, map[string]any{"t": cnMsgGuess, "tile": firstTile(g, cnAssassin)})
	if g.phase != cnOver || g.winner != 1-turn {
		t.Fatalf("assassin: phase %q winner %d, want over and %d", g.phase, g.winner, 1-turn)
	}
}

func TestCodenamesRevealingAllAgentsWins(t *testing.T) {
	g, _ := startedGame(t, 5)
	turn := g.turn
	cnCmd(t, g, g.curSpymaster(), false, map[string]any{"t": cnMsgClue, "word": "x", "count": 9})
	// Guess every one of the current team's agents in this one turn.
	for {
		tile := firstTile(g, teamColour(turn))
		if tile == -1 {
			break
		}
		cnCmd(t, g, g.curGuesser(), false, map[string]any{"t": cnMsgGuess, "tile": tile})
	}
	if g.phase != cnOver || g.winner != turn {
		t.Fatalf("clearing all agents: phase %q winner %d, want over and %d", g.phase, g.winner, turn)
	}
}

func TestCodenamesRejectsOutOfTurnAndWrongRole(t *testing.T) {
	g, _ := startedGame(t, 9)
	// A guesser cannot give a clue.
	cnCmd(t, g, g.curGuesser(), false, map[string]any{"t": cnMsgClue, "word": "x", "count": 1})
	if g.clue != nil || g.phase != cnClue {
		t.Fatalf("a guesser set a clue")
	}
	// The other team's spymaster cannot clue on this turn.
	other := 2
	if g.turn == 1 {
		other = 0
	}
	cnCmd(t, g, other, false, map[string]any{"t": cnMsgClue, "word": "x", "count": 1})
	if g.clue != nil {
		t.Fatalf("the off-turn spymaster set a clue")
	}
	// A spymaster cannot guess.
	cnCmd(t, g, g.curSpymaster(), false, map[string]any{"t": cnMsgClue, "word": "x", "count": 1})
	before := g.revealed
	cnCmd(t, g, g.curSpymaster(), false, map[string]any{"t": cnMsgGuess, "tile": 0})
	for i := range before {
		if g.revealed[i] {
			t.Fatalf("a spymaster revealed a tile")
		}
	}
}
