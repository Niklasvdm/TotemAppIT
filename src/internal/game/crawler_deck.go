package game

// Deck and monster generation for Totem Crawler: the tile templates and weighted
// draw pool, the banded stack builder (buildDeck) with the boss and per-level
// special-tile gating, the robot stat lines (newRobot), the per-band enemy roll
// (rollMonster), and swarm square assignment. This is the subsystem to edit when
// tuning which tiles and which robots a run contains; the turn flow that consumes
// the deck lives in crawler.go.

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

// buildDeck lays out the shuffled stack from the design's **per-level tile table**: the
// exact count of each path tile (levelPathCounts) and the specials, split into `g.bands`
// difficulty tiers so monsters escalate, with the boss near the end. The ESSENTIAL service
// tiles are then dealt in above the boss so a shuffle can never bury one behind it.
//
// This is THE place to tune the tile mix: edit `levelPathCounts` for the base paths,
// `essentialSpecialTiles` / `mixableSpecialTiles` for the specials, `g.bands` for the
// difficulty tiers, and `rollMonster` for the enemies (and swarms) per band. The boss stat
// line is built in levelBossTile. levelPathCounts is the EXACT number of each base path tile
// per level (1..crMaxLevel), straight from the design's per-level tile table. The deck also
// carries the specials (essential services dealt in above the boss, the rest shuffled into
// the bands) and the boss (levelBossTile); the start tile is placed separately by startLevel
// and so is not in the deck.
var levelPathCounts = map[string][6]int{
	"straight": {4, 4, 2, 5, 2, 3}, // dropped per level to make room for the growing fog/trap rooms
	"bend":     {4, 4, 3, 3, 2, 2}, // (see levelFogCount / levelTrapCount); each column still sums to the Total
	"tee":      {3, 3, 1, 2, 1, 3},
	"cross":    {2, 2, 1, 2, 1, 2},
	"dead-end": {1, 1, 1, 1, 1, 1},
}

func (g *crawlerGame) buildDeck() {
	g.deck = nil
	g.deckPos = 0
	bands := g.bands
	if bands < 1 {
		bands = 1
	}
	li := g.level - 1 // index into the per-level tables; clamp so a deck can be built even
	if li < 0 {       // before startLevel has set a level (treat as level 1 / the last level).
		li = 0
	} else if li >= 6 {
		li = 5
	}

	// 1. Path tiles: the exact per-level counts, shuffled, then split into `bands` equal
	//    tiers so monster strength still escalates across the run (rollMonster is per band:
	//    band 0 easy, the last band nasty).
	var paths []tileSpec
	for _, tt := range []crawlerTileType{ttStraight, ttBend, ttTee, ttCross, ttDeadEnd} {
		for i := 0; i < levelPathCounts[tt.kind][li]; i++ {
			paths = append(paths, tileSpec{kind: tt.kind, base: tt.base})
		}
	}
	g.rng.Shuffle(len(paths), func(i, j int) { paths[i], paths[j] = paths[j], paths[i] })

	bandTiles := make([][]tileSpec, bands)
	for i := range paths {
		b := i * bands / len(paths) // 0..bands-1, ascending
		paths[i].band = b
		paths[i].monsters = g.rollMonster(b)
		bandTiles[b] = append(bandTiles[b], paths[i])
	}

	// 2. MIXABLE specials (the Vault, hazard rooms, the shrine): spread across the bands and
	//    shuffled in. These may land anywhere, even in the last few tiles behind the boss (the
	//    boss-clock tail); the ESSENTIAL services are dealt in above the boss in step 6.
	for i, s := range g.mixableSpecialTiles() {
		s.band = i % bands
		bandTiles[s.band] = append(bandTiles[s.band], s)
	}

	// 3. Shuffle within each band, so the order inside a difficulty tier is random.
	for b := range bandTiles {
		g.rng.Shuffle(len(bandTiles[b]), func(i, j int) { bandTiles[b][i], bandTiles[b][j] = bandTiles[b][j], bandTiles[b][i] })
	}

	// 4. The boss goes near the end of the last band, so the run races to it.
	last := bands - 1
	pos := len(bandTiles[last])
	if pos > 0 {
		pos -= g.rng.IntN(min2(pos, 3) + 1)
	}
	boss := g.levelBossTile()
	bandTiles[last] = append(bandTiles[last][:pos], append([]tileSpec{boss}, bandTiles[last][pos:]...)...)

	// 5. Assemble band 0, 1, ... in order: earlier tiles carry the easier monsters.
	for b := 0; b < bands; b++ {
		g.deck = append(g.deck, bandTiles[b]...)
	}

	// 6. ESSENTIAL service tiles (healing springs, transporters, shop) are dealt into the
	//    stack ABOVE the boss, exactly like the board game: after mixing and dropping the boss
	//    near the bottom, slot each at a random index strictly before the boss. No shuffle can
	//    then bury a service behind the boss, where it could never be laid (laying stops the
	//    moment the boss lair is on the map).
	g.insertBeforeBoss(g.essentialSpecialTiles())
}

