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
//   - Static enemies (Armour/Health/Damage); combat resolves at end of round.
//   - Courage (reroll, unlock two specials) and Shields.
//   - Win = boss defeated; lose = party wiped, or the stack runs out first.
//
// V0 simplifications (see the spec): a hero walks the tile's PATH only (the centre
// plus the doorway of each open edge; corners are off-path), so a straight is a
// corridor and a bend turns; there are no finer inner walls yet. The co-op view is
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
	crMsgStart    = "start"
	crMsgSpend    = "spend"   // {die, action, dir|monster|ally}
	crMsgLay      = "layTile" // {x, y, rotation} from the offered options
	crMsgReroll   = "reroll"  // {die}
	crMsgEndTurn  = "endTurn"
	crMsgEndLevel = "endLevel" // descend now, skipping the post-boss victory lap (current player only)
	crMsgRestart  = "restart"
	crMsgRetry    = "retryLevel" // host, after a loss: replay the current level from scratch (objects lost)
	crMsgStep     = "step"       // {dir} one bonus step of a multi-square move (no die)
	crMsgPortal   = "portal"     // {tile} teleport from a portal tile to another portal tile (no die)
	crMsgBuy      = "buy"        // {obj} buy an object from the shop you stand on (spends Courage)
	crMsgSell     = "sell"       // {obj} sell an object back to the shop for +1 Courage
	crMsgUse      = "use"        // {obj, monster} use a consumable (free action)
	crMsgLucky    = "lucky"      // {die} Lucky Charm: nudge a die +1 (free, once a turn)
	crMsgPickup   = "pickup"     // {square} take a ground object on your tile
	crMsgDrop     = "drop"       // {obj} drop an object onto your square
	crMsgTotem    = "totem"      // {totem} pick/change your animal while in the lobby
	crMsgRevive   = "revive"     // {ally} spend a die to revive a downed ally on your tile
	crMsgHeal     = "heal"       // spend 2 Courage to heal 1 (always available on your turn)
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
	crKindGate     = "gate"     // the broken city gate: L2 boss lair + L3 start
	crKindDescent  = "descent"  // the stairwell down to the machine rooms: L4 boss lair
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
	// Feature-tile shapes. IMPORTANT: each kind's open-edge COUNT must match its `base` in
	// TILE_ART (src/web/src/games/crawler/logic.ts) or the art can't rotate to fit and the
	// tile falls back to the plain cell. Keep the two tables in sync.
	crKindShrine:   {true, true, false, false},  // bend (a landmark)
	crKindGate:     {true, false, false, false}, // dead-end (the city gate)
	crKindDescent:  {true, false, false, false}, // dead-end (the stairwell down)
	crKindWiring:   {true, false, true, false},  // straight corridor of live floor
	crKindCoolant:  {true, true, true, true},    // open flooded room
	crKindFog:      {true, true, true, true},    // open fogged room
	crKindRust:     {true, true, true, true},    // open room (Ruins)
	crKindTrap:     {true, false, true, false},  // straight corridor (a street between tall buildings)
	crKindOverheat: {true, true, true, true},    // open room (Machine)
}

// Actions on the menu. Attack has no fixed die minimum: per the design, ANY die may
// be spent as an attack die and it lands a hit only if it meets the target monster's
// Armour (see tryAttack), so the threshold is the enemy, not the menu.
const (
	actMove    = "move"
	actAttack  = "attack"
	actCourage = "courage"
	actBrace   = "brace"  // Beaver special: 2 Shields to self or an ally on the tile
	actMend    = "mend"   // Treefrog special: heal 2 to a hero on your tile
	actCarry   = "carry"  // Comoros Weaver special: give an ally on your tile a 1-square move
	actPilfer  = "pilfer" // Raccoon (trickster) special: pull a ground item from an adjacent tile
	actGuard   = "guard"  // generic/placeholder special: 1 Shield to self
)

const crCourageCap = 10
const crSnareMoveDice = 2 // Snared: most dice a hero may spend on Move in a turn

// crGraceTurns is how many turns the party gets to defeat the boss after the tile
// stack runs out, before the dungeon collapses (a loss). The stack running out is no
// longer an instant loss.
const crGraceTurns = 5

// crPostBossTurns is how many turns EACH hero may take after the boss falls, before the
// level force-descends: a short victory lap to grab loot and regroup. The current player
// can also end the level early (crMsgEndLevel). See endTurn.
const crPostBossTurns = 2

