package game

import (
	"log/slog"
	"math/rand/v2"
	"time"
)

// skyjoGame is Skyjo: a push-your-luck card game for 2-8. Each player has a face-down
// 3x4 grid (12 cards, values -2..12); you lower your grid's total by swapping cards
// against the draw or discard pile and by clearing columns of three equal cards. When
// one player turns their whole grid face-up the round ends after one more turn each,
// everyone's grid is revealed and scored, and the game ends once someone passes 100
// points. Lowest total wins. Event-driven (TickHz 0).
//
// The board is almost fully public (every face-up card is visible to all, every
// face-down card is hidden from everyone, including its owner). The one private thing
// is the card the ACTIVE player has drawn from the deck and is deciding on: it is sent
// only to that seat (Outbox.Send). Rules from the published edition; the closer-penalty
// doubling follows the common reading (doubled only when positive and not strictly
// lowest).
//
// The 150-card deck (value -> count): -2 x5, -1 x10, 0 x15, 1..12 x10 each.

const (
	skMinPlayers = 2
	skMaxPlayers = 8
	skCols       = 4
	skRows       = 3
	skCells      = skCols * skRows // 12
	skEndScore   = 100             // game ends once a total reaches this
	skNone       = -100            // "no card in hand" sentinel for drawn
)

const (
	skLobby    = "lobby"
	skSetup    = "setup"    // every player flips 2 of their own cards
	skPlay     = "play"     // active seat draws then resolves
	skRoundEnd = "roundEnd" // a round scored; host starts the next
	skOver     = "over"     // someone passed skEndScore
)

const (
	skMsgStart       = "start"
	skMsgDraw        = "draw"        // take the top of the draw pile into hand
	skMsgTakeDiscard = "takeDiscard" // take the top discard into hand (must then swap)
	skMsgSwap        = "swap"        // {cell} put the held card into a grid cell
	skMsgDiscardDraw = "discardDraw" // discard the held (deck-drawn) card, then flip
	skMsgFlip        = "flip"        // {cell} reveal a face-down card
	skMsgNext        = "next"        // host: deal the next round
	skMsgRestart     = "restart"
)

// skDeck is the full 150-card multiset, built once and copied+shuffled each round.
var skDeck = buildSkyjoDeck()

func buildSkyjoDeck() []int {
	d := make([]int, 0, 150)
	add := func(v, n int) {
		for i := 0; i < n; i++ {
			d = append(d, v)
		}
	}
	add(-2, 5)
	add(-1, 10)
	add(0, 15)
	for v := 1; v <= 12; v++ {
		add(v, 10)
	}
	return d
}

type skyjoCell struct {
	value int
	up    bool // face-up (its value is public)
	gone  bool // removed as part of a cleared column (scores 0)
}

type skyjoPlayer struct {
	name  string
	grid  [skCells]skyjoCell
	score int // running total across rounds
}

type skyjoGame struct {
	out Outbox
	rng *rand.Rand

	players map[int]*skyjoPlayer
	order   []int

	phase string
	turn  int

	deck    []int
	discard []int // top is the last element

	drawn    int  // card held by the active player this turn, or skNone
	fromDeck bool // the held card came from the deck (so it may be discarded)
	mustFlip bool // after discarding a deck-drawn card, the active seat must flip one

	closer    int // seat that turned its whole grid up (-1 while none)
	finalLeft int // final turns remaining once a round is closing

	gameWinner int
	logs       []string
}

func newSkyjoGame(out Outbox, seed uint64) Game {
	return &skyjoGame{
		out:        out,
		rng:        rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15)),
		players:    map[int]*skyjoPlayer{},
		phase:      skLobby,
		drawn:      skNone,
		closer:     -1,
		gameWinner: -1,
	}
}

// --- Game interface ---------------------------------------------------------

func (g *skyjoGame) AddPlayer(name, meta string) (int, bool) {
	seat := -1
	for s := 0; s < skMaxPlayers; s++ {
		if _, taken := g.players[s]; !taken {
			seat = s
			break
		}
	}
	if seat == -1 {
		return 0, false
	}
	g.players[seat] = &skyjoPlayer{name: name}
	g.order = append(g.order, seat)
	return seat, true
}

func (g *skyjoGame) RemovePlayer(seat int) {
	delete(g.players, seat)
	for i, s := range g.order {
		if s == seat {
			g.order = append(g.order[:i], g.order[i+1:]...)
			break
		}
	}
	g.broadcastViews()
}

