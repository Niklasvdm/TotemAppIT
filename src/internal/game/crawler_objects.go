package game

import "strconv"

// Loot and objects for Totem Crawler (see crawler.go and the design note's "Loot and
// objects" section). Everything is buyable at the kangaroo's shop for Courage;
// consumables also drop from defeated robots. Objects carry across levels, Courage does
// not. Permanents are passives wired into the turn/combat code; consumables are one-use
// actions. Costs, effects and drop weights are the note's values (placeholders to tune).

const crObjectCapDefault = 3 // object slots when a sheet does not set its own

// heroSlots is how many objects a hero can carry: its sheet's objectSlots (2-3 per
// totem, e.g. the Beaver carries 2), or the default. The Raccoon's Big Score (second
// special) adds one slot while active (hold 4).
func heroSlots(h *crawlerHero) int {
	n := crObjectCapDefault
	if h.sheet.objectSlots > 0 {
		n = h.sheet.objectSlots
	}
	if h.sheet.role == roleTrickster && h.specialActive(1) {
		n++
	}
	return n
}

// Rarity tiers. Consumables are all common; permanents split across the three higher tiers.
// Rarity drives how often an object drops from a kill and how it is weighted in the shop.
const (
	rarCommon   = "common"
	rarUncommon = "uncommon"
	rarRare     = "rare"
	rarEpic     = "epic"
)

type crObjectDef struct {
	slug   string
	name   string
	emoji  string
	cost   int    // Courage to buy
	perm   bool   // permanent gear (passive); else a one-use consumable
	weight int    // consumable drop-table weight (common tier); permanents drop by rarity
	effect string // one-line effect, for tooltips/docs
	rarity string // rar* tier (common for every consumable)
}

// crObjects is the full catalogue, keyed by slug. Order within perm/consumable groups
// follows the note.
var crObjects = []crObjectDef{
	// Permanent gear (shop majority; a kill can drop one by rarity, the boss still guarantees one).
	{"spare-die", "Spare Die", "🎲", 5, true, 0, "add a High attack die to your pool", rarEpic},
	{"whetstone", "Whetstone", "🔪", 4, true, 0, "your 6s count as two hits", rarEpic},
	{"lucky-charm", "Lucky Charm", "🍀", 4, true, 0, "once a turn, nudge one die +1", rarRare},
	{"gamblers-glove", "Gambler's Glove", "🧤", 4, true, 0, "one free reroll each turn", rarRare},
	{"scope", "Scope", "🎯", 3, true, 0, "your attacks reach an adjacent tile", rarRare},
	{"plating", "Plating", "🛡️", 3, true, 0, "start each turn with Shields topped to 2", rarUncommon},
	{"harness", "Harness", "🎒", 3, true, 0, "carry an ally when you move", rarUncommon},
	{"grounding-boots", "Grounding Boots", "🥾", 3, true, 0, "immune to Electrocuted/Electrified", rarUncommon},
	// Consumables (shop or found, one-use). All common.
	{"medkit", "Medkit", "🩹", 2, false, 20, "heal 3", rarCommon},
	{"grenade", "Grenade", "💣", 2, false, 18, "auto-kill one Health-1 robot on your tile", rarCommon},
	{"recall-whistle", "Recall Whistle", "↩️", 2, false, 12, "teleport yourself to the start or a transporter", rarCommon},
	{"battery", "Battery", "🔋", 3, false, 14, "+2 Courage", rarCommon},
	{"torch", "Torch", "🔦", 3, false, 12, "clears a hazard", rarCommon},
	{"courage-horn", "Courage Horn", "📯", 3, false, 10, "every ally on your tile gains +1 Courage", rarCommon},
	{"beacon", "Beacon", "📍", 4, false, 8, "place a transporter tile next to you", rarCommon},
	{"sap-seed", "Sap Seed", "🌱", 5, false, 6, "place a healing tile next to you", rarCommon},
}

// dropWeight is an object's weight in the unified KILL-drop pool (enemies can drop both
// consumables and permanents). Consumables use their table weight; permanents drop by rarity,
// rarer tiers much less often, so most kill drops are still consumables.
func dropWeight(o *crObjectDef) int {
	switch o.rarity {
	case rarUncommon:
		return crDropWeightUncommon
	case rarRare:
		return crDropWeightRare
	case rarEpic:
		return crDropWeightEpic
	default: // common (consumables)
		return o.weight
	}
}