// crMaxDieFace is the highest value any die can roll (every faces table tops out at 6).
// Effective Armour is capped at it so a 6 can ALWAYS land: without the cap a boss whose
// Armour climbs (Mossback's Adaptive Plating) or is buffed (Pylon's Jammer Field) could
// reach 7+, which no die can meet, making it permanently unkillable. Armour is never a
// full damage gate; the weak-point/immunity specials are the only hard gate.
const crMaxDieFace = 6

// Courage needed to unlock the two specials. Unlocking does NOT spend it; you just
// need to have earned this much (the "ready" threshold).
const (
	crUnlock0Courage = 3
	crUnlock1Courage = 5
)

// --- sheets -----------------------------------------------------------------

// --- state ------------------------------------------------------------------

type crawlerHero struct {
	name          string
	sheet         crawlerSheet
	health        int
	courage       int
	shields       int
	tileID        int
	square        int
	alive         bool
	downed        bool // at 0 Health but not dead: out of the fight until an ally revives them
	postBossTurns int  // turns this hero has ENDED since the boss fell (victory-lap cap, crPostBossTurns)
	volleyUsed    bool // Arrow Squid: Deep Volley spent this turn (once-a-turn, reset in beginTurn)
	electrified   bool // status: loses 1 health each round until cleared (boss Overload, fire)
	electrocuted  bool // status: -1 to each die value until cleared (Scrapling sting, live wiring)
	snared        bool // status: may spend at most 2 dice on Move this turn (Grappler, coolant)
	drained       bool // status: can't gain Courage until cleared (Jammer, boss)
	rusted        bool // status: one fewer die until cleared (rust tiles, Scrapling L5-6)
	moveDiceUsed  int  // dice spent on Move this turn (for the Snared cap); reset in beginTurn

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
	Bonus   bool   `json:"bonus,omitempty"` // an acquired attack die (Spare Die, Wolf Frenzy), drawn in a distinct colour
	// faces is the die's OWN distribution, set when it is rolled. A reroll draws from this, so a
	// High/bonus die stays High and a Low die stays Low without the reroll having to know where
	// the die came from (unexported: engine-only, never serialised). All dice are born via
	// rollDie, so this is always set.
	faces []int
}

