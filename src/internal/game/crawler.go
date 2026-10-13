package game

import (
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
	crMsgBuy     = "buy"    // {obj} buy an object from the shop you stand on (spends Courage)
	crMsgUse     = "use"    // {obj, monster} use a consumable (free action)
	crMsgLucky   = "lucky"  // {die} Lucky Charm: nudge a die +1 (free, once a turn)
	crMsgPickup  = "pickup" // {square} take a ground object on your tile
	crMsgDrop    = "drop"   // {obj} drop an object onto your square
)

// Special (non-combat) tile kinds placed into the deck. They carry no monster; their
// behaviour is their kind: portals teleport between each other, a healing spring heals
// a hero who steps onto its centre, the shop is a (V0-inert) vendor stall.
const (
	crKindPortal  = "portal"
	crKindHealing = "healing"
	crKindShop    = "shop"
	crKindVault   = "vault" // one-time loot cache (one guaranteed consumable), from Level 2
	// Level-2+ feature tiles (see the note's per-level tile table). The hazard rooms seed
	// a hazard on their squares when laid (see crawler_hazards.go seedHazards); the shrine
	// and gateway are cosmetic (no mechanical effect: level transitions are automatic).
	crKindShrine   = "shrine"   // a landmark (cosmetic), from Level 2
	crKindGate     = "gate"     // the broken city gate (woodland -> ruins), cosmetic
	crKindWiring   = "wiring"   // live-wiring hazard room (Woodland L2+)
	crKindCoolant  = "coolant"  // coolant-flood hazard room (Woodland L2+)
	crKindFog      = "fog"      // sensor-fog hazard room (Woodland L2+)
	crKindRust     = "rust"     // rust hazard room (Ruins L3+)
	crKindTrap     = "trap"     // trap/mine hazard room (Ruins L3+)
	crKindOverheat = "overheat" // overheat/fire hazard room (Machine L5+)
)

// crSpecialBase is each special tile's OWN edge shape (open edges N,E,S,W), so a kind
// owns its layout here rather than borrowing a base path type. The client's TILE_ART
// base for the same kind must describe this same shape (so the art rotates to match).
var crSpecialBase = map[string][4]bool{
	crKindPortal:  {true, true, true, true},    // crossroads: reachable from any side
	crKindHealing: {true, true, true, false},   // T-junction (3 doorways)
	crKindShop:    {true, false, false, false}, // dead-end nook
	crKindVault:   {true, false, false, false}, // dead-end nook
	// Hazard rooms are open rooms (reachable from every side so a hazard is a real
	// routing choice); the shrine and gate are open landmarks too.
	crKindShrine:   {true, true, true, true},
	crKindGate:     {true, true, true, true},
	crKindWiring:   {true, true, true, true},
	crKindCoolant:  {true, true, true, true},
	crKindFog:      {true, true, true, true},
	crKindRust:     {true, true, true, true},
	crKindTrap:     {true, true, true, true},
	crKindOverheat: {true, true, true, true},
}

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

// --- state ------------------------------------------------------------------