func (g *skyjoGame) Roster() []RosterEntry {
	out := make([]RosterEntry, 0, len(g.players))
	for s, p := range g.players {
		out = append(out, RosterEntry{S: s, N: p.name})
	}
	return out
}

func (g *skyjoGame) WelcomeExtra() WelcomeExtra { return WelcomeExtra{} }
func (g *skyjoGame) Joined(int)                 { g.broadcastViews() }
func (g *skyjoGame) Dropped(int)                { g.broadcastViews() }
func (g *skyjoGame) ToLobby()                   { g.resetToLobby(); g.broadcastViews() }
func (g *skyjoGame) TickHz() int                { return 0 }
func (g *skyjoGame) Tick(time.Time)             {}

func (g *skyjoGame) Command(seat int, isHost bool, raw []byte) {
	c, ok := decodeCmd[struct {
		T    string `json:"t"`
		Cell int    `json:"cell"`
	}](raw)
	if !ok {
		return
	}
	if _, ok := g.players[seat]; !ok {
		return
	}
	slog.Debug("skyjo command", "event", "cmd", "game", "skyjo", "seat", seat, "cmd", c.T)

	switch c.T {
	case skMsgStart:
		if !isHost || g.phase != skLobby {
			return
		}
		if n := len(g.players); n < skMinPlayers || n > skMaxPlayers {
			return
		}
		for _, p := range g.players {
			p.score = 0
		}
		g.gameWinner = -1
		g.newRound()
	case skMsgFlip:
		g.flip(seat, c.Cell)
	case skMsgDraw:
		if g.phase != skPlay || seat != g.turn || g.drawn != skNone {
			return
		}
		g.drawn = g.drawCard()
		g.fromDeck = true
	case skMsgTakeDiscard:
		if g.phase != skPlay || seat != g.turn || g.drawn != skNone || len(g.discard) == 0 {
			return
		}
		g.drawn = g.discard[len(g.discard)-1]
		g.discard = g.discard[:len(g.discard)-1]
		g.fromDeck = false
	case skMsgSwap:
		g.swap(seat, c.Cell)
	case skMsgDiscardDraw:
		if g.phase != skPlay || seat != g.turn || g.drawn == skNone || !g.fromDeck {
			return
		}
		g.discard = append(g.discard, g.drawn)
		g.drawn = skNone
		g.mustFlip = true
	case skMsgNext:
		if !isHost || g.phase != skRoundEnd {
			return
		}
		g.newRound()
	case skMsgRestart:
		if !isHost || g.phase != skOver {
			return
		}
		g.resetToLobby()
	default:
		return
	}
	g.broadcastViews()
}

// --- round lifecycle --------------------------------------------------------

func (g *skyjoGame) newRound() {
	g.deck = append([]int{}, skDeck...)
	g.rng.Shuffle(len(g.deck), func(i, j int) { g.deck[i], g.deck[j] = g.deck[j], g.deck[i] })
	for _, s := range g.order {
		p := g.players[s]
		for i := 0; i < skCells; i++ {
			p.grid[i] = skyjoCell{value: g.drawCard()}
		}
	}
	g.discard = []int{g.drawCard()} // flip one card to open the discard pile
	g.drawn = skNone
	g.fromDeck = false
	g.mustFlip = false
	g.closer = -1
	g.finalLeft = 0
	g.phase = skSetup
	g.turn = g.order[0]
}

// drawCard pops the top of the deck, reshuffling the discard (all but its top) back in
// when the deck runs dry, so a long round never stalls.
func (g *skyjoGame) drawCard() int {
	if len(g.deck) == 0 {
		if len(g.discard) > 1 {
			keep := g.discard[len(g.discard)-1]
			g.deck = append(g.deck, g.discard[:len(g.discard)-1]...)
			g.discard = []int{keep}
			g.rng.Shuffle(len(g.deck), func(i, j int) { g.deck[i], g.deck[j] = g.deck[j], g.deck[i] })
		}
		if len(g.deck) == 0 {
			return 0 // degenerate: nothing left anywhere
		}
	}
	c := g.deck[len(g.deck)-1]
	g.deck = g.deck[:len(g.deck)-1]
	return c
}

