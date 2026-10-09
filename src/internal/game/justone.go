package game

import (
	"encoding/json"
	"math/rand/v2"
	"sort"
	"strings"
	"time"
)

// justOneGame is the co-op party game Just One: a mystery word is shown to
// everyone EXCEPT the guesser; each other player secretly writes a one-word
// clue; identical clues cancel out; the guesser sees what's left and makes one
// guess. The private view is that the guesser alone cannot see the word
// (Outbox.Send). Event-driven (TickHz 0). Rules from the base game; the word
// deck is reused from Codenames.
//
// Simplifications vs the boxed game: exact (case-insensitive) duplicate clues and
// clues equal to the word are auto-removed; a wrong guess just scores nothing
// (no penalty card). A game is a fixed number of rounds.

const (
	joMinPlayers = 3
	joMaxPlayers = 8
	joRounds     = 13 // cards in a game, as in the box
)

const (
	joLobby  = "lobby"
	joClue   = "clue"   // writers submit one clue each
	joGuess  = "guess"  // the guesser reads the surviving clues and guesses
	joResult = "result" // show the word + whether it was right
	joOver   = "over"
)

const (
	joMsgStart   = "start"
	joMsgClue    = "clue"  // {clue} — a writer's one-word clue
	joMsgGuess   = "guess" // {guess} — the guesser's answer
	joMsgPass    = "pass"  // the guesser gives up this card
	joMsgNext    = "next"  // host — next round
	joMsgRestart = "restart"
)

type joPlayer struct{ name string }

type justOneGame struct {
	out Outbox
	rng *rand.Rand

	players map[int]*joPlayer
	order   []int

	phase     string
	guesser   int
	gIdx      int // rotation
	word      string
	clues     map[int]string // seat -> submitted clue (writers only)
	surviving []string       // clues left after cancelling duplicates

	round, score int
	lastGuess    string
	lastCorrect  bool

	deck    []int
	deckPos int
}

func newJustOneGame(out Outbox, seed uint64) Game {
	return &justOneGame{
		out:     out,
		rng:     rand.New(rand.NewPCG(seed, 0x243f6a8885a308d3)),
		players: map[int]*joPlayer{},
		phase:   joLobby,
		clues:   map[int]string{},
	}
}

// --- Game interface ---------------------------------------------------------

func (g *justOneGame) AddPlayer(name, meta string) (int, bool) {
	seat := -1
	for s := 0; s < joMaxPlayers; s++ {
		if _, taken := g.players[s]; !taken {
			seat = s
			break
		}
	}
	if seat == -1 {
		return 0, false
	}
	g.players[seat] = &joPlayer{name: name}
	g.order = append(g.order, seat)
	return seat, true
}

func (g *justOneGame) RemovePlayer(seat int) {
	delete(g.players, seat)
	delete(g.clues, seat)
	for i, s := range g.order {
		if s == seat {
			g.order = append(g.order[:i], g.order[i+1:]...)
			break
		}
	}
	g.broadcastViews()
}

func (g *justOneGame) Roster() []RosterEntry {
	out := make([]RosterEntry, 0, len(g.players))
	for s, p := range g.players {
		out = append(out, RosterEntry{S: s, N: p.name})
	}
	return out
}

func (g *justOneGame) WelcomeExtra() WelcomeExtra { return WelcomeExtra{} }
func (g *justOneGame) Joined(int)                 { g.broadcastViews() }
func (g *justOneGame) Dropped(int)                { g.broadcastViews() }
func (g *justOneGame) ToLobby()                   { g.resetToLobby(); g.broadcastViews() }
func (g *justOneGame) TickHz() int                { return 0 }
func (g *justOneGame) Tick(time.Time)             {}

