package game

import (
	"encoding/json"
	"math/rand/v2"
	"sort"
	"time"
)

// theGameGame is the co-op card game "The Game" (Steffen Benndorf): a 2..99 deck
// onto four piles — two ascending from 1, two descending from 100. On your turn
// you must play at least two cards (one once the draw deck is empty), following
// each pile's direction, except the "backwards trick": an ascending pile also
// takes a card exactly 10 lower, a descending pile one exactly 10 higher. Empty
// the deck and every hand to win; if a player can't make their minimum, all lose.
// Event-driven (TickHz 0). Private hands via Outbox.Send. Rules from the base game.
//
// FOUNDATION note: this is the full core; the only simplification is that the
// "no naming numbers" talking rule is social, not enforced.

const (
	tgMinCard    = 2
	tgMaxCard    = 99
	tgPiles      = 4 // 0,1 ascending (from 1); 2,3 descending (from 100)
	tgMaxPlayers = 5
)

const (
	tgLobby   = "lobby"
	tgPlaying = "playing"
	tgWon     = "won"
	tgLost    = "lost"
)

const (
	tgMsgStart   = "start"
	tgMsgPlay    = "play"    // {card, pile}
	tgMsgEndTurn = "endturn" // finish your turn once the minimum is met
	tgMsgRestart = "restart"
)

type tgPlayer struct {
	name   string
	avatar string
}

type theGameGame struct {
	out Outbox
	rng *rand.Rand

	players map[int]*tgPlayer
	order   []int

	phase   string
	piles   [tgPiles]int    // current top of each pile
	history [tgPiles][]int  // every card played on each pile, for display
	deck    []int           // draw pile
	hands   map[int][]int
	turn    int // seat whose turn it is
	played  int // cards played so far this turn
}

func newTheGameGame(out Outbox, seed uint64) Game {
	return &theGameGame{
		out:     out,
		rng:     rand.New(rand.NewPCG(seed, 0x243f6a8885a308d3)),
		players: map[int]*tgPlayer{},
		phase:   tgLobby,
		hands:   map[int][]int{},
	}
}

// --- Game interface ---------------------------------------------------------

func (g *theGameGame) AddPlayer(name, meta string) (int, bool) {
	seat := -1
	for s := 0; s < tgMaxPlayers; s++ {
		if _, taken := g.players[s]; !taken {
			seat = s
			break
		}
	}
	if seat == -1 {
		return 0, false
	}
	g.players[seat] = &tgPlayer{name: name, avatar: meta}
	g.order = append(g.order, seat)
	return seat, true
}

func (g *theGameGame) RemovePlayer(seat int) {
	delete(g.players, seat)
	delete(g.hands, seat)
	for i, s := range g.order {
		if s == seat {
			g.order = append(g.order[:i], g.order[i+1:]...)
			break
		}
	}
	g.broadcastViews()
}

func (g *theGameGame) Roster() []RosterEntry {
	out := make([]RosterEntry, 0, len(g.players))
	for s, p := range g.players {
		out = append(out, RosterEntry{S: s, N: p.name, M: p.avatar})
	}
	return out
}

func (g *theGameGame) WelcomeExtra() WelcomeExtra { return WelcomeExtra{} }

// Joined/Dropped refresh every connected seat (no tick to do it otherwise).
func (g *theGameGame) Joined(int)  { g.broadcastViews() }
func (g *theGameGame) Dropped(int) { g.broadcastViews() }
func (g *theGameGame) ToLobby()    { g.resetToLobby(); g.broadcastViews() }
func (g *theGameGame) TickHz() int { return 0 }
func (g *theGameGame) Tick(time.Time) {}

