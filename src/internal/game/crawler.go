package game

import (
	"encoding/json"
	"math/rand/v2"
	"strconv"
	"time"
)

// crawlerGame is Totem Crawler V0: an original co-op dungeon crawler (1-6) on the
// engine. It is turn-based and event-driven (TickHz 0). See
// docs/crawler-architecture.md for the full spec; this is the V0 slice:
//
//   - Numbered dice spent through a per-sheet action menu (each action has a
//     MINIMUM die value; a die pays for any action whose minimum it meets).
//   - A round is a LAY phase (every hero lays one tile anywhere legal on the
//     frontier, choosing cell + rotation) then a SPEND phase (dice turns).
//     Square-by-square movement on 3x3 tiles; stepping onto a frontier mid-spend
//     also lays a tile there (rotation only, the cell fixed by the step).
//   - Random banded tile stack (easy early, nasty late), boss in the final band.
//   - Static enemies (Armour/Health/Damage); combat resolves at end of turn.
//   - Courage (reroll, unlock two specials) and Shields.
//   - Win = boss defeated; lose = party wiped, or the stack runs out first.
//
// V0 simplifications (see the spec): a hero walks the tile's PATH only (the centre
// plus the doorway of each open edge; corners are off-path), so a straight is a
// corridor and a bend turns; there are no finer inner walls yet. One monster per
// monster-tile sits on the centre square, there are no loot items or hazards yet,
// and combat is the active hero alone (group fights are V1). The co-op view is
// shared (viewFor ignores the seat) but kept per-seat for future private info.

