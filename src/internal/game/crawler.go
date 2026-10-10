package game

import (
	"encoding/json"
	"math/rand/v2"
	"time"
)

// crawlerGame is Totem Crawler V0: an original co-op dungeon crawler (1-6) on the
// engine. It is turn-based and event-driven (TickHz 0). See
// docs/crawler-architecture.md for the full spec; this is the V0 slice:
//
//   - Numbered dice spent through a per-sheet action menu (each action has a
//     MINIMUM die value; a die pays for any action whose minimum it meets).
//   - Square-by-square movement on 3x3 tiles; leaving a tile lets the PLAYER lay
//     the next tile, choosing an orientation from the legal options.
//   - Random banded tile stack (easy early, nasty late), boss in the final band.
//   - Static enemies (Armour/Health/Damage); combat resolves at end of turn.
//   - Courage (reroll, unlock two specials) and Shields.
//   - Win = boss defeated; lose = party wiped, or the stack runs out first.
//
// V0 simplifications (see the spec): tiles have no inner walls (the 3x3 is fully
// open; variation is the open edges), one monster per monster-tile sits on the
// centre square, there are no loot items or hazards yet, and combat is the active
// hero alone (group fights are V1). The co-op view is shared (viewFor ignores the
// seat) but kept per-seat for future private info.