func (g *theGameGame) Command(seat int, isHost bool, raw []byte) {
	var c struct {
		T    string `json:"t"`
		Card int    `json:"card"`
		Pile int    `json:"pile"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return
	}
	if _, ok := g.players[seat]; !ok {
		return
	}

	switch c.T {
	case tgMsgStart:
		if !isHost || g.phase != tgLobby {
			return
		}
		if n := len(g.players); n < 1 || n > tgMaxPlayers {
			return
		}
		g.deal()
		g.phase = tgPlaying

	case tgMsgPlay:
		if g.phase != tgPlaying || seat != g.turn {
			return
		}
		g.play(seat, c.Card, c.Pile)

	case tgMsgEndTurn:
		if g.phase != tgPlaying || seat != g.turn {
			return
		}
		g.endTurn(seat)

	case tgMsgRestart:
		if !isHost || (g.phase != tgWon && g.phase != tgLost) {
			return
		}
		g.resetToLobby()

	default:
		return
	}
	g.broadcastViews()
}

// --- rules ------------------------------------------------------------------

func tgHandSize(players int) int {
	switch players {
	case 1:
		return 8
	case 2:
		return 7
	default:
		return 6 // 3..5 players
	}
}

func (g *theGameGame) deal() {
	deck := make([]int, 0, tgMaxCard-tgMinCard+1)
	for n := tgMinCard; n <= tgMaxCard; n++ {
		deck = append(deck, n)
	}
	g.rng.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	g.piles = [tgPiles]int{1, 1, 100, 100}
	g.history = [tgPiles][]int{{1}, {1}, {100}, {100}}
	g.hands = map[int][]int{}
	hs := tgHandSize(len(g.players))
	i := 0
	for _, seat := range g.order {
		h := append([]int(nil), deck[i:i+hs]...)
		sort.Ints(h)
		g.hands[seat] = h
		i += hs
	}
	g.deck = deck[i:]
	g.turn = g.order[g.rng.IntN(len(g.order))] // a random player begins
	g.played = 0
}

// canPlay reports whether card may go on pile: an ascending pile takes a higher
// card or one exactly 10 lower; a descending pile a lower card or one exactly 10
// higher (the "backwards trick").
func (g *theGameGame) canPlay(card, pile int) bool {
	if pile < 0 || pile >= tgPiles {
		return false
	}
	top := g.piles[pile]
	if pile < 2 { // ascending
		return card > top || card == top-10
	}
	return card < top || card == top+10 // descending
}

func (g *theGameGame) play(seat, card, pile int) {
	h := g.hands[seat]
	idx := -1
	for i, c := range h {
		if c == card {
			idx = i
			break
		}
	}
	if idx == -1 || !g.canPlay(card, pile) {
		return
	}
	g.hands[seat] = append(h[:idx], h[idx+1:]...)
	g.piles[pile] = card
	g.history[pile] = append(g.history[pile], card)
	g.played++
	g.checkWin()
}

func (g *theGameGame) endTurn(seat int) {
	if g.played < g.minCards() {
		// Not done yet. If a legal move still exists they must keep playing; if
		// none does, they cannot make their minimum and everyone loses.
		if !g.hasValidMove(seat) {
			g.phase = tgLost
		}
		return
	}
	g.refill(seat)
	if g.checkWin() {
		return
	}
	g.turn = g.nextSeat(seat)
	g.played = 0
	// The next player is stuck before they start: no legal move at all.
	if len(g.hands[g.turn]) > 0 && !g.hasValidMove(g.turn) {
		g.phase = tgLost
	}
}

func (g *theGameGame) minCards() int {
	if len(g.deck) == 0 {
		return 1
	}
	return 2
}

func (g *theGameGame) hasValidMove(seat int) bool {
	for _, c := range g.hands[seat] {
		for p := 0; p < tgPiles; p++ {
			if g.canPlay(c, p) {
				return true
			}
		}
	}
	return false
}

func (g *theGameGame) refill(seat int) {
	hs := tgHandSize(len(g.players))
	for len(g.hands[seat]) < hs && len(g.deck) > 0 {
		g.hands[seat] = append(g.hands[seat], g.deck[0])
		g.deck = g.deck[1:]
	}
	sort.Ints(g.hands[seat])
}

func (g *theGameGame) nextSeat(seat int) int {
	for i, s := range g.order {
		if s == seat {
			return g.order[(i+1)%len(g.order)]
		}
	}
	return seat
}

func (g *theGameGame) checkWin() bool {
	if len(g.deck) > 0 {
		return false
	}
	for _, h := range g.hands {
		if len(h) > 0 {
			return false
		}
	}
	g.phase = tgWon
	return true
}

func (g *theGameGame) resetToLobby() {
	g.phase = tgLobby
	g.piles = [tgPiles]int{}
	g.history = [tgPiles][]int{}
	g.deck = nil
	g.hands = map[int][]int{}
	g.played = 0
	g.turn = 0
}

// --- per-seat views ---------------------------------------------------------

type tgRosterEntry struct {
	S     int    `json:"s"`
	N     string `json:"n"`
	Cards int    `json:"cards"`
	Gone  bool   `json:"gone,omitempty"`
}

type theGameView struct {
	T        string          `json:"t"` // "state"
	Phase    string          `json:"ph"`
	Piles    [tgPiles]int    `json:"piles"`
	History  [tgPiles][]int  `json:"history"` // every card played per pile, for display
	Deck     int             `json:"deck"`    // cards left in the draw pile
	Hand     []int           `json:"hand"` // YOUR cards (private)
	Played   int             `json:"played"`
	Min      int             `json:"min"`
	Turn     int             `json:"turn"`
	HandSize int             `json:"handSize"`
	You      int             `json:"you"`
	Roster   []tgRosterEntry `json:"roster"`
}

func (g *theGameGame) viewFor(seat int) theGameView {
	v := theGameView{
		T: MsgState, Phase: g.phase, Piles: g.piles, History: g.history, Deck: len(g.deck),
		Played: g.played, Min: g.minCards(), Turn: g.turn,
		HandSize: tgHandSize(len(g.players)), You: seat,
		Hand: append([]int(nil), g.hands[seat]...),
	}
	connected := map[int]bool{}
	for _, s := range g.out.Seats() {
		connected[s] = true
	}
	for s, p := range g.players {
		v.Roster = append(v.Roster, tgRosterEntry{
			S: s, N: p.name, Cards: len(g.hands[s]), Gone: !connected[s],
		})
	}
	return v
}

func (g *theGameGame) broadcastViews() {
	for _, seat := range g.out.Seats() {
		g.out.Send(seat, g.viewFor(seat))
	}
}