// Directions: N,E,S,W.
var crawlerDelta = [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

// doorway is the 3x3 square index (row-major, 0..8) in the middle of each side.
var crawlerDoor = [4]int{1, 5, 7, 3}

func crawlerOpp(d int) int { return (d + 2) % 4 }

// crawlerWalkable is the set of 3x3 squares a hero may stand on within a tile:
// the centre, plus the doorway square of each OPEN edge. The four corners are
// never walkable, so movement follows the tile's path. On a straight the only
// squares are the two doorways and the centre (a corridor); a bend turns; a
// cross opens all four arms. This is what stops a hero wandering to the side of
// a straight tile.
func crawlerWalkable(edges [4]bool) [9]bool {
	var w [9]bool
	w[4] = true
	for d := 0; d < 4; d++ {
		if edges[d] {
			w[crawlerDoor[d]] = true
		}
	}
	return w
}

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

// A round has two phases that each sweep the party once in seat order:
//   - LAY phase (step crStepLay): every living hero draws one tile and lays it
//     anywhere legal on the frontier, choosing the cell and the rotation.
//   - SPEND phase (step crStepSpend): every living hero takes a dice turn; moving
//     onto a frontier draws and lays a tile there too (rotation only, the cell is
//     fixed by the step), then steps onto it. endTurn after the last hero starts a
//     new round at the LAY phase.
//
// crStepLay therefore covers both the proactive lay (whole LAY phase) and the
// transient move-triggered lay inside a spend turn; crawlerPending.phaseLay tells
// them apart.
const (
	crStepSpend = "spend"
	crStepLay   = "lay" // a drawn tile awaits the player's rotation
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

// Courage needed to unlock the two specials. Unlocking does NOT spend it; you just
// need to have earned this much (the "ready" threshold).
const (
	crUnlock0Courage = 3
	crUnlock1Courage = 5
)

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
	kind                              string // art/type: drone (weak) / brute (tough) / boss
	engaged                           bool   // attacked this turn (retaliates at end of turn)
	alive                             bool
	isBoss                            bool
	reward                            int // Courage granted on defeat
}

type crawlerTile struct {
	id       int
	band     int
	kind     string // straight / bend / tee / cross / dead-end / boss
	x, y     int
	rot      int // quarter-turns the chosen placement rotated the base shape (for art)
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
	// phaseLay marks a proactive lay in the LAY phase (the hero chooses any legal
	// cell and rotation on the frontier and does NOT move onto it) versus a
	// move-triggered lay in the SPEND phase (the cell is fixed by where the hero
	// stepped, rotation only, and the move completes onto it). This flag decides what
	// happens once the tile is placed (see layTile).
	phaseLay bool
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
	g.log = append(g.log, "You enter the dungeon.")
	g.beginLayPhase()
}

// firstAlive is the first living hero in seat order (the hero each phase opens on).
func (g *crawlerGame) firstAlive() int {
	for _, s := range g.order {
		if g.players[s].alive {
			return s
		}
	}
	return g.order[0]
}

// nextInOrder is the next living hero AFTER `from` in seat order, or -1 when `from`
// is the last living hero: a phase sweeps the party once, so -1 means the phase is
// done (switch LAY -> SPEND, or SPEND -> a new round's LAY).
func (g *crawlerGame) nextInOrder(from int) int {
	fi := -1
	for i, s := range g.order {
		if s == from {
			fi = i
			break
		}
	}
	for i := fi + 1; i < len(g.order); i++ {
		if g.players[g.order[i]].alive {
			return g.order[i]
		}
	}
	return -1
}

// beginLayPhase opens the LAY phase of a round: the first living hero lays a tile.
func (g *crawlerGame) beginLayPhase() {
	g.beginLayTurn(g.firstAlive())
}

// beginLayTurn hands the LAY phase to `seat`: it draws the next tile and offers
// EVERY legal placement of it on the frontier (any cell, any rotation that fits the
// neighbours and joins the map). The player chooses both the cell and the rotation,
// so tiles can be laid where their doorways line up. If nothing legal fits anywhere
// the hero is skipped; an empty stack ends the run.
func (g *crawlerGame) beginLayTurn(seat int) {
	g.turn = seat
	g.step = crStepLay
	g.dice = nil
	if g.deckPos >= len(g.deck) {
		g.phase = crLost
		g.log = append(g.log, "The tile stack is empty and the boss still lurks. The dungeon claims you.")
		return
	}
	spec := g.deck[g.deckPos]
	opts := g.allLayOptions(spec)
	if len(opts) == 0 {
		g.advanceLay(seat) // nowhere legal to lay: pass to the next hero
		return
	}
	g.deckPos++
	g.pending = &crawlerPending{spec: spec, options: opts, phaseLay: true}
}

// advanceLay moves the LAY phase to the next living hero, or begins the SPEND phase
// once every hero has laid.
func (g *crawlerGame) advanceLay(seat int) {
	if next := g.nextInOrder(seat); next >= 0 {
		g.beginLayTurn(next)
		return
	}
	g.beginSpendPhase()
}

// beginSpendPhase opens the SPEND phase: the first living hero takes a dice turn.
func (g *crawlerGame) beginSpendPhase() {
	g.pending = nil
	g.turn = g.firstAlive()
	g.beginTurn(g.turn)
}

// connects reports whether a tile with edges e at (x,y) joins the existing map on at
// least one side: a shared OPEN-OPEN edge with a placed neighbour. Combined with
// edgesFit (no open-vs-closed mismatches), this keeps a laid tile both consistent
// with its neighbours and actually reachable, never dropped in an isolated pocket.
func (g *crawlerGame) connects(x, y int, e [4]bool) bool {
	for d := 0; d < 4; d++ {
		if !e[d] {
			continue
		}
		ox, oy := x+crawlerDelta[d][0], y+crawlerDelta[d][1]
		if id, ok := g.at[[2]int{ox, oy}]; ok && g.tiles[id].edges[crawlerOpp(d)] {
			return true
		}
	}
	return false
}

// allLayOptions is the proactive (LAY-phase) placement set: every frontier cell and
// every rotation of spec that fits the neighbours (edgesFit) and joins the map
// (connects). The player picks the cell and the rotation, so a tile can be laid
// where its doorways line up with the paths already on the board.
func (g *crawlerGame) allLayOptions(spec tileSpec) []crawlerLayOption {
	var out []crawlerLayOption
	for _, f := range g.frontierCoords() {
		for r := 0; r < 4; r++ {
			e := rotateEdges(spec.base, r)
			if !g.edgesFit(f[0], f[1], e) || !g.connects(f[0], f[1], e) {
				continue
			}
			out = append(out, crawlerLayOption{X: f[0], Y: f[1], Rotation: r, Edges: e})
		}
	}
	return out
}

// beginLay draws the next tile when a hero steps onto a frontier during the SPEND
// phase and offers the orientations (rotations) that fit at THAT cell. The player
// chooses the rotation only; the location is fixed (it is where they stepped). Once
// laid, the move completes onto the new tile (see layTile).
func (g *crawlerGame) beginLay(seat, dir, nx, ny int) bool {
	if g.deckPos >= len(g.deck) {
		g.phase = crLost
		g.log = append(g.log, "The tile stack is empty and the boss still lurks. The dungeon claims you.")
		return true
	}
	spec := g.deck[g.deckPos]
	opts := g.layOptions(spec, dir, nx, ny)
	if len(opts) == 0 {
		return false // no legal placement at this frontier: the move is blocked
	}
	g.deckPos++
	g.pending = &crawlerPending{spec: spec, entryDir: dir, options: opts}
	g.step = crStepLay
	return true
}

// layOptions returns every rotation of spec legal at the single frontier cell
// (nx,ny) entered from `dir`: it must open on the entry side (so the hero can step
// in, which also guarantees the path connects) and match any placed neighbours.
func (g *crawlerGame) layOptions(spec tileSpec, dir, nx, ny int) []crawlerLayOption {
	entry := crawlerOpp(dir)
	var out []crawlerLayOption
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
				monster: &crawlerMonster{armour: 5, health: 4, maxHealth: 4, damage: 4, square: 4, kind: "boss", alive: true, isBoss: true, reward: 3}}
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
	mk := func(a, h, d int, kind string) *crawlerMonster {
		return &crawlerMonster{armour: a, health: h, maxHealth: h, damage: d, square: 4, kind: kind, alive: true, reward: 1}
	}
	switch band {
	case 0:
		if roll < 50 {
			return mk(3, 1, 1, "drone") // the weakest enemy
		}
	case 1:
		if roll < 70 {
			return mk(4, 2, 2, "brute") // tougher second-tier enemy
		}
	default:
		if roll < 80 {
			return mk(4, 2, 3, "brute")
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

// tryMove steps one square in direction dir; crossing an open edge enters an
// already-placed neighbour tile, or, at a frontier, draws and lays the next tile
// there (the player picks the rotation) and then steps onto it.
func (g *crawlerGame) tryMove(seat, dir int) bool {
	if dir < 0 || dir > 3 {
		return false
	}
	h := g.players[seat]
	t := g.tiles[h.tileID]
	col, row := h.square%3, h.square/3
	ncol, nrow := col+crawlerDelta[dir][0], row+crawlerDelta[dir][1]
	if ncol >= 0 && ncol < 3 && nrow >= 0 && nrow < 3 {
		nsq := nrow*3 + ncol
		if !crawlerWalkable(t.edges)[nsq] {
			return false // off the tile's path: only the centre + open doorways are walkable
		}
		h.square = nsq
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
	// Frontier: draw a tile, lay it HERE (the player picks only the rotation), then
	// the move completes onto it (layTile).
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

// edgesFit checks a candidate tile's edges against every already-placed
// neighbour: a shared side must match (both open or both closed). With the
// entry-side opening required by layOptions, this also guarantees the path
// connects (you can never lay a tile whose path does not join the one you came
// from).
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

// frontierCoords are the empty cells adjacent to at least one placed tile's open
// edge: the only cells a new tile may be laid on.
func (g *crawlerGame) frontierCoords() [][2]int {
	seen := map[[2]int]bool{}
	var out [][2]int
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
			out = append(out, [2]int{nx, ny})
		}
	}
	return out
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
	entryDir := g.pending.entryDir
	phaseLay := g.pending.phaseLay
	var mons []*crawlerMonster
	if spec.monster != nil {
		m := *spec.monster
		mons = []*crawlerMonster{&m}
	}
	t := g.placeTile(chosen.X, chosen.Y, chosen.Edges, spec.band, spec.kind, spec.isBoss, mons)
	t.rot = chosen.Rotation
	h := g.players[seat]
	g.pending = nil
	if phaseLay {
		// Proactive LAY phase: the tile joins the map next to the hero, who stays put
		// (movement is for the spend phase). Pass to the next hero's lay turn.
		if spec.isBoss {
			g.log = append(g.log, h.name+" uncovers the boss lair!")
		} else {
			g.log = append(g.log, h.name+" lays a "+spec.kind+" tile.")
		}
		g.advanceLay(seat)
		return
	}
	// Move-triggered lay (spend phase): complete the move onto the new tile's doorway.
	h.tileID = t.id
	h.square = crawlerDoor[crawlerOpp(entryDir)]
	if spec.isBoss {
		g.log = append(g.log, h.name+" uncovers the boss lair!")
	} else {
		g.log = append(g.log, h.name+" lays a "+spec.kind+" tile and steps through.")
	}
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
		return // the second unlock still needs the first
	}
	need := crUnlock0Courage
	if which == 1 {
		need = crUnlock1Courage
	}
	if h.courage < need {
		return // you must have earned the Courage to unlock, though it is NOT spent
	}
	h.unlocked[which] = true
}

func (g *crawlerGame) endTurn(seat int) {
	g.resolveCombat(seat)
	// Shields are a per-turn resource: whatever you had or gained this turn (Thick
	// Hide's auto-shields, Brace, Guard) is cleared now, so they never stack across
	// turns. Thick Hide re-grants its 2 at the start of your next turn.
	g.players[seat].shields = 0
	if g.phase != crPlaying {
		return
	}
	if g.aliveHeroes() == 0 {
		g.phase = crLost
		g.log = append(g.log, "The party has fallen. The dungeon wins.")
		return
	}
	if next := g.nextInOrder(seat); next >= 0 {
		g.turn = next
		g.beginTurn(g.turn)
		return
	}
	// Everyone has spent: the round is over, so a fresh round opens with its LAY phase.
	g.beginLayPhase()
}

// resolveCombat is the end-of-turn damage step: every monster still alive on the
// active hero's tile strikes for its Damage (reduced by Shields), whether or not
// the hero attacked it. Kills happen immediately in tryAttack and a monster's lost
// Health persists, so a tough enemy is whittled down over successive turns; but
// until it is dead it keeps hurting whoever ends their turn on its tile.
func (g *crawlerGame) resolveCombat(seat int) {
	h := g.players[seat]
	t := g.tiles[h.tileID]
	if t == nil {
		return
	}
	for _, m := range t.monsters {
		if !m.alive {
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
			g.log = append(g.log, h.name+" takes "+strconv.Itoa(dmg)+" damage.")
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
	ID      int    `json:"id"`
	Kind    string `json:"kind"`
	Armour  int    `json:"armour"`
	Health  int    `json:"health"` // current, persists across turns
	MaxHP   int    `json:"maxHP"`
	Damage  int    `json:"damage"`
	Square  int    `json:"square"`
	Engaged bool   `json:"engaged"`
	Alive   bool   `json:"alive"`
	IsBoss  bool   `json:"isBoss"`
}

type crawlerTileView struct {
	ID       int                  `json:"id"`
	X        int                  `json:"x"`
	Y        int                  `json:"y"`
	Kind     string               `json:"kind"`
	Rot      int                  `json:"rot"`
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
	PhaseLay bool               `json:"phaseLay"` // true = proactive LAY phase, false = move-triggered
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
		tv := crawlerTileView{ID: t.id, X: t.x, Y: t.y, Kind: t.kind, Rot: t.rot, Edges: t.edges, IsBoss: t.isBoss, Monsters: []crawlerMonsterView{}}
		for i, m := range t.monsters {
			tv.Monsters = append(tv.Monsters, crawlerMonsterView{
				ID: i, Kind: m.kind, Armour: m.armour, Health: m.health, MaxHP: m.maxHealth, Damage: m.damage,
				Square: m.square, Engaged: m.engaged, Alive: m.alive, IsBoss: m.isBoss,
			})
		}
		v.Tiles = append(v.Tiles, tv)
	}
	v.Frontiers = g.frontierViews()
	if g.pending != nil {
		v.Pending = &crawlerPendingView{Kind: g.pending.spec.kind, EntryDir: g.pending.entryDir, Options: append([]crawlerLayOption{}, g.pending.options...), PhaseLay: g.pending.phaseLay}
	}
	return v
}

func perBandSize() int { return 4 }

func (g *crawlerGame) frontierViews() []crawlerFrontierView {
	out := []crawlerFrontierView{}
	for _, f := range g.frontierCoords() {
		out = append(out, crawlerFrontierView{X: f[0], Y: f[1]})
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
