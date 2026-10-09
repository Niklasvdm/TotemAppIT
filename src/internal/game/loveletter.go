package game

import (
	"encoding/json"
	"math/rand/v2"
	"time"
)

// loveLetterGame is Love Letter: a quick deduction game for 2–4. Each player
// holds ONE secret card; on your turn you draw a second and play one, resolving
// its effect. Get knocked out and you're out for the round; the last player in,
// or the highest card when the deck runs dry, wins the round and a token. First
// to enough tokens wins the game. Event-driven (TickHz 0).
//
// The private view is the heart of it: each seat sees only its own hand
// (Outbox.Send). A Priest reveal is sent privately to the one who played it.
//
// The 16-card deck (value → count):
//
//	1 Guard ×5   2 Priest ×2   3 Baron ×2   4 Handmaid ×2
//	5 Prince ×2  6 King ×1     7 Countess ×1 8 Princess ×1
//
// Rules from the standard edition. Tiebreak at a drained deck is by highest card
// then by discard-pile sum, as in the box.

const (
	llGuard    = 1
	llPriest   = 2
	llBaron    = 3
	llHandmaid = 4
	llPrince   = 5
	llKing     = 6
	llCountess = 7
	llPrincess = 8
)

const (
	llMinPlayers = 2
	llMaxPlayers = 4
)

const (
	llLobby    = "lobby"
	llPlay     = "play"     // active seat draws then plays one card
	llRoundEnd = "roundEnd" // a round was won; host starts the next
	llOver     = "over"     // someone reached the token goal
)

const (
	llMsgStart   = "start"
	llMsgPlay    = "play" // {card, target, guess}
	llMsgNext    = "next" // host — next round
	llMsgRestart = "restart"
)

// llFullDeck is the 16-card deck by value; shuffled each round.
var llFullDeck = []int{
	llGuard, llGuard, llGuard, llGuard, llGuard,
	llPriest, llPriest,
	llBaron, llBaron,
	llHandmaid, llHandmaid,
	llPrince, llPrince,
	llKing,
	llCountess,
	llPrincess,
}

type llPlayer struct {
	name      string
	hand      []int // 0, 1 or 2 card values; only this seat sees it
	discard   []int // cards played/revealed this round, public
	out       bool
	protected bool // Handmaid: untargetable until this seat's next turn
	tokens    int
}

type llSaw struct {
	Target int `json:"target"`
	Card   int `json:"card"`
}

type loveLetterGame struct {
	out Outbox
	rng *rand.Rand

	players map[int]*llPlayer
	order   []int

	phase  string
	active int // seat whose turn it is

	deck   []int
	burned int        // set-aside card, hidden all round
	aside  []int      // 2-player face-up cards, public
	saw    map[int]llSaw // seat -> what a Priest showed them this round
	logs   []string   // short public round log

	winner     int // round winner seat, -1 if none yet
	gameWinner int // -1 until someone hits the token goal
}

func newLoveLetterGame(out Outbox, seed uint64) Game {
	return &loveLetterGame{
		out:        out,
		rng:        rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15)),
		players:    map[int]*llPlayer{},
		phase:      llLobby,
		saw:        map[int]llSaw{},
		burned:     -1,
		winner:     -1,
		gameWinner: -1,
	}
}

// --- Game interface ---------------------------------------------------------

func (g *loveLetterGame) AddPlayer(name, meta string) (int, bool) {
	seat := -1
	for s := 0; s < llMaxPlayers; s++ {
		if _, taken := g.players[s]; !taken {
			seat = s
			break
		}
	}
	if seat == -1 {
		return 0, false
	}
	g.players[seat] = &llPlayer{name: name}
	g.order = append(g.order, seat)
	return seat, true
}

func (g *loveLetterGame) RemovePlayer(seat int) {
	delete(g.players, seat)
	delete(g.saw, seat)
	for i, s := range g.order {
		if s == seat {
			g.order = append(g.order[:i], g.order[i+1:]...)
			break
		}
	}
	g.broadcastViews()
}

