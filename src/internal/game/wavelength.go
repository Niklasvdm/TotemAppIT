package game

import (
	"encoding/json"
	"math/rand/v2"
	"sort"
	"strings"
	"time"
)

// wavelengthGame is the party game Wavelength as a Game: two teams and a hidden
// target on a 0..20 spectrum between two opposite concepts. The active team's
// "psychic" alone sees the target and gives a one-line clue; their team rotates
// a dial to guess where it is; the other team bets whether the real target is to
// the left or right of that guess. It is the "one seat sees a hidden value" +
// teams + analog-input shape — the psychic's target is the private view
// (Outbox.Send). Rules/scoring from the reference cynicaloptimist/longwave.
//
// Scoring: the guessing team gets 4/3/2/0 for a dial within 0/1/2/>2 of the
// target; the other team gets +1 for a correct left/right bet. First to 10 wins.
// Event-driven (TickHz 0).

const (
	wlScale      = 20 // target and dial run 0..20
	wlWin        = 10
	wlMaxPlayers = 12
)

const (
	wlLobby  = "lobby"
	wlClue   = "clue"   // the psychic gives a clue
	wlGuess  = "guess"  // the active team moves the dial
	wlBet    = "bet"    // the other team bets left/right
	wlReveal = "reveal" // show the target + score
	wlOver   = "over"
)

const (
	wlMsgSetup   = "setup"   // {team}
	wlMsgStart   = "start"   // host
	wlMsgClue    = "clue"    // {clue} — psychic
	wlMsgDial    = "dial"    // {value} — active team moves the dial
	wlMsgLock    = "lock"    // active team locks the guess in
	wlMsgBet     = "bet"     // {side:"left"|"right"} — other team
	wlMsgNext    = "next"    // host — next round
	wlMsgRestart = "restart" // host
)

type wlPlayer struct {
	name string
	team int // 0 left, 1 right
}

type wavelengthGame struct {
	out Outbox
	rng *rand.Rand

	players map[int]*wlPlayer

	phase    string
	spectrum [2]string
	target   int // 0..20, hidden except to the psychic (and everyone at reveal)
	clue     string
	guess    int // the dial, 0..20
	counter  string // the other team's bet: "left" | "right"

	active     int // active team this round (0/1)
	psychic    int // seat giving the clue
	psychicIdx [2]int
	scores     [2]int
	winner     int // -1 none

	lastScore int  // points the guessing team just scored, for the reveal
	betOK     bool // whether the counter-bet was right

	deckOrder []int // shuffled card indices
	deckPos   int
}

func newWavelengthGame(out Outbox, seed uint64) Game {
	return &wavelengthGame{
		out:     out,
		rng:     rand.New(rand.NewPCG(seed, 0x243f6a8885a308d3)),
		players: map[int]*wlPlayer{},
		phase:   wlLobby,
		winner:  -1,
	}
}

// --- Game interface ---------------------------------------------------------

func (g *wavelengthGame) AddPlayer(name, meta string) (int, bool) {
	seat := -1
	for s := 0; s < wlMaxPlayers; s++ {
		if _, taken := g.players[s]; !taken {
			seat = s
			break
		}
	}
	if seat == -1 {
		return 0, false
	}
	g.players[seat] = &wlPlayer{name: name, team: g.smallerTeam()}
	return seat, true
}

func (g *wavelengthGame) RemovePlayer(seat int) {
	delete(g.players, seat)
	g.broadcastViews()
}

func (g *wavelengthGame) Roster() []RosterEntry {
	out := make([]RosterEntry, 0, len(g.players))
	for s, p := range g.players {
		out = append(out, RosterEntry{S: s, N: p.name})
	}
	return out
}

func (g *wavelengthGame) WelcomeExtra() WelcomeExtra { return WelcomeExtra{} }
func (g *wavelengthGame) Joined(int)                 { g.broadcastViews() }
func (g *wavelengthGame) Dropped(int)                { g.broadcastViews() }
func (g *wavelengthGame) ToLobby()                   { g.resetToLobby(); g.broadcastViews() }
func (g *wavelengthGame) TickHz() int                { return 0 }
func (g *wavelengthGame) Tick(time.Time)             {}

