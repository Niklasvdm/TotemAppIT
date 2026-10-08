package game

import (
	"encoding/json"
	"math/rand/v2"
	"sort"
	"time"
)

// theMindGame is the co-op card game The Mind as a Game: players hold cards from
// a 1..100 deck and must play them onto a shared pile in ascending order WITHOUT
// communicating; playing out of order costs a life. It is event-driven (TickHz
// 0) — the "timing" tension is felt on the client, the server just validates
// plays. Rules follow the base game (deck 1..100, level N deals N cards each,
// a mistake discards every lower card and loses a life, a throwing star has
// everyone discard their lowest; win at level 12/10/8 for 2/3/4 players).
//
// FOUNDATION: the core loop (deal, play, mistake, star, level/lives, win/lose)
// is here; bonus life/star rewards at milestone levels are a TODO. Like
// Codenames, each seat's PRIVATE view shows only its own hand.

const (
	mindMaxPlayers = 4
	mindMinPlayers = 2
	mindDeckSize   = 100
)

const (
	mindLobby   = "lobby"
	mindPlaying = "playing"
	mindWon     = "won"
	mindLost    = "lost"
)

const (
	mindMsgStart   = "start"   // host: deal level 1 and begin
	mindMsgPlay    = "play"    // play YOUR lowest card onto the pile
	mindMsgStar    = "star"    // toggle your vote to use a throwing star
	mindMsgRestart = "restart" // host: back to the lobby
)

type mindPlayer struct {
	name   string
	avatar string
}

type theMindGame struct {
	out Outbox
	rng *rand.Rand

	players map[int]*mindPlayer
	order   []int // seat join order, stable for turn-free play

	phase    string
	level    int
	lives    int
	stars    int
	pile     int // highest card played so far (0 = none)
	hands    map[int][]int
	starVote map[int]bool
}

func newTheMindGame(out Outbox, seed uint64) Game {
	return &theMindGame{
		out:      out,
		rng:      rand.New(rand.NewPCG(seed, 0x243f6a8885a308d3)),
		players:  map[int]*mindPlayer{},
		phase:    mindLobby,
		hands:    map[int][]int{},
		starVote: map[int]bool{},
	}
}

// --- Game interface ---------------------------------------------------------

func (g *theMindGame) AddPlayer(name, meta string) (int, bool) {
	seat := -1
	for s := 0; s < mindMaxPlayers; s++ {
		if _, taken := g.players[s]; !taken {
			seat = s
			break
		}
	}
	if seat == -1 {
		return 0, false
	}
	g.players[seat] = &mindPlayer{name: name, avatar: meta}
	g.order = append(g.order, seat)
	return seat, true
}

func (g *theMindGame) RemovePlayer(seat int) {
	delete(g.players, seat)
	delete(g.hands, seat)
	delete(g.starVote, seat)
	for i, s := range g.order {
		if s == seat {
			g.order = append(g.order[:i], g.order[i+1:]...)
			break
		}
	}
	g.broadcastViews()
}

func (g *theMindGame) Roster() []RosterEntry {
	out := make([]RosterEntry, 0, len(g.players))
	for s, p := range g.players {
		out = append(out, RosterEntry{S: s, N: p.name, M: p.avatar})
	}
	return out
}

func (g *theMindGame) WelcomeExtra() WelcomeExtra { return WelcomeExtra{} }
func (g *theMindGame) Joined(seat int)            { g.out.Send(seat, g.viewFor(seat)) }
func (g *theMindGame) Dropped(int)                {}
func (g *theMindGame) ToLobby()                   { g.resetToLobby(); g.broadcastViews() }
func (g *theMindGame) TickHz() int                { return 0 }
func (g *theMindGame) Tick(time.Time)             {}