// flip reveals a face-down card. It serves two phases: the opening setup (each player
// turns up exactly 2) and the post-discard flip during play (the active seat must turn
// up one). Anything else is rejected.
func (g *skyjoGame) flip(seat, cell int) {
	if cell < 0 || cell >= skCells {
		return
	}
	p := g.players[seat]
	switch g.phase {
	case skSetup:
		if g.upCount(p) >= 2 || p.grid[cell].up || p.grid[cell].gone {
			return
		}
		p.grid[cell].up = true
		g.maybeFinishSetup()
	case skPlay:
		if seat != g.turn || !g.mustFlip || p.grid[cell].up || p.grid[cell].gone {
			return
		}
		p.grid[cell].up = true
		g.mustFlip = false
		g.postMove(seat)
	}
}

func (g *skyjoGame) upCount(p *skyjoPlayer) int {
	n := 0
	for _, c := range p.grid {
		if c.up && !c.gone {
			n++
		}
	}
	return n
}

// maybeFinishSetup starts play once every player has flipped their 2 opening cards. The
// player with the highest face-up sum goes first (ties broken by seat order).
func (g *skyjoGame) maybeFinishSetup() {
	for _, s := range g.order {
		if g.upCount(g.players[s]) < 2 {
			return
		}
	}
	best, bestSum := g.order[0], -1<<30
	for _, s := range g.order {
		if sum := g.faceSum(g.players[s]); sum > bestSum {
			best, bestSum = s, sum
		}
	}
	g.turn = best
	g.phase = skPlay
}

func (g *skyjoGame) faceSum(p *skyjoPlayer) int {
	sum := 0
	for _, c := range p.grid {
		if c.up && !c.gone {
			sum += c.value
		}
	}
	return sum
}

// swap places the held card into a grid cell, sending the replaced card (revealed if it
// was face-down) to the discard pile. It ends the active player's action.
func (g *skyjoGame) swap(seat, cell int) {
	if g.phase != skPlay || seat != g.turn || g.drawn == skNone {
		return
	}
	if cell < 0 || cell >= skCells || g.players[seat].grid[cell].gone {
		return
	}
	p := g.players[seat]
	old := p.grid[cell].value
	p.grid[cell] = skyjoCell{value: g.drawn, up: true}
	g.discard = append(g.discard, old)
	g.drawn = skNone
	g.fromDeck = false
	g.postMove(seat)
}

// postMove runs after a completed action: clear any full matching column, check whether
// this seat just turned its whole grid up (closing the round), then pass the turn.
func (g *skyjoGame) postMove(seat int) {
	g.clearColumns(seat)
	if g.closer == -1 && g.allUp(g.players[seat]) {
		g.closer = seat
		g.finalLeft = len(g.order) - 1 // one more turn for everyone else
		g.logs = append(g.logs, g.players[seat].name+" closed the round.")
	}
	g.nextTurn()
}

// clearColumns removes any column whose three cards are all face-up with the same value
// (they score nothing); the removed cards go to the discard pile.
func (g *skyjoGame) clearColumns(seat int) {
	p := g.players[seat]
	for col := 0; col < skCols; col++ {
		a, b, c := col, col+skCols, col+2*skCols
		ca, cb, cc := p.grid[a], p.grid[b], p.grid[c]
		if ca.gone || cb.gone || cc.gone {
			continue
		}
		if ca.up && cb.up && cc.up && ca.value == cb.value && cb.value == cc.value {
			g.discard = append(g.discard, ca.value, cb.value, cc.value)
			p.grid[a].gone, p.grid[b].gone, p.grid[c].gone = true, true, true
		}
	}
}

func (g *skyjoGame) allUp(p *skyjoPlayer) bool {
	for _, c := range p.grid {
		if !c.up && !c.gone {
			return false
		}
	}
	return true
}

// nextTurn advances to the next seat, or ends the round once the final lap after a close
// is spent.
func (g *skyjoGame) nextTurn() {
	if g.closer != -1 {
		if g.finalLeft <= 0 {
			g.endRound()
			return
		}
		g.finalLeft--
	}
	fi := 0
	for i, s := range g.order {
		if s == g.turn {
			fi = i
			break
		}
	}
	g.turn = g.order[(fi+1)%len(g.order)]
}