// shopWeight is an object's weight when stocking the shop. Permanents are rarity-weighted
// (uncommon common-est, epic rarest); consumables are uniform in their slots.
func shopWeight(o *crObjectDef) int {
	if !o.perm {
		return 1
	}
	switch o.rarity {
	case rarUncommon:
		return crShopWeightUncommon
	case rarRare:
		return crShopWeightRare
	default: // epic
		return crShopWeightEpic
	}
}

const (
	crDropWeightUncommon = 5
	crDropWeightRare     = 2
	crDropWeightEpic     = 2 // bumped from 1: Epic (Spare Die, Whetstone) was too scarce in practice
	crShopWeightUncommon = 4
	crShopWeightRare     = 2
	crShopWeightEpic     = 2
	crShopPerms          = 4 // of crShopSize, how many slots are permanents (the majority)
)

// rollDrop picks one object from the whole catalogue weighted by dropWeight (a kill drop:
// mostly consumables, occasionally a permanent by rarity).
func (g *crawlerGame) rollDrop() string {
	return g.rollPool(func(o *crObjectDef) int { return dropWeight(o) })
}

// rollPermanentDrop picks one permanent weighted by rarity (the boss's guaranteed drop).
func (g *crawlerGame) rollPermanentDrop() string {
	return g.rollPool(func(o *crObjectDef) int {
		if !o.perm {
			return 0
		}
		return shopWeight(o)
	})
}

// rollPool picks a slug from crObjects by an arbitrary weight function.
func (g *crawlerGame) rollPool(w func(*crObjectDef) int) string {
	total := 0
	for i := range crObjects {
		total += w(&crObjects[i])
	}
	if total <= 0 {
		return "" // nothing matches the weight function (e.g. the shop ran out of this kind)
	}
	r := g.rng.IntN(total)
	for i := range crObjects {
		x := w(&crObjects[i])
		if r < x {
			return crObjects[i].slug
		}
		r -= x
	}
	return crObjects[len(crObjects)-1].slug
}

// pickShop picks a distinct in-/out-of-stock object of the requested kind (perm or not),
// weighted by shopWeight, excluding what is already stocked. Returns "" if none remain.
func (g *crawlerGame) pickShop(perm bool, inStock map[string]bool) string {
	return g.rollPool(func(o *crObjectDef) int {
		if o.perm != perm || inStock[o.slug] {
			return 0
		}
		return shopWeight(o)
	})
}

func objBySlug(slug string) *crObjectDef {
	for i := range crObjects {
		if crObjects[i].slug == slug {
			return &crObjects[i]
		}
	}
	return nil
}

func heroHas(h *crawlerHero, slug string) bool {
	for _, s := range h.objects {
		if s == slug {
			return true
		}
	}
	return false
}

// giveObject adds an object to a hero if there is a free slot; returns false (and logs)
// when the hero is already carrying the maximum.
func (g *crawlerGame) giveObject(h *crawlerHero, slug string) bool {
	if len(h.objects) >= heroSlots(h) {
		g.log = append(g.log, h.name+"'s pack is full ("+objName(slug)+" lost).")
		return false
	}
	h.objects = append(h.objects, slug)
	return true
}

func (g *crawlerGame) takeObject(h *crawlerHero, slug string) bool {
	for i, s := range h.objects {
		if s == slug {
			h.objects = append(h.objects[:i], h.objects[i+1:]...)
			return true
		}
	}
	return false
}

func objName(slug string) string {
	if d := objBySlug(slug); d != nil {
		return d.name
	}
	return slug
}

const crShopSize = 5 // objects on offer at once; a bought slot refills with a random object

// shopInStock returns the set of slugs currently on offer and the number of permanents.
func (g *crawlerGame) shopInStock() (map[string]bool, int) {
	inStock := map[string]bool{}
	perms := 0
	for _, s := range g.shopStock {
		inStock[s] = true
		if d := objBySlug(s); d != nil && d.perm {
			perms++
		}
	}
	return inStock, perms
}

