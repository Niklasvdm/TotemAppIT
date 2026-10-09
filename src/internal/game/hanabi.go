package game

import (
	"encoding/json"
	"math/rand/v2"
	"time"
)

// hanabiGame is Hanabi: a co-op for 2–5 where you see everyone's hand EXCEPT
// your own. The team builds five colour stacks from 1 up to 5 by giving hints,
// discarding and playing — spending from 8 hint tokens and guarding 3 fuses.
//
// The private view is the game's whole twist, inverted from every other game
// here: a seat is sent the FACES of the other players' cards but only what hints
// have revealed about its own (Outbox.Send). A hint marks every matching card in
// the target's hand (positive info) and every non-matching card as "not that"
// (negative info), which is how real Hanabi knowledge accrues.
//
// The 50-card deck is five colours, each with values 1,1,1,2,2,3,3,4,4,5.
// Event-driven (TickHz 0). Rules from the standard edition; the final score is
// the sum of the stack tops (25 is a perfect game).

const (
	hanMinPlayers = 2
	hanMaxPlayers = 5
	hanColors     = 5
	hanMaxHints   = 8
	hanFuses      = 3
)

const (
	hanLobby = "lobby"
	hanPlay  = "play"
	hanOver  = "over"
)

const (
	hanMsgStart   = "start"
	hanMsgPlay    = "play"    // {card} — hand index
	hanMsgDiscard = "discard" // {card} — hand index
	hanMsgHint    = "hint"    // {target, kind:"color"|"value", value}
	hanMsgRestart = "restart"
)

// hanDealValues is the multiset of values for one colour.
var hanDealValues = []int{1, 1, 1, 2, 2, 3, 3, 4, 4, 5}

type hanCard struct {
	color int // 0..4
	value int // 1..5
	// what this card's HOLDER has been told about it:
	knownColor bool
	knownValue bool
	notColor   [hanColors]bool
	notValue   [6]bool // indexed 1..5
}

type hanPlayer struct {
	name string
	hand []*hanCard
}

type hanabiGame struct {
	out Outbox
	rng *rand.Rand

	players map[int]*hanPlayer
	order   []int

	phase    string
	turn     int
	deck     []*hanCard
	stacks   [hanColors]int // top value per colour, 0 = empty
	discard  []*hanCard
	hints    int
	fuses    int
	handSize int

	lastRound     bool // deck has emptied; everyone gets one final turn
	lastTurnsLeft int
	win           bool // perfect 25
	lost          bool // all fuses gone
}

func newHanabiGame(out Outbox, seed uint64) Game {
	return &hanabiGame{
		out:     out,
		rng:     rand.New(rand.NewPCG(seed, 0x510e527fade682d1)),
		players: map[int]*hanPlayer{},
		phase:   hanLobby,
	}
}

// --- Game interface ---------------------------------------------------------

func (g *hanabiGame) AddPlayer(name, meta string) (int, bool) {
	seat := -1
	for s := 0; s < hanMaxPlayers; s++ {
		if _, taken := g.players[s]; !taken {
			seat = s
			break
		}
	}
	if seat == -1 {
		return 0, false
	}
	g.players[seat] = &hanPlayer{name: name}
	g.order = append(g.order, seat)
	return seat, true
}

func (g *hanabiGame) RemovePlayer(seat int) {
	delete(g.players, seat)
	for i, s := range g.order {
		if s == seat {
			g.order = append(g.order[:i], g.order[i+1:]...)
			break
		}
	}
	g.broadcastViews()
}

func (g *hanabiGame) Roster() []RosterEntry {
	out := make([]RosterEntry, 0, len(g.players))
	for s, p := range g.players {
		out = append(out, RosterEntry{S: s, N: p.name})
	}
	return out
}

func (g *hanabiGame) WelcomeExtra() WelcomeExtra { return WelcomeExtra{} }
func (g *hanabiGame) Joined(int)                 { g.broadcastViews() }
func (g *hanabiGame) Dropped(int)                { g.broadcastViews() }
func (g *hanabiGame) ToLobby()                   { g.resetToLobby(); g.broadcastViews() }
func (g *hanabiGame) TickHz() int                { return 0 }
func (g *hanabiGame) Tick(time.Time)             {}