type crawlerMonster struct {
	armour, health, maxHealth, damage int // health is CURRENT and persists across turns
	square                            int
	kind                              string // art/type: drone (weak) / brute (tough) / a boss slug
	engaged                           bool   // attacked this turn (retaliates at end of round)
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
	id          int
	band        int
	kind        string // straight / bend / tee / cross / dead-end / boss
	x, y        int
	rot         int // quarter-turns the chosen placement rotated the base shape (for art)
	edges       [4]bool
	monsters    []*crawlerMonster
	isBoss      bool
	loot        []crawlerGroundItem // objects lying on the floor of this tile, by square
	hazards     []crawlerHazard     // hazards on this tile's squares (see crawler_hazards.go)
	fogRevealed bool                // a sensor-fog tile whose contents a hero has uncovered (stays revealed)
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
	out  Outbox
	seed uint64 // the run's RNG seed, surfaced in the debug overlay
	rng  *rand.Rand

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

	// forcedBoss, when set from the debug start panel, pins the boss for the NEXT level
	// built to that slug instead of a random pick. It is consumed after that one level
	// (rollBossMonsters clears it), so later levels go back to a random draw. "" = random.
	forcedBoss string

	// defeatedBosses records the slug of every boss already beaten this run, so a beaten
	// boss is never drawn again (he is gone); excluded from each level's pool.
	defeatedBosses map[string]bool

	// debug is set when the run was started from the debug panel; it gates the extra
	// debug payload in the view (deck peek, state, spawn odds) that the client's F3-style
	// overlay reads. Off for a normal run, so players never see it.
	debug bool

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
		seed:    seed,
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

// setTotem rebuilds a seated hero's sheet for the chosen animal while in the lobby, so a
// player picks (or changes) their totem after joining rather than at the name screen. The
// sheet carries the health/dice/specials, so health is reset to the new sheet's max.
func (g *crawlerGame) setTotem(seat int, totem string) {
	h, ok := g.players[seat]
	if !ok {
		return
	}
	h.sheet = sheetFor(totem)
	h.health = h.sheet.maxHP
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
		T        string   `json:"t"`
		Die      int      `json:"die"`
		Action   string   `json:"action"`
		Dir      int      `json:"dir"`
		Monster  int      `json:"monster"`
		Tile     int      `json:"tile"` // target tile for a ranged attack (own tile if 0/omitted)
		Ally     int      `json:"ally"`
		X        int      `json:"x"`
		Y        int      `json:"y"`
		Rotation int      `json:"rotation"`
		Obj      string   `json:"obj"`    // object slug for buy/use/drop
		Totem    string   `json:"totem"`  // totem slug for the lobby animal picker
		Square   int      `json:"square"` // ground-item square for pickup
		Level    int      `json:"level"`  // start: which level to begin at (1..crMaxLevel)
		Boss     string   `json:"boss"`   // start (dev): force this boss slug, "" = random
		Loot     []string `json:"loot"`   // start (dev): grant these object slugs to each hero
		Debug    bool     `json:"debug"`  // start (dev): run in debug mode (enables the F3 overlay)
	}](raw)
	if !ok {
		return
	}
	if _, ok := g.players[seat]; !ok {
		return
	}

	// Per-room gameplay trace: emitted only once the room is started in debug, so a
	// normal room stays silent. vlog is always-on, so a debug-activated room is verbose
	// regardless of the quiet global level (that is what "activated per game" means).
	if g.debug {
		vlog.Debug("crawler command", "event", "cmd", "cmd", c.T, "seat", seat, "level", g.level, "turn", g.turn, "step", g.step)
	}

	switch c.T {
	case crMsgStart:
		if !isHost || g.phase != crLobby || len(g.players) < crMinPlayers {
			return
		}
		level := c.Level
		if level < 1 || level > crMaxLevel {
			level = 1
		}
		g.forcedBoss = c.Boss // "" = a random boss from the level's pool
		g.debug = c.Debug     // the debug overlay (deck peek, state, spawn odds) rides on this
		g.startRun(level, c.Loot)
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
	case crMsgSell:
		g.sellObject(seat, c.Obj)
	case crMsgUse:
		g.useObject(seat, c.Obj, c.Monster)
	case crMsgPickup:
		g.pickup(seat, c.Square)
	case crMsgDrop:
		g.dropObject(seat, c.Obj)
	case crMsgRevive:
		if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
			return
		}
		g.revive(seat, c.Ally)
	case crMsgHeal:
		if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
			return
		}
		g.courageHeal(seat)
	case crMsgEndTurn:
		if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
			return
		}
		g.endTurn(seat)
	case crMsgEndLevel:
		// Skip the post-boss victory lap and descend now. Only in the cleared state, and
		// only the current player (per the UI's placement in their turn-action row).
		if g.phase != crPlaying || !g.levelCleared || seat != g.turn {
			return
		}
		g.descendLevel()
	case crMsgTotem:
		if g.phase != crLobby {
			return // animal is locked once the run starts
		}
		g.setTotem(seat, c.Totem)
	case crMsgRestart:
		if !isHost || (g.phase != crWon && g.phase != crLost) {
			return
		}
		g.resetToLobby()
	case crMsgRetry:
		// Replay the current level from scratch after a loss: objects are lost and the party
		// starts the level fresh (startLevel refills health, resets Courage and rebuilds the map).
		if !isHost || g.phase != crLost {
			return
		}
		for _, s := range g.order {
			g.players[s].objects = nil
		}
		g.log = append(g.log, "The party tries Level "+strconv.Itoa(g.level)+" again, empty-handed.")
		g.startLevel(g.level)
	default:
		return
	}
	g.broadcastViews()
}

// --- setup ------------------------------------------------------------------

