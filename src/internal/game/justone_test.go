package game

import (
	"encoding/json"
	"testing"
)

func joLast(t *testing.T, f *fakeOutbox, seat int) justOneView {
	t.Helper()
	v, ok := f.last[seat].(justOneView)
	if !ok {
		t.Fatalf("seat %d has no justOneView", seat)
	}
	return v
}

func joCmd(t *testing.T, g *justOneGame, seat int, host bool, m map[string]any) {
	t.Helper()
	b, _ := json.Marshal(m)
	g.Command(seat, host, b)
}

func startedJustOne(t *testing.T, seed uint64) (*justOneGame, *fakeOutbox) {
	t.Helper()
	fo := newFakeOutbox(0, 1, 2)
	g := newJustOneGame(fo, seed).(*justOneGame)
	for i := 0; i < 3; i++ {
		g.AddPlayer("p", "")
	}
	joCmd(t, g, 0, true, map[string]any{"t": joMsgStart})
	if g.phase != joClue {
		t.Fatalf("after start: phase %q", g.phase)
	}
	return g, fo
}

func TestJustOnePrivateWordHiddenFromGuesser(t *testing.T) {
	g, fo := startedJustOne(t, 1)
	if joLast(t, fo, g.guesser).Word != nil {
		t.Fatal("the guesser was sent the word")
	}
	// a writer (non-guesser) must see it
	writer := (g.guesser + 1) % 3
	if joLast(t, fo, writer).Word == nil {
		t.Fatal("a writer did not get the word")
	}
}

func TestJustOneCluesAndGuessFlow(t *testing.T) {
	g, _ := startedJustOne(t, 2)
	g.word = "Beaver"
	writers := []int{}
	for s := 0; s < 3; s++ {
		if s != g.guesser {
			writers = append(writers, s)
		}
	}
	joCmd(t, g, writers[0], false, map[string]any{"t": joMsgClue, "clue": "dam"})
	if g.phase != joClue {
		t.Fatal("phase advanced before all clues were in")
	}
	joCmd(t, g, writers[1], false, map[string]any{"t": joMsgClue, "clue": "wood"})
	if g.phase != joGuess {
		t.Fatalf("phase after both clues: %q", g.phase)
	}
	if len(g.surviving) != 2 {
		t.Fatalf("surviving clues = %v, want 2", g.surviving)
	}
	joCmd(t, g, g.guesser, false, map[string]any{"t": joMsgGuess, "guess": "beaver"}) // case-insensitive
	if g.phase != joResult || !g.lastCorrect || g.score != 1 {
		t.Fatalf("correct guess: phase=%q correct=%v score=%d", g.phase, g.lastCorrect, g.score)
	}
}

func TestJustOneDuplicateCluesCancel(t *testing.T) {
	g, _ := startedJustOne(t, 3)
	g.word = "Beaver"
	writers := []int{}
	for s := 0; s < 3; s++ {
		if s != g.guesser {
			writers = append(writers, s)
		}
	}
	joCmd(t, g, writers[0], false, map[string]any{"t": joMsgClue, "clue": "Dam"})
	joCmd(t, g, writers[1], false, map[string]any{"t": joMsgClue, "clue": "dam"}) // same word, different case
	if len(g.surviving) != 0 {
		t.Fatalf("identical clues should both cancel, got %v", g.surviving)
	}
}

func TestJustOnePassScoresNothing(t *testing.T) {
	g, _ := startedJustOne(t, 4)
	g.phase = joGuess
	g.surviving = []string{"x"}
	joCmd(t, g, g.guesser, false, map[string]any{"t": joMsgPass})
	if g.phase != joResult || g.score != 0 {
		t.Fatalf("pass: phase=%q score=%d", g.phase, g.score)
	}
}
