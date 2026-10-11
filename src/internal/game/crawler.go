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
	crMsgEndTurn = "endTurn"
	crMsgRestart = "restart"
	crMsgStep    = "step"   // {dir} one bonus step of a multi-square move (no die)
	crMsgPortal  = "portal" // {tile} teleport from a portal tile to another portal tile (no die)
)

// Special (non-combat) tile kinds placed into the deck. They carry no monster; their
// behaviour is their kind: portals teleport between each other, a healing spring heals
// a hero who steps onto its centre, the shop is a (V0-inert) vendor stall.
const (
	crKindPortal  = "portal"
	crKindHealing = "healing"
	crKindShop    = "shop"
)

// crSpringHeal is how much health a healing spring restores each time a hero steps onto
// its centre square. It is not consumed (the spring keeps healing on every fresh entry).
const crSpringHeal = 2

// Actions on the menu. Attack has no fixed die minimum: per the design, ANY die may
// be spent as an attack die and it lands a hit only if it meets the target monster's
// Armour (see tryAttack), so the threshold is the enemy, not the menu.
const (
	actMove    = "move"
	actAttack  = "attack"
	actCourage = "courage"
	actBrace   = "brace" // Beaver special: 2 Shields to self or an ally on the tile
	actMend    = "mend"  // Treefrog special: heal 2 to a hero on your tile
	actCarry   = "carry" // Comoros Weaver special: give an ally on your tile a 1-square move
	actGuard   = "guard" // generic/placeholder special: 1 Shield to self
)

const crCourageCap = 10

// crGraceTurns is how many turns the party gets to defeat the boss after the tile
// stack runs out, before the dungeon collapses (a loss). The stack running out is no
// longer an instant loss.
const crGraceTurns = 5

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
	totem     string
	maxHP     int
	diceN     int
	faces     []int // the die's weighted faces (the three types below); roll picks one
	menu      []crawlerMenuEntry
	special   string // the inherent special action's name (brace/mend/carry/guard), or "" for none
	moveRange int    // squares per Move die (1, or 2 for a fast mover)
	ranged    bool   // may attack a monster on an adjacent tile, not only its own
	unlock0   string // first unlock label
	unlock1   string // second unlock label
}

// The three die types from the design. Every number rolled stays useful: the sheet,
// not the face, decides what it can become. A High die meets Armour far more often; a
// Low die rarely attacks but feeds Move/Special.
var (
	diceLow      = []int{1, 2, 2, 3, 3, 6} // favours Move/Special, weak attack
	diceBalanced = []int{1, 2, 3, 4, 5, 6} // flexible, a plain d6
	diceHigh     = []int{2, 4, 5, 5, 6, 6} // favours Attack/Courage
)

// baseMenu is the default die-value map shared by every sheet, with the sheet's own
// special in the 2+ slot. Move is any die (>=1), the special >=2, Courage >=5, and
// Attack has no minimum (any die; the hit is decided by the monster's Armour).
func baseMenu(special string) []crawlerMenuEntry {
	return []crawlerMenuEntry{{actMove, 1}, {special, 2}, {actAttack, 1}, {actCourage, 5}}
}

// Beaver (Guardian): tanky, Low dice whose weak numbers become Shields.
func beaverSheet() crawlerSheet {
	return crawlerSheet{
		totem: "bever", maxHP: 7, diceN: 4, faces: diceLow, moveRange: 1,
		menu: baseMenu(actBrace), special: actBrace, unlock0: "Thick Hide", unlock1: "Bulwark",
	}
}

// Arrow Squid (Striker): fragile, High dice that meet Armour often. Its signature is
// RANGED damage: its Attack can reach a monster on an adjacent tile (ranged), so it has
// no active special-action slot (its edge is reach, plus the Courage unlocks below). The
// menu is the default map minus the Special entry.
func squidSheet() crawlerSheet {
	return crawlerSheet{
		totem: "pijlinktvis", maxHP: 4, diceN: 4, faces: diceHigh, moveRange: 1, ranged: true,
		menu:    []crawlerMenuEntry{{actMove, 1}, {actAttack, 1}, {actCourage, 5}},
		special: "", unlock0: "Barbed Shot", unlock1: "Deep Volley",
	}
}

// Treefrog (Healer): staying power, Low dice that become frequent Mends.
func treefrogSheet() crawlerSheet {
	return crawlerSheet{
		totem: "boomkikker", maxHP: 6, diceN: 4, faces: diceLow, moveRange: 1,
		menu: baseMenu(actMend), special: actMend, unlock0: "Spores", unlock1: "Bloom",
	}
}

