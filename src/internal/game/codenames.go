package game

import (
	"encoding/json"
	"math/rand/v2"
	"strings"
	"time"
)

// codenamesGame is Codenames as a Game: two teams, a hidden colour key only the
// spymasters see, a 5x5 word grid, clue -> guess turns, and an assassin that ends
// the game. It is event-driven (TickHz 0): the room pushes only when a command
// changes something. Rules follow the base game (see docs/games-architecture.md).
//
// The whole point of this game for the engine is the per-seat PRIVATE view:
// spymasters receive the full key, guessers never do. That is done by rendering a
// different view per seat and pushing each seat its own through Outbox.Send.

const (
	cnTiles      = 25 // 5x5 board
	cnStartAgents = 9 // the team that goes first gets the extra agent
	cnOtherAgents = 8
	cnNeutrals    = 7
	// 9 + 8 + 7 + 1 assassin = 25.
	cnMaxPlayers = 10
)

// tile colours (also the win/loss discriminators).
const (
	cnRedCol     = "red"
	cnBlueCol    = "blue"
	cnNeutralCol = "neutral"
	cnAssassin   = "assassin"
)

// phases.
const (
	cnLobby = "lobby"
	cnClue  = "clue"  // waiting for the current team's spymaster
	cnGuess = "guess" // waiting for the current team's guessers
	cnOver  = "over"
)

// client message types.
const (
	cnMsgSetup   = "setup"   // choose team + role in the lobby
	cnMsgStart   = "start"   // host deals a board and begins
	cnMsgClue    = "clue"    // spymaster: word + count
	cnMsgGuess   = "guess"   // guesser: reveal a tile
	cnMsgPass    = "pass"    // guesser: end the turn early
	cnMsgRestart = "restart" // host: back to the lobby for a new game
)

type cnPlayer struct {
	name      string
	avatar    string // optional totem slug, from join meta
	team      int    // 0 = red, 1 = blue
	spymaster bool
}

type cnClueInfo struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

type codenamesGame struct {
	out Outbox
	rng *rand.Rand

	players map[int]*cnPlayer

	phase    string
	words    []string // 25 words
	key      []string // 25 colours (the secret)
	revealed []bool   // 25

	turn    int // current team (0 red, 1 blue)
	clue    *cnClueInfo
	guesses int // guesses remaining this clue
	winner  int // -1 none, else team
}

func newCodenamesGame(out Outbox, seed uint64) Game {
	return &codenamesGame{
		out:     out,
		rng:     rand.New(rand.NewPCG(seed, 0x243f6a8885a308d3)),
		players: map[int]*cnPlayer{},
		phase:   cnLobby,
		winner:  -1,
	}
}

// --- Game interface ---------------------------------------------------------

func (g *codenamesGame) AddPlayer(name, meta string) (int, bool) {
	// Lowest free seat, balancing the new player onto the smaller team.
	seat := -1
	for s := 0; s < cnMaxPlayers; s++ {
		if _, taken := g.players[s]; !taken {
			seat = s
			break
		}
	}
	if seat == -1 {
		return 0, false
	}
	g.players[seat] = &cnPlayer{name: name, avatar: meta, team: g.smallerTeam()}
	return seat, true
}

func (g *codenamesGame) RemovePlayer(seat int) {
	delete(g.players, seat)
	g.broadcastViews()
}

func (g *codenamesGame) Roster() []RosterEntry {
	out := make([]RosterEntry, 0, len(g.players))
	for s, p := range g.players {
		out = append(out, RosterEntry{S: s, N: p.name, M: p.avatar})
	}
	return out
}

func (g *codenamesGame) WelcomeExtra() WelcomeExtra { return WelcomeExtra{} }

func (g *codenamesGame) Joined(seat int) { g.out.Send(seat, g.viewFor(seat)) }

func (g *codenamesGame) Dropped(int) {} // turn-based: a held seat just waits

func (g *codenamesGame) ToLobby() { g.resetToLobby(); g.broadcastViews() }

func (g *codenamesGame) TickHz() int        { return 0 } // event-driven
func (g *codenamesGame) Tick(_ time.Time)   {}