func (g *loveLetterGame) Roster() []RosterEntry {
	out := make([]RosterEntry, 0, len(g.players))
	for s, p := range g.players {
		out = append(out, RosterEntry{S: s, N: p.name})
	}
	return out
}

func (g *loveLetterGame) WelcomeExtra() WelcomeExtra { return WelcomeExtra{} }
func (g *loveLetterGame) Joined(int)                 { g.broadcastViews() }
func (g *loveLetterGame) Dropped(int)                { g.broadcastViews() }
func (g *loveLetterGame) ToLobby()                   { g.resetToLobby(); g.broadcastViews() }
func (g *loveLetterGame) TickHz() int                { return 0 }
func (g *loveLetterGame) Tick(time.Time)             {}

func (g *loveLetterGame) Command(seat int, isHost bool, raw []byte) {
	var c struct {
		T      string `json:"t"`
		Card   int    `json:"card"`
		Target int    `json:"target"`
		Guess  int    `json:"guess"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return
	}
	if _, ok := g.players[seat]; !ok {
		return
	}

	switch c.T {
	case llMsgStart:
		if !isHost || g.phase != llLobby {
			return
		}
		if n := len(g.players); n < llMinPlayers || n > llMaxPlayers {
			return
		}
		for _, p := range g.players {
			p.tokens = 0
		}
		g.gameWinner = -1
		g.newRound(g.order[0])

	case llMsgPlay:
		if g.phase != llPlay || seat != g.active {
			return
		}
		g.playCard(seat, c.Card, c.Target, c.Guess)

	case llMsgNext:
		if !isHost || g.phase != llRoundEnd {
			return
		}
		start := g.winner
		if _, ok := g.players[start]; !ok {
			start = g.order[0]
		}
		g.newRound(start)

	case llMsgRestart:
		if !isHost || g.phase != llOver {
			return
		}
		g.resetToLobby()

	default:
		return
	}
	g.broadcastViews()
}

// --- rules ------------------------------------------------------------------

func (g *loveLetterGame) tokensToWin() int {
	switch len(g.players) {
	case 2:
		return 7
	case 3:
		return 5
	default:
		return 4
	}
}

func (g *loveLetterGame) newRound(starter int) {
	g.deck = append([]int{}, llFullDeck...)
	g.rng.Shuffle(len(g.deck), func(i, j int) { g.deck[i], g.deck[j] = g.deck[j], g.deck[i] })
	g.burned = g.drawDeck()
	g.aside = nil
	if len(g.players) == 2 {
		for i := 0; i < 3; i++ {
			g.aside = append(g.aside, g.drawDeck())
		}
	}
	g.saw = map[int]llSaw{}
	g.logs = nil
	g.winner = -1

	for _, s := range g.order {
		p := g.players[s]
		p.out = false
		p.protected = false
		p.hand = []int{g.drawDeck()}
		p.discard = nil
	}
	g.active = starter
	g.players[starter].protected = false
	g.players[starter].hand = append(g.players[starter].hand, g.drawDeck())
	g.phase = llPlay
}

func (g *loveLetterGame) drawDeck() int {
	n := len(g.deck)
	c := g.deck[n-1]
	g.deck = g.deck[:n-1]
	return c
}

// drawFor refills a seat to one card; falls back to the set-aside card when the
// deck is empty (Prince on the very last turn).
func (g *loveLetterGame) drawFor(seat int) {
	var c int
	if len(g.deck) > 0 {
		c = g.drawDeck()
	} else {
		c = g.burned
		g.burned = -1
	}
	g.players[seat].hand = append(g.players[seat].hand, c)
}

func (g *loveLetterGame) canTarget(seat int) bool {
	p, ok := g.players[seat]
	return ok && !p.out && !p.protected
}

func (g *loveLetterGame) log(s string) { g.logs = append(g.logs, s) }

func (g *loveLetterGame) name(seat int) string {
	if p, ok := g.players[seat]; ok {
		return p.name
	}
	return "?"
}

var llCardNames = map[int]string{
	llGuard: "Guard", llPriest: "Priest", llBaron: "Baron", llHandmaid: "Handmaid",
	llPrince: "Prince", llKing: "King", llCountess: "Countess", llPrincess: "Princess",
}

func (g *loveLetterGame) eliminate(seat int) {
	p := g.players[seat]
	if p.out {
		return
	}
	p.out = true
	p.discard = append(p.discard, p.hand...)
	p.hand = nil
	g.log(g.name(seat) + " is out")
}

// playCard removes card from active's hand, resolves its effect, then advances.
func (g *loveLetterGame) playCard(seat, card, target, guess int) {
	p := g.players[seat]
	idx := indexOf(p.hand, card)
	if idx == -1 {
		return
	}
	// Countess must be played if you also hold the King or the Prince.
	if card != llCountess && contains(p.hand, llCountess) && (contains(p.hand, llKing) || contains(p.hand, llPrince)) {
		return
	}

	p.hand = append(p.hand[:idx], p.hand[idx+1:]...)
	p.discard = append(p.discard, card)
	g.log(g.name(seat) + " played " + llCardNames[card])

	directedValid := target != seat && g.canTarget(target)

	switch card {
	case llGuard:
		if directedValid && guess >= llPriest && guess <= llPrincess {
			if g.players[target].hand[0] == guess {
				g.log(g.name(seat) + " guessed " + llCardNames[guess] + " on " + g.name(target) + " — correct")
				g.eliminate(target)
			} else {
				g.log(g.name(seat) + " guessed " + llCardNames[guess] + " on " + g.name(target) + " — wrong")
			}
		}
	case llPriest:
		if directedValid {
			g.saw[seat] = llSaw{Target: target, Card: g.players[target].hand[0]}
		}
	case llBaron:
		if directedValid {
			mine, theirs := p.hand[0], g.players[target].hand[0]
			if mine > theirs {
				g.eliminate(target)
			} else if theirs > mine {
				g.eliminate(seat)
			}
		}
	case llHandmaid:
		p.protected = true
	case llPrince:
		tgt := -1
		if target == seat || g.canTarget(target) {
			tgt = target
		}
		if tgt != -1 {
			tp := g.players[tgt]
			disc := tp.hand[0]
			tp.hand = tp.hand[:0]
			tp.discard = append(tp.discard, disc)
			g.log(g.name(tgt) + " discarded " + llCardNames[disc])
			if disc == llPrincess {
				tp.out = true
				g.log(g.name(tgt) + " is out")
			} else {
				g.drawFor(tgt)
			}
		}
	case llKing:
		if directedValid {
			tp := g.players[target]
			p.hand, tp.hand = tp.hand, p.hand
		}
	case llCountess:
		// no effect
	case llPrincess:
		g.eliminate(seat)
	}

	g.afterPlay()
}

func (g *loveLetterGame) aliveCount() int {
	n := 0
	for _, s := range g.order {
		if !g.players[s].out {
			n++
		}
	}
	return n
}

func (g *loveLetterGame) nextAlive(from int) int {
	fi := indexOf(g.order, from)
	for i := 1; i <= len(g.order); i++ {
		s := g.order[(fi+i)%len(g.order)]
		if !g.players[s].out {
			return s
		}
	}
	return from
}

func (g *loveLetterGame) afterPlay() {
	if g.aliveCount() <= 1 {
		g.endRound()
		return
	}
	if len(g.deck) == 0 {
		g.endRound()
		return
	}
	next := g.nextAlive(g.active)
	g.active = next
	g.players[next].protected = false
	g.players[next].hand = append(g.players[next].hand, g.drawDeck())
}

func (g *loveLetterGame) endRound() {
	win := -1
	bestCard, bestSum := -1, -1
	for _, s := range g.order {
		p := g.players[s]
		if p.out || len(p.hand) == 0 {
			continue
		}
		card := p.hand[0]
		sum := 0
		for _, d := range p.discard {
			sum += d
		}
		if card > bestCard || (card == bestCard && sum > bestSum) {
			bestCard, bestSum, win = card, sum, s
		}
	}
	if win == -1 { // everyone somehow out — pick first seat
		win = g.order[0]
	}
	g.winner = win
	g.players[win].tokens++
	g.log(g.name(win) + " wins the round")

	if g.players[win].tokens >= g.tokensToWin() {
		g.gameWinner = win
		g.phase = llOver
	} else {
		g.phase = llRoundEnd
	}
}

func (g *loveLetterGame) resetToLobby() {
	g.phase = llLobby
	g.winner = -1
	g.gameWinner = -1
	g.logs = nil
	g.saw = map[int]llSaw{}
	for _, p := range g.players {
		p.hand = nil
		p.discard = nil
		p.out = false
		p.protected = false
		p.tokens = 0
	}
}

// --- per-seat views ---------------------------------------------------------

type llPlayerView struct {
	S         int    `json:"s"`
	N         string `json:"n"`
	Tokens    int    `json:"tokens"`
	Out       bool   `json:"out"`
	Protected bool   `json:"protected"`
	Hand      int    `json:"hand"`             // card COUNT (face down to others)
	Reveal    []int  `json:"reveal,omitempty"` // shown at round end
	Discard   []int  `json:"discard"`
	Gone      bool   `json:"gone,omitempty"`
}

type loveLetterView struct {
	T          string          `json:"t"`
	Phase      string          `json:"ph"`
	Active     int             `json:"active"`
	You        int             `json:"you"`
	Hand       []int           `json:"yourHand"` // your own cards only
	Saw        *llSaw          `json:"saw,omitempty"`
	DeckLeft   int             `json:"deckLeft"`
	Aside      []int           `json:"aside"` // 2-player face-up cards (public)
	Need       int             `json:"need"`  // tokens to win the game
	Logs       []string        `json:"logs"`
	Winner     int             `json:"winner"`
	GameWinner int             `json:"gameWinner"`
	Players    []llPlayerView  `json:"players"`
}

func (g *loveLetterGame) viewFor(seat int) loveLetterView {
	revealed := g.phase == llRoundEnd || g.phase == llOver
	v := loveLetterView{
		T: MsgState, Phase: g.phase, Active: g.active, You: seat,
		Hand:     append([]int{}, g.players[seat].hand...),
		DeckLeft: len(g.deck), Aside: append([]int{}, g.aside...),
		Need: g.tokensToWin(), Logs: append([]string{}, g.logs...),
		Winner: g.winner, GameWinner: g.gameWinner,
	}
	if s, ok := g.saw[seat]; ok {
		sv := s
		v.Saw = &sv
	}
	connected := map[int]bool{}
	for _, s := range g.out.Seats() {
		connected[s] = true
	}
	for _, s := range g.order {
		p := g.players[s]
		pv := llPlayerView{
			S: s, N: p.name, Tokens: p.tokens, Out: p.out, Protected: p.protected,
			Hand: len(p.hand), Discard: append([]int{}, p.discard...), Gone: !connected[s],
		}
		if revealed && !p.out && len(p.hand) > 0 {
			pv.Reveal = append([]int{}, p.hand...)
		}
		v.Players = append(v.Players, pv)
	}
	return v
}

func (g *loveLetterGame) broadcastViews() {
	for _, seat := range g.out.Seats() {
		g.out.Send(seat, g.viewFor(seat))
	}
}

// --- small int-slice helpers ------------------------------------------------

func indexOf(xs []int, v int) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return -1
}

func contains(xs []int, v int) bool { return indexOf(xs, v) != -1 }