// Raccoon (Trickster): 5 Balanced dice. Pilfer is a V0 placeholder (1 Shield) until
// the loot system exists; specials are spec'd.
func raccoonSheet() crawlerSheet {
	return crawlerSheet{
		totem: "wasbeer", maxHP: 5, diceN: 5, faces: diceBalanced, moveRange: 1,
		menu: baseMenu(actGuard), special: actGuard, unlock0: "Lucky Paws", unlock1: "Big Score",
	}
}

// Comoros Weaver (Carrier): 5 Low dice, the fast mover (2 squares per Move die).
// Carry gives an ally on your tile a free 1-square move.
func comorosSheet() crawlerSheet {
	return crawlerSheet{
		totem: "comorenwever", maxHP: 5, diceN: 5, faces: diceLow, moveRange: 2,
		menu: baseMenu(actCarry), special: actCarry, unlock0: "Ferry", unlock1: "Flock",
	}
}

// genericSheet is the fallback for totems without a bespoke design: a standard 4
// Balanced dice, Guard special.
func genericSheet(totem string) crawlerSheet {
	return crawlerSheet{
		totem: totem, maxHP: 5, diceN: 4, faces: diceBalanced, moveRange: 1,
		menu: baseMenu(actGuard), special: actGuard, unlock0: "Resolve", unlock1: "Second Wind",
	}
}