// startRun begins a run at the given level (1 normally; the host may pick another to
// jump straight to a biome). devLoot, when set from the debug start panel, grants those
// objects to every hero so gear can be tested from the off.
func (g *crawlerGame) startRun(level int, devLoot []string) {
	g.log = nil
	g.log = append(g.log, "You enter the dungeon.")
	g.defeatedBosses = map[string]bool{} // a fresh run: no boss beaten yet
	for _, s := range g.order {
		g.players[s].objects = nil // a fresh run starts with an empty pack
	}
	g.startLevel(level)
	for _, slug := range devLoot {
		if objBySlug(slug) == nil {
			continue
		}
		for _, s := range g.order {
			g.giveObject(g.players[s], slug)
		}
	}
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

	// Start tile at (0,0); heroes start on its centre. Normally the transporter (a
	// crossroads portal, one end of the teleport network). Level 3 instead opens on the
	// broken gate the party descends through: a one-door dead-end (the same tile that caps
	// the Level 2 boss lair), so the Ruins unfold from a single doorway.
	startKind, startEdges := crKindPortal, [4]bool{true, true, true, true}
	if n == 3 {
		startKind, startEdges = crKindGate, crSpecialBase[crKindGate]
	}
	start := g.placeTile(0, 0, startEdges, 0, startKind, false, nil)
	// Heroes start on the tile's centre, EXCEPT the Level 3 gate: there they emerge on the
	// city side (the single open-edge street square), facing into the ruins, not pinned
	// against the arch in the middle.
	startSq := 4
	if startKind == crKindGate {
		for d := 0; d < 4; d++ {
			if startEdges[d] {
				startSq = crawlerDoor[d]
				break
			}
		}
	}
	for _, s := range g.order {
		h := g.players[s]
		h.health = h.sheet.maxHP
		h.courage = 0
		h.shields = 0
		h.alive = true
		h.downed = false // a downed hero is back on their feet at the next level
		h.postBossTurns = 0
		h.volleyUsed = false
		heroClearStatuses(h)
		h.tileID = start.id
		h.square = startSq
	}
	g.phase = crPlaying
	g.log = append(g.log, "Level "+strconv.Itoa(n)+": the "+biomeLabel(g.biome)+".")
	g.beginLayPhase()
}

// canAct reports whether a hero can take a turn and fight: alive and not downed. A downed
// hero stays on the map (alive) but is skipped by turns and combat until an ally revives.
func canAct(h *crawlerHero) bool { return h.alive && !h.downed }

// actingHeroes counts the heroes who can still act (not downed, not dead); the party is
// lost only when this hits 0 (everyone is down at once, with nobody left to revive).
func (g *crawlerGame) actingHeroes() int {
	n := 0
	for _, s := range g.order {
		if canAct(g.players[s]) {
			n++
		}
	}
	return n
}