// rollShop stocks the shop: a MAJORITY of permanents (crShopPerms, rarity-weighted) plus
// consumables for the rest. Rolled fresh each level; a purchase refills the gap (buyObject).
func (g *crawlerGame) rollShop() {
	g.shopStock = g.shopStock[:0]
	inStock := map[string]bool{}
	for len(g.shopStock) < crShopPerms {
		pick := g.pickShop(true, inStock)
		if pick == "" {
			break
		}
		g.shopStock = append(g.shopStock, pick)
		inStock[pick] = true
	}
	for len(g.shopStock) < crShopSize {
		pick := g.pickShop(false, inStock)
		if pick == "" {
			break
		}
		g.shopStock = append(g.shopStock, pick)
		inStock[pick] = true
	}
}

// refillShop tops the shop back up to crShopSize, keeping the permanent majority: it refills
// a bought permanent with another permanent (and a consumable with a consumable), falling back
// to the other kind only when one is exhausted. No-op once nothing new can be stocked.
func (g *crawlerGame) refillShop() {
	for len(g.shopStock) < crShopSize {
		inStock, perms := g.shopInStock()
		wantPerm := perms < crShopPerms
		pick := g.pickShop(wantPerm, inStock)
		if pick == "" {
			pick = g.pickShop(!wantPerm, inStock)
		}
		if pick == "" {
			return
		}
		g.shopStock = append(g.shopStock, pick)
	}
}

// sellObject lets a hero on a shop tile sell one object back for a flat +1 Courage. Blocked
// when already at the Courage cap (the sale would be wasted), off a shop tile, or not carrying
// the object. Consumables and permanents sell alike (1 Courage), so a mis-bought permanent can
// be offloaded without being a profitable dump.
func (g *crawlerGame) sellObject(seat int, slug string) {
	if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
		return
	}
	h := g.players[seat]
	t := g.tiles[h.tileID]
	if t == nil || t.kind != crKindShop || h.courage >= crCourageCap {
		return
	}
	d := objBySlug(slug)
	idx := -1
	for i, s := range h.objects {
		if s == slug {
			idx = i
			break
		}
	}
	if d == nil || idx < 0 {
		return
	}
	h.objects = append(h.objects[:idx], h.objects[idx+1:]...)
	h.courage++ // flat 1, already gated below the cap
	g.log = append(g.log, h.name+" sells "+d.name+" for 1 Courage.")
}

// buyObject lets a hero on a shop tile spend Courage on a stocked object. It fails
// (silently) if not on a shop, the object is not in stock, Courage is short, or the pack
// is full. A bought entry is removed and the shop refills with a random object.
func (g *crawlerGame) buyObject(seat int, slug string) {
	if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
		return
	}
	h := g.players[seat]
	t := g.tiles[h.tileID]
	if t == nil || t.kind != crKindShop { // the merchant holds the centre; buy from the doorway
		return
	}
	idx := -1
	for i, s := range g.shopStock {
		if s == slug {
			idx = i
			break
		}
	}
	d := objBySlug(slug)
	if idx < 0 || d == nil || h.courage < d.cost || len(h.objects) >= heroSlots(h) {
		return
	}
	h.courage -= d.cost
	g.shopStock = append(g.shopStock[:idx], g.shopStock[idx+1:]...)
	g.giveObject(h, slug)
	g.refillShop()
	g.log = append(g.log, h.name+" buys "+d.name+".")
}

// --- drops ------------------------------------------------------------------

// robotDropChance is the percent chance a defeated robot drops a consumable, by kind.
func robotDropChance(kind string) int {
	switch kind {
	case "scrapling":
		return 15
	case "sentry", "jammer", "grappler":
		return 25
	case "hulk":
		return 35
	}
	return 0
}

// rollConsumableDrop picks a consumable by the weighted drop table.
func (g *crawlerGame) rollConsumableDrop() string {
	total := 0
	for _, o := range crObjects {
		total += o.weight
	}
	r := g.rng.IntN(total)
	for _, o := range crObjects {
		if o.weight == 0 {
			continue
		}
		if r < o.weight {
			return o.slug
		}
		r -= o.weight
	}
	return "medkit"
}

