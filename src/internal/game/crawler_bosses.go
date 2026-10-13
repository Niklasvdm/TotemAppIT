package game

// Boss pool and multi-level progression for Totem Crawler (see crawler.go).
//
// A run is six levels across three biomes (Woodland 1-2, Ruins 3-4, Machine 5-6). Each
// level draws one boss at random from the bosses whose level range includes it (the
// note's "Boss pool" table); beating it descends to the next level (a fresh map), and
// beating the Machine boss (The Core) wins the run. Courage resets and health refills
// between levels; objects carry over.
//
// Each boss is a named character with a baked-in SIGNATURE SPECIAL (the note's "Boss
// specials" table), resolved in crawler_combat.go. Stats are the note's placeholders.

const crMaxLevel = 6

// Biomes, in descent order; two levels each (names per the design note).
const (
	crWoodland = "woodland"
	crRuins    = "ruins"
	crMachine  = "machine"
)

// Signature boss specials (effects per the note's table).
const (
	bsAdaptive  = "adaptive"  // Adaptive Plating: +1 Armour each round until a 6 lands, then resets
	bsSummon1   = "summon1"   // Summoner I: 1 Scrapling on its tile each round
	bsSummon2   = "summon2"   // Summoner II: 2 Scraplings each round
	bsSummon3   = "summon3"   // Summoner III: a Sentry and a Scrapling each round
	bsJammer    = "jammer"    // Jammer Field: +1 Armour to every other robot on the map
	bsShockwave = "shockwave" // Shockwave: each round, every hero on its tile takes 1 (extra)
	bsOverload  = "overload"  // Overload: applies Electrified to a random hero each round
	bsHardcore  = "hardcore"  // Hardened Core: immune to hits until its weak-point is destroyed
	bsEnrage    = "enrage"    // Enrage: +1 Damage each round it survives
	bsSplit     = "split"     // Split: at half health, breaks off a second weaker core
)

type bossDef struct {
	name           string
	slug           string // sprite kind; falls back to the generic "boss" art when none exists
	armour         int
	damage         int
	reward         int
	baseHP         int // Health = baseHP + hpPerHero * players (the note's "base + per player")
	hpPerHero      int
	minLvl, maxLvl int // level range this boss is drawn in (inclusive)
	specials       []string
}

// bossRoster is the full pool; rollBoss draws from the entries eligible at the level.
// Stats are the design note's "Enemies (robots)" and "Boss pool" tables: every boss is
// Armour 5-6, Damage 4, Courage reward 3, and Health = base + per-player (the "X + Y"
// column), so a 2-player Mossback is 6+2*2 = 10, a 6-player one 6+2*6 = 18.
var bossRoster = []bossDef{
	{name: "Mossback", slug: "boss", armour: 5, damage: 4, reward: 3, baseHP: 6, hpPerHero: 2, minLvl: 1, maxLvl: 2, specials: []string{bsAdaptive}},
	{name: "Vespex", slug: "vespex", armour: 5, damage: 4, reward: 3, baseHP: 5, hpPerHero: 3, minLvl: 1, maxLvl: 2, specials: []string{bsSummon1}},
	{name: "Pylon", slug: "pylon", armour: 6, damage: 4, reward: 3, baseHP: 5, hpPerHero: 2, minLvl: 1, maxLvl: 2, specials: []string{bsJammer}},
	{name: "Thornstag", slug: "thornstag", armour: 5, damage: 4, reward: 3, baseHP: 5, hpPerHero: 3, minLvl: 2, maxLvl: 2, specials: []string{bsShockwave}},
	{name: "Hauler", slug: "hauler", armour: 5, damage: 4, reward: 3, baseHP: 6, hpPerHero: 3, minLvl: 3, maxLvl: 4, specials: []string{bsHardcore}},
	{name: "Vultrix", slug: "vultrix", armour: 5, damage: 4, reward: 3, baseHP: 5, hpPerHero: 3, minLvl: 3, maxLvl: 4, specials: []string{bsSummon2}},
	{name: "Teslacrane", slug: "teslacrane", armour: 6, damage: 4, reward: 3, baseHP: 6, hpPerHero: 2, minLvl: 3, maxLvl: 4, specials: []string{bsOverload}},
	{name: "Widowframe", slug: "widowframe", armour: 5, damage: 4, reward: 3, baseHP: 5, hpPerHero: 3, minLvl: 5, maxLvl: 6, specials: []string{bsSplit}},
	{name: "Ramhead", slug: "ramhead", armour: 6, damage: 4, reward: 3, baseHP: 6, hpPerHero: 3, minLvl: 5, maxLvl: 6, specials: []string{bsEnrage}},
	{name: "The Core", slug: "core", armour: 6, damage: 4, reward: 3, baseHP: 6, hpPerHero: 2, minLvl: 6, maxLvl: 6, specials: []string{bsSummon3, bsOverload}},
}

