package game

import (
	"encoding/json"
	"math/rand/v2"
	"time"
)

// decryptoGame is Decrypto: two teams, each with four secret words known only to
// that team. Each round the active team's encryptor draws a 3-digit code (digits
// 1–4, all different) and gives one clue per digit, each hinting at the team word
// in that slot. The active team must decode its own code; the opposing team tries
// to intercept it from the clues (and the growing public history of past clues).
//
//	intercept the enemy code  -> +1 interception  (2 interceptions win the game)
//	miss your own code        -> +1 miscommunication (2 miscommunications lose it)
//
// The private view: a team sees its own four words (Outbox.Send); the opposing
// team never does. The code is shown only to the encryptor until the reveal.
// Event-driven (TickHz 0). Rules from the standard edition, with one
// simplification: the two teams take turns being the encrypting team rather than
// both encrypting every round.

const (
	dcMinPlayers = 4 // two teams of two
	dcMaxPlayers = 8
	dcWords      = 4 // secret words per team
	dcMaxRounds  = 8
	dcWinTokens  = 2 // interceptions to win / miscommunications to lose
)

const (
	dcLobby  = "lobby"
	dcClue   = "clue"   // the active encryptor writes three clues
	dcGuess  = "guess"  // both teams lock in a code guess
	dcReveal = "reveal" // show the code + outcome
	dcOver   = "over"
)

const (
	dcMsgStart     = "start"
	dcMsgClue      = "clue"      // {clues:[3]} — encryptor only
	dcMsgDecode    = "decode"    // {guess:[3]} — active team, not the encryptor
	dcMsgIntercept = "intercept" // {guess:[3]} — opposing team
	dcMsgNext      = "next"      // host
	dcMsgRestart   = "restart"
)

type dcPlayer struct {
	name string
	team int // 0 or 1
}

type dcTeam struct {
	words      []string
	encIdx     int // rotation into the team's members for the encryptor
	intercepts int
	miscomms   int
}

// dcHist is one past round's clues made public for future interception.
type dcHist struct {
	Team  int      `json:"team"`
	Code  []int    `json:"code"`
	Clues []string `json:"clues"`
}

type decryptoGame struct {
	out Outbox
	rng *rand.Rand

	players map[int]*dcPlayer
	order   []int
	teams   [2]*dcTeam

	phase      string
	activeTeam int
	encryptor  int // seat

	code           []int
	clues          []string
	decodeGuess    []int
	interceptGuess []int
	decodeDone     bool
	interceptDone  bool

	round   int
	history []dcHist
	winner  int // team, -1
}

func newDecryptoGame(out Outbox, seed uint64) Game {
	return &decryptoGame{
		out:     out,
		rng:     rand.New(rand.NewPCG(seed, 0xbb67ae8584caa73b)),
		players: map[int]*dcPlayer{},
		teams:   [2]*dcTeam{{}, {}},
		phase:   dcLobby,
		winner:  -1,
	}
}

// --- Game interface ---------------------------------------------------------

func (g *decryptoGame) AddPlayer(name, meta string) (int, bool) {
	seat := -1
	for s := 0; s < dcMaxPlayers; s++ {
		if _, taken := g.players[s]; !taken {
			seat = s
			break
		}
	}
	if seat == -1 {
		return 0, false
	}
	// Balance the teams as players arrive.
	team := 0
	if g.teamCount(0) > g.teamCount(1) {
		team = 1
	}
	g.players[seat] = &dcPlayer{name: name, team: team}
	g.order = append(g.order, seat)
	return seat, true
}

func (g *decryptoGame) RemovePlayer(seat int) {
	delete(g.players, seat)
	for i, s := range g.order {
		if s == seat {
			g.order = append(g.order[:i], g.order[i+1:]...)
			break
		}
	}
	g.broadcastViews()
}

func (g *decryptoGame) Roster() []RosterEntry {
	out := make([]RosterEntry, 0, len(g.players))
	for s, p := range g.players {
		out = append(out, RosterEntry{S: s, N: p.name})
	}
	return out
}

func (g *decryptoGame) WelcomeExtra() WelcomeExtra { return WelcomeExtra{} }
func (g *decryptoGame) Joined(int)                 { g.broadcastViews() }
func (g *decryptoGame) Dropped(int)                { g.broadcastViews() }
func (g *decryptoGame) ToLobby()                   { g.resetToLobby(); g.broadcastViews() }
func (g *decryptoGame) TickHz() int                { return 0 }
func (g *decryptoGame) Tick(time.Time)             {}