// dropFrom resolves a defeated robot's loot onto the floor of its tile. A boss is
// guaranteed 2 consumables + 1 permanent; an ordinary robot rolls a visible d100 against
// its per-kind chance for one consumable. Dropped objects land on the monster's square
// (spilling to free squares); the killer, if standing on that tile, picks them up at
// once, otherwise they stay on the ground for a hero on the tile to grab.
func (g *crawlerGame) dropFrom(t *crawlerTile, m *crawlerMonster, killer *crawlerHero) {
	var drops []string
	if m.isBoss {
		drops = append(drops, g.rollConsumableDrop(), g.rollConsumableDrop())
		drops = append(drops, g.rollPermanentDrop()) // the boss still guarantees a permanent (rarity-weighted)
		g.log = append(g.log, m.bossName+" drops salvage.")
	} else {
		chance := robotDropChance(m.kind)
		roll := g.rng.IntN(100) + 1 // 1..100, shown to the players
		hit := roll <= chance
		name := ""
		if hit {
			slug := g.rollDrop() // a kill can drop a consumable OR (rarely, by rarity) a permanent
			drops = append(drops, slug)
			name = objName(slug)
		}
		// Big Score (Raccoon, second special): the killer's hits drop one extra consumable.
		if hit && killer != nil && killer.sheet.role == roleTrickster && killer.specialActive(1) {
			extra := g.rollConsumableDrop()
			drops = append(drops, extra)
			g.log = append(g.log, killer.name+"'s Big Score shakes loose a "+objName(extra)+".")
		}
		g.lootRoll = &crawlerLootRoll{roll: roll, chance: chance, hit: hit, name: name}
		if killer != nil {
			g.lootRoll.killer = killer.name
		}
		if hit {
			g.log = append(g.log, "Drop roll "+strconv.Itoa(roll)+"/100 vs "+strconv.Itoa(chance)+"%: a "+name+" falls.")
		} else {
			g.log = append(g.log, "Drop roll "+strconv.Itoa(roll)+"/100 vs "+strconv.Itoa(chance)+"%: nothing.")
		}
	}
	// Trickster (Raccoon's inherent "Loot" passive): one extra consumable on every kill,
	// on top of the normal drop chance and independent of Big Score (which adds a further
	// one above). This is the Trickster's identity: it hoovers up gear.
	if killer != nil && killer.sheet.role == roleTrickster {
		bonus := g.rollConsumableDrop()
		drops = append(drops, bonus)
		g.log = append(g.log, killer.name+" roots out a "+objName(bonus)+".")
	}
	for _, slug := range drops {
		g.dropGround(t, m.square, slug)
	}
	if killer != nil && killer.alive && killer.tileID == t.id {
		g.autoPickup(t, killer)
	}
}

// pilfer (Raccoon's signature) pulls one ground item from the ADJACENT tile through the
// open doorway in dir into the hero's pack, extending the normal own-tile pickup by one
// tile. Fails if there is no open neighbour there, nothing lies on it, or the pack is full.
func (g *crawlerGame) pilfer(seat, dir int) bool {
	h := g.players[seat]
	if dir < 0 || dir > 3 {
		return false
	}
	t := g.tiles[h.tileID]
	if t == nil || !t.edges[dir] {
		return false
	}
	id, ok := g.at[[2]int{t.x + crawlerDelta[dir][0], t.y + crawlerDelta[dir][1]}]
	if !ok {
		return false
	}
	nt := g.tiles[id]
	if nt == nil || len(nt.loot) == 0 || len(h.objects) >= heroSlots(h) {
		return false
	}
	l := nt.loot[0]
	nt.loot = append(nt.loot[:0], nt.loot[1:]...)
	h.objects = append(h.objects, l.slug)
	g.log = append(g.log, h.name+" pilfers a "+objName(l.slug)+" from the next tile.")
	return true
}

// populateVault drops a Vault's guaranteed consumable onto its centre floor the moment the
// tile is laid, so the loot is visible and picked up like any floor drop (a hero grabs it
// by standing on its square), rather than silently landing in a pack on reaching the centre.
func (g *crawlerGame) populateVault(t *crawlerTile) {
	g.dropGround(t, 4, g.rollConsumableDrop())
}

// dropGround lays an object on tile t, preferring the given square, else the next free
// walkable square (so a stack of boss drops spreads out). Loot can share a square with
// a corpse but not with another loot item.
func (g *crawlerGame) dropGround(t *crawlerTile, square int, slug string) {
	occupied := func(sq int) bool {
		for _, l := range t.loot {
			if l.square == sq {
				return true
			}
		}
		return false
	}
	sq := square
	if occupied(sq) {
		sq = -1
		// Prefer a free walkable square (centre + open doorways) — near the action.
		for _, s := range crawlerSquareOrder {
			if crawlerWalkable(t.edges)[s] && !occupied(s) {
				sq = s
				break
			}
		}
		// Then ANY free square, corners included: loot is picked up tile-wide, so
		// it need not sit on a walkable square, and spreading over all 9 squares
		// keeps co-located (stacked) loot rare instead of starting at the 3rd item.
		if sq < 0 {
			for s := 0; s < 9; s++ {
				if !occupied(s) {
					sq = s
					break
				}
			}
		}
		if sq < 0 {
			sq = square // every square taken: stack it (still pickable)
		}
	}
	t.loot = append(t.loot, crawlerGroundItem{square: sq, slug: slug})
}