func (g *wavelengthGame) Command(seat int, isHost bool, raw []byte) {
	var c struct {
		T     string `json:"t"`
		Team  int    `json:"team"`
		Clue  string `json:"clue"`
		Value int    `json:"value"`
		Side  string `json:"side"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return
	}
	p := g.players[seat]
	if p == nil {
		return
	}

	switch c.T {
	case wlMsgSetup:
		if g.phase == wlLobby && (c.Team == 0 || c.Team == 1) {
			p.team = c.Team
		}

	case wlMsgStart:
		if !isHost || g.phase != wlLobby || !g.canStart() {
			return
		}
		g.startGame()

	case wlMsgClue:
		if g.phase != wlClue || seat != g.psychic {
			return
		}
		clue := strings.TrimSpace(c.Clue)
		if clue == "" || len(clue) > 60 {
			return
		}
		g.clue = clue
		g.phase = wlGuess

	case wlMsgDial:
		if g.phase != wlGuess || p.team != g.active || seat == g.psychic {
			return
		}
		g.guess = clampInt(c.Value, 0, wlScale)

	case wlMsgLock:
		if g.phase != wlGuess || p.team != g.active || seat == g.psychic {
			return
		}
		g.phase = wlBet

	case wlMsgBet:
		if g.phase != wlBet || p.team == g.active || (c.Side != "left" && c.Side != "right") {
			return
		}
		g.score(c.Side)

	case wlMsgNext:
		if !isHost || g.phase != wlReveal {
			return
		}
		if g.gameWon() {
			g.winner = g.leader()
			g.phase = wlOver
		} else {
			g.newRound(1 - g.active)
		}

	case wlMsgRestart:
		if !isHost || g.phase != wlOver {
			return
		}
		g.resetToLobby()

	default:
		return
	}
	g.broadcastViews()
}

// --- rules ------------------------------------------------------------------

// wlGetScore: 4 for an exact dial, 3 within 1, 2 within 2, else 0.
func wlGetScore(target, guess int) int {
	d := target - guess
	if d < 0 {
		d = -d
	}
	if d > 2 {
		return 0
	}
	return 4 - d
}

func (g *wavelengthGame) startGame() {
	g.deckOrder = g.rng.Perm(len(wavelengthCards))
	g.deckPos = 0
	g.scores = [2]int{}
	g.psychicIdx = [2]int{}
	g.winner = -1
	g.newRound(g.rng.IntN(2))
}

func (g *wavelengthGame) newRound(active int) {
	g.active = active
	members := g.teamSeats(active)
	g.psychic = members[g.psychicIdx[active]%len(members)]
	g.psychicIdx[active]++
	g.spectrum = wavelengthCards[g.deckOrder[g.deckPos%len(g.deckOrder)]]
	g.deckPos++
	g.target = g.rng.IntN(wlScale + 1)
	g.clue = ""
	g.guess = wlScale / 2
	g.counter = ""
	g.lastScore = 0
	g.betOK = false
	g.phase = wlClue
}

func (g *wavelengthGame) score(side string) {
	pts := wlGetScore(g.target, g.guess)
	correct := (side == "left" && g.target < g.guess) || (side == "right" && g.target > g.guess)
	g.scores[g.active] += pts
	if correct {
		g.scores[1-g.active]++
	}
	g.counter = side
	g.lastScore = pts
	g.betOK = correct
	g.phase = wlReveal
}

func (g *wavelengthGame) gameWon() bool {
	return (g.scores[0] >= wlWin || g.scores[1] >= wlWin) && g.scores[0] != g.scores[1]
}

func (g *wavelengthGame) leader() int {
	if g.scores[1] > g.scores[0] {
		return 1
	}
	return 0
}

func (g *wavelengthGame) canStart() bool {
	return len(g.teamSeats(0)) >= 2 && len(g.teamSeats(1)) >= 2
}

func (g *wavelengthGame) teamSeats(team int) []int {
	var out []int
	for s, p := range g.players {
		if p.team == team {
			out = append(out, s)
		}
	}
	sort.Ints(out)
	return out
}

func (g *wavelengthGame) smallerTeam() int {
	var n [2]int
	for _, p := range g.players {
		n[p.team]++
	}
	if n[1] < n[0] {
		return 1
	}
	return 0
}

func (g *wavelengthGame) resetToLobby() {
	g.phase = wlLobby
	g.spectrum = [2]string{}
	g.clue = ""
	g.counter = ""
	g.scores = [2]int{}
	g.winner = -1
	g.target = 0
	g.guess = 0
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// --- per-seat views ---------------------------------------------------------

type wlRosterEntry struct {
	S    int    `json:"s"`
	N    string `json:"n"`
	Team int    `json:"team"`
	Gone bool   `json:"gone,omitempty"`
}

type wlView struct {
	T        string          `json:"t"` // "state"
	Phase    string          `json:"ph"`
	Spectrum [2]string       `json:"spectrum"`
	Clue     string          `json:"clue"`
	Guess    int             `json:"guess"`
	Active   int             `json:"active"`
	Psychic  int             `json:"psychic"`
	Counter  string          `json:"counter"` // the bet, once placed
	Left     int             `json:"left"`    // team scores
	Right    int             `json:"right"`
	Winner   int             `json:"winner"`
	LastScore int            `json:"lastScore"` // reveal: points the team scored
	BetOK     bool           `json:"betOk"`     // reveal: was the bet right
	// Target is the hidden spectrum target — sent ONLY to the psychic until the
	// reveal, when everyone sees it. nil means hidden.
	Target *int            `json:"target"`
	You    int             `json:"you"`
	YouTeam int            `json:"youTeam"`
	Roster []wlRosterEntry `json:"roster"`
}

func (g *wavelengthGame) viewFor(seat int) wlView {
	me := g.players[seat]
	v := wlView{
		T: MsgState, Phase: g.phase, Spectrum: g.spectrum, Clue: g.clue, Guess: g.guess,
		Active: g.active, Psychic: g.psychic, Counter: g.counter,
		Left: g.scores[0], Right: g.scores[1], Winner: g.winner,
		LastScore: g.lastScore, BetOK: g.betOK, You: seat,
	}
	if me != nil {
		v.YouTeam = me.team
	}
	// Private view: the target is visible to the psychic while a round is live,
	// and to everyone once it's revealed.
	if g.phase != wlLobby && (seat == g.psychic || g.phase == wlReveal || g.phase == wlOver) {
		t := g.target
		v.Target = &t
	}
	connected := map[int]bool{}
	for _, s := range g.out.Seats() {
		connected[s] = true
	}
	for s, p := range g.players {
		v.Roster = append(v.Roster, wlRosterEntry{S: s, N: p.name, Team: p.team, Gone: !connected[s]})
	}
	return v
}

func (g *wavelengthGame) broadcastViews() {
	for _, seat := range g.out.Seats() {
		g.out.Send(seat, g.viewFor(seat))
	}
}