// Directions: N,E,S,W.
var crawlerDelta = [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

// doorway is the 3x3 square index (row-major, 0..8) in the middle of each side.
var crawlerDoor = [4]int{1, 5, 7, 3}

func crawlerOpp(d int) int { return (d + 2) % 4 }

const (
	crMinPlayers = 1
	crMaxPlayers = 6
)

const (
	crLobby   = "lobby"
	crPlaying = "playing"
	crWon     = "won"
	crLost    = "lost"
)

const (
	crStepSpend = "spend"
	crStepLay   = "lay" // a drawn tile awaits the player's placement
)

const (
	crMsgStart   = "start"
	crMsgSpend   = "spend"   // {die, action, dir|monster|ally}
	crMsgLay     = "layTile" // {x, y, rotation} from the offered options
	crMsgReroll  = "reroll"  // {die}
	crMsgUnlock  = "unlock"  // {which: 0|1}
	crMsgEndTurn = "endTurn"
	crMsgRestart = "restart"
)

// Actions on the menu.
const (
	actMove    = "move"
	actAttack  = "attack"
	actCourage = "courage"
	actBrace   = "brace" // Beaver special: 2 Shields to self or an ally on the tile
	actGuard   = "guard" // generic special: 1 Shield to self
)

const crCourageCap = 10

// --- sheets -----------------------------------------------------------------

type crawlerMenuEntry struct {
	Action string
	Min    int // lowest die value that can pay for this action
}

type crawlerSheet struct {
	totem   string
	maxHP   int
	diceN   int
	menu    []crawlerMenuEntry
	special string // the special action's name (brace/guard)
	unlock0 string // first unlock label
	unlock1 string // second unlock label
}

func beaverSheet() crawlerSheet {
	return crawlerSheet{
		totem: "bever", maxHP: 7, diceN: 4,
		menu: []crawlerMenuEntry{
			{actMove, 1}, {actBrace, 2}, {actAttack, 4}, {actCourage, 5},
		},
		special: actBrace, unlock0: "Thick Hide", unlock1: "Bulwark",
	}
}

func genericSheet(totem string) crawlerSheet {
	return crawlerSheet{
		totem: totem, maxHP: 5, diceN: 4,
		menu: []crawlerMenuEntry{
			{actMove, 1}, {actGuard, 3}, {actAttack, 4}, {actCourage, 5},
		},
		special: actGuard, unlock0: "Resolve", unlock1: "Second Wind",
	}
}

func (s crawlerSheet) min(action string) (int, bool) {
	for _, m := range s.menu {
		if m.Action == action {
			return m.Min, true
		}
	}
	return 0, false
}

// --- state ------------------------------------------------------------------

type crawlerHero struct {
	name     string
	sheet    crawlerSheet
	health   int
	courage  int
	shields  int
	tileID   int
	square   int
	unlocked [2]bool
	alive    bool
}

type crawlerDie struct {
	Value   int    `json:"value"`
	Spent   bool   `json:"spent"`
	UsedFor string `json:"usedFor"`
}

type crawlerMonster struct {
	armour, health, maxHealth, damage int // health is CURRENT and persists across turns
	square                            int
	engaged                           bool // attacked this turn (retaliates at end of turn)
	alive                             bool
	isBoss                            bool
	reward                            int // Courage granted on defeat
}

type crawlerTile struct {
	id       int
	band     int
	kind     string // straight / bend / tee / cross / dead-end / boss
	x, y     int
	edges    [4]bool
	monsters []*crawlerMonster
	isBoss   bool
}

// tileSpec is one entry in the shuffled stack, drawn when a tile is laid.
type tileSpec struct {
	band    int
	kind    string
	base    [4]bool
	monster *crawlerMonster
	isBoss  bool
}

type crawlerLayOption struct {
	X        int     `json:"x"`
	Y        int     `json:"y"`
	Rotation int     `json:"rotation"`
	Edges    [4]bool `json:"edges"`
}

type crawlerPending struct {
	spec     tileSpec
	entryDir int
	options  []crawlerLayOption
}

type crawlerGame struct {
	out Outbox
	rng *rand.Rand

	players map[int]*crawlerHero
	order   []int

	phase string
	step  string
	turn  int

	dice []crawlerDie

	tiles  map[int]*crawlerTile
	at     map[[2]int]int
	nextID int

	deck    []tileSpec
	deckPos int
	bands   int

	pending *crawlerPending
	log     []string
}

func newCrawlerGame(out Outbox, seed uint64) Game {
	return &crawlerGame{
		out:     out,
		rng:     rand.New(rand.NewPCG(seed, 0x2545f4914f6cdd1d)),
		players: map[int]*crawlerHero{},
		phase:   crLobby,
		tiles:   map[int]*crawlerTile{},
		at:      map[[2]int]int{},
	}
}

// --- Game interface ---------------------------------------------------------

func (g *crawlerGame) AddPlayer(name, meta string) (int, bool) {
	seat := -1
	for s := 0; s < crMaxPlayers; s++ {
		if _, taken := g.players[s]; !taken {
			seat = s
			break
		}
	}
	if seat == -1 {
		return 0, false
	}
	sheet := genericSheet(meta)
	if meta == "bever" {
		sheet = beaverSheet()
	}
	if meta == "" {
		sheet = genericSheet("totem")
	}
	g.players[seat] = &crawlerHero{name: name, sheet: sheet, health: sheet.maxHP, alive: true}
	g.order = append(g.order, seat)
	return seat, true
}

func (g *crawlerGame) RemovePlayer(seat int) {
	delete(g.players, seat)
	for i, s := range g.order {
		if s == seat {
			g.order = append(g.order[:i], g.order[i+1:]...)
			break
		}
	}
	g.broadcastViews()
}

func (g *crawlerGame) Roster() []RosterEntry {
	out := make([]RosterEntry, 0, len(g.players))
	for s, p := range g.players {
		out = append(out, RosterEntry{S: s, N: p.name})
	}
	return out
}

func (g *crawlerGame) WelcomeExtra() WelcomeExtra { return WelcomeExtra{} }
func (g *crawlerGame) Joined(int)                 { g.broadcastViews() }
func (g *crawlerGame) Dropped(int)                { g.broadcastViews() }
func (g *crawlerGame) ToLobby()                   { g.resetToLobby(); g.broadcastViews() }
func (g *crawlerGame) TickHz() int                { return 0 }
func (g *crawlerGame) Tick(time.Time)             {}

func (g *crawlerGame) Command(seat int, isHost bool, raw []byte) {
	var c struct {
		T        string `json:"t"`
		Die      int    `json:"die"`
		Action   string `json:"action"`
		Dir      int    `json:"dir"`
		Monster  int    `json:"monster"`
		Ally     int    `json:"ally"`
		X        int    `json:"x"`
		Y        int    `json:"y"`
		Rotation int    `json:"rotation"`
		Which    int    `json:"which"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return
	}
	if _, ok := g.players[seat]; !ok {
		return
	}

	switch c.T {
	case crMsgStart:
		if !isHost || g.phase != crLobby || len(g.players) < crMinPlayers {
			return
		}
		g.startRun()
	case crMsgSpend:
		if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
			return
		}
		g.spendDie(seat, c.Die, c.Action, c.Dir, c.Monster, c.Ally)
	case crMsgLay:
		if g.phase != crPlaying || g.step != crStepLay || seat != g.turn {
			return
		}
		g.layTile(seat, c.X, c.Y, c.Rotation)
	case crMsgReroll:
		if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
			return
		}
		g.reroll(seat, c.Die)
	case crMsgUnlock:
		if g.phase != crPlaying || seat != g.turn {
			return
		}
		g.unlock(seat, c.Which)
	case crMsgEndTurn:
		if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
			return
		}
		g.endTurn(seat)
	case crMsgRestart:
		if !isHost || (g.phase != crWon && g.phase != crLost) {
			return
		}
		g.resetToLobby()
	default:
		return
	}
	g.broadcastViews()
}

// --- setup ------------------------------------------------------------------

func (g *crawlerGame) startRun() {
	g.tiles = map[int]*crawlerTile{}
	g.at = map[[2]int]int{}
	g.nextID = 0
	g.pending = nil
	g.log = nil
	g.bands = 3
	g.buildDeck()

	// Start tile: a cross at (0,0) with no monster; heroes start on the centre.
	start := g.placeTile(0, 0, [4]bool{true, true, true, true}, 0, "cross", false, nil)
	for _, s := range g.order {
		h := g.players[s]
		h.health = h.sheet.maxHP
		h.courage = 0
		h.shields = 0
		h.unlocked = [2]bool{}
		h.alive = true
		h.tileID = start.id
		h.square = 4
	}
	g.phase = crPlaying
	g.turn = g.order[0]
	g.log = append(g.log, "You enter the dungeon.")
	g.beginTurn(g.turn)
}

// crawlerTileType is a named base shape; rotations are applied when it is laid,
// so the base orientation only needs to be one representative. Edges are N,E,S,W.
type crawlerTileType struct {
	kind string
	base [4]bool
}

var (
	ttStraight = crawlerTileType{"straight", [4]bool{true, false, true, false}}  // corridor
	ttBend     = crawlerTileType{"bend", [4]bool{true, true, false, false}}      // corner
	ttTee      = crawlerTileType{"tee", [4]bool{true, true, true, false}}        // junction
	ttCross    = crawlerTileType{"cross", [4]bool{true, true, true, true}}       // open crossroads
	ttDeadEnd  = crawlerTileType{"dead-end", [4]bool{true, false, false, false}} // a single doorway
)

// crawlerTilePool is the weighted draw pool: corridors and bends are common,
// junctions a little rarer, dead-ends rare (they cap a branch). Tiles can be
// laid on any of the four sides because rotations cover every orientation.
var crawlerTilePool = []crawlerTileType{
	ttStraight, ttStraight, ttStraight,
	ttBend, ttBend, ttBend,
	ttTee, ttTee,
	ttCross, ttCross,
	ttDeadEnd,
}

// buildDeck lays out the shuffled stack: tiles in difficulty bands, the boss
// shuffled into the final band.
func (g *crawlerGame) buildDeck() {
	perBand := 4
	g.deck = nil
	g.deckPos = 0
	for b := 0; b < g.bands; b++ {
		band := make([]tileSpec, 0, perBand)
		for i := 0; i < perBand; i++ {
			tt := crawlerTilePool[g.rng.IntN(len(crawlerTilePool))]
			band = append(band, tileSpec{band: b, kind: tt.kind, base: tt.base, monster: g.rollMonster(b)})
		}
		g.rng.Shuffle(len(band), func(i, j int) { band[i], band[j] = band[j], band[i] })
		if b == g.bands-1 {
			// Boss (a dead-end lair) shuffled into the last few of the final band.
			boss := tileSpec{band: b, kind: "boss", base: ttDeadEnd.base, isBoss: true,
				monster: &crawlerMonster{armour: 5, health: 4, maxHealth: 4, damage: 4, square: 4, alive: true, isBoss: true, reward: 3}}
			pos := len(band)
			if len(band) > 0 {
				pos = len(band) - g.rng.IntN(min2(len(band), 3)+1)
			}
			band = append(band[:pos], append([]tileSpec{boss}, band[pos:]...)...)
		}
		g.deck = append(g.deck, band...)
	}
}

func (g *crawlerGame) rollMonster(band int) *crawlerMonster {
	roll := g.rng.IntN(100)
	mk := func(a, h, d int) *crawlerMonster {
		return &crawlerMonster{armour: a, health: h, maxHealth: h, damage: d, square: 4, alive: true, reward: 1}
	}
	switch band {
	case 0:
		if roll < 50 {
			return mk(3, 1, 1)
		}
	case 1:
		if roll < 70 {
			return mk(4, 2, 2)
		}
	default:
		if roll < 80 {
			return mk(4, 2, 3)
		}
	}
	return nil
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- turn flow --------------------------------------------------------------

func (g *crawlerGame) beginTurn(seat int) {
	h := g.players[seat]
	g.step = crStepSpend
	// Thick Hide: start the turn with 2 Shields if unlocked (Beaver, unlock 0).
	if h.unlocked[0] && h.sheet.totem == "bever" {
		h.shields += 2
	}
	g.dice = make([]crawlerDie, h.sheet.diceN)
	for i := range g.dice {
		g.dice[i] = crawlerDie{Value: 1 + g.rng.IntN(6)}
	}
	// Fresh turn: clear "engaged this turn" on the hero's current tile (damage
	// already dealt to monsters persists; only the retaliation flag resets).
	if t := g.tiles[h.tileID]; t != nil {
		for _, m := range t.monsters {
			m.engaged = false
		}
	}
}

func (g *crawlerGame) spendDie(seat, dieIdx int, action string, dir, monsterIdx, ally int) {
	h := g.players[seat]
	if dieIdx < 0 || dieIdx >= len(g.dice) || g.dice[dieIdx].Spent {
		return
	}
	minV, ok := h.sheet.min(action)
	if !ok || g.dice[dieIdx].Value < minV {
		return
	}
	switch action {
	case actMove:
		if !g.tryMove(seat, dir) {
			return
		}
	case actAttack:
		if !g.tryAttack(seat, monsterIdx, g.dice[dieIdx].Value) {
			return
		}
	case actBrace:
		if !g.grantShields(seat, ally, 2) {
			return
		}
	case actGuard:
		h.shields++
	case actCourage:
		if h.courage < crCourageCap {
			h.courage++
		}
	default:
		return
	}
	g.dice[dieIdx].Spent = true
	g.dice[dieIdx].UsedFor = action
}

// tryMove steps one square in direction dir; crossing an open edge enters the
// neighbour tile, or (at a frontier) draws the next tile for the player to lay.
func (g *crawlerGame) tryMove(seat, dir int) bool {
	if dir < 0 || dir > 3 {
		return false
	}
	h := g.players[seat]
	t := g.tiles[h.tileID]
	col, row := h.square%3, h.square/3
	ncol, nrow := col+crawlerDelta[dir][0], row+crawlerDelta[dir][1]
	if ncol >= 0 && ncol < 3 && nrow >= 0 && nrow < 3 {
		h.square = nrow*3 + ncol // V0: no inner walls, always open
		return true
	}
	// Crossing the tile edge: must be at that side's doorway and the edge open.
	if h.square != crawlerDoor[dir] || !t.edges[dir] {
		return false
	}
	nx, ny := t.x+crawlerDelta[dir][0], t.y+crawlerDelta[dir][1]
	if id, ok := g.at[[2]int{nx, ny}]; ok {
		h.square = crawlerDoor[crawlerOpp(dir)] // enter the neighbour's doorway
		h.tileID = id
		g.enterTile(seat, id)
		return true
	}
	// Frontier: draw and let the player lay the tile.
	return g.beginLay(seat, dir, nx, ny)
}

func (g *crawlerGame) enterTile(seat, id int) {
	t := g.tiles[id]
	for _, m := range t.monsters {
		m.engaged = false
	}
}

// tryAttack applies one Attack die immediately: a die that meets the Armour
// reduces the monster's (persistent) Health, and a lethal blow kills it right
// away. Damage carries across turns, so combat continues until it is defeated.
func (g *crawlerGame) tryAttack(seat, monsterIdx, value int) bool {
	h := g.players[seat]
	t := g.tiles[h.tileID]
	if monsterIdx < 0 || monsterIdx >= len(t.monsters) {
		return false
	}
	m := t.monsters[monsterIdx]
	if !m.alive {
		return false
	}
	m.engaged = true
	if value >= m.armour {
		m.health--
		if m.health <= 0 {
			m.health = 0
			m.alive = false
			if h.courage < crCourageCap {
				h.courage += m.reward
				if h.courage > crCourageCap {
					h.courage = crCourageCap
				}
			}
			g.log = append(g.log, h.name+" defeats a monster.")
			if m.isBoss {
				g.phase = crWon
				g.log = append(g.log, "The boss falls! You win.")
			}
		}
	}
	return true
}

func (g *crawlerGame) grantShields(seat, ally, n int) bool {
	src := g.players[seat]
	tgt := src
	if ally != seat {
		a, ok := g.players[ally]
		if !ok || !a.alive || a.tileID != src.tileID {
			return false
		}
		tgt = a
	}
	tgt.shields += n
	return true
}

// beginLay draws the next tile and computes the legal placements for the player.
func (g *crawlerGame) beginLay(seat, dir, nx, ny int) bool {
	if g.deckPos >= len(g.deck) {
		// The stack ran out before the boss fell: too slow.
		g.phase = crLost
		g.log = append(g.log, "The last tile is gone and the boss still lurks. The dungeon claims you.")
		return true
	}
	spec := g.deck[g.deckPos]
	opts := g.layOptions(spec, dir, nx, ny)
	if len(opts) == 0 {
		return false // no legal placement: the move is blocked
	}
	g.deckPos++
	g.pending = &crawlerPending{spec: spec, entryDir: dir, options: opts}
	g.step = crStepLay
	return true
}

// layOptions returns every rotation of spec that connects at (nx,ny): an opening
// on the entry side, and edges compatible with any already-placed neighbours.
func (g *crawlerGame) layOptions(spec tileSpec, dir, nx, ny int) []crawlerLayOption {
	var out []crawlerLayOption
	entry := crawlerOpp(dir) // the new tile's side facing back toward the hero
	for r := 0; r < 4; r++ {
		e := rotateEdges(spec.base, r)
		if !e[entry] {
			continue
		}
		if !g.edgesFit(nx, ny, e) {
			continue
		}
		out = append(out, crawlerLayOption{X: nx, Y: ny, Rotation: r, Edges: e})
	}
	return out
}

// edgesFit checks a candidate tile's edges against every already-placed
// neighbour: a shared side must match (both open or both closed).
func (g *crawlerGame) edgesFit(x, y int, e [4]bool) bool {
	for d := 0; d < 4; d++ {
		ox, oy := x+crawlerDelta[d][0], y+crawlerDelta[d][1]
		id, ok := g.at[[2]int{ox, oy}]
		if !ok {
			continue // empty neighbour: an open edge here is just a new frontier
		}
		if g.tiles[id].edges[crawlerOpp(d)] != e[d] {
			return false
		}
	}
	return true
}

func rotateEdges(base [4]bool, r int) [4]bool {
	var out [4]bool
	for d := 0; d < 4; d++ {
		out[d] = base[(d-r+4)%4]
	}
	return out
}

func (g *crawlerGame) layTile(seat, x, y, rotation int) {
	if g.pending == nil {
		return
	}
	var chosen *crawlerLayOption
	for i := range g.pending.options {
		o := g.pending.options[i]
		if o.X == x && o.Y == y && o.Rotation == rotation {
			chosen = &g.pending.options[i]
			break
		}
	}
	if chosen == nil {
		return // not one of the offered options
	}
	spec := g.pending.spec
	var mons []*crawlerMonster
	if spec.monster != nil {
		m := *spec.monster
		mons = []*crawlerMonster{&m}
	}
	t := g.placeTile(chosen.X, chosen.Y, chosen.Edges, spec.band, spec.kind, spec.isBoss, mons)
	// Complete the move that triggered the lay: enter the new tile's doorway.
	h := g.players[seat]
	h.tileID = t.id
	h.square = crawlerDoor[crawlerOpp(g.pending.entryDir)]
	if spec.isBoss {
		g.log = append(g.log, "A boss tile! The big bad is here.")
	} else {
		g.log = append(g.log, "You lay a new tile and step through.")
	}
	g.pending = nil
	g.step = crStepSpend
}

func (g *crawlerGame) placeTile(x, y int, edges [4]bool, band int, kind string, isBoss bool, mons []*crawlerMonster) *crawlerTile {
	t := &crawlerTile{id: g.nextID, band: band, kind: kind, x: x, y: y, edges: edges, monsters: mons, isBoss: isBoss}
	g.nextID++
	g.tiles[t.id] = t
	g.at[[2]int{x, y}] = t.id
	return t
}

func (g *crawlerGame) reroll(seat, dieIdx int) {
	h := g.players[seat]
	if dieIdx < 0 || dieIdx >= len(g.dice) || g.dice[dieIdx].Spent || h.courage < 1 {
		return
	}
	h.courage--
	g.dice[dieIdx].Value = 1 + g.rng.IntN(6)
}

func (g *crawlerGame) unlock(seat, which int) {
	h := g.players[seat]
	if which < 0 || which > 1 || h.unlocked[which] {
		return
	}
	if which == 1 && !h.unlocked[0] {
		return // the second unlock needs the first
	}
	cost := 3
	if which == 1 {
		cost = 5
	}
	if h.courage < cost {
		return
	}
	h.courage -= cost
	h.unlocked[which] = true
}

func (g *crawlerGame) endTurn(seat int) {
	g.resolveCombat(seat)
	if g.phase != crPlaying {
		return
	}
	if g.aliveHeroes() == 0 {
		g.phase = crLost
		g.log = append(g.log, "The party has fallen. The dungeon wins.")
		return
	}
	g.turn = g.nextAlive(seat)
	g.beginTurn(g.turn)
}

// resolveCombat is the end-of-turn retaliation: a monster the hero attacked this
// turn that is still alive strikes back once for its Damage (reduced by Shields).
// Kills happen immediately in tryAttack, and a monster's lost Health persists, so
// a tough enemy is whittled down over successive turns until it is defeated.
func (g *crawlerGame) resolveCombat(seat int) {
	h := g.players[seat]
	t := g.tiles[h.tileID]
	if t == nil {
		return
	}
	for _, m := range t.monsters {
		if !m.alive || !m.engaged {
			continue
		}
		dmg := m.damage - h.shields
		if h.shields > m.damage {
			h.shields -= m.damage
		} else {
			h.shields = 0
		}
		if dmg > 0 {
			h.health -= dmg
		}
		if h.health <= 0 {
			h.health = 0
			h.alive = false
			g.log = append(g.log, h.name+" falls.")
		}
		m.engaged = false
	}
}

func (g *crawlerGame) aliveHeroes() int {
	n := 0
	for _, s := range g.order {
		if g.players[s].alive {
			n++
		}
	}
	return n
}

func (g *crawlerGame) nextAlive(from int) int {
	fi := 0
	for i, s := range g.order {
		if s == from {
			fi = i
			break
		}
	}
	for i := 1; i <= len(g.order); i++ {
		s := g.order[(fi+i)%len(g.order)]
		if g.players[s].alive {
			return s
		}
	}
	return from
}

func (g *crawlerGame) resetToLobby() {
	g.phase = crLobby
	g.step = ""
	g.tiles = map[int]*crawlerTile{}
	g.at = map[[2]int]int{}
	g.dice = nil
	g.pending = nil
	g.log = nil
	g.deck = nil
	g.deckPos = 0
	for _, h := range g.players {
		h.health = h.sheet.maxHP
		h.courage = 0
		h.shields = 0
		h.unlocked = [2]bool{}
		h.alive = true
	}
}

// --- view -------------------------------------------------------------------

type crawlerHeroView struct {
	S        int     `json:"s"`
	N        string  `json:"n"`
	Totem    string  `json:"totem"`
	Health   int     `json:"health"`
	MaxHP    int     `json:"maxHP"`
	Courage  int     `json:"courage"`
	Shields  int     `json:"shields"`
	TileID   int     `json:"tile"`
	Square   int     `json:"square"`
	Unlocked [2]bool `json:"unlocked"`
	Alive    bool    `json:"alive"`
	Special  string  `json:"special"`
	Gone     bool    `json:"gone,omitempty"`
}

type crawlerMonsterView struct {
	ID      int  `json:"id"`
	Armour  int  `json:"armour"`
	Health  int  `json:"health"` // current, persists across turns
	MaxHP   int  `json:"maxHP"`
	Damage  int  `json:"damage"`
	Square  int  `json:"square"`
	Engaged bool `json:"engaged"`
	Alive   bool `json:"alive"`
	IsBoss  bool `json:"isBoss"`
}

type crawlerTileView struct {
	ID       int                  `json:"id"`
	X        int                  `json:"x"`
	Y        int                  `json:"y"`
	Kind     string               `json:"kind"`
	Edges    [4]bool              `json:"edges"`
	IsBoss   bool                 `json:"isBoss"`
	Monsters []crawlerMonsterView `json:"monsters"`
}

type crawlerFrontierView struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type crawlerView struct {
	T         string                `json:"t"`
	Phase     string                `json:"ph"`
	Step      string                `json:"step"`
	Turn      int                   `json:"turn"`
	You       int                   `json:"you"`
	Dice      []crawlerDie          `json:"dice"`
	Heroes    []crawlerHeroView     `json:"heroes"`
	Tiles     []crawlerTileView     `json:"tiles"`
	Frontiers []crawlerFrontierView `json:"frontiers"`
	Pending   *crawlerPendingView   `json:"pending"`
	DeckLeft  int                   `json:"deckLeft"`
	Bands     int                   `json:"bands"`
	BossNear  bool                  `json:"bossNear"`
	Log       []string              `json:"log"`
}

type crawlerPendingView struct {
	Kind     string             `json:"kind"`
	EntryDir int                `json:"entryDir"`
	Options  []crawlerLayOption `json:"options"`
}

func (g *crawlerGame) viewFor(seat int) crawlerView {
	v := crawlerView{
		T: MsgState, Phase: g.phase, Step: g.step, Turn: g.turn, You: seat,
		Dice: append([]crawlerDie{}, g.dice...), DeckLeft: len(g.deck) - g.deckPos,
		Bands: g.bands, Log: lastLog(g.log, 12),
		Heroes: []crawlerHeroView{}, Tiles: []crawlerTileView{}, Frontiers: []crawlerFrontierView{},
	}
	// Boss band reached once the final band is the only thing left in the stack.
	if g.phase == crPlaying && g.deckPos >= len(g.deck)-perBandSize() {
		v.BossNear = true
	}
	connected := map[int]bool{}
	for _, s := range g.out.Seats() {
		connected[s] = true
	}
	for _, s := range g.order {
		h := g.players[s]
		v.Heroes = append(v.Heroes, crawlerHeroView{
			S: s, N: h.name, Totem: h.sheet.totem, Health: h.health, MaxHP: h.sheet.maxHP,
			Courage: h.courage, Shields: h.shields, TileID: h.tileID, Square: h.square,
			Unlocked: h.unlocked, Alive: h.alive, Special: h.sheet.special, Gone: !connected[s],
		})
	}
	for _, t := range g.tiles {
		tv := crawlerTileView{ID: t.id, X: t.x, Y: t.y, Kind: t.kind, Edges: t.edges, IsBoss: t.isBoss, Monsters: []crawlerMonsterView{}}
		for i, m := range t.monsters {
			tv.Monsters = append(tv.Monsters, crawlerMonsterView{
				ID: i, Armour: m.armour, Health: m.health, MaxHP: m.maxHealth, Damage: m.damage,
				Square: m.square, Engaged: m.engaged, Alive: m.alive, IsBoss: m.isBoss,
			})
		}
		v.Tiles = append(v.Tiles, tv)
	}
	v.Frontiers = g.frontierViews()
	if g.pending != nil {
		v.Pending = &crawlerPendingView{Kind: g.pending.spec.kind, EntryDir: g.pending.entryDir, Options: append([]crawlerLayOption{}, g.pending.options...)}
	}
	return v
}

func perBandSize() int { return 4 }

func (g *crawlerGame) frontierViews() []crawlerFrontierView {
	seen := map[[2]int]bool{}
	var out []crawlerFrontierView
	for _, t := range g.tiles {
		for d := 0; d < 4; d++ {
			if !t.edges[d] {
				continue
			}
			nx, ny := t.x+crawlerDelta[d][0], t.y+crawlerDelta[d][1]
			if _, ok := g.at[[2]int{nx, ny}]; ok {
				continue
			}
			if seen[[2]int{nx, ny}] {
				continue
			}
			seen[[2]int{nx, ny}] = true
			out = append(out, crawlerFrontierView{X: nx, Y: ny})
		}
	}
	return out
}

func lastLog(xs []string, n int) []string {
	if len(xs) > n {
		xs = xs[len(xs)-n:]
	}
	return append([]string{}, xs...)
}

func (g *crawlerGame) broadcastViews() {
	for _, seat := range g.out.Seats() {
		g.out.Send(seat, g.viewFor(seat))
	}
}