// seedTileLoot gives a laid base-path tile a chance at a loose floor consumable. The chance
// is set by HOW the tile was laid, not its contents (the design's loot rule):
//   - EXPLORATION lay (a hero stepping onto a fresh frontier): a flat 30%, ALWAYS, for at
//     least one item (and ~40% of those a second piece).
//   - proactive LAY-phase lay (where enemies spawn): a 1-in-6 chance of one item.
//
// Only base path tiles roll — service tiles (shop / healing / portal), the hazard rooms and
// the boss lair get nothing; the Vault and fog room seed their own loot (populateVault /
// populateFogRoom). Enemy kill drops (dropFrom) are separate.
func (g *crawlerGame) seedTileLoot(t *crawlerTile, exploration bool) {
	switch t.kind {
	case "straight", "bend", "tee", "cross", "dead-end":
		// a base path tile: roll below
	default:
		return // specials: services, the self-seeding Vault/fog, hazard rooms, the boss lair
	}
	chance := 100 / 6 // proactive LAY-phase tile (where enemies spawn): 1-in-6
	if exploration {
		chance = 30 // an explored frontier: a flat 30% for loot, always
	}
	if g.rng.IntN(100) >= chance {
		return
	}
	g.dropGround(t, 4, g.rollConsumableDrop())
	if exploration && g.rng.IntN(100) < 40 {
		g.dropGround(t, 4, g.rollConsumableDrop()) // an explored room may turn up a second piece
	}
}

// autoPickup moves ground loot on t into the hero's pack while it has room (used when a
// hero kills something on their own tile).
func (g *crawlerGame) autoPickup(t *crawlerTile, h *crawlerHero) {
	kept := t.loot[:0]
	for _, l := range t.loot {
		if len(h.objects) < heroSlots(h) {
			h.objects = append(h.objects, l.slug)
			g.log = append(g.log, h.name+" picks up a "+objName(l.slug)+".")
		} else {
			kept = append(kept, l)
		}
	}
	t.loot = append([]crawlerGroundItem{}, kept...)
}

// pickup lets a hero take one ground item from the tile it STANDS on (never an adjacent
// tile), identified by its square. Fails if the pack is full or nothing is there.
func (g *crawlerGame) pickup(seat, square int) {
	// Picking up floor loot is free and does NOT need to be your turn: any living hero
	// standing on the tile may grab what is lying there (so loot a boss dropped is still
	// reachable once the turn has moved on). It costs no die and never ends a turn.
	if g.phase != crPlaying {
		return
	}
	h := g.players[seat]
	if h == nil || !h.alive {
		return
	}
	t := g.tiles[h.tileID]
	if t == nil {
		return
	}
	for i, l := range t.loot {
		if l.square == square {
			if len(h.objects) >= heroSlots(h) {
				return // no room
			}
			h.objects = append(h.objects, l.slug)
			t.loot = append(t.loot[:i], t.loot[i+1:]...)
			g.log = append(g.log, h.name+" picks up a "+objName(l.slug)+".")
			return
		}
	}
}

// dropObject puts one of a hero's objects on the floor of its current square, leaving it
// exactly there for anyone (on that tile) to grab later.
func (g *crawlerGame) dropObject(seat int, slug string) {
	if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
		return
	}
	h := g.players[seat]
	if !g.takeObject(h, slug) {
		return
	}
	t := g.tiles[h.tileID]
	if t != nil {
		g.dropGround(t, h.square, slug)
		g.log = append(g.log, h.name+" drops a "+objName(slug)+".")
	}
}

// --- consumables ------------------------------------------------------------

