package game

import (
	"encoding/json"
	"testing"
)

func dcLast(t *testing.T, f *fakeOutbox, seat int) decryptoView {
	t.Helper()
	v, ok := f.last[seat].(decryptoView)
	if !ok {
		t.Fatalf("seat %d has no decryptoView", seat)
	}
	return v
}

func dcCmd(t *testing.T, g *decryptoGame, seat int, host bool, m map[string]any) {
	t.Helper()
	b, _ := json.Marshal(m)
	g.Command(seat, host, b)
}

func startedDecrypto(t *testing.T, seed uint64) (*decryptoGame, *fakeOutbox) {
	t.Helper()
	fo := newFakeOutbox(0, 1, 2, 3) // 2v2: seats 0,2 vs 1,3
	g := newDecryptoGame(fo, seed).(*decryptoGame)
	for i := 0; i < 4; i++ {
		g.AddPlayer("p", "")
	}
	dcCmd(t, g, 0, true, map[string]any{"t": dcMsgStart})
	if g.phase != dcClue {
		t.Fatalf("after start: phase %q", g.phase)
	}
	return g, fo
}

func TestDecryptoTeamsBalancedAndWordsPrivate(t *testing.T) {
	g, fo := startedDecrypto(t, 1)
	if g.teamCount(0) != 2 || g.teamCount(1) != 2 {
		t.Fatalf("teams not balanced: %d vs %d", g.teamCount(0), g.teamCount(1))
	}
	// A seat sees its own team's four words, never the opposing team's.
	for s := 0; s < 4; s++ {
		v := dcLast(t, fo, s)
		if len(v.YourWords) != dcWords {
			t.Fatalf("seat %d got %d words, want %d", s, len(v.YourWords), dcWords)
		}
		own := g.teams[g.players[s].team].words
		if v.YourWords[0] != own[0] {
			t.Fatalf("seat %d shown the wrong team's words", s)
		}
	}
}

func TestDecryptoCodeHiddenFromNonEncryptor(t *testing.T) {
	g, fo := startedDecrypto(t, 2)
	if v := dcLast(t, fo, g.encryptor); len(v.Code) != 3 {
		t.Fatal("the encryptor must see the code")
	}
	// Someone who is not the encryptor must not see it mid-round.
	for s := 0; s < 4; s++ {
		if s == g.encryptor {
			continue
		}
		if v := dcLast(t, fo, s); v.Code != nil {
			t.Fatalf("seat %d (not encryptor) saw the code: %v", s, v.Code)
		}
	}
}

func TestDecryptoInterceptAndMiscomm(t *testing.T) {
	g, _ := startedDecrypto(t, 3)
	enc := g.encryptor
	active := g.activeTeam
	opp := 1 - active
	decoder := -1
	for _, s := range g.members(active) {
		if s != enc {
			decoder = s
		}
	}
	interceptor := g.members(opp)[0]

	dcCmd(t, g, enc, false, map[string]any{"t": dcMsgClue, "clues": []string{"a", "b", "c"}})
	if g.phase != dcGuess {
		t.Fatalf("phase after clues: %q", g.phase)
	}
	// The opposing team nails the code (interception); the active team misses.
	dcCmd(t, g, interceptor, false, map[string]any{"t": dcMsgIntercept, "guess": g.code})
	wrong := wrongCode(g.code)
	dcCmd(t, g, decoder, false, map[string]any{"t": dcMsgDecode, "guess": wrong})

	if g.teams[opp].intercepts != 1 {
		t.Fatalf("opposing team should have 1 interception, got %d", g.teams[opp].intercepts)
	}
	if g.teams[active].miscomms != 1 {
		t.Fatalf("active team should have 1 miscommunication, got %d", g.teams[active].miscomms)
	}
}

func TestDecryptoTwoInterceptionsWin(t *testing.T) {
	g, _ := startedDecrypto(t, 4)
	// Each round the opposing team intercepts correctly. Because the active team
	// alternates, interceptions land on alternating teams; by round 3 the team
	// that was opponent in rounds 1 and 3 reaches two interceptions and wins.
	for i := 0; i < 4; i++ {
		enc := g.encryptor
		opp := 1 - g.activeTeam
		var decoder, interceptor int
		for _, s := range g.members(g.activeTeam) {
			if s != enc {
				decoder = s
			}
		}
		interceptor = g.members(opp)[0]
		dcCmd(t, g, enc, false, map[string]any{"t": dcMsgClue, "clues": []string{"a", "b", "c"}})
		dcCmd(t, g, interceptor, false, map[string]any{"t": dcMsgIntercept, "guess": g.code})
		dcCmd(t, g, decoder, false, map[string]any{"t": dcMsgDecode, "guess": g.code})
		if g.phase == dcOver {
			break
		}
		if g.phase == dcReveal {
			dcCmd(t, g, 0, true, map[string]any{"t": dcMsgNext})
		}
	}
	if g.phase != dcOver || g.winner < 0 {
		t.Fatalf("two interceptions should win: phase=%q winner=%d", g.phase, g.winner)
	}
}

// wrongCode returns any valid code different from the given one.
func wrongCode(code []int) []int {
	alts := [][]int{{1, 2, 3}, {1, 2, 4}, {4, 3, 2}, {2, 1, 3}}
	for _, a := range alts {
		if !eqInts(a, code) {
			return a
		}
	}
	return []int{1, 2, 3}
}