// sheetFor maps a totem slug to its design sheet, falling back to the generic one.
func sheetFor(totem string) crawlerSheet {
	switch totem {
	case "bever":
		return beaverSheet()
	case "pijlinktvis":
		return squidSheet()
	case "boomkikker":
		return treefrogSheet()
	case "wasbeer":
		return raccoonSheet()
	case "comorenwever":
		return comorosSheet()
	case "":
		return genericSheet("totem")
	default:
		return genericSheet(totem)
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
	name       string
	sheet      crawlerSheet
	health     int
	courage    int
	shields    int
	tileID     int
	square     int
	alive      bool
	volleyUsed bool // Arrow Squid: Deep Volley spent this turn (once-a-turn, reset in beginTurn)
}

// specialActive reports whether a Courage-unlocked special is currently active:
// specials auto-unlock at their Courage threshold and switch OFF again the moment
// Courage drops below it (there is no manual unlock). which 0 needs 3, which 1 needs 5.
func (h *crawlerHero) specialActive(which int) bool {
	need := crUnlock0Courage
	if which == 1 {
		need = crUnlock1Courage
	}
	return h.courage >= need
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

// tileSpec is one entry in the shuffled stack, drawn when a tile is laid. A tile may
// carry several monsters (a swarm); their squares are assigned when the tile is laid
// and its final edges (so its walkable squares) are known (see layTile).
type tileSpec struct {
	band     int
	kind     string
	base     [4]bool
	monsters []*crawlerMonster
	isBoss   bool
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

	// grace is the "dungeon collapse" countdown once the tile stack runs out: -1
	// while tiles remain, then crGraceTurns, decremented at each end of turn. The
	// party has that many turns to defeat the boss (which is on the map by then,
	// since every drawn tile has been laid) before the run is lost.
	grace int

	// bonusMoves is how many extra single-square steps the active hero may still take
	// from a multi-square move (a fast mover's Move die, e.g. the Comoros Weaver).
	// Each step is directed separately (crMsgStep), so a 2-move can go left then up, not
	// only straight. Reset at the start of each turn.
	bonusMoves int

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
	sheet := sheetFor(meta)
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
		Tile     int    `json:"tile"` // target tile for a ranged attack (own tile if 0/omitted)
		Ally     int    `json:"ally"`
		X        int    `json:"x"`
		Y        int    `json:"y"`
		Rotation int    `json:"rotation"`
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
		g.spendDie(seat, c.Die, c.Action, c.Dir, c.Monster, c.Tile, c.Ally)
	case crMsgLay:
		if g.phase != crPlaying || g.step != crStepLay || seat != g.turn {
			return
		}
		g.layTile(seat, c.X, c.Y, c.Rotation)
	case crMsgStep:
		if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
			return
		}
		g.bonusStep(seat, c.Dir)
	case crMsgPortal:
		if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
			return
		}
		g.usePortal(seat, c.Tile)
	case crMsgReroll:
		if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
			return
		}
		g.reroll(seat, c.Die)
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
	g.grace = -1
	g.buildDeck()

	// Start tile: the transporter (a crossroads portal) at (0,0), no monster; heroes
	// start on its centre. Being a portal, it is one end of the teleport network, so a
	// single portal drawn later makes a usable pair.
	start := g.placeTile(0, 0, [4]bool{true, true, true, true}, 0, crKindPortal, false, nil)
	for _, s := range g.order {
		h := g.players[s]
		h.health = h.sheet.maxHP
		h.courage = 0
		h.shields = 0
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
// Once the stack is empty there is nothing to lay, so the round skips straight to the
// spend phase and the collapse countdown runs instead.
func (g *crawlerGame) beginLayPhase() {
	if g.deckPos >= len(g.deck) {
		g.startGrace()
		g.beginSpendPhase()
		return
	}
	g.beginLayTurn(g.firstAlive())
}

// startGrace begins the "dungeon collapse" countdown the first time the tile stack
// runs out: the party now has crGraceTurns turns to defeat the boss (already on the
// map, since every drawn tile was laid) before the run is lost. A no-op if already
// counting down.
func (g *crawlerGame) startGrace() {
	if g.grace >= 0 {
		return
	}
	g.grace = crGraceTurns
	g.log = append(g.log, "The tile stack is empty! The dungeon starts to collapse: "+strconv.Itoa(crGraceTurns)+" turns to reach and defeat the boss.")
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
		// Stack emptied mid-phase: no tile to lay, start the collapse countdown and
		// pass on (the remaining heroes skip to the spend phase).
		g.startGrace()
		g.advanceLay(seat)
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
		// No tiles left to lay: the map can't grow, so the move off the edge is
		// blocked and the collapse countdown runs. Navigate what's already there.
		g.startGrace()
		return false
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
//
// This is THE place to tune tile mix and difficulty pacing: `perBand` and `g.bands`
// (set in startRun) size the stack and the boss clock; `crawlerTilePool` weights the
// tile shapes drawn; `rollMonster` decides the enemies (and swarms) per band. The boss
// stat line is built here too.
func (g *crawlerGame) buildDeck() {
	perBand := 4
	g.deck = nil
	g.deckPos = 0
	for b := 0; b < g.bands; b++ {
		band := make([]tileSpec, 0, perBand)
		for i := 0; i < perBand; i++ {
			tt := crawlerTilePool[g.rng.IntN(len(crawlerTilePool))]
			band = append(band, tileSpec{band: b, kind: tt.kind, base: tt.base, monsters: g.rollMonster(b)})
		}
		g.rng.Shuffle(len(band), func(i, j int) { band[i], band[j] = band[j], band[i] })
		// Special tiles, one per band, shuffled in at a random spot: a healing spring in
		// each outer band (two total), and in the middle band the second portal (paired
		// with the start tile) and the shop. They carry no monster; their kind is their
		// effect. Edge shapes: portal = crossroads, healing = T-junction, shop = dead-end.
		switch b {
		case 0:
			band = g.insertSpec(band, tileSpec{band: b, kind: crKindHealing, base: ttTee.base})
		case 1:
			band = g.insertSpec(band, tileSpec{band: b, kind: crKindPortal, base: ttCross.base})
			band = g.insertSpec(band, tileSpec{band: b, kind: crKindShop, base: ttDeadEnd.base})
		case g.bands - 1:
			band = g.insertSpec(band, tileSpec{band: b, kind: crKindHealing, base: ttTee.base})
		}
		if b == g.bands-1 {
			// Boss (a dead-end lair) shuffled into the last few of the final band.
			// Design: Armour 4, Damage 4, and Health that scales with party size
			// (~3 per hero) so the fight lasts about the same few rounds at any count,
			// rather than a flat value that a big party melts (see the spec).
			bossHP := 3 * len(g.order)
			if bossHP < 3 {
				bossHP = 3
			}
			boss := tileSpec{band: b, kind: "boss", base: ttDeadEnd.base, isBoss: true,
				monsters: []*crawlerMonster{{armour: 4, health: bossHP, maxHealth: bossHP, damage: 4, square: 4, kind: "boss", alive: true, isBoss: true, reward: 3}}}
			pos := len(band)
			if len(band) > 0 {
				pos = len(band) - g.rng.IntN(min2(len(band), 3)+1)
			}
			band = append(band[:pos], append([]tileSpec{boss}, band[pos:]...)...)
		}
		g.deck = append(g.deck, band...)
	}
}

// rollMonster rolls the enemies for one tile in a band: 0, 1 or a small swarm. A 3x3
// tile has at most five walkable squares (centre + up to four doorways), so a tile
// holds up to three robots comfortably; their squares are assigned when the tile is
// laid (layTile). Tougher types show up deeper in the stack.
//
// To tweak the enemy mix or add a swarm, this is the single place to look: edit the
// per-band weights and the slices returned. `mk` builds one robot (Armour, Health,
// Damage, Courage reward, art kind).
// insertSpec shuffles one special tile into a band at a random position.
func (g *crawlerGame) insertSpec(band []tileSpec, spec tileSpec) []tileSpec {
	pos := g.rng.IntN(len(band) + 1)
	out := make([]tileSpec, 0, len(band)+1)
	out = append(out, band[:pos]...)
	out = append(out, spec)
	out = append(out, band[pos:]...)
	return out
}

func (g *crawlerGame) rollMonster(band int) []*crawlerMonster {
	roll := g.rng.IntN(100)
	mk := func(a, h, d, reward int, kind string) *crawlerMonster {
		return &crawlerMonster{armour: a, health: h, maxHealth: h, damage: d, square: 4, kind: kind, alive: true, reward: reward}
	}
	// Archetypes (design stat lines + Courage reward scaled by toughness): drone =
	// Scrapling 3/1/1 reward 1, brute (mid bruiser) 4/2/3 reward 2, tank = Hulk 3/4/3
	// reward 2.
	drone := func() *crawlerMonster { return mk(3, 1, 1, 1, "drone") }
	brute := func() *crawlerMonster { return mk(4, 2, 3, 2, "brute") }
	tank := func() *crawlerMonster { return mk(3, 4, 3, 2, "tank") }
	switch band {
	case 0:
		switch {
		case roll < 45:
			return []*crawlerMonster{drone()} // a lone drone
		case roll < 55:
			return []*crawlerMonster{drone(), drone()} // early swarm
		}
	case 1:
		switch {
		case roll < 25:
			return []*crawlerMonster{drone()}
		case roll < 50:
			return []*crawlerMonster{brute()}
		case roll < 65:
			return []*crawlerMonster{tank()}
		case roll < 85:
			return []*crawlerMonster{drone(), drone()}
		}
	default:
		switch {
		case roll < 20:
			return []*crawlerMonster{brute()}
		case roll < 45:
			return []*crawlerMonster{tank()}
		case roll < 65:
			return []*crawlerMonster{brute(), drone()}
		case roll < 85:
			return []*crawlerMonster{drone(), drone(), drone()} // late scrap swarm
		}
	}
	return nil
}

// crawlerSquareOrder is the preference order for placing several monsters on a tile:
// the centre first, then each doorway (N, E, S, W). assignMonsterSquares drops each
// monster on the next free walkable square so a swarm spreads out instead of stacking.
var crawlerSquareOrder = [5]int{4, crawlerDoor[0], crawlerDoor[1], crawlerDoor[2], crawlerDoor[3]}

func assignMonsterSquares(edges [4]bool, mons []*crawlerMonster) {
	w := crawlerWalkable(edges)
	slots := make([]int, 0, 5)
	for _, sq := range crawlerSquareOrder {
		if w[sq] {
			slots = append(slots, sq)
		}
	}
	for i, m := range mons {
		if i < len(slots) {
			m.square = slots[i]
		} else {
			m.square = 4 // more monsters than squares (shouldn't happen): stack on centre
		}
	}
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
	g.bonusMoves = 0
	h.volleyUsed = false
	// Thick Hide (Beaver, first special): start the turn with 2 Shields while it is
	// active, i.e. while Courage is at or above its threshold.
	if h.sheet.totem == "bever" && h.specialActive(0) {
		h.shields += 2
	}
	g.dice = make([]crawlerDie, h.sheet.diceN)
	for i := range g.dice {
		g.dice[i] = crawlerDie{Value: g.rollFace(h.sheet)}
	}
}

// clearEngaged drops the "attacked" flag on every monster, done at the start of each
// round so a monster's attack animation shows for exactly the round after it struck.
func (g *crawlerGame) clearEngaged() {
	for _, t := range g.tiles {
		for _, m := range t.monsters {
			m.engaged = false
		}
	}
}

func (g *crawlerGame) spendDie(seat, dieIdx int, action string, dir, monsterIdx, tile, ally int) {
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
		// First step now, in the chosen direction; a fast mover (moveRange > 1) then
		// gets the rest of its movement as directed bonus steps (crMsgStep), so one die
		// can turn a corner (e.g. left then up) instead of only going straight.
		if !g.tryMove(seat, dir) {
			return
		}
		if h.sheet.moveRange > 1 && g.pending == nil {
			g.bonusMoves = h.sheet.moveRange - 1
		}
	case actCarry: // give an ally on your tile a free 1-square move
		if !g.tryCarry(seat, ally, dir) {
			return
		}
	case actAttack:
		if !g.tryAttack(seat, tile, monsterIdx, g.dice[dieIdx].Value) {
			return
		}
	case actBrace:
		if !g.grantShields(seat, ally, 2) {
			return
		}
	case actMend:
		if !g.grantHeal(seat, ally, 2) {
			return
		}
	case actGuard:
		h.shields++
	case actCourage:
		if h.courage >= crCourageCap {
			return // already at the cap: the die is not consumed, pick another action
		}
		h.courage++
	default:
		return
	}
	g.dice[dieIdx].Spent = true
	g.dice[dieIdx].UsedFor = action
	g.maybeAutoEnd(seat)
}

// bonusStep spends one of the active hero's remaining multi-move steps (from a fast
// mover's Move die) in a direction the player picks, so
// the two squares of a 2-move need not be a straight line. Each step is validated like
// a normal move (and may itself trigger a frontier tile lay).
func (g *crawlerGame) bonusStep(seat, dir int) {
	if g.bonusMoves <= 0 || g.pending != nil {
		return
	}
	if !g.tryMove(seat, dir) {
		return
	}
	g.bonusMoves--
	g.maybeAutoEnd(seat)
}

// tryCarry (Comoros Weaver's Carry) gives a living ally on the hero's own tile a
// single square move in dir, within that tile's path.
func (g *crawlerGame) tryCarry(seat, ally, dir int) bool {
	src := g.players[seat]
	a, ok := g.players[ally]
	if !ok || ally == seat || !a.alive || a.tileID != src.tileID || dir < 0 || dir > 3 {
		return false
	}
	t := g.tiles[a.tileID]
	col, row := a.square%3, a.square/3
	ncol, nrow := col+crawlerDelta[dir][0], row+crawlerDelta[dir][1]
	if ncol < 0 || ncol >= 3 || nrow < 0 || nrow >= 3 {
		return false // V0 Carry stays within the tile
	}
	nsq := nrow*3 + ncol
	if !crawlerWalkable(t.edges)[nsq] {
		return false
	}
	a.square = nsq
	g.maybeSpring(ally) // a carried ally also heals if set down on a spring centre
	g.log = append(g.log, src.name+" carries "+a.name+".")
	return true
}

// maybeAutoEnd ends the turn automatically once every die is spent (and the hero is
// not paused mid-lay), so a player with no dice left does not have to click End turn.
func (g *crawlerGame) maybeAutoEnd(seat int) {
	if g.phase != crPlaying || g.step != crStepSpend || g.pending != nil || seat != g.turn {
		return
	}
	if g.bonusMoves > 0 {
		return // a multi-move still has directed steps left; let the player use or skip them
	}
	for _, d := range g.dice {
		if !d.Spent {
			return
		}
	}
	g.endTurn(seat)
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
		g.maybeSpring(seat) // landed; a healing spring centre heals on entry
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

func (g *crawlerGame) enterTile(seat, id int) {} // reserved: per-tile entry hooks

// tryAttack applies one Attack die immediately: a die that meets the Armour reduces
// the monster's (persistent) Health, and a lethal blow kills it right away. The
// target is a monster on the hero's own tile, or, for a ranged totem (Arrow Squid),
// a monster on a tile reachable through an open edge of the hero's tile.
func (g *crawlerGame) tryAttack(seat, tileID, monsterIdx, value int) bool {
	h := g.players[seat]
	t := g.attackTarget(h, tileID)
	if t == nil || monsterIdx < 0 || monsterIdx >= len(t.monsters) {
		return false
	}
	m := t.monsters[monsterIdx]
	if !m.alive {
		return false
	}
	// A die below the monster's (effective) Armour cannot land, so it is not a legal
	// attack: refuse it rather than burning the die on a guaranteed miss. The player
	// spends low dice on Move/Special instead, which is the point of the die map.
	if value < g.effectiveArmour(h, m) {
		return false
	}
	m.engaged = true
	hits := 1
	// Arrow Squid, Deep Volley (second special): once a turn a 6 lands two hits.
	if h.sheet.totem == "pijlinktvis" && h.specialActive(1) && !h.volleyUsed && value >= 6 {
		hits = 2
		h.volleyUsed = true
		g.log = append(g.log, h.name+"'s Deep Volley strikes twice.")
	}
	m.health -= hits
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
	return true
}

// effectiveArmour is the die value an attack needs to land on m: the monster's Armour,
// lowered by 1 while the Arrow Squid's Barbed Shot (first special) is active (minimum
// 1). The client mirrors this so the Attack buttons it offers match what the server
// will accept.
func (g *crawlerGame) effectiveArmour(h *crawlerHero, m *crawlerMonster) int {
	a := m.armour
	if h.sheet.totem == "pijlinktvis" && h.specialActive(0) {
		a--
	}
	if a < 1 {
		a = 1
	}
	return a
}

// maybeSpring heals a hero the moment it lands on a healing spring's centre square. It
// costs nothing and is not consumed: stepping onto the centre again heals again (capped
// at the hero's max). Called from every landing (a normal move, a bonus step, a carried
// ally), so it fires on entry, not merely for standing there.
func (g *crawlerGame) maybeSpring(seat int) {
	h := g.players[seat]
	t := g.tiles[h.tileID]
	if t == nil || t.kind != crKindHealing || h.square != 4 || !h.alive {
		return
	}
	if h.health >= h.sheet.maxHP {
		return
	}
	h.health += crSpringHeal
	if h.health > h.sheet.maxHP {
		h.health = h.sheet.maxHP
	}
	g.log = append(g.log, h.name+" is healed by the spring.")
}

// usePortal teleports a hero standing on a portal tile's centre to the centre of
// another portal tile the player chose. Free (no die); it is a repositioning tool. The
// destination must be a different, placed portal tile.
func (g *crawlerGame) usePortal(seat, destID int) {
	h := g.players[seat]
	src := g.tiles[h.tileID]
	if src == nil || src.kind != crKindPortal || h.square != 4 {
		return
	}
	dest, ok := g.tiles[destID]
	if !ok || dest.id == src.id || dest.kind != crKindPortal {
		return
	}
	h.tileID = dest.id
	h.square = 4
	g.log = append(g.log, h.name+" steps through the portal.")
}

// attackTarget resolves which tile an attack lands on. tileID 0 or the hero's own
// tile means melee (your tile). Any other tile is allowed only for a ranged totem and
// only if it is directly reachable through an open edge of the hero's tile.
func (g *crawlerGame) attackTarget(h *crawlerHero, tileID int) *crawlerTile {
	own := g.tiles[h.tileID]
	if tileID == 0 || tileID == h.tileID || own == nil {
		return own
	}
	if !h.sheet.ranged {
		return nil
	}
	for d := 0; d < 4; d++ {
		if !own.edges[d] {
			continue
		}
		nx, ny := own.x+crawlerDelta[d][0], own.y+crawlerDelta[d][1]
		if id, ok := g.at[[2]int{nx, ny}]; ok && id == tileID {
			return g.tiles[id]
		}
	}
	return nil
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

// grantHeal (Treefrog's Mend) restores health to a hero on the acting hero's tile,
// capped at that hero's max. The target must be alive and sharing the tile.
func (g *crawlerGame) grantHeal(seat, ally, n int) bool {
	src := g.players[seat]
	tgt := src
	if ally != seat {
		a, ok := g.players[ally]
		if !ok || !a.alive || a.tileID != src.tileID {
			return false
		}
		tgt = a
	}
	if tgt.health >= tgt.sheet.maxHP {
		return false // already full: do not waste the die
	}
	tgt.health += n
	if tgt.health > tgt.sheet.maxHP {
		tgt.health = tgt.sheet.maxHP
	}
	g.log = append(g.log, src.name+" mends "+tgt.name+".")
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
	for _, sm := range spec.monsters {
		m := *sm // copy the template so each laid tile owns its monsters
		mons = append(mons, &m)
	}
	assignMonsterSquares(chosen.Edges, mons) // spread a swarm over the walkable squares
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
	g.maybeAutoEnd(seat) // the move that triggered this lay may have been the last die
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
	g.dice[dieIdx].Value = g.rollFace(h.sheet)
}

// diceTypeName labels a face set as low/balanced/high for the character sheet.
func diceTypeName(faces []int) string {
	sum := 0
	for _, f := range faces {
		sum += f
	}
	switch {
	case len(faces) == 0:
		return "balanced"
	case sum <= 17: // {1,2,2,3,3,6}=17
		return "low"
	case sum >= 28: // {2,4,5,5,6,6}=28
		return "high"
	default:
		return "balanced"
	}
}

// rollFace rolls one weighted die face for the sheet's die type (falls back to a
// plain d6 for a sheet with no faces set).
func (g *crawlerGame) rollFace(s crawlerSheet) int {
	if len(s.faces) == 0 {
		return 1 + g.rng.IntN(6)
	}
	return s.faces[g.rng.IntN(len(s.faces))]
}

func (g *crawlerGame) endTurn(seat int) {
	if g.phase != crPlaying {
		return
	}
	g.bonusMoves = 0 // any unused multi-move steps are forfeited when the turn ends
	if next := g.nextInOrder(seat); next >= 0 {
		// Mid-round: just pass the dice to the next hero. Monsters do not strike yet.
		g.turn = next
		g.beginTurn(g.turn)
		return
	}
	// Everyone has taken their turn: NOW the monsters strike (group fight), then the
	// collapse countdown ticks and a new round opens with its LAY phase.
	g.resolveRound()
	if g.phase != crPlaying {
		return
	}
	if g.grace > 0 {
		g.grace--
		if g.grace == 0 {
			g.phase = crLost
			g.log = append(g.log, "The dungeon collapses. You did not reach the boss in time.")
			return
		}
	}
	g.beginLayPhase()
}

// resolveRound is the end-of-ROUND damage step: once every hero has acted, every
// monster still alive strikes EVERY living hero sharing its tile for its Damage
// (reduced by that hero's Shields). This is the group fight: allies on a tile pool
// their attacks across the round (persistent monster Health) and share the retaliation
// at the end. The Beaver's Bulwark (when active) soaks an ally's hit onto himself.
// Shields are then cleared for everyone (a per-round resource), and the monster is
// flagged engaged so the client plays its attack animation.
func (g *crawlerGame) resolveRound() {
	g.clearEngaged() // drop last round's flags; this round's strikes set them afresh
	for _, t := range g.tiles {
		for _, m := range t.monsters {
			if !m.alive {
				continue
			}
			struck := false
			for _, s := range g.order {
				h := g.players[s]
				if !h.alive || h.tileID != t.id {
					continue
				}
				g.strike(m, h)
				struck = true
			}
			if struck {
				m.engaged = true // show the monster's attack pose next round
			}
		}
	}
	for _, s := range g.order {
		g.players[s].shields = 0
	}
	if g.aliveHeroes() == 0 {
		g.phase = crLost
		g.log = append(g.log, "The party has fallen. The dungeon wins.")
	}
}

// strike applies one monster's Damage to one hero, routing it to the Beaver's Bulwark
// if an ally would be hit on the Beaver's tile and the Beaver's Bulwark is active.
func (g *crawlerGame) strike(m *crawlerMonster, h *crawlerHero) {
	tgt := h
	if bw := g.bulwarkFor(h); bw != nil {
		tgt = bw
		g.log = append(g.log, bw.name+" shields "+h.name+" with Bulwark.")
	}
	dmg := m.damage - tgt.shields
	if tgt.shields > m.damage {
		tgt.shields -= m.damage
	} else {
		tgt.shields = 0
	}
	if dmg > 0 {
		tgt.health -= dmg
		g.log = append(g.log, tgt.name+" takes "+strconv.Itoa(dmg)+" damage.")
	}
	if tgt.health <= 0 {
		tgt.health = 0
		tgt.alive = false
		g.log = append(g.log, tgt.name+" falls.")
	}
}

// bulwarkFor returns the Beaver on hero h's tile whose Bulwark (second special) is
// active and who is not h, or nil. Such a Beaver takes an ally's hit instead.
func (g *crawlerGame) bulwarkFor(h *crawlerHero) *crawlerHero {
	for _, s := range g.order {
		b := g.players[s]
		if b == h || !b.alive || b.tileID != h.tileID {
			continue
		}
		if b.sheet.totem == "bever" && b.specialActive(1) {
			return b
		}
	}
	return nil
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
	g.grace = -1
	g.bonusMoves = 0
	for _, h := range g.players {
		h.health = h.sheet.maxHP
		h.courage = 0
		h.shields = 0
		h.alive = true
		h.volleyUsed = false
	}
}

// --- view -------------------------------------------------------------------

type crawlerMenuView struct {
	Action string `json:"action"`
	Min    int    `json:"min"`
}

type crawlerHeroView struct {
	S        int               `json:"s"`
	N        string            `json:"n"`
	Totem    string            `json:"totem"`
	Health   int               `json:"health"`
	MaxHP    int               `json:"maxHP"`
	Courage  int               `json:"courage"`
	Shields  int               `json:"shields"`
	TileID   int               `json:"tile"`
	Square   int               `json:"square"`
	Unlocked [2]bool           `json:"unlocked"` // specials currently ACTIVE (auto, by Courage)
	Alive    bool              `json:"alive"`
	Special  string            `json:"special"`
	Menu     []crawlerMenuView `json:"menu,omitempty"`     // sheet's action menu (only on your own hero)
	Unlock0  string            `json:"unlock0,omitempty"`  // first special's name
	Unlock1  string            `json:"unlock1,omitempty"`  // second special's name
	DiceType string            `json:"diceType,omitempty"` // low/balanced/high, for display
	Ranged   bool              `json:"ranged,omitempty"`   // attack can reach adjacent tiles
	Gone     bool              `json:"gone,omitempty"`
}

type crawlerMonsterView struct {
	ID      int    `json:"id"`
	Kind    string `json:"kind"`
	Armour  int    `json:"armour"`
	Health  int    `json:"health"` // current, persists across turns
	MaxHP   int    `json:"maxHP"`
	Damage  int    `json:"damage"`
	Reward  int    `json:"reward"` // Courage granted on defeat (scales with toughness)
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
	T          string                `json:"t"`
	Phase      string                `json:"ph"`
	Step       string                `json:"step"`
	Turn       int                   `json:"turn"`
	You        int                   `json:"you"`
	Dice       []crawlerDie          `json:"dice"`
	Heroes     []crawlerHeroView     `json:"heroes"`
	Tiles      []crawlerTileView     `json:"tiles"`
	Frontiers  []crawlerFrontierView `json:"frontiers"`
	Pending    *crawlerPendingView   `json:"pending"`
	DeckLeft   int                   `json:"deckLeft"`
	Bands      int                   `json:"bands"`
	BossNear   bool                  `json:"bossNear"`
	BonusMoves int                   `json:"bonusMoves"` // directed steps left in the active hero's multi-move
	Grace      int                   `json:"grace"`      // collapse countdown once the stack is empty; -1 while tiles remain
	Log        []string              `json:"log"`
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
		Bands: g.bands, Grace: g.grace, BonusMoves: g.bonusMoves, Log: lastLog(g.log, 12),
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
		hv := crawlerHeroView{
			S: s, N: h.name, Totem: h.sheet.totem, Health: h.health, MaxHP: h.sheet.maxHP,
			Courage: h.courage, Shields: h.shields, TileID: h.tileID, Square: h.square,
			Unlocked: [2]bool{h.specialActive(0), h.specialActive(1)}, Alive: h.alive, Special: h.sheet.special, Gone: !connected[s],
		}
		if s == seat { // the viewer's own sheet carries its menu, specials and die type
			hv.Menu = make([]crawlerMenuView, 0, len(h.sheet.menu))
			for _, m := range h.sheet.menu {
				hv.Menu = append(hv.Menu, crawlerMenuView{Action: m.Action, Min: m.Min})
			}
			hv.Unlock0, hv.Unlock1 = h.sheet.unlock0, h.sheet.unlock1
			hv.DiceType = diceTypeName(h.sheet.faces)
			hv.Ranged = h.sheet.ranged
		}
		v.Heroes = append(v.Heroes, hv)
	}
	for _, t := range g.tiles {
		tv := crawlerTileView{ID: t.id, X: t.x, Y: t.y, Kind: t.kind, Rot: t.rot, Edges: t.edges, IsBoss: t.isBoss, Monsters: []crawlerMonsterView{}}
		for i, m := range t.monsters {
			tv.Monsters = append(tv.Monsters, crawlerMonsterView{
				ID: i, Kind: m.kind, Armour: m.armour, Health: m.health, MaxHP: m.maxHealth, Damage: m.damage,
				Reward: m.reward, Square: m.square, Engaged: m.engaged, Alive: m.alive, IsBoss: m.isBoss,
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