type crawlerHero struct {
	name         string
	sheet        crawlerSheet
	health       int
	courage      int
	shields      int
	tileID       int
	square       int
	alive        bool
	volleyUsed   bool // Arrow Squid: Deep Volley spent this turn (once-a-turn, reset in beginTurn)
	electrified  bool // status: loses 1 health each round until cleared (boss Overload, fire)
	electrocuted bool // status: -1 to each die value until cleared (Scrapling sting, live wiring)
	snared       bool // status: can't leave your tile until cleared (Grappler, coolant)
	drained      bool // status: can't gain Courage until cleared (Jammer, boss)
	rusted       bool // status: one fewer die until cleared (rust tiles, Scrapling L5-6)

	// Objects/loot (see crawler_objects.go): object slugs held, capped per totem by
	// heroSlots (the Beaver carries 2), carried ACROSS levels. Per-turn flags reset in
	// beginTurn.
	objects   []string
	luckyUsed bool // Lucky Charm nudge spent this turn
	gloveUsed bool // Gambler's Glove free reroll spent this turn
	pawsUsed  bool // Raccoon Lucky Paws free reroll spent this turn
	bloomUsed bool // Treefrog Bloom group-heal spent this turn (once a round = once per its turn)
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
	kind                              string // art/type: drone (weak) / brute (tough) / a boss slug
	engaged                           bool   // attacked this turn (retaliates at end of turn)
	alive                             bool
	isBoss                            bool
	reward                            int // Courage granted on defeat

	// Boss-only fields: a named character with one or more signature specials (see
	// crawler_bosses.go). Blank/empty for ordinary robots.
	bossName string
	specials []string
	// Per-special bookkeeping:
	plating   int  // Adaptive Plating: bonus Armour that climbs each round, reset by a 6
	enrage    int  // Enrage: bonus Damage that climbs each round it survives
	didSplit  bool // Split: already broke off its second core
	weakPoint bool // this monster is a Hardened Core boss's destroy-me-first weak point
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
	used     bool                // a one-time tile (the Vault) that has been looted
	loot     []crawlerGroundItem // objects lying on the floor of this tile, by square
	hazards  []crawlerHazard     // hazards on this tile's squares (see crawler_hazards.go)
}