func (g *codenamesGame) Command(seat int, isHost bool, raw []byte) {
	var c struct {
		T         string `json:"t"`
		Team      int    `json:"team"`
		Spymaster bool   `json:"spymaster"`
		Word      string `json:"word"`
		Count     int    `json:"count"`
		Tile      int    `json:"tile"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return
	}
	p := g.players[seat]
	if p == nil {
		return
	}

	switch c.T {
	case cnMsgSetup:
		if g.phase != cnLobby {
			return
		}
		if c.Team == 0 || c.Team == 1 {
			p.team = c.Team
		}
		p.spymaster = c.Spymaster

	case cnMsgStart:
		if !isHost || g.phase != cnLobby || !g.canStart() {
			return
		}
		g.deal()

	case cnMsgClue:
		if g.phase != cnClue || p.team != g.turn || !p.spymaster {
			return
		}
		word := strings.TrimSpace(c.Word)
		if word == "" || strings.ContainsAny(word, " \t\n") || c.Count < 0 || c.Count > 9 {
			return // a clue is a single word; the count is 0..9
		}
		g.clue = &cnClueInfo{Word: word, Count: c.Count}
		if c.Count == 0 {
			g.guesses = g.unrevealed() // 0 means "unlimited" in the base rules
		} else {
			g.guesses = c.Count + 1 // the team may guess the number, plus one
		}
		g.phase = cnGuess

	case cnMsgGuess:
		if g.phase != cnGuess || p.team != g.turn || p.spymaster {
			return
		}
		if c.Tile < 0 || c.Tile >= cnTiles || g.revealed[c.Tile] {
			return
		}
		g.reveal(c.Tile)

	case cnMsgPass:
		if g.phase != cnGuess || p.team != g.turn || p.spymaster {
			return
		}
		g.endTurn()

	case cnMsgRestart:
		if !isHost || g.phase != cnOver {
			return
		}
		g.resetToLobby()

	default:
		return
	}
	g.broadcastViews()
}

// --- rules ------------------------------------------------------------------

// deal lays out 25 random words and a fresh key: the starting team gets 9 agents,
// the other 8, plus 7 bystanders and 1 assassin, shuffled across the grid.
func (g *codenamesGame) deal() {
	deck := append([]string(nil), codenamesWords...)
	g.rng.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	g.words = deck[:cnTiles]

	start := g.rng.IntN(2) // which team goes first (and gets the 9th agent)
	key := make([]string, 0, cnTiles)
	for i := 0; i < cnStartAgents; i++ {
		key = append(key, teamColour(start))
	}
	for i := 0; i < cnOtherAgents; i++ {
		key = append(key, teamColour(1-start))
	}
	for i := 0; i < cnNeutrals; i++ {
		key = append(key, cnNeutralCol)
	}
	key = append(key, cnAssassin)
	g.rng.Shuffle(len(key), func(i, j int) { key[i], key[j] = key[j], key[i] })

	g.key = key
	g.revealed = make([]bool, cnTiles)
	g.turn = start
	g.clue = nil
	g.guesses = 0
	g.winner = -1
	g.phase = cnClue
}

// reveal flips a tile for the current team and resolves the outcome.
func (g *codenamesGame) reveal(tile int) {
	g.revealed[tile] = true
	col := g.key[tile]
	switch col {
	case cnAssassin:
		g.win(1 - g.turn) // touching the assassin loses it for the current team
	case teamColour(g.turn):
		if g.agentsLeft(g.turn) == 0 {
			g.win(g.turn)
			return
		}
		g.guesses--
		if g.guesses <= 0 {
			g.endTurn()
		}
	case teamColour(1 - g.turn):
		// A guess onto the other team's agent helps them — and can win it for them.
		if g.agentsLeft(1-g.turn) == 0 {
			g.win(1 - g.turn)
			return
		}
		g.endTurn()
	default: // neutral bystander
		g.endTurn()
	}
}

func (g *codenamesGame) endTurn() {
	g.turn = 1 - g.turn
	g.clue = nil
	g.guesses = 0
	g.phase = cnClue
}

func (g *codenamesGame) win(team int) {
	g.winner = team
	g.phase = cnOver
}

func (g *codenamesGame) resetToLobby() {
	g.phase = cnLobby
	g.words = nil
	g.key = nil
	g.revealed = nil
	g.clue = nil
	g.guesses = 0
	g.turn = 0
	g.winner = -1
}

// agentsLeft counts a team's unrevealed agents.
func (g *codenamesGame) agentsLeft(team int) int {
	n := 0
	col := teamColour(team)
	for i, k := range g.key {
		if k == col && !g.revealed[i] {
			n++
		}
	}
	return n
}

func (g *codenamesGame) unrevealed() int {
	n := 0
	for _, r := range g.revealed {
		if !r {
			n++
		}
	}
	return n
}

// canStart requires both teams to field a spymaster and at least one guesser.
func (g *codenamesGame) canStart() bool {
	var spy, guess [2]int
	for _, p := range g.players {
		if p.spymaster {
			spy[p.team]++
		} else {
			guess[p.team]++
		}
	}
	return spy[0] > 0 && spy[1] > 0 && guess[0] > 0 && guess[1] > 0
}

func (g *codenamesGame) smallerTeam() int {
	var n [2]int
	for _, p := range g.players {
		n[p.team]++
	}
	if n[1] < n[0] {
		return 1
	}
	return 0
}

// --- per-seat views ---------------------------------------------------------

type cnRosterEntry struct {
	S         int    `json:"s"`
	N         string `json:"n"`
	Team      int    `json:"team"`
	Spymaster bool   `json:"spymaster"`
	Gone      bool   `json:"gone,omitempty"`
}

type cnView struct {
	T        string `json:"t"` // always "state"
	Phase    string `json:"ph"`
	Words    []string `json:"words"`
	Revealed []bool   `json:"rev"`
	// Colours of REVEALED tiles, visible to everyone ("" while hidden).
	Shown []string `json:"shown"`
	// Key is the full 25-colour key — sent ONLY to spymasters; nil for guessers.
	Key []string `json:"key"`

	Turn     int      `json:"turn"`
	Clue     *cnClueInfo  `json:"clue"`
	Guesses  int      `json:"guesses"`
	RedLeft  int      `json:"redLeft"`
	BlueLeft int      `json:"blueLeft"`
	Winner   int      `json:"winner"`

	You    cnRosterEntry   `json:"you"`
	Roster []cnRosterEntry `json:"roster"`
}

func (g *codenamesGame) viewFor(seat int) cnView {
	v := cnView{
		T: MsgState, Phase: g.phase, Words: g.words, Revealed: g.revealed,
		Turn: g.turn, Clue: g.clue, Guesses: g.guesses, Winner: g.winner,
	}
	if len(g.key) == cnTiles {
		v.RedLeft, v.BlueLeft = g.agentsLeft(0), g.agentsLeft(1)
		v.Shown = make([]string, cnTiles)
		for i := range g.key {
			if g.revealed[i] {
				v.Shown[i] = g.key[i]
			}
		}
	}

	me := g.players[seat]
	if me != nil {
		v.You = cnRosterEntry{S: seat, N: me.name, Team: me.team, Spymaster: me.spymaster}
		// The private view: only a spymaster sees the key.
		if me.spymaster && len(g.key) == cnTiles {
			v.Key = g.key
		}
	}

	connected := map[int]bool{}
	for _, s := range g.out.Seats() {
		connected[s] = true
	}
	for s, p := range g.players {
		v.Roster = append(v.Roster, cnRosterEntry{
			S: s, N: p.name, Team: p.team, Spymaster: p.spymaster, Gone: !connected[s],
		})
	}
	return v
}

// broadcastViews pushes every connected seat its own view (spymasters with the
// key, guessers without).
func (g *codenamesGame) broadcastViews() {
	for _, seat := range g.out.Seats() {
		g.out.Send(seat, g.viewFor(seat))
	}
}

// teamColour maps a team index to its agent colour.
func teamColour(team int) string {
	if team == 1 {
		return cnBlueCol
	}
	return cnRedCol
}