func (g *justOneGame) Command(seat int, isHost bool, raw []byte) {
	var c struct {
		T     string `json:"t"`
		Clue  string `json:"clue"`
		Guess string `json:"guess"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return
	}
	if _, ok := g.players[seat]; !ok {
		return
	}

	switch c.T {
	case joMsgStart:
		if !isHost || g.phase != joLobby {
			return
		}
		if n := len(g.players); n < joMinPlayers || n > joMaxPlayers {
			return
		}
		g.deck = g.rng.Perm(len(codenamesWords))
		g.deckPos = 0
		g.score = 0
		g.round = 0
		g.gIdx = 0
		g.newRound()

	case joMsgClue:
		if g.phase != joClue || seat == g.guesser {
			return
		}
		clue := strings.TrimSpace(c.Clue)
		if clue == "" || strings.ContainsAny(clue, " \t\n") || len(clue) > 40 {
			return // a single word
		}
		g.clues[seat] = clue
		if len(g.clues) == g.writers() {
			g.cancelDuplicates()
			g.phase = joGuess
		}

	case joMsgGuess:
		if g.phase != joGuess || seat != g.guesser {
			return
		}
		guess := strings.TrimSpace(c.Guess)
		if guess == "" {
			return
		}
		g.lastGuess = guess
		g.lastCorrect = strings.EqualFold(guess, g.word)
		if g.lastCorrect {
			g.score++
		}
		g.phase = joResult

	case joMsgPass:
		if g.phase != joGuess || seat != g.guesser {
			return
		}
		g.lastGuess = ""
		g.lastCorrect = false
		g.phase = joResult

	case joMsgNext:
		if !isHost || g.phase != joResult {
			return
		}
		if g.round >= joRounds {
			g.phase = joOver
		} else {
			g.newRound()
		}

	case joMsgRestart:
		if !isHost || g.phase != joOver {
			return
		}
		g.resetToLobby()

	default:
		return
	}
	g.broadcastViews()
}

// --- rules ------------------------------------------------------------------

func (g *justOneGame) writers() int { return len(g.players) - 1 } // everyone but the guesser

func (g *justOneGame) newRound() {
	g.round++
	g.guesser = g.order[g.gIdx%len(g.order)]
	g.gIdx++
	g.word = codenamesWords[g.deck[g.deckPos%len(g.deck)]]
	g.deckPos++
	g.clues = map[int]string{}
	g.surviving = nil
	g.lastGuess = ""
	g.lastCorrect = false
	g.phase = joClue
}

// cancelDuplicates drops clues that match another clue (case-insensitively) or
// the mystery word itself — only the unique, valid ones reach the guesser.
func (g *justOneGame) cancelDuplicates() {
	counts := map[string]int{}
	for _, c := range g.clues {
		counts[strings.ToLower(c)]++
	}
	// Stable order by seat for a deterministic list.
	seats := make([]int, 0, len(g.clues))
	for s := range g.clues {
		seats = append(seats, s)
	}
	sort.Ints(seats)
	g.surviving = nil
	for _, s := range seats {
		c := g.clues[s]
		lc := strings.ToLower(c)
		if counts[lc] == 1 && !strings.EqualFold(c, g.word) {
			g.surviving = append(g.surviving, c)
		}
	}
}

func (g *justOneGame) resetToLobby() {
	g.phase = joLobby
	g.word = ""
	g.clues = map[int]string{}
	g.surviving = nil
	g.round, g.score = 0, 0
	g.lastGuess = ""
	g.lastCorrect = false
}

// --- per-seat views ---------------------------------------------------------

type joRosterEntry struct {
	S     int    `json:"s"`
	N     string `json:"n"`
	Wrote bool   `json:"wrote"` // submitted their clue this round
	Gone  bool   `json:"gone,omitempty"`
}

type justOneView struct {
	T         string `json:"t"`
	Phase     string `json:"ph"`
	Guesser   int    `json:"guesser"`
	Round     int    `json:"round"`
	Rounds    int    `json:"rounds"`
	Score     int    `json:"score"`
	Submitted int    `json:"submitted"` // clues in so far
	Writers   int    `json:"writers"`   // clues expected
	// Word is the mystery word: shown to everyone EXCEPT the guesser while a
	// round is live, and to everyone at the result. nil = hidden.
	Word      *string         `json:"word"`
	YourClue  string          `json:"yourClue"`
	Clues     []string        `json:"clues"` // surviving clues (guess phase onward)
	LastGuess string          `json:"lastGuess"`
	Correct   bool            `json:"correct"`
	You       int             `json:"you"`
	Roster    []joRosterEntry `json:"roster"`
}

func (g *justOneGame) viewFor(seat int) justOneView {
	v := justOneView{
		T: MsgState, Phase: g.phase, Guesser: g.guesser, Round: g.round, Rounds: joRounds,
		Score: g.score, Submitted: len(g.clues), Writers: g.writers(),
		YourClue: g.clues[seat], LastGuess: g.lastGuess, Correct: g.lastCorrect, You: seat,
	}
	revealed := g.phase == joResult || g.phase == joOver
	if g.phase != joLobby && (seat != g.guesser || revealed) {
		w := g.word
		v.Word = &w
	}
	if g.phase == joGuess || revealed {
		v.Clues = append([]string{}, g.surviving...)
	}
	connected := map[int]bool{}
	for _, s := range g.out.Seats() {
		connected[s] = true
	}
	for s, p := range g.players {
		_, wrote := g.clues[s]
		v.Roster = append(v.Roster, joRosterEntry{S: s, N: p.name, Wrote: wrote, Gone: !connected[s]})
	}
	return v
}

func (g *justOneGame) broadcastViews() {
	for _, seat := range g.out.Seats() {
		g.out.Send(seat, g.viewFor(seat))
	}
}