func (g *theMindGame) Command(seat int, isHost bool, raw []byte) {
	var c struct {
		T string `json:"t"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return
	}
	if _, ok := g.players[seat]; !ok {
		return
	}

	switch c.T {
	case mindMsgStart:
		if !isHost || g.phase != mindLobby {
			return
		}
		if n := len(g.players); n < mindMinPlayers || n > mindMaxPlayers {
			return
		}
		g.level = 1
		g.lives = len(g.players)
		g.stars = 1
		g.deal()
		g.phase = mindPlaying

	case mindMsgPlay:
		if g.phase != mindPlaying {
			return
		}
		g.play(seat)

	case mindMsgStar:
		if g.phase != mindPlaying || g.stars <= 0 {
			return
		}
		g.starVote[seat] = !g.starVote[seat]
		g.maybeStar()

	case mindMsgRestart:
		if !isHost || (g.phase != mindWon && g.phase != mindLost) {
			return
		}
		g.resetToLobby()

	default:
		return
	}
	g.broadcastViews()
}

// --- rules ------------------------------------------------------------------

func mindLevelsToWin(players int) int {
	switch players {
	case 2:
		return 12
	case 3:
		return 10
	default:
		return 8 // 4 players
	}
}

// deal draws `level` distinct cards for each player from 1..100, sorted.
func (g *theMindGame) deal() {
	deck := make([]int, mindDeckSize)
	for i := range deck {
		deck[i] = i + 1
	}
	g.rng.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	g.hands = map[int][]int{}
	g.starVote = map[int]bool{}
	g.pile = 0
	i := 0
	for _, seat := range g.order {
		h := append([]int(nil), deck[i:i+g.level]...)
		sort.Ints(h)
		g.hands[seat] = h
		i += g.level
	}
}

// play puts a seat's LOWEST card on the pile — the only sensible move, since any
// higher card is strictly worse. A card lower than it still in another hand is a
// mistake: a life is lost and every lower card is discarded from every hand.
func (g *theMindGame) play(seat int) {
	hand := g.hands[seat]
	if len(hand) == 0 {
		return
	}
	card := hand[0] // sorted
	g.hands[seat] = hand[1:]
	g.pile = card

	mistake := false
	for s, h := range g.hands {
		if s != seat && len(h) > 0 && h[0] < card {
			mistake = true
			break
		}
	}
	if mistake {
		g.lives--
		for s, h := range g.hands {
			k := 0
			for k < len(h) && h[k] < card {
				k++
			}
			g.hands[s] = h[k:]
		}
		if g.lives <= 0 {
			g.phase = mindLost
			return
		}
	}
	g.checkLevelDone()
}

// maybeStar fires a throwing star once every seated player has voted: each
// player discards their lowest card simultaneously.
func (g *theMindGame) maybeStar() {
	if g.stars <= 0 {
		return
	}
	for s := range g.players {
		if !g.starVote[s] {
			return // someone has not voted yet
		}
	}
	g.stars--
	for s, h := range g.hands {
		if len(h) > 0 {
			if h[0] > g.pile {
				g.pile = h[0]
			}
			g.hands[s] = h[1:]
		}
	}
	g.starVote = map[int]bool{}
	g.checkLevelDone()
}

func (g *theMindGame) checkLevelDone() {
	for _, h := range g.hands {
		if len(h) > 0 {
			return
		}
	}
	if g.level >= mindLevelsToWin(len(g.players)) {
		g.phase = mindWon
		return
	}
	g.level++
	g.deal()
}

func (g *theMindGame) resetToLobby() {
	g.phase = mindLobby
	g.level, g.lives, g.stars, g.pile = 0, 0, 0, 0
	g.hands = map[int][]int{}
	g.starVote = map[int]bool{}
}

// --- per-seat views ---------------------------------------------------------

type mindRosterEntry struct {
	S     int    `json:"s"`
	N     string `json:"n"`
	Cards int    `json:"cards"`
	Voted bool   `json:"voted"`
	Gone  bool   `json:"gone,omitempty"`
}

type mindView struct {
	T           string            `json:"t"` // "state"
	Phase       string            `json:"ph"`
	Level       int               `json:"level"`
	Lives       int               `json:"lives"`
	Stars       int               `json:"stars"`
	Pile        int               `json:"pile"`
	LevelsToWin int               `json:"levelsToWin"`
	Hand        []int             `json:"hand"` // YOUR cards only (private)
	You         int               `json:"you"`
	Roster      []mindRosterEntry `json:"roster"`
}

func (g *theMindGame) viewFor(seat int) mindView {
	v := mindView{
		T: MsgState, Phase: g.phase, Level: g.level, Lives: g.lives, Stars: g.stars,
		Pile: g.pile, LevelsToWin: mindLevelsToWin(len(g.players)), You: seat,
		Hand: append([]int(nil), g.hands[seat]...),
	}
	connected := map[int]bool{}
	for _, s := range g.out.Seats() {
		connected[s] = true
	}
	for s, p := range g.players {
		v.Roster = append(v.Roster, mindRosterEntry{
			S: s, N: p.name, Cards: len(g.hands[s]), Voted: g.starVote[s], Gone: !connected[s],
		})
	}
	return v
}

func (g *theMindGame) broadcastViews() {
	for _, seat := range g.out.Seats() {
		g.out.Send(seat, g.viewFor(seat))
	}
}