// crSpec builds a special tile spec; its kind is its effect, and it carries no monsters.
func (g *crawlerGame) crSpec(kind string) tileSpec {
	return tileSpec{kind: kind, base: crSpecialBase[kind]}
}

// essentialSpecialTiles are the SERVICE tiles the party must always be able to reach before
// the boss: the two healing springs, the transporter(s), and the shop. buildDeck deals them
// into the stack ABOVE the boss (insertBeforeBoss), so a shuffle can never bury one where it
// would be lost. One transporter is normally the pre-placed start tile; Level 3 (where the
// gate is the start) keeps BOTH transporters in the deck.
func (g *crawlerGame) essentialSpecialTiles() []tileSpec {
	// Healing spring: a 3-way T-junction in the Woodland, a 4-way cross in Ruins/Machine
	// (whose fountain/pad art reads as a crossroads). TWO per level.
	heal := g.crSpec(crKindHealing)
	if biomeForLevel(g.level) != crWoodland {
		heal.base = ttCross.base
	}
	out := []tileSpec{heal, heal, g.crSpec(crKindShop), g.crSpec(crKindPortal)}
	if g.level == 3 {
		out = append(out, g.crSpec(crKindPortal))
	}
	return out
}

// mixableSpecialTiles are the non-service specials (the Vault, hazard rooms, the shrine).
// They are shuffled into the banded stack and MAY sit in the last few tiles behind the boss
// (that is the intended boss-clock tail): losing one there is acceptable, unlike a service.
// Level-gated to match the design's per-level table.
// Sensor-fog (hidden) rooms scale up with depth and appear from Level 1; trap/mine rooms
// scale up from Level 3. These counts progressively replace plain crossings as the run goes
// deeper (the straight/bend rows in levelPathCounts drop to match, keeping each Total fixed).
var (
	levelFogCount  = [6]int{1, 2, 2, 3, 3, 4}
	levelTrapCount = [6]int{0, 0, 1, 1, 2, 2}
)

func (g *crawlerGame) mixableSpecialTiles() []tileSpec {
	li := g.level - 1
	if li < 0 {
		li = 0
	} else if li >= 6 {
		li = 5
	}
	var out []tileSpec
	for i := 0; i < levelFogCount[li]; i++ {
		out = append(out, g.crSpec(crKindFog))
	}
	for i := 0; i < levelTrapCount[li]; i++ {
		out = append(out, g.crSpec(crKindTrap))
	}
	if g.level >= 2 {
		out = append(out, g.crSpec(crKindVault), g.crSpec(crKindWiring), g.crSpec(crKindCoolant))
	}
	if g.level == 2 {
		out = append(out, g.crSpec(crKindShrine)) // Woodland L2 landmark only for now
	}
	if g.level >= 3 {
		out = append(out, g.crSpec(crKindRust))
	}
	if g.level >= 5 {
		out = append(out, g.crSpec(crKindOverheat))
	}
	return out
}

// bossIndex returns the position of the boss tile in the deck, or -1 if none is present.
func (g *crawlerGame) bossIndex() int {
	for i := range g.deck {
		if g.deck[i].isBoss {
			return i
		}
	}
	return -1
}

// insertBeforeBoss slots each tile into the deck at a random index strictly before the boss,
// recomputing the boss position per insert (each one pushes the boss back). A tile placed
// this way is guaranteed to be laid before the boss lair ends the run.
func (g *crawlerGame) insertBeforeBoss(specs []tileSpec) {
	for _, s := range specs {
		bi := g.bossIndex()
		if bi < 0 {
			bi = len(g.deck)
		}
		at := g.rng.IntN(bi + 1) // 0..bi: at or before the boss slot, so the boss shifts back
		g.deck = append(g.deck[:at], append([]tileSpec{s}, g.deck[at:]...)...)
	}
}

// levelBossTile is the boss lair for the level: a crossroads room, except the biome seams —
// Level 2's boss guards the broken city gate (which also starts Level 3) and Level 4's the
// descent stairwell; both are one-doorway dead-ends fought from the single side.
func (g *crawlerGame) levelBossTile() tileSpec {
	kind, base := "boss", ttCross.base
	switch g.level {
	case 2:
		kind, base = crKindGate, crSpecialBase[crKindGate]
	case 4:
		kind, base = crKindDescent, crSpecialBase[crKindDescent]
	}
	return tileSpec{kind: kind, base: base, isBoss: true, band: g.bands - 1, monsters: g.rollBossMonsters()}
}

