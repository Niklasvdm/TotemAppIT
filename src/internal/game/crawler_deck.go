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
		// The healing spring is a 3-way T-junction in the Woodland, but a 4-way cross in the
		// Ruins (its fountain art reads as a crossroads). Same kind, biome-dependent shape.
		heal := func() tileSpec {
			s := spec(crKindHealing)
			if biomeForLevel(g.level) == crRuins {
				s.base = ttCross.base
			}
			return s
		}
		switch b {
		case 0:
			band = g.insertSpec(band, heal())
			// Statuses and hazards begin at Level 2: the first hazard room and a landmark
			// shrine come in here (see the note's per-level tile table).
			if g.level == 2 {
				band = g.insertSpec(band, spec(crKindShrine)) // a landmark, Woodland L2 only for now
			}
			if g.level >= 2 {
				band = g.insertSpec(band, spec(crKindWiring))
			}
		case 1:
			band = g.insertSpec(band, spec(crKindPortal))
			// Level 3 opens on the broken gate, not a transporter, so the start tile is no
			// longer half the teleport pair. Add a second deck portal there so the network
			// still has a usable pair (every level keeps two transporters).
			if g.level == 3 {
				band = g.insertSpec(band, spec(crKindPortal))
			}
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
			band = g.insertSpec(band, heal())
			if g.level >= 2 {
				band = g.insertSpec(band, spec(crKindFog))
			}
			// At Level 2 the broken city gate is the BOSS chamber (see below), not a
			// standalone tile, so nothing extra is injected here for it.
			if g.level >= 3 { // the Ruins add the trap room
				band = g.insertSpec(band, spec(crKindTrap))
			}
			if g.level >= 5 { // the Machine rooms add the overheat room
				band = g.insertSpec(band, spec(crKindOverheat))
			}
		}
		if b == g.bands-1 {
			// The boss is drawn from the level's pool (rollBossMonsters); it holds the
			// centre (heroes can't stand on it, see tryMove). The lair is normally a
			// crossroads room (surround it from four sides), EXCEPT at the biome seams:
			// Level 2's boss guards the broken city gate (and that gate starts Level 3),
			// and Level 4's boss guards the stairwell down into the machine rooms. Both are
			// one-doorway dead-ends fought from the single side.
			bossKind, bossBase := "boss", ttCross.base
			if g.level == 2 {
				bossKind, bossBase = crKindGate, crSpecialBase[crKindGate]
			} else if g.level == 4 {
				bossKind, bossBase = crKindDescent, crSpecialBase[crKindDescent]
			}
			boss := tileSpec{band: b, kind: bossKind, base: bossBase, isBoss: true,
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

// insertSpec shuffles one special tile into a band at a random position.
func (g *crawlerGame) insertSpec(band []tileSpec, spec tileSpec) []tileSpec {
	pos := g.rng.IntN(len(band) + 1)
	out := make([]tileSpec, 0, len(band)+1)
	out = append(out, band[:pos]...)
	out = append(out, spec)
	out = append(out, band[pos:]...)
	return out
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