// crawlerGroundItem is an object dropped on a tile's square (from a kill or a hero
// dropping it). It stays put until a hero standing on that tile picks it up.
type crawlerGroundItem struct {
	square int
	slug   string
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

	// Multi-level run: level 1..crMaxLevel across three biomes (Forest, Ruins,
	// Machine, two levels each). Each level draws its boss from the biome's pool and is
	// a fresh map; Courage resets and health refills between levels. levelCleared marks
	// that this level's boss just fell, so the next end-of-turn descends a level.
	level        int
	biome        string
	levelCleared bool

	// shopStock is the kangaroo shop's offer for this level (object slugs), rolled in
	// startLevel; a hero on a shop tile buys from it with Courage (see crawler_objects.go).
	shopStock []string

	// lootRoll is the most recent kill's drop roll, shown briefly by the client as a
	// visible die result, then cleared at the start of the next turn.
	lootRoll *crawlerLootRoll

	// statusRoll is the most recent status die (a robot's sting roll), shown briefly by
	// the client. Cleared at the start of each turn, like lootRoll.
	statusRoll *crawlerStatusRoll

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

// crawlerLootRoll records the most recent kill's drop roll so the client can show a
// visible die: the d100 result, the kind's drop chance, whether an item dropped, its
// name and who scored the kill. Cleared at the start of each turn.
type crawlerLootRoll struct {
	roll, chance int    // the d100 roll and the kind's drop chance (percent)
	hit          bool   // did an item drop
	name         string // the dropped object's name (if hit)
	killer       string // who scored the kill
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
	c, ok := decodeCmd[struct {
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
		Obj      string `json:"obj"`    // object slug for buy/use/drop
		Square   int    `json:"square"` // ground-item square for pickup
	}](raw)
	if !ok {
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
	case crMsgLucky:
		if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
			return
		}
		g.nudgeDie(seat, c.Die)
	case crMsgBuy:
		g.buyObject(seat, c.Obj)
	case crMsgUse:
		g.useObject(seat, c.Obj, c.Monster)
	case crMsgPickup:
		g.pickup(seat, c.Square)
	case crMsgDrop:
		g.dropObject(seat, c.Obj)
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
	g.log = nil
	g.log = append(g.log, "You enter the dungeon.")
	for _, s := range g.order {
		g.players[s].objects = nil // a fresh run starts with an empty pack
	}
	g.startLevel(1)
}

// startLevel builds a fresh map for level n (1..crMaxLevel) in its biome, with a boss
// drawn from that biome's pool. Courage resets and health refills between levels, and
// any gear/objects carry over (none exist in V0). The party regroups at full strength.
func (g *crawlerGame) startLevel(n int) {
	g.level = n
	g.biome = biomeForLevel(n)
	g.tiles = map[int]*crawlerTile{}
	g.at = map[[2]int]int{}
	g.nextID = 0
	g.pending = nil
	g.dice = nil
	g.bands = 3
	g.grace = -1
	g.bonusMoves = 0
	g.levelCleared = false
	g.lootRoll = nil
	g.rollShop() // the shop restocks each level
	g.buildDeck()

	// Start tile: the transporter (a crossroads portal) at (0,0); heroes start on its
	// centre. Being a portal, it is one end of the teleport network.
	start := g.placeTile(0, 0, [4]bool{true, true, true, true}, 0, crKindPortal, false, nil)
	for _, s := range g.order {
		h := g.players[s]
		h.health = h.sheet.maxHP
		h.courage = 0
		h.shields = 0
		h.alive = true
		h.volleyUsed = false
		heroClearStatuses(h)
		h.tileID = start.id
		h.square = 4
	}
	g.phase = crPlaying
	g.log = append(g.log, "Level "+strconv.Itoa(n)+": the "+biomeLabel(g.biome)+".")
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
		spec := func(kind string) tileSpec { return tileSpec{band: b, kind: kind, base: crSpecialBase[kind]} }
		switch b {
		case 0:
			band = g.insertSpec(band, spec(crKindHealing))
			// Statuses and hazards begin at Level 2: the first hazard room and a landmark
			// shrine come in here (see the note's per-level tile table).
			if g.level >= 2 {
				band = g.insertSpec(band, spec(crKindShrine))
				band = g.insertSpec(band, spec(crKindWiring))
			}
		case 1:
			band = g.insertSpec(band, spec(crKindPortal))
			band = g.insertSpec(band, spec(crKindShop))
			// The Vault (a dead-end loot cache) appears from Level 2 on (see the note's
			// per-level tile table). Future level-gated tiles slot in the same way.
			if g.level >= 2 {
				band = g.insertSpec(band, spec(crKindVault))
				band = g.insertSpec(band, spec(crKindCoolant))
			}
			if g.level >= 3 { // the Ruins add the rust hazard room
				band = g.insertSpec(band, spec(crKindRust))
			}
		case g.bands - 1:
			band = g.insertSpec(band, spec(crKindHealing))
			if g.level >= 2 {
				band = g.insertSpec(band, spec(crKindFog))
			}
			if g.level == 2 { // the broken city gate: the woodland -> ruins gateway
				band = g.insertSpec(band, spec(crKindGate))
			}
			if g.level >= 3 { // the Ruins add the trap room
				band = g.insertSpec(band, spec(crKindTrap))
			}
			if g.level >= 5 { // the Machine rooms add the overheat room
				band = g.insertSpec(band, spec(crKindOverheat))
			}
		}
		if b == g.bands-1 {
			// Boss (a dead-end lair) shuffled into the last few of the final band. The
			// boss is drawn from the current biome's pool (rollBoss); its stat line and
			// signature special live in crawler_bosses.go.
			boss := tileSpec{band: b, kind: "boss", base: ttDeadEnd.base, isBoss: true,
				monsters: g.rollBossMonsters()}
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
	// Robot archetypes, the note's "Enemies" stat table (A / H / D / Courage):
	scrapling := func() *crawlerMonster { return mk(3, 1, 1, 1, "scrapling") } // cheap swarm filler
	sentry := func() *crawlerMonster { return mk(5, 1, 3, 1, "sentry") }       // hard to hit, folds once you connect
	hulk := func() *crawlerMonster { return mk(3, 4, 3, 2, "hulk") }           // heavy, soak it, team it
	jammer := func() *crawlerMonster { return mk(3, 2, 1, 2, "jammer") }       // +1 Armour to other robots on its tile
	grappler := func() *crawlerMonster { return mk(4, 2, 1, 2, "grappler") }   // clamp can Snare
	deep := g.level >= 2                                                       // Jammers and Grapplers are parked for deeper levels
	switch band {
	case 0: // early tiles: empty or a lone Scrapling
		switch {
		case roll < 45:
			return []*crawlerMonster{scrapling()}
		case roll < 55:
			return []*crawlerMonster{scrapling(), scrapling()}
		}
	case 1: // mid tiles: a Sentry, a lone Hulk, a small swarm, or (deeper) a Grappler
		switch {
		case roll < 25:
			return []*crawlerMonster{scrapling()}
		case roll < 45:
			return []*crawlerMonster{sentry()}
		case roll < 60:
			return []*crawlerMonster{hulk()}
		case roll < 75 && deep:
			return []*crawlerMonster{grappler()}
		case roll < 90:
			return []*crawlerMonster{scrapling(), scrapling()}
		}
	default: // late tiles: Hulks, mixed packs, swarms, or (deeper) a Jammer to kill first
		switch {
		case roll < 20:
			return []*crawlerMonster{hulk()}
		case roll < 35 && deep:
			return []*crawlerMonster{jammer(), scrapling()}
		case roll < 50:
			return []*crawlerMonster{sentry(), scrapling()}
		case roll < 70:
			return []*crawlerMonster{hulk(), scrapling()}
		case roll < 90:
			return []*crawlerMonster{scrapling(), scrapling(), scrapling()} // late scrap swarm
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
	g.lootRoll = nil   // the previous kill's drop roll clears when the next hero acts
	g.statusRoll = nil // likewise the previous sting's status die
	h.volleyUsed = false
	h.luckyUsed = false
	h.gloveUsed = false
	h.pawsUsed = false
	h.bloomUsed = false
	// Thick Hide (Beaver, first special): start the turn with 2 Shields while it is
	// active, i.e. while Courage is at or above its threshold.
	if h.sheet.totem == "bever" && h.specialActive(0) {
		h.shields += 2
	}
	// Resolve / Second Wind (generic totems' unlocks): a small self-buff at the start of
	// your turn while active: Resolve (first) tops Shields to 1, Second Wind (second)
	// heals 1. Concrete placeholders so the generic sheet's unlocks are not inert.
	if h.sheet.unlock0 == "Resolve" && h.specialActive(0) && h.shields < 1 {
		h.shields = 1
	}
	if h.sheet.unlock1 == "Second Wind" && h.specialActive(1) && h.health < h.sheet.maxHP {
		h.health++
	}
	// Group strength (Wolf, signature): heal 1 at the start of your turn while a living
	// ally shares your tile (it wants the pack together, not scattered).
	if h.sheet.totem == "wolf" && h.health < h.sheet.maxHP && g.allyOnTile(h) {
		h.health++
	}
	// Plating (object): top Shields up to 2 at the start of your turn.
	if heroHas(h, "plating") && h.shields < 2 {
		h.shields = 2
	}
	// Spare Die (object): each one adds a High attacker die to the pool.
	extra := 0
	for _, o := range h.objects {
		if o == "spare-die" {
			extra++
		}
	}
	// Rusted (status): one fewer die until cleared (floor of 1 so a turn is never empty).
	n := h.sheet.diceN + extra
	if h.rusted && n > 1 {
		n--
	}
	g.dice = make([]crawlerDie, n)
	for i := range g.dice {
		if i < h.sheet.diceN {
			g.dice[i] = crawlerDie{Value: g.rollFace(h.sheet)}
		} else {
			g.dice[i] = crawlerDie{Value: diceHigh[g.rng.IntN(len(diceHigh))]}
		}
	}
	// Electrocuted (status): -1 to each die value (floored at 1) until cleared.
	if h.electrocuted {
		for i := range g.dice {
			if g.dice[i].Value > 1 {
				g.dice[i].Value--
			}
		}
	}
}

func (g *crawlerGame) spendDie(seat, dieIdx int, action string, dir, monsterIdx, tile, ally int) {
	h := g.players[seat]
	if dieIdx < 0 || dieIdx >= len(g.dice) || g.dice[dieIdx].Spent {
		return
	}
	minV, ok := h.sheet.min(action)
	// Harness (object): anyone may Carry an ally (>=2), even if their sheet has no Carry.
	if !ok && action == actCarry && heroHas(h, "harness") {
		minV, ok = 2, true
	}
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
		healed := g.grantHeal(seat, ally, 2)
		// Spores (Treefrog, first special): a Mend also clears the target's statuses. This
		// makes a Mend worthwhile on a full-health but afflicted ally, so the die counts
		// even when there was no health to restore.
		spored := false
		if h.sheet.totem == "boomkikker" && h.specialActive(0) {
			tgt := h
			if ally != seat {
				tgt = g.players[ally]
			}
			if tgt != nil && tgt.alive && tgt.tileID == h.tileID && heroClearStatuses(tgt) {
				spored = true
				g.log = append(g.log, h.name+"'s Spores cleanse "+tgt.name+".")
			}
		}
		if !healed && !spored {
			return
		}
		// Bloom (Treefrog, second special): once a round, a Mend also heals every other
		// hero on your tile 1. Its own turn is the only time it can Mend, so once-a-turn
		// is once-a-round here.
		if h.sheet.totem == "boomkikker" && h.specialActive(1) && !h.bloomUsed {
			h.bloomUsed = true
			g.bloomHeal(h)
		}
	case actGuard:
		h.shields++
	case actCourage:
		if h.drained {
			return // Drained (status): cannot gain Courage; the die is not consumed
		}
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
	// Ferry (Comoros Weaver, first special): the carried ally may cross a tile edge into
	// an already-placed neighbour, not only shuffle within the tile. Flock (second
	// special): the carry is TWO squares (a free extra step). Base Carry (or via the
	// Harness object on another totem) is one square within the tile.
	isWeaver := src.sheet.totem == "comorenwever"
	crossTiles := isWeaver && (src.specialActive(0) || src.specialActive(1))
	steps := 1
	if isWeaver && src.specialActive(1) {
		steps = 2
	}
	moved := false
	for i := 0; i < steps; i++ {
		if !g.carryStep(a, dir, crossTiles) {
			break
		}
		moved = true
	}
	if !moved {
		return false
	}
	g.maybeSpring(ally)
	g.maybeVault(ally)
	g.maybeShop(ally)
	g.enterHazard(a)
	g.log = append(g.log, src.name+" carries "+a.name+".")
	return true
}

// carryStep moves one hero a single square in dir. It stays on the tile unless
// crossTiles is set, in which case it may step through an open doorway into an
// already-placed neighbour tile (a carry never draws a new tile at a frontier).
func (g *crawlerGame) carryStep(a *crawlerHero, dir int, crossTiles bool) bool {
	t := g.tiles[a.tileID]
	col, row := a.square%3, a.square/3
	ncol, nrow := col+crawlerDelta[dir][0], row+crawlerDelta[dir][1]
	if ncol >= 0 && ncol < 3 && nrow >= 0 && nrow < 3 {
		nsq := nrow*3 + ncol
		if !crawlerWalkable(t.edges)[nsq] {
			return false
		}
		a.square = nsq
		return true
	}
	if !crossTiles || a.square != crawlerDoor[dir] || !t.edges[dir] {
		return false
	}
	nx, ny := t.x+crawlerDelta[dir][0], t.y+crawlerDelta[dir][1]
	id, ok := g.at[[2]int{nx, ny}]
	if !ok {
		return false // no tile there yet: a carry does not lay tiles
	}
	a.tileID = id
	a.square = crawlerDoor[crawlerOpp(dir)]
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
		g.maybeSpring(seat) // landed: a spring centre heals, a Vault centre yields loot
		g.maybeVault(seat)
		g.maybeShop(seat) // a shop centre repairs statuses
		g.enterHazard(h)  // a hazard square bites on entry (wiring/mine)
		return true
	}
	// Crossing the tile edge: must be at that side's doorway and the edge open. Snared
	// (status) pins a hero to their tile, so an edge crossing is refused until cleared.
	if h.snared || h.square != crawlerDoor[dir] || !t.edges[dir] {
		return false
	}
	nx, ny := t.x+crawlerDelta[dir][0], t.y+crawlerDelta[dir][1]
	if id, ok := g.at[[2]int{nx, ny}]; ok {
		h.square = crawlerDoor[crawlerOpp(dir)] // enter the neighbour's doorway
		h.tileID = id
		g.enterTile(seat, id)
		g.maybeSpring(seat)
		g.maybeVault(seat)
		g.maybeShop(seat)
		g.enterHazard(h)
		return true
	}
	// Frontier: draw a tile, lay it HERE (the player picks only the rotation), then
	// the move completes onto it (layTile).
	return g.beginLay(seat, dir, nx, ny)
}

func (g *crawlerGame) enterTile(seat, id int) {} // reserved: per-tile entry hooks

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
	g.seedHazards(t) // a hazard-room tile seeds its hazard on its squares (no-op otherwise)
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
	if dieIdx < 0 || dieIdx >= len(g.dice) || g.dice[dieIdx].Spent {
		return
	}
	// A free reroll comes from the Gambler's Glove (object) or the Raccoon's Lucky Paws
	// (first special, active by Courage): one each per turn. Otherwise a reroll costs 1
	// Courage. Spend the glove before Lucky Paws so both can be used in a turn.
	switch {
	case heroHas(h, "gamblers-glove") && !h.gloveUsed:
		h.gloveUsed = true
	case h.sheet.totem == "wasbeer" && h.specialActive(0) && !h.pawsUsed:
		h.pawsUsed = true
	case h.courage >= 1:
		h.courage--
	default:
		return
	}
	g.dice[dieIdx].Value = g.rollFace(h.sheet)
}

// nudgeDie is the Lucky Charm (object): once a turn, raise one unspent die by 1 (to a
// max face of 6). Rejected if the hero lacks the charm, has used it, or the die is 6.
func (g *crawlerGame) nudgeDie(seat, dieIdx int) {
	h := g.players[seat]
	if !heroHas(h, "lucky-charm") || h.luckyUsed {
		return
	}
	if dieIdx < 0 || dieIdx >= len(g.dice) || g.dice[dieIdx].Spent || g.dice[dieIdx].Value >= 6 {
		return
	}
	g.dice[dieIdx].Value++
	h.luckyUsed = true
}

func (g *crawlerGame) endTurn(seat int) {
	if g.phase != crPlaying {
		return
	}
	g.bonusMoves = 0 // any unused multi-move steps are forfeited when the turn ends
	// The level's boss fell this turn: descend to the next level now (a fresh map), so
	// the round does not continue in a cleared biome.
	if g.levelCleared {
		g.levelCleared = false
		g.startLevel(g.level + 1)
		return
	}
	// End-of-turn hazards bite the hero whose turn just ended if they are standing on one
	// (live wiring Electrocutes, fire deals 2 + Electrifies).
	g.endTurnHazards(g.players[seat])
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
	g.level = 0
	g.biome = ""
	g.levelCleared = false
	g.shopStock = nil
	g.lootRoll = nil
	g.statusRoll = nil
	for _, h := range g.players {
		h.health = h.sheet.maxHP
		h.courage = 0
		h.shields = 0
		h.alive = true
		h.volleyUsed = false
		heroClearStatuses(h)
		h.objects = nil
	}
}

// --- view -------------------------------------------------------------------