// firstAlive is the first hero who can act, in seat order (the hero each phase opens on).
func (g *crawlerGame) firstAlive() int {
	for _, s := range g.order {
		if canAct(g.players[s]) {
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
		if canAct(g.players[g.order[i]]) {
			return g.order[i]
		}
	}
	return -1
}

// beginLayPhase opens the LAY phase of a round: the first living hero lays a tile.
// Once the stack is empty there is nothing to lay, so the round skips straight to the
// spend phase and the collapse countdown runs instead.
func (g *crawlerGame) beginLayPhase() {
	// No laying once the boss lair is on the map (grace >= 0) or the stack is empty: the
	// party races the collapse countdown to the boss instead.
	if g.deckPos >= len(g.deck) || g.grace >= 0 {
		g.startGrace()
		g.beginSpendPhase()
		return
	}
	g.beginLayTurn(g.firstAlive())
}

// startGrace begins the "dungeon collapse" countdown: the party now has graceTurns()
// turns to reach and defeat the boss (on the map by then) before the run is lost, and no
// further tiles are laid. It fires the moment the boss lair is laid, or (as a fallback)
// when the tile stack runs out. A no-op if already counting down.
func (g *crawlerGame) startGrace() {
	if g.grace >= 0 {
		return
	}
	g.grace = g.graceTurns()
	g.log = append(g.log, "The boss lair is open! The dungeon starts to collapse: "+strconv.Itoa(g.grace)+" turns to reach and defeat the boss.")
}

// graceTurns is the collapse countdown length. A weak-point boss (Hauler, bsHardcore)
// must have its hidden weak-point hunted down and destroyed before it can be hurt at all,
// which burns turns, so it gets a longer countdown that scales DOWN with party size (more
// heroes clear the weak-point and close the distance faster): 10 solo, 7 at two, 6 at
// three, 5 at four or more. Every other boss uses the flat crGraceTurns.
func (g *crawlerGame) graceTurns() int {
	if !g.bossHasSpecial(bsHardcore) {
		return crGraceTurns
	}
	players := len(g.order)
	switch {
	case players <= 1:
		return 10
	case players == 2:
		return 7
	case players == 3:
		return 6
	default:
		return 5
	}
}

// bossHasSpecial reports whether a boss currently on the map carries the given signature
// special. Used to size the collapse countdown to the boss that was just laid.
func (g *crawlerGame) bossHasSpecial(s string) bool {
	for _, t := range g.tiles {
		for _, m := range t.monsters {
			if hasSpecial(m, s) {
				return true
			}
		}
	}
	return false
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
	if g.deckPos >= len(g.deck) || g.grace >= 0 {
		// No tile to lay (stack empty) or the boss lair is already on the map (grace
		// running): start/continue the collapse countdown and pass on. The remaining
		// heroes skip straight to the spend phase.
		g.startGrace()
		g.advanceLay(seat)
		return
	}
	spec, opts, ok := g.drawLayable(func(s tileSpec) []crawlerLayOption { return g.allLayOptions(s) })
	if !ok || len(opts) == 0 {
		// Nothing in the stack can be laid (every remaining forest tile was buried, or the
		// boss lair itself has no frontier): stop laying and run the collapse countdown.
		g.startGrace()
		g.advanceLay(seat)
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

// keepsForestOpen reports whether laying a tile with edges e at (x,y) would leave at
// least one open edge still facing an empty cell, i.e. a frontier for future tiles.
// It is the guard behind Mapmaking Rule 4: a non-boss tile is never laid in the one
// position that would seal the map shut (close the last opening). (x,y) is treated as
// already filled by this tile.
func (g *crawlerGame) keepsForestOpen(x, y int, e [4]bool) bool {
	// The new tile's own open edges onto an empty neighbour are fresh frontier.
	for d := 0; d < 4; d++ {
		if !e[d] {
			continue
		}
		ox, oy := x+crawlerDelta[d][0], y+crawlerDelta[d][1]
		if _, occupied := g.at[[2]int{ox, oy}]; !occupied {
			return true
		}
	}
	// Any already-placed tile's open edge onto a still-empty cell keeps the map open,
	// unless that cell is (x,y) itself (which this tile now fills).
	for _, t := range g.tiles {
		for d := 0; d < 4; d++ {
			if !t.edges[d] {
				continue
			}
			ox, oy := t.x+crawlerDelta[d][0], t.y+crawlerDelta[d][1]
			if ox == x && oy == y {
				continue
			}
			if _, occupied := g.at[[2]int{ox, oy}]; !occupied {
				return true
			}
		}
	}
	return false
}

// drawLayable finds the next tile that can actually be laid under `gen` (the placement
// generator, which already drops map-sealing placements via keepsForestOpen) and CYCLES it
// to the top (deckPos), shifting the skipped tiles right by one so NONE is lost: a forest
// tile that does not fit right now stays in the stack and is retried as the map grows. It
// prefers a non-boss tile (the forest keeps growing); only when no forest tile fits anywhere
// does it fall back to the boss (which ends the forest). ok is false when the stack is empty.
// It does NOT advance deckPos: the caller does that once it commits the draw.
func (g *crawlerGame) drawLayable(gen func(tileSpec) []crawlerLayOption) (tileSpec, []crawlerLayOption, bool) {
	for k := g.deckPos; k < len(g.deck); k++ {
		spec := g.deck[k]
		opts := gen(spec)
		if spec.isBoss || len(opts) > 0 {
			// The boss (kept in its deck position, it ends the forest) or the first tile
			// that fits SOMEWHERE on the frontier: cycle it to the top. Any skipped
			// unplaceable tiles stay in the stack (slid behind) and are retried next round.
			return g.cycleToTop(k), opts, true
		}
	}
	return tileSpec{}, nil, false
}

// cycleToTop moves the tile at deck position k to deckPos, sliding the tiles in between
// right by one (so the order is preserved and nothing is discarded), and returns it.
func (g *crawlerGame) cycleToTop(k int) tileSpec {
	spec := g.deck[k]
	copy(g.deck[g.deckPos+1:k+1], g.deck[g.deckPos:k])
	g.deck[g.deckPos] = spec
	return spec
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
			if !spec.isBoss && !g.keepsForestOpen(f[0], f[1], e) {
				continue // Mapmaking Rule 4: never lay the tile that seals the map (only the boss lair may end it)
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
	// Find the next NON-BOSS tile in the stack that actually fits this doorway and pull it
	// to the top, so moving to a new tile reliably spawns one: the top tile often doesn't
	// fit the specific cell you stepped through, and before, that silently blocked the move
	// ("it won't spawn a tile"). No tile is lost: the fitting tile is moved to deckPos and
	// the skipped ones keep their order; the boss is never laid by exploring.
	opts := g.drawFittingAt(dir, nx, ny)
	if len(opts) == 0 {
		return false // nothing in the stack fits this doorway (every option would seal or clash)
	}
	g.pending = &crawlerPending{spec: g.deck[g.deckPos], entryDir: dir, options: opts}
	g.deckPos++
	g.step = crStepLay
	return true
}

// drawFittingAt finds the first non-boss tile in the stack (from deckPos) that can be laid
// at the single frontier cell (nx,ny) entered from dir, pulls it to deckPos (shifting the
// skipped tiles right by one so none is lost), and returns its placement options. Returns
// nil when nothing in the stack fits this doorway.
func (g *crawlerGame) drawFittingAt(dir, nx, ny int) []crawlerLayOption {
	for k := g.deckPos; k < len(g.deck); k++ {
		if g.deck[k].isBoss {
			continue // the boss lair is laid in the lay phase / at the end, not by exploring
		}
		opts := g.layOptions(g.deck[k], dir, nx, ny)
		if len(opts) == 0 {
			continue
		}
		g.cycleToTop(k) // pull this fitting tile to the top; nothing is lost
		return opts
	}
	return nil
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
		if !spec.isBoss && !g.keepsForestOpen(nx, ny, e) {
			continue // Mapmaking Rule 4: never lay the tile that seals the map (only the boss lair may end it)
		}
		out = append(out, crawlerLayOption{X: nx, Y: ny, Rotation: r, Edges: e})
	}
	return out
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
	h.moveDiceUsed = 0
	// Thick Hide (Beaver, first special): start the turn with 2 Shields while it is
	// active, i.e. while Courage is at or above its threshold.
	if h.sheet.role == roleGuardian && h.specialActive(0) {
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
	if h.sheet.role == roleMelee && h.health < h.sheet.maxHP && g.allyOnTile(h) {
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
	// The pool is diceN base dice (the sheet's type) + attackDiceN innate High attack dice +
	// one High die per Spare Die; everything past diceN is a Bonus (distinct-colour) High die.
	n := h.sheet.diceN + h.sheet.attackDiceN + extra
	g.dice = make([]crawlerDie, n)
	for i := range g.dice {
		// Base dice roll on the sheet's faces; everything past diceN is a Bonus (distinct-colour)
		// High die (innate attack die / Spare Die). rollDie records each die's face set and
		// applies the Electrocuted penalty, so a later reroll follows suit.
		if i < h.sheet.diceN {
			g.dice[i] = g.rollDie(h, sheetFaces(h.sheet), false)
		} else {
			g.dice[i] = g.rollDie(h, diceHigh, true)
		}
	}
	// Rusted (status): lose one RANDOM die until cleared (floor of 1 so a turn is never empty),
	// rather than always dropping the last (which was the hero's High attack die).
	if h.rusted && len(g.dice) > 1 {
		i := g.rng.IntN(len(g.dice))
		g.dice = append(g.dice[:i], g.dice[i+1:]...)
	}
}

// electro applies the Electrocuted penalty (-1, floored at 1) to a freshly rolled die
// value, so the opening roll, the Spare Die, and rerolls are all reduced alike.
func electro(h *crawlerHero, v int) int {
	if h.electrocuted && v > 1 {
		return v - 1
	}
	return v
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
		// Snared (status): at most 2 dice may be spent on Move this turn (a softer leash
		// than the old "can't leave your tile").
		if h.snared && h.moveDiceUsed >= crSnareMoveDice {
			return
		}
		// First step now, in the chosen direction; a fast mover (moveRange > 1) then
		// gets the rest of its movement as directed bonus steps (crMsgStep), so one die
		// can turn a corner (e.g. left then up) instead of only going straight.
		if !g.tryMove(seat, dir) {
			return
		}
		h.moveDiceUsed++
		// A fast mover keeps its remaining steps even when this step LAID a new tile (which
		// leaves g.pending set): the bonus steps run once the lay completes, so exploring onto
		// a new tile does not swallow the whole die.
		if h.sheet.moveRange > 1 {
			g.bonusMoves = h.sheet.moveRange - 1
		}
	case actCarry: // give an ally on your tile a free 1-square move
		if !g.tryCarry(seat, ally, dir) {
			return
		}
	case actPilfer: // Raccoon: pull a ground item from the adjacent tile in dir
		if !g.pilfer(seat, dir) {
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
		if h.sheet.role == roleHealer && h.specialActive(0) {
			tgt := h
			if ally != seat {
				tgt = g.players[ally]
			}
			if tgt != nil && tgt.alive && tgt.tileID == h.tileID && g.clearHeroStatuses(tgt) {
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
		if h.sheet.role == roleHealer && h.specialActive(1) && !h.bloomUsed {
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
	isWeaver := src.sheet.role == roleCarrier
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
	g.maybeShop(ally)
	g.enterHazard(a)
	g.revealFog(a)
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
		if !crawlerWalkable(t.edges)[nsq] || centreBlocked(t, nsq) {
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
// firstUnspentDie returns the index of the first unspent die in the pool, or -1.
func (g *crawlerGame) firstUnspentDie() int {
	for i := range g.dice {
		if !g.dice[i].Spent {
			return i
		}
	}
	return -1
}

// revive spends one of the active hero's dice to bring a downed ally back on 1 Health. The
// ally must be downed and on the reviver's own tile (you pick them up where they lie).
func (g *crawlerGame) revive(seat, allySeat int) {
	h := g.players[seat]
	a, ok := g.players[allySeat]
	if !ok || a == h || !a.downed || a.tileID != h.tileID {
		return
	}
	di := g.firstUnspentDie()
	if di < 0 {
		return // reviving costs an action: an unspent die
	}
	g.dice[di].Spent = true
	g.dice[di].UsedFor = "revive"
	a.downed = false
	a.health = 2
	g.log = append(g.log, h.name+" revives "+a.name+", back on their feet (2 Health).")
	g.maybeAutoEnd(seat)
}

// courageHeal spends 2 Courage to heal 1, available to any hero on their turn (not role-
// gated, no die cost). Refused at full Health or below 2 Courage.
func (g *crawlerGame) courageHeal(seat int) {
	h := g.players[seat]
	if !canAct(h) || h.courage < 2 || h.health >= h.sheet.maxHP {
		return
	}
	h.courage -= 2
	h.health++
	g.log = append(g.log, h.name+" spends 2 Courage to patch up 1 Health.")
}

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
		if centreBlocked(t, nsq) {
			return false // the boss / merchant holds the centre; you work the tile from the sides
		}
		h.square = nsq
		g.maybeSpring(seat) // landed: a spring centre heals (the vault's loot sits on the floor, grabbed like any drop)
		g.maybeShop(seat)   // stepping onto a shop tile opens it (and repairs statuses)
		g.enterHazard(h)    // a hazard square bites on entry (wiring/mine)
		g.revealFog(h)      // reaching a fog tile's centre uncovers it for good
		return true
	}
	// Locked in the boss lair: once you are on the boss's tile you cannot leave it while the
	// boss lives — you fight it out from here (moving between the side squares is still fine).
	// Leaving opens up again once the boss falls (the post-boss victory lap). Exception: Hauler
	// (Hardened Core) is immune while its weak-point still stands elsewhere, so it must NOT lock
	// you in then (you would be unable to fight it AND unable to leave to destroy the weak-point);
	// the lock-in applies once the weak-point is gone and it can actually be fought.
	if t.isBoss && g.bossLocksHeroes(t) {
		return false
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
		g.maybeSpring(seat)
		g.maybeShop(seat)
		g.enterHazard(h)
		return true
	}
	// Frontier: draw a tile, lay it HERE (the player picks only the rotation), then
	// the move completes onto it (layTile).
	return g.beginLay(seat, dir, nx, ny)
}

func (g *crawlerGame) enterTile(seat, id int) {} // reserved: per-tile entry hooks

// bossOnSquare reports whether a living boss occupies square sq of tile t. Heroes may
// not stand there: the boss owns the centre of its lair, so the party fights it from the
// surrounding side squares.
func bossOnSquare(t *crawlerTile, sq int) bool {
	for _, m := range t.monsters {
		if m.alive && m.isBoss && m.square == sq {
			return true
		}
	}
	return false
}

// bossAliveOn reports whether a living boss stands anywhere on tile t (used to lock heroes
// into the lair until the boss falls).
func (g *crawlerGame) bossAliveOn(t *crawlerTile) bool {
	for _, m := range t.monsters {
		if m.alive && m.isBoss {
			return true
		}
	}
	return false
}

// bossLocksHeroes reports whether tile t's living boss currently traps heroes on it. True for a
// normal boss; FALSE for Hauler (Hardened Core) while its weak-point still stands, since it is
// immune then and the party must be free to leave and destroy the weak-point first.
func (g *crawlerGame) bossLocksHeroes(t *crawlerTile) bool {
	for _, m := range t.monsters {
		if m.alive && m.isBoss {
			if hasSpecial(m, bsHardcore) && g.weakPointAlive() {
				return false
			}
			return true
		}
	}
	return false
}

// centreBlocked reports whether a hero may NOT stand on square sq of tile t: a living boss
// holds its lair centre, and the kangaroo merchant holds the shop centre. In both cases the
// party works the tile from the surrounding squares, so stepping onto the centre is refused.
func centreBlocked(t *crawlerTile, sq int) bool {
	return bossOnSquare(t, sq) || (t.kind == crKindShop && sq == 4)
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
	if g.debug {
		vlog.Debug("crawler portal", "event", "portal", "seat", seat, "from", src.id, "to", dest.id)
	}
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
	if g.debug {
		vlog.Debug("crawler tile laid", "event", "lay", "kind", spec.kind, "x", chosen.X, "y", chosen.Y, "rot", chosen.Rotation, "boss", spec.isBoss, "seat", seat)
	}
	g.seedHazards(t)             // a hazard-room tile seeds its hazard on its squares (no-op otherwise)
	g.seedTileLoot(t, !phaseLay) // exploration lays only: a chance at loose floor consumables, by tile type
	if t.kind == crKindFog {
		g.populateFogRoom(t) // a fog room hides either a loot cache or an ambush
	}
	if t.kind == crKindVault {
		g.populateVault(t) // a loot cache: its consumable sits visibly on the floor
	}
	if spec.isBoss {
		g.startGrace() // the boss lair is on the map: no more tiles are laid, the countdown begins
		for _, m := range mons {
			if hasSpecial(m, bsHardcore) { // Hauler: expose the weak-point on a different tile
				g.placeWeakPoint(t.id)
				break
			}
		}
	}
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
	case h.sheet.role == roleTrickster && h.specialActive(0) && !h.pawsUsed:
		h.pawsUsed = true
	case h.courage >= 1:
		h.courage--
	default:
		return
	}
	// Re-roll on the die's OWN face set, so a High/bonus die stays High and a Low die stays Low;
	// the Electrocuted penalty follows the reroll too.
	g.dice[dieIdx].Value = electro(h, g.rollFrom(g.dice[dieIdx].faces))
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
	// The level's boss has fallen: play a short victory lap (crPostBossTurns turns each)
	// so the party can loot and regroup, instead of descending the instant the killer
	// ends their turn. Each End turn counts against the ending hero's cap; once every
	// still-acting hero has used theirs, the level descends. The current player may also
	// skip the lap with End level (crMsgEndLevel).
	if g.levelCleared {
		g.players[seat].postBossTurns++
		if next := g.nextPostBossPlayer(seat); next >= 0 {
			g.turn = next
			g.beginTurn(next)
			return
		}
		g.descendLevel()
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

// descendLevel ends the cleared level and builds the next one.
func (g *crawlerGame) descendLevel() {
	g.levelCleared = false
	g.startLevel(g.level + 1)
}

// nextPostBossPlayer returns the next hero (walking turn order from `from`) who can act
// and still has a post-boss turn left, or -1 when every acting hero has used their cap.
// It can return `from` again if they are the only one with turns remaining.
func (g *crawlerGame) nextPostBossPlayer(from int) int {
	n := len(g.order)
	if n == 0 {
		return -1
	}
	idx := 0
	for i, s := range g.order {
		if s == from {
			idx = i
			break
		}
	}
	for i := 1; i <= n; i++ {
		s := g.order[(idx+i)%n]
		if h := g.players[s]; canAct(h) && h.postBossTurns < crPostBossTurns {
			return s
		}
	}
	return -1
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
	g.forcedBoss = ""
	g.defeatedBosses = nil
	g.debug = false
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
		h.downed = false
		h.volleyUsed = false
		heroClearStatuses(h)
		h.objects = nil
	}
}

// --- view -------------------------------------------------------------------