// endRound reveals every grid, scores the round (the closer's score is doubled if it is
// positive and not strictly the lowest), adds it to totals, and ends the game if anyone
// passed skEndScore.
func (g *skyjoGame) endRound() {
	for _, s := range g.order {
		p := g.players[s]
		for i := range p.grid {
			if !p.grid[i].gone {
				p.grid[i].up = true
			}
		}
	}
	round := map[int]int{}
	for _, s := range g.order {
		round[s] = g.faceSum(g.players[s])
	}
	if g.closer != -1 {
		cs := round[g.closer]
		lowestElsewhere := true
		for _, s := range g.order {
			if s != g.closer && round[s] <= cs {
				lowestElsewhere = false
				break
			}
		}
		if cs > 0 && !lowestElsewhere {
			round[g.closer] = cs * 2
			g.logs = append(g.logs, g.players[g.closer].name+" did not finish lowest: round score doubled.")
		}
	}
	for _, s := range g.order {
		g.players[s].score += round[s]
	}
	g.phase = skRoundEnd
	for _, s := range g.order {
		if g.players[s].score >= skEndScore {
			g.phase = skOver
			break
		}
	}
	if g.phase == skOver {
		best, bestScore := g.order[0], 1<<30
		for _, s := range g.order {
			if g.players[s].score < bestScore {
				best, bestScore = s, g.players[s].score
			}
		}
		g.gameWinner = best
	}
}

func (g *skyjoGame) resetToLobby() {
	g.phase = skLobby
	g.deck = nil
	g.discard = nil
	g.drawn = skNone
	g.fromDeck = false
	g.mustFlip = false
	g.closer = -1
	g.finalLeft = 0
	g.gameWinner = -1
	g.logs = nil
	for _, p := range g.players {
		p.score = 0
		p.grid = [skCells]skyjoCell{}
	}
}

// --- view -------------------------------------------------------------------

type skyjoCellView struct {
	V    int  `json:"v"`    // value, only meaningful when up
	Up   bool `json:"up"`   // face-up (value visible to everyone)
	Gone bool `json:"gone"` // removed in a cleared column
}

type skyjoPlayerView struct {
	S     int             `json:"s"`
	N     string          `json:"n"`
	Score int             `json:"score"`
	Grid  []skyjoCellView `json:"grid"`
	Flips int             `json:"flips"` // face-up count (setup progress)
	Gone  bool            `json:"gone"`  // socket dropped
}

type skyjoView struct {
	T          string            `json:"t"`
	Phase      string            `json:"ph"`
	Turn       int               `json:"turn"`
	You        int               `json:"you"`
	Players    []skyjoPlayerView `json:"players"`
	DeckLeft   int               `json:"deckLeft"`
	DiscardTop int               `json:"discardTop"` // top of the discard pile, or skNone if empty
	HasDiscard bool              `json:"hasDiscard"`
	Drawn      int               `json:"drawn"`    // the card YOU are holding this turn, else skNone
	FromDeck   bool              `json:"fromDeck"` // your held card may be discarded
	MustFlip   bool              `json:"mustFlip"` // you must flip a card now
	Closer     int               `json:"closer"`
	EndScore   int               `json:"endScore"`
	GameWinner int               `json:"gameWinner"`
	Logs       []string          `json:"logs"`
}

func (g *skyjoGame) viewFor(seat int) skyjoView {
	v := skyjoView{
		T: MsgState, Phase: g.phase, Turn: g.turn, You: seat,
		Players: []skyjoPlayerView{}, DeckLeft: len(g.deck),
		DiscardTop: skNone, Drawn: skNone, Closer: g.closer, EndScore: skEndScore,
		GameWinner: g.gameWinner, Logs: lastLog(g.logs, 8),
	}
	if n := len(g.discard); n > 0 {
		v.DiscardTop = g.discard[n-1]
		v.HasDiscard = true
	}
	if seat == g.turn && g.drawn != skNone {
		v.Drawn = g.drawn
		v.FromDeck = g.fromDeck
	}
	if seat == g.turn {
		v.MustFlip = g.mustFlip
	}
	connected := map[int]bool{}
	for _, s := range g.out.Seats() {
		connected[s] = true
	}
	for _, s := range g.order {
		p := g.players[s]
		pv := skyjoPlayerView{S: s, N: p.name, Score: p.score, Flips: g.upCount(p), Gone: !connected[s], Grid: []skyjoCellView{}}
		for _, c := range p.grid {
			cv := skyjoCellView{Gone: c.gone}
			if c.up && !c.gone {
				cv.Up = true
				cv.V = c.value
			}
			pv.Grid = append(pv.Grid, cv)
		}
		v.Players = append(v.Players, pv)
	}
	return v
}

func (g *skyjoGame) broadcastViews() {
	for _, seat := range g.out.Seats() {
		g.out.Send(seat, g.viewFor(seat))
	}
}