// newRobot builds one ordinary robot by archetype slug, the single source of their stat
// lines (the note's "Enemies" table: A / H / D / Courage). Used by rollMonster, boss
// summons (makeAdd) and the fog room. An unknown slug returns a Scrapling.
func newRobot(kind string) *crawlerMonster {
	mk := func(a, h, d, reward int) *crawlerMonster {
		return &crawlerMonster{armour: a, health: h, maxHealth: h, damage: d, square: 4, kind: kind, alive: true, reward: reward}
	}
	switch kind {
	case "sentry":
		return mk(5, 1, 3, 1) // hard to hit, folds once you connect
	case "hulk":
		return mk(3, 4, 3, 2) // heavy, soak it, team it
	case "jammer":
		return mk(3, 2, 1, 2) // +1 Armour to other robots on its tile
	case "grappler":
		return mk(4, 2, 1, 2) // clamp can Snare
	default: // scrapling: cheap swarm filler
		return &crawlerMonster{armour: 3, health: 1, maxHealth: 1, damage: 1, square: 4, kind: "scrapling", alive: true, reward: 1}
	}
}

// rollMonster rolls the enemies for one tile in a band: 0, 1 or a small swarm. A 3x3
// tile has at most five walkable squares (centre + up to four doorways), so a tile
// holds up to three robots comfortably; their squares are assigned when the tile is
// laid (layTile). Tougher types show up deeper in the stack.
//
// To tweak the enemy mix or add a swarm, this is the single place to look: edit the
// per-band weights and the slices returned. newRobot builds each robot (its stat line).
func (g *crawlerGame) rollMonster(band int) []*crawlerMonster {
	roll := g.rng.IntN(100)
	scrapling := func() *crawlerMonster { return newRobot("scrapling") }
	sentry := func() *crawlerMonster { return newRobot("sentry") }
	hulk := func() *crawlerMonster { return newRobot("hulk") }
	jammer := func() *crawlerMonster { return newRobot("jammer") }
	grappler := func() *crawlerMonster { return newRobot("grappler") }
	deep := g.level >= 2 // Jammers and Grapplers are parked for deeper levels
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

// bandOddsText summarises rollMonster's per-band spawn weights for the debug overlay. It
// MIRRORS the switch in rollMonster (Jammers/Grapplers are "deep", Level 2+), so keep the
// two in sync when retuning. The concrete deck peek is authoritative; this is the shape.
func bandOddsText(level int) []string {
	deep := level >= 2
	grappler := "15% Grappler, "
	jammer := "15% Jammer+Scrapling, "
	if !deep {
		grappler, jammer = "", ""
	}
	return []string{
		"band 0: 45% Scrapling, 10% 2x Scrapling, else empty",
		"band 1: 25% Scrapling, 20% Sentry, 15% Hulk, " + grappler + "~15-30% 2x Scrapling, else empty",
		"band 2+: 20% Hulk, " + jammer + "15% Sentry+Scrapling, 20% Hulk+Scrapling, 20% 3x Scrapling, else empty",
	}
}

// crawlerSquareOrder is the preference order for placing several items on a tile: the
// centre first, then each doorway (N, E, S, W). Used for ground loot / vault drops, which
// may sit on a walkable square.
var crawlerSquareOrder = [5]int{4, crawlerDoor[0], crawlerDoor[1], crawlerDoor[2], crawlerDoor[3]}

// crawlerRobotSquareOrder is the preference order for placing ROBOTS: the four corners
// first, then the edge midpoints. A robot only lands on a square that is NOT walkable
// (a corner, or a closed-edge midpoint), so it never shares a square with a hero (who
// only ever stands on the centre and OPEN doorways). This keeps a crowded tile from
// rendering heroes and robots stacked on top of each other. The boss is the exception.
var crawlerRobotSquareOrder = [8]int{0, 2, 6, 8, 1, 3, 5, 7}

func assignMonsterSquares(edges [4]bool, mons []*crawlerMonster) {
	w := crawlerWalkable(edges)
	// Preference: hero-unreachable squares first (corners / closed-edge midpoints, so a robot
	// never shares with a hero), then the walkable non-centre squares as a fallback for a big
	// swarm, then the centre last. Each robot gets its OWN square until the tile is full, so
	// robots never silently stack (the client offset-renders any genuine overflow).
	slots := make([]int, 0, 9)
	for _, sq := range crawlerRobotSquareOrder { // the 8 non-centre squares, corners first
		if !w[sq] {
			slots = append(slots, sq)
		}
	}
	for _, sq := range crawlerRobotSquareOrder {
		if w[sq] {
			slots = append(slots, sq)
		}
	}
	slots = append(slots, 4) // the centre, last resort (and the boss's own square)
	ri := 0
	for _, m := range mons {
		if m.isBoss {
			m.square = 4 // the boss holds the lair centre (heroes fight it from the sides)
			continue
		}
		if ri < len(slots) {
			m.square = slots[ri]
			ri++
		} else {
			m.square = 4 // more robots than squares (never for <=8): stack as the last resort
		}
	}
}