func (g *hanabiGame) Command(seat int, isHost bool, raw []byte) {
	var c struct {
		T      string `json:"t"`
		Card   int    `json:"card"`
		Target int    `json:"target"`
		Kind   string `json:"kind"`
		Value  int    `json:"value"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return
	}
	if _, ok := g.players[seat]; !ok {
		return
	}

	switch c.T {
	case hanMsgStart:
		if !isHost || g.phase != hanLobby {
			return
		}
		if n := len(g.players); n < hanMinPlayers || n > hanMaxPlayers {
			return
		}
		g.deal()

	case hanMsgPlay:
		if g.phase != hanPlay || seat != g.turn {
			return
		}
		g.playCard(seat, c.Card)

	case hanMsgDiscard:
		if g.phase != hanPlay || seat != g.turn || g.hints >= hanMaxHints {
			return // can't discard with a full set of hint tokens
		}
		g.discardCard(seat, c.Card)

	case hanMsgHint:
		if g.phase != hanPlay || seat != g.turn {
			return
		}
		g.giveHint(seat, c.Target, c.Kind, c.Value)

	case hanMsgRestart:
		if !isHost || g.phase != hanOver {
			return
		}
		g.resetToLobby()

	default:
		return
	}
	g.broadcastViews()
}

// --- rules ------------------------------------------------------------------

func (g *hanabiGame) deal() {
	g.deck = nil
	for col := 0; col < hanColors; col++ {
		for _, v := range hanDealValues {
			g.deck = append(g.deck, &hanCard{color: col, value: v})
		}
	}
	g.rng.Shuffle(len(g.deck), func(i, j int) { g.deck[i], g.deck[j] = g.deck[j], g.deck[i] })

	g.handSize = 5
	if len(g.players) >= 4 {
		g.handSize = 4
	}
	for _, s := range g.order {
		g.players[s].hand = nil
		for i := 0; i < g.handSize; i++ {
			g.players[s].hand = append(g.players[s].hand, g.drawCard())
		}
	}
	for i := range g.stacks {
		g.stacks[i] = 0
	}
	g.discard = nil
	g.hints = hanMaxHints
	g.fuses = hanFuses
	g.lastRound = false
	g.lastTurnsLeft = 0
	g.win = false
	g.lost = false
	g.turn = g.order[g.rng.IntN(len(g.order))]
	g.phase = hanPlay
}

func (g *hanabiGame) drawCard() *hanCard {
	if len(g.deck) == 0 {
		return nil
	}
	c := g.deck[len(g.deck)-1]
	g.deck = g.deck[:len(g.deck)-1]
	return c
}

// take removes the hand card at idx and returns it, or nil if idx is invalid.
func (g *hanabiGame) take(seat, idx int) *hanCard {
	p := g.players[seat]
	if idx < 0 || idx >= len(p.hand) {
		return nil
	}
	c := p.hand[idx]
	p.hand = append(p.hand[:idx], p.hand[idx+1:]...)
	return c
}

func (g *hanabiGame) refill(seat int) {
	if c := g.drawCard(); c != nil {
		g.players[seat].hand = append(g.players[seat].hand, c)
	}
	if len(g.deck) == 0 && !g.lastRound {
		g.lastRound = true
		g.lastTurnsLeft = len(g.order) // one final turn each
	}
}

func (g *hanabiGame) playCard(seat, idx int) {
	c := g.take(seat, idx)
	if c == nil {
		return
	}
	if g.stacks[c.color] == c.value-1 {
		g.stacks[c.color] = c.value
		if c.value == 5 && g.hints < hanMaxHints {
			g.hints++ // completing a stack returns a hint token
		}
	} else {
		g.fuses--
		g.discard = append(g.discard, c)
	}
	g.refill(seat)
	g.advance()
}

func (g *hanabiGame) discardCard(seat, idx int) {
	c := g.take(seat, idx)
	if c == nil {
		return
	}
	g.discard = append(g.discard, c)
	if g.hints < hanMaxHints {
		g.hints++
	}
	g.refill(seat)
	g.advance()
}

func (g *hanabiGame) giveHint(seat, target int, kind string, value int) {
	if g.hints < 1 || target == seat {
		return
	}
	tp, ok := g.players[target]
	if !ok {
		return
	}
	matches := 0
	switch kind {
	case "color":
		if value < 0 || value >= hanColors {
			return
		}
		for _, c := range tp.hand {
			if c.color == value {
				matches++
			}
		}
	case "value":
		if value < 1 || value > 5 {
			return
		}
		for _, c := range tp.hand {
			if c.value == value {
				matches++
			}
		}
	default:
		return
	}
	if matches == 0 {
		return // a hint must touch at least one card
	}

	for _, c := range tp.hand {
		if kind == "color" {
			if c.color == value {
				c.knownColor = true
			} else {
				c.notColor[value] = true
			}
		} else {
			if c.value == value {
				c.knownValue = true
			} else {
				c.notValue[value] = true
			}
		}
	}
	g.hints--
	g.advance()
}

func (g *hanabiGame) advance() {
	if g.fuses <= 0 {
		g.lost = true
		g.phase = hanOver
		return
	}
	if g.allComplete() {
		g.win = true
		g.phase = hanOver
		return
	}
	if g.lastRound {
		g.lastTurnsLeft--
		if g.lastTurnsLeft <= 0 {
			g.phase = hanOver
			return
		}
	}
	fi := indexOf(g.order, g.turn)
	g.turn = g.order[(fi+1)%len(g.order)]
}

func (g *hanabiGame) allComplete() bool {
	for _, v := range g.stacks {
		if v != 5 {
			return false
		}
	}
	return true
}

func (g *hanabiGame) score() int {
	s := 0
	for _, v := range g.stacks {
		s += v
	}
	return s
}

func (g *hanabiGame) resetToLobby() {
	g.phase = hanLobby
	for _, p := range g.players {
		p.hand = nil
	}
	g.deck = nil
	g.discard = nil
	for i := range g.stacks {
		g.stacks[i] = 0
	}
	g.hints = hanMaxHints
	g.fuses = hanFuses
	g.lastRound = false
	g.win = false
	g.lost = false
}

// --- per-seat views ---------------------------------------------------------

type hanCardView struct {
	Color    int   `json:"color"` // -1 when hidden from this viewer
	Value    int   `json:"value"` // 0 when hidden
	KC       bool  `json:"kc"`    // holder has been told the colour
	KV       bool  `json:"kv"`    // holder has been told the value
	NotColor []int `json:"nc"`
	NotValue []int `json:"nv"`
}

type hanHandView struct {
	S     int           `json:"s"`
	N     string        `json:"n"`
	Gone  bool          `json:"gone,omitempty"`
	Cards []hanCardView `json:"cards"`
}

type hanDiscardView struct {
	Color int `json:"color"`
	Value int `json:"value"`
}

type hanabiView struct {
	T         string           `json:"t"`
	Phase     string           `json:"ph"`
	Turn      int              `json:"turn"`
	You       int              `json:"you"`
	Hints     int              `json:"hints"`
	Fuses     int              `json:"fuses"`
	Deck      int              `json:"deck"`
	Stacks    []int            `json:"stacks"`
	Discard   []hanDiscardView `json:"discard"`
	LastRound bool             `json:"lastRound"`
	Score     int              `json:"score"`
	Win       bool             `json:"win"`
	Lost      bool             `json:"lost"`
	Hands     []hanHandView    `json:"hands"`
}

func cardView(c *hanCard, hidden bool) hanCardView {
	v := hanCardView{Color: -1, Value: 0, KC: c.knownColor, KV: c.knownValue, NotColor: []int{}, NotValue: []int{}}
	if hidden {
		// Your own card: show a face only where a hint has pinned it down.
		if c.knownColor {
			v.Color = c.color
		}
		if c.knownValue {
			v.Value = c.value
		}
	} else {
		v.Color, v.Value = c.color, c.value
	}
	for col := 0; col < hanColors; col++ {
		if c.notColor[col] {
			v.NotColor = append(v.NotColor, col)
		}
	}
	for val := 1; val <= 5; val++ {
		if c.notValue[val] {
			v.NotValue = append(v.NotValue, val)
		}
	}
	return v
}

func (g *hanabiGame) viewFor(seat int) hanabiView {
	v := hanabiView{
		T: MsgState, Phase: g.phase, Turn: g.turn, You: seat,
		Hints: g.hints, Fuses: g.fuses, Deck: len(g.deck),
		Stacks: append([]int{}, g.stacks[:]...), Discard: []hanDiscardView{},
		LastRound: g.lastRound, Score: g.score(), Win: g.win, Lost: g.lost,
	}
	for _, c := range g.discard {
		v.Discard = append(v.Discard, hanDiscardView{Color: c.color, Value: c.value})
	}
	connected := map[int]bool{}
	for _, s := range g.out.Seats() {
		connected[s] = true
	}
	for _, s := range g.order {
		p := g.players[s]
		hv := hanHandView{S: s, N: p.name, Gone: !connected[s], Cards: []hanCardView{}}
		for _, c := range p.hand {
			hv.Cards = append(hv.Cards, cardView(c, s == seat))
		}
		v.Hands = append(v.Hands, hv)
	}
	return v
}

func (g *hanabiGame) broadcastViews() {
	for _, seat := range g.out.Seats() {
		g.out.Send(seat, g.viewFor(seat))
	}
}