func (g *decryptoGame) Command(seat int, isHost bool, raw []byte) {
	var c struct {
		T     string   `json:"t"`
		Clues []string `json:"clues"`
		Guess []int    `json:"guess"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return
	}
	p, ok := g.players[seat]
	if !ok {
		return
	}

	switch c.T {
	case dcMsgStart:
		if !isHost || g.phase != dcLobby {
			return
		}
		if n := len(g.players); n < dcMinPlayers || n > dcMaxPlayers {
			return
		}
		if g.teamCount(0) < 2 || g.teamCount(1) < 2 {
			return
		}
		g.startGame()

	case dcMsgClue:
		if g.phase != dcClue || seat != g.encryptor || len(c.Clues) != 3 {
			return
		}
		for _, cl := range c.Clues {
			if cl == "" {
				return
			}
		}
		g.clues = append([]string{}, c.Clues...)
		g.phase = dcGuess

	case dcMsgDecode:
		if g.phase != dcGuess || p.team != g.activeTeam || seat == g.encryptor || !validCode(c.Guess) {
			return
		}
		g.decodeGuess = append([]int{}, c.Guess...)
		g.decodeDone = true
		g.maybeResolve()

	case dcMsgIntercept:
		if g.phase != dcGuess || p.team == g.activeTeam || !validCode(c.Guess) {
			return
		}
		g.interceptGuess = append([]int{}, c.Guess...)
		g.interceptDone = true
		g.maybeResolve()

	case dcMsgNext:
		if !isHost || g.phase != dcReveal {
			return
		}
		if g.round >= dcMaxRounds {
			g.finish()
		} else {
			g.newRound()
		}

	case dcMsgRestart:
		if !isHost || g.phase != dcOver {
			return
		}
		g.resetToLobby()

	default:
		return
	}
	g.broadcastViews()
}

// --- rules ------------------------------------------------------------------

func (g *decryptoGame) teamCount(team int) int {
	n := 0
	for _, p := range g.players {
		if p.team == team {
			n++
		}
	}
	return n
}

func (g *decryptoGame) members(team int) []int {
	var out []int
	for _, s := range g.order {
		if g.players[s].team == team {
			out = append(out, s)
		}
	}
	return out
}

func (g *decryptoGame) startGame() {
	deck := g.rng.Perm(len(codenamesWords))
	pos := 0
	for t := 0; t < 2; t++ {
		g.teams[t] = &dcTeam{}
		for i := 0; i < dcWords; i++ {
			g.teams[t].words = append(g.teams[t].words, codenamesWords[deck[pos]])
			pos++
		}
	}
	g.round = 0
	g.history = nil
	g.winner = -1
	g.activeTeam = 1 // newRound flips it to 0 first
	g.newRound()
}

func (g *decryptoGame) newRound() {
	g.round++
	g.activeTeam = 1 - g.activeTeam
	team := g.teams[g.activeTeam]
	mem := g.members(g.activeTeam)
	g.encryptor = mem[team.encIdx%len(mem)]
	team.encIdx++
	// A 3-digit code of distinct digits 1–4.
	p := g.rng.Perm(dcWords)
	g.code = []int{p[0] + 1, p[1] + 1, p[2] + 1}
	g.clues = nil
	g.decodeGuess = nil
	g.interceptGuess = nil
	g.decodeDone = false
	g.interceptDone = false
	g.phase = dcClue
}

func (g *decryptoGame) maybeResolve() {
	if !g.decodeDone || !g.interceptDone {
		return
	}
	opp := 1 - g.activeTeam
	if eqInts(g.interceptGuess, g.code) {
		g.teams[opp].intercepts++
	}
	if !eqInts(g.decodeGuess, g.code) {
		g.teams[g.activeTeam].miscomms++
	}
	g.history = append(g.history, dcHist{Team: g.activeTeam, Code: append([]int{}, g.code...), Clues: append([]string{}, g.clues...)})

	switch {
	case g.teams[0].intercepts >= dcWinTokens || g.teams[1].miscomms >= dcWinTokens:
		g.winner = 0
		g.phase = dcOver
	case g.teams[1].intercepts >= dcWinTokens || g.teams[0].miscomms >= dcWinTokens:
		g.winner = 1
		g.phase = dcOver
	default:
		g.phase = dcReveal
	}
}

// finish ends the game after the round cap: most interceptions wins, fewest
// miscommunications breaks a tie, else a draw (winner -1).
func (g *decryptoGame) finish() {
	a, b := g.teams[0], g.teams[1]
	switch {
	case a.intercepts != b.intercepts:
		g.winner = boolTeam(a.intercepts > b.intercepts)
	case a.miscomms != b.miscomms:
		g.winner = boolTeam(a.miscomms < b.miscomms)
	default:
		g.winner = -1
	}
	g.phase = dcOver
}

func (g *decryptoGame) resetToLobby() {
	g.phase = dcLobby
	g.teams = [2]*dcTeam{{}, {}}
	g.code = nil
	g.clues = nil
	g.decodeGuess = nil
	g.interceptGuess = nil
	g.decodeDone = false
	g.interceptDone = false
	g.round = 0
	g.history = nil
	g.winner = -1
}

// --- per-seat views ---------------------------------------------------------

type dcTeamView struct {
	Intercepts int `json:"intercepts"`
	Miscomms   int `json:"miscomms"`
	Members    int `json:"members"`
}

type dcRosterEntry struct {
	S    int    `json:"s"`
	N    string `json:"n"`
	Team int    `json:"team"`
	Gone bool   `json:"gone,omitempty"`
}

type decryptoView struct {
	T          string          `json:"t"`
	Phase      string          `json:"ph"`
	Round      int             `json:"round"`
	Rounds     int             `json:"rounds"`
	You        int             `json:"you"`
	Team       int             `json:"team"`
	ActiveTeam int             `json:"activeTeam"`
	Encryptor  int             `json:"encryptor"`
	// YourWords is your own team's four secret words; nil for the other team.
	YourWords []string      `json:"yourWords"`
	// Code is shown to the encryptor while cluing, and to everyone at the reveal.
	Code         []int         `json:"code"`
	Clues        []string      `json:"clues"`
	DecodeGuess  []int         `json:"decodeGuess"`
	Intercept    []int         `json:"interceptGuess"`
	DecodeDone   bool          `json:"decodeDone"`
	InterceptHas bool          `json:"interceptDone"`
	History      []dcHist      `json:"history"`
	Teams        []dcTeamView  `json:"teams"`
	Winner       int           `json:"winner"`
	Roster       []dcRosterEntry `json:"roster"`
}

func (g *decryptoGame) viewFor(seat int) decryptoView {
	me := g.players[seat]
	revealed := g.phase == dcReveal || g.phase == dcOver
	v := decryptoView{
		T: MsgState, Phase: g.phase, Round: g.round, Rounds: dcMaxRounds, You: seat,
		Team: me.team, ActiveTeam: g.activeTeam, Encryptor: g.encryptor,
		Clues: append([]string{}, g.clues...), History: append([]dcHist{}, g.history...),
		DecodeDone: g.decodeDone, InterceptHas: g.interceptDone, Winner: g.winner,
	}
	if g.phase != dcLobby && g.teams[me.team] != nil {
		v.YourWords = append([]string{}, g.teams[me.team].words...)
	}
	if revealed || (seat == g.encryptor && (g.phase == dcClue || g.phase == dcGuess)) {
		v.Code = append([]int{}, g.code...)
	}
	if revealed {
		v.DecodeGuess = append([]int{}, g.decodeGuess...)
		v.Intercept = append([]int{}, g.interceptGuess...)
	}
	for t := 0; t < 2; t++ {
		tv := dcTeamView{Members: g.teamCount(t)}
		if g.teams[t] != nil {
			tv.Intercepts = g.teams[t].intercepts
			tv.Miscomms = g.teams[t].miscomms
		}
		v.Teams = append(v.Teams, tv)
	}
	connected := map[int]bool{}
	for _, s := range g.out.Seats() {
		connected[s] = true
	}
	for _, s := range g.order {
		p := g.players[s]
		v.Roster = append(v.Roster, dcRosterEntry{S: s, N: p.name, Team: p.team, Gone: !connected[s]})
	}
	return v
}

func (g *decryptoGame) broadcastViews() {
	for _, seat := range g.out.Seats() {
		g.out.Send(seat, g.viewFor(seat))
	}
}

// --- helpers ----------------------------------------------------------------

// validCode checks a guess is three distinct digits in 1–4.
func validCode(xs []int) bool {
	if len(xs) != 3 {
		return false
	}
	seen := map[int]bool{}
	for _, x := range xs {
		if x < 1 || x > dcWords || seen[x] {
			return false
		}
		seen[x] = true
	}
	return true
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func boolTeam(team0Wins bool) int {
	if team0Wins {
		return 0
	}
	return 1
}