// useObject spends a one-use consumable from the active hero's pack. monster/tile is the
// Grenade target. Free action (no die). Returns false (no consume) when the use is not
// valid, so an item is never wasted.
func (g *crawlerGame) useObject(seat int, slug string, monsterIdx int) {
	if g.phase != crPlaying || g.step != crStepSpend || seat != g.turn {
		return
	}
	h := g.players[seat]
	if !heroHas(h, slug) {
		return
	}
	ok := false
	switch slug {
	case "medkit":
		if h.health < h.sheet.maxHP || h.electrified {
			h.health += 3
			if h.health > h.sheet.maxHP {
				h.health = h.sheet.maxHP
			}
			h.electrified = false // a heal clears Electrified
			g.log = append(g.log, h.name+" uses a Medkit.")
			ok = true
		}
	case "grenade":
		if t := g.tiles[h.tileID]; t != nil && monsterIdx >= 0 && monsterIdx < len(t.monsters) {
			m := t.monsters[monsterIdx]
			if m.alive && m.health <= 1 && !m.isBoss {
				m.health = 0
				m.alive = false
				g.awardKillCourage(t, h, m.reward)
				g.log = append(g.log, h.name+" grenades a robot.")
				ok = true
			}
		}
	case "battery":
		if h.drained {
			break // Drained (status): cannot gain Courage, so the Battery is not spent
		}
		if h.courage < crCourageCap {
			h.courage += 2
			if h.courage > crCourageCap {
				h.courage = crCourageCap
			}
			g.log = append(g.log, h.name+" charges a Battery (+2 Courage).")
			ok = true
		}
	case "courage-horn":
		t := g.tiles[h.tileID]
		for _, s := range g.order {
			a := g.players[s]
			if a.alive && !a.drained && a.tileID == h.tileID && a.courage < crCourageCap {
				a.courage++
			}
		}
		_ = t
		g.log = append(g.log, h.name+" sounds the Courage Horn.")
		ok = true
	case "recall-whistle":
		if dest := g.firstPortal(); dest >= 0 {
			h.tileID = dest
			h.square = 4
			g.log = append(g.log, h.name+" recalls to a transporter.")
			ok = true
		}
	case "beacon":
		ok = g.placeFeatureTile(seat, crKindPortal)
		if ok {
			g.log = append(g.log, h.name+" plants a Beacon (a transporter).")
		}
	case "sap-seed":
		ok = g.placeFeatureTile(seat, crKindHealing)
		if ok {
			g.log = append(g.log, h.name+" plants a Sap Seed (a spring).")
		}
	case "torch":
		// A Torch clears the hazards on your current tile (a light source lifts the fog,
		// and doubles as the tool that neutralises a room's hazard). Not wasted if clean.
		if t := g.tiles[h.tileID]; t != nil && len(t.hazards) > 0 {
			t.hazards = nil
			g.log = append(g.log, h.name+" clears the room's hazards with a Torch.")
			ok = true
		}
	}
	if ok {
		g.takeObject(h, slug)
	}
}

// firstPortal returns the id of a portal (transporter) tile, preferring the start tile,
// or -1 if none.
func (g *crawlerGame) firstPortal() int {
	best := -1
	for id, t := range g.tiles {
		if t.kind == crKindPortal {
			if t.x == 0 && t.y == 0 {
				return id
			}
			if best < 0 {
				best = id
			}
		}
	}
	return best
}

// placeFeatureTile drops a new feature tile (a cross-shaped portal or healing spring)
// on an empty cell beyond an open edge of the hero's tile. Off the main stack, so it
// does not touch the boss clock. Returns false when the hero has no open frontier edge.
func (g *crawlerGame) placeFeatureTile(seat int, kind string) bool {
	h := g.players[seat]
	t := g.tiles[h.tileID]
	if t == nil {
		return false
	}
	base := crSpecialBase[kind]
	for d := 0; d < 4; d++ {
		if !t.edges[d] {
			continue
		}
		nx, ny := t.x+crawlerDelta[d][0], t.y+crawlerDelta[d][1]
		if _, taken := g.at[[2]int{nx, ny}]; taken {
			continue
		}
		// Orient the feature tile (its own shape) so the side facing the hero is open,
		// so it connects back to the tile we are standing on.
		entry := crawlerOpp(d)
		for r := 0; r < 4; r++ {
			e := rotateEdges(base, r)
			if e[entry] {
				nt := g.placeTile(nx, ny, e, t.band, kind, false, nil)
				nt.rot = r
				return true
			}
		}
	}
	return false
}