func biomeForLevel(n int) string {
	switch {
	case n <= 2:
		return crWoodland
	case n <= 4:
		return crRuins
	default:
		return crMachine
	}
}

func biomeLabel(biome string) string {
	switch biome {
	case crWoodland:
		return "Woodland"
	case crRuins:
		return "Ruins"
	case crMachine:
		return "Machine"
	default:
		return biome
	}
}

// rollBossMonsters builds the boss (and, for Hardened Core, its weak-point) for the
// current level, drawn at random from the bosses eligible at that level. Health scales
// with the party size so the fight runs a few rounds at any count.
func (g *crawlerGame) rollBossMonsters() []*crawlerMonster {
	var pool []bossDef
	for _, d := range bossRoster {
		if g.level >= d.minLvl && g.level <= d.maxLvl {
			pool = append(pool, d)
		}
	}
	if len(pool) == 0 {
		pool = bossRoster[:3] // defensive: the Woodland openers
	}
	d := pool[g.rng.IntN(len(pool))]
	players := len(g.order)
	if players < 1 {
		players = 1
	}
	hp := d.baseHP + d.hpPerHero*players // the note's "base + per player" Health
	boss := &crawlerMonster{
		armour: d.armour, health: hp, maxHealth: hp, damage: d.damage, square: 4,
		kind: d.slug, alive: true, isBoss: true, reward: d.reward,
		bossName: d.name, specials: append([]string{}, d.specials...),
	}
	out := []*crawlerMonster{boss}
	// Hardened Core fights behind a separate weak-point that must be destroyed first.
	if hasSpecial(boss, bsHardcore) {
		out = append(out, &crawlerMonster{
			armour: 3, health: 2, maxHealth: 2, damage: 0, kind: "weakpoint", alive: true, weakPoint: true,
		})
	}
	return out
}

// hasSpecial reports whether a monster carries a given signature special.
func hasSpecial(m *crawlerMonster, s string) bool {
	for _, x := range m.specials {
		if x == s {
			return true
		}
	}
	return false
}

// summonAdds is the robots a summoner spawns each round, per the note: Summoner I one
// Scrapling, II two Scraplings, III a Sentry and a Scrapling.
func summonAdds(m *crawlerMonster) []*crawlerMonster {
	switch {
	case hasSpecial(m, bsSummon3):
		return []*crawlerMonster{makeAdd("sentry"), makeAdd("scrapling")}
	case hasSpecial(m, bsSummon2):
		return []*crawlerMonster{makeAdd("scrapling"), makeAdd("scrapling")}
	case hasSpecial(m, bsSummon1):
		return []*crawlerMonster{makeAdd("scrapling")}
	}
	return nil
}

// makeAdd builds a summoned/spawned robot by kind (Scrapling or Sentry turret).
func makeAdd(kind string) *crawlerMonster {
	switch kind {
	case "sentry":
		return &crawlerMonster{armour: 5, health: 1, maxHealth: 1, damage: 3, kind: "sentry", alive: true, reward: 1}
	default: // Scrapling
		return &crawlerMonster{armour: 3, health: 1, maxHealth: 1, damage: 1, kind: "scrapling", alive: true, reward: 1}
	}
}
