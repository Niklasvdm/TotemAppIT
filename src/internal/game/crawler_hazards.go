package game

// Hazards and status effects for Totem Crawler (see crawler.go and the design note's
// "Hazards" and "Status effects" sections). Both begin at Level 2; Level 1 is clean.
//
// A hazard sits on a tile's square and is a tax / routing puzzle, not a kill: every
// value is below hero health, so one hazard never downs a healthy hero. The electrical
// ones also apply a status. A status is a small, machine-flavoured debuff carried on the
// hero until cleared (a spring, the shop, the Treefrog's Spores, or a repair item).
//
// This file owns the whole subsystem: the hazard kinds and how they bite on entry and at
// end of turn, the per-robot status die, and the clear-all helper. The effects of a
// status are applied where they bite (dice in beginTurn, movement in tryMove, Courage in
// the Courage paths); this file is the source of truth for what each one is.

// Hazard kinds (crawlerHazard.kind). Live wiring, coolant and fog are the Woodland set
// (Level 2); rust and traps join in the Ruins (Level 3), overheat/mines in the Machine
// rooms (Level 5). The kind decides the bite; the hazard-room tile just seeds it.
const (
	hzWiring  = "wiring"  // 1 damage each time you ENTER; Electrocuted if you END on it
	hzFire    = "fire"    // 2 damage on entry AND at end of turn; can inflict Electrified
	hzCoolant = "coolant" // no damage; Snares you when you end on it (a movement tax)
	hzFog     = "fog"     // no damage; hides the tile's robots and loot until a hero is on/adjacent
	hzMine    = "mine"    // 3 damage one-off when a hero ENTERS, then spent
	hzRust    = "rust"    // no damage; Rusted (one fewer die next turn) if you END on it
)

// crawlerHazard is one hazard occupying a tile square (square -1 = the whole tile, used
// by fog). `spent` marks a one-off hazard (a mine) that has already fired.
type crawlerHazard struct {
	square int
	kind   string
	spent  bool
}

// --- statuses ---------------------------------------------------------------

// heroClearStatuses removes every status from a hero (a spring, the shop, the Treefrog's
// Spores, or a repair item do this). Returns true if anything was cleared, for logging.
func heroClearStatuses(h *crawlerHero) bool {
	had := h.electrocuted || h.snared || h.electrified || h.drained || h.rusted
	h.electrocuted, h.snared, h.electrified, h.drained, h.rusted = false, false, false, false, false
	return had
}

// heroHasStatus reports whether a hero carries any status (for the view/log).
func heroHasStatus(h *crawlerHero) bool {
	return h.electrocuted || h.snared || h.electrified || h.drained || h.rusted
}

// clearHeroStatuses washes off a hero's statuses and, when that lifts Rusted from the hero
// whose turn it is, immediately GIVES BACK the die Rusted had taken (rolled fresh and added
// to the pool), instead of the repair only taking effect next turn. Returns whether anything
// was cleared, like heroClearStatuses.
func (g *crawlerGame) clearHeroStatuses(h *crawlerHero) bool {
	wasRusted := h.rusted
	cleared := heroClearStatuses(h)
	if wasRusted && len(g.dice) > 0 {
		if cur, ok := g.players[g.turn]; ok && cur == h {
			g.dice = append(g.dice, g.rollDie(h, sheetFaces(h.sheet), false))
		}
	}
	return cleared
}

// --- the status die ---------------------------------------------------------

// monsterStatus is the status an ordinary robot's hit can inflict, by art kind, plus the
// level at which the Scrapling's sting switches from Electrocuted to Rusted. Only the
// Scrapling (drone) carries a status among the wired robot kinds; the Hulk (tank) is
// pure muscle, and bosses apply their own specials (Overload). "" = no status.
func monsterStatus(kind string, level int) string {
	switch kind {
	case "scrapling": // sting: Electrocuted, Rusted in the Machine rooms (L5-6)
		if level >= 5 {
			return "rusted"
		}
		return "electrocuted"
	case "sentry": // shot: Electrocuted
		return "electrocuted"
	case "grappler": // clamp: Snared
		return "snared"
	case "jammer": // pulse: Drained (from Level 3)
		return "drained"
	}
	return ""
}

// statusThreshold is the lowest d6 face that lands a robot's status at a given level (the
// note's status-die table): the deeper you go, the easier it lands. 7 = never.
func statusThreshold(kind string, level int) int {
	switch kind {
	case "scrapling": // L2 6, L3 5, L4 4, L5 5 (Rusted), L6 4 (Rusted)
		switch level {
		case 2:
			return 6
		case 3:
			return 5
		case 4:
			return 4
		case 5:
			return 5
		case 6:
			return 4
		}
	case "sentry", "grappler": // L2 6, L3 5, L4 5, L5 4, L6 4
		switch level {
		case 2:
			return 6
		case 3, 4:
			return 5
		case 5, 6:
			return 4
		}
	case "jammer": // Drained from L3: L3 6, L4 5, L5 5, L6 4
		switch level {
		case 3:
			return 6
		case 4, 5:
			return 5
		case 6:
			return 4
		}
	}
	return 7
}

// applyStatus sets the named status on a hero, respecting immunities (Grounding Boots
// shrug off the electrical ones). Returns true if it actually took hold.
func (g *crawlerGame) applyStatus(h *crawlerHero, status string) bool {
	switch status {
	case "electrocuted":
		if heroHas(h, "grounding-boots") || h.electrocuted {
			return false
		}
		h.electrocuted = true
	case "electrified":
		if heroHas(h, "grounding-boots") || h.electrified {
			return false
		}
		h.electrified = true
	case "snared":
		if h.snared {
			return false
		}
		h.snared = true
	case "drained":
		if h.drained {
			return false
		}
		h.drained = true
	case "rusted":
		if h.rusted {
			return false
		}
		h.rusted = true
	default:
		return false
	}
	return true
}

// rollStatusOnHit is called when a robot lands a hit: if its kind carries a status at
// this level, roll a visible d6 and, on a face at or above the threshold, inflict it.
// The roll is recorded on g.statusRoll so the client can show the die.
func (g *crawlerGame) rollStatusOnHit(m *crawlerMonster, h *crawlerHero) {
	if m.isBoss {
		return // bosses inflict via their own specials (Overload), not the sting die
	}
	status := monsterStatus(m.kind, g.level)
	if status == "" {
		return
	}
	th := statusThreshold(m.kind, g.level)
	roll := g.rng.IntN(6) + 1
	hit := roll >= th
	g.statusRoll = &crawlerStatusRoll{roll: roll, need: th, status: status, hit: hit, target: h.name}
	if hit && g.applyStatus(h, status) {
		g.log = append(g.log, h.name+" is "+statusLabel(status)+".")
	}
}

// statusLabel is the past-tense word used in the log.
func statusLabel(status string) string {
	switch status {
	case "electrocuted":
		return "Electrocuted"
	case "electrified":
		return "Electrified"
	case "snared":
		return "Snared"
	case "drained":
		return "Drained"
	case "rusted":
		return "Rusted"
	}
	return status
}

// crawlerStatusRoll records the most recent status die for the client (mirrors the loot
// roll). Cleared at the start of each turn.
type crawlerStatusRoll struct {
	roll, need int
	status     string
	hit        bool
	target     string
}

// --- hazards on the map -----------------------------------------------------

// hazardAt returns the active (non-spent) hazard on a tile's square, or nil.
func hazardAt(t *crawlerTile, square int) *crawlerHazard {
	for i := range t.hazards {
		if t.hazards[i].square == square && !t.hazards[i].spent {
			return &t.hazards[i]
		}
	}
	return nil
}

// tileHasFog reports whether a tile carries a fog hazard (hides its contents).
func tileHasFog(t *crawlerTile) bool {
	for _, hz := range t.hazards {
		if hz.kind == hzFog {
			return true
		}
	}
	return false
}

// revealFog permanently lifts a sensor-fog tile's veil once a hero reaches its centre
// square. The reveal sticks: the tile stays uncovered after everyone leaves.
func (g *crawlerGame) revealFog(h *crawlerHero) {
	t := g.tiles[h.tileID]
	if t != nil && h.square == 4 && tileHasFog(t) {
		t.fogRevealed = true
	}
}

// coolantBlocks reports whether a hero may NOT step onto this square yet: a coolant
// square is a movement tax, so a hero without a drain tool (none built) ends their step
// there and is Snared rather than passing freely. We let the step happen (so the hero
// can cross eventually) but Snare them on arrival; it is never hard-impassable in V0.

// enterHazard fires a square hazard the moment a hero steps onto it (a move, a bonus
// step, or a carried ally): live wiring chips 1, a mine bursts once for 3 then is spent,
// coolant Snares, fire burns for 2 (and can Electrify). Called from the movement
// primitives. Fog reveals on arrival, not here.
func (g *crawlerGame) enterHazard(h *crawlerHero) {
	t := g.tiles[h.tileID]
	if t == nil || g.level < 2 {
		return
	}
	hz := hazardAt(t, h.square)
	if hz == nil {
		return
	}
	switch hz.kind {
	case hzWiring:
		if heroHas(h, "grounding-boots") {
			break // Grounding Boots ground out the live floor: no damage, no Electrocuted
		}
		g.hurt(h, 1)
		g.log = append(g.log, h.name+" crosses live wiring (1 damage).")
	case hzMine:
		hz.spent = true
		g.hurt(h, 3)
		g.log = append(g.log, h.name+" triggers a mine (3 damage)!")
	case hzCoolant:
		if g.applyStatus(h, "snared") {
			g.log = append(g.log, h.name+" wades into coolant and is Snared.")
		}
	case hzFire:
		// Fire bites on ENTRY too (like live wiring), not only when you end on it: moving
		// across it burns for 2 and can Electrify. (It also bites again at end of turn.)
		g.hurt(h, 2)
		if h.alive {
			if g.applyStatus(h, "electrified") {
				g.log = append(g.log, h.name+" runs through fire (2 damage) and is Electrified.")
			} else {
				g.log = append(g.log, h.name+" runs through fire (2 damage).")
			}
		}
	}
}

// endTurnHazards bites the hazards that trigger on ENDING a turn on their square: live
// wiring Electrocutes, rust Rusts, fire deals 2 and can Electrify. Called once per hero as
// the round resolves. Coolant's Snare is applied on arrival (enterHazard).
func (g *crawlerGame) endTurnHazards(h *crawlerHero) {
	t := g.tiles[h.tileID]
	if t == nil || g.level < 2 || !h.alive {
		return
	}
	hz := hazardAt(t, h.square)
	if hz == nil {
		return
	}
	switch hz.kind {
	case hzWiring:
		if g.applyStatus(h, "electrocuted") {
			g.log = append(g.log, h.name+" ends on live wiring and is Electrocuted.")
		}
	case hzRust:
		if g.applyStatus(h, "rusted") {
			g.log = append(g.log, h.name+" ends in the rust and is Rusted.")
		}
	case hzFire:
		g.hurt(h, 2)
		if h.alive {
			if g.applyStatus(h, "electrified") {
				g.log = append(g.log, h.name+" stands in fire (2 damage) and is Electrified.")
			} else {
				g.log = append(g.log, h.name+" stands in fire (2 damage).")
			}
		}
	}
}

// seedHazards places a hazard room's hazards on its squares when it is laid. A hazard
// room is built around one hazard: fog covers the whole tile; the others sit on the
// doorway squares so the centre stays a safe perch (you choose whether to brave them).
func (g *crawlerGame) seedHazards(t *crawlerTile) {
	kind := hazardForRoom(t.kind)
	if kind == "" {
		return
	}
	if kind == hzFog {
		t.hazards = append(t.hazards, crawlerHazard{square: -1, kind: hzFog})
		return
	}
	// Seed the hazard on every open doorway square (the centre stays clear).
	for d := 0; d < 4; d++ {
		if t.edges[d] {
			t.hazards = append(t.hazards, crawlerHazard{square: crawlerDoor[d], kind: kind})
		}
	}
}

// crFogAmbushBudget caps the ambush's total monster Health. Three Hulks would be 12, which
// is too much for one fog room, so the combined Health of the pack must stay strictly below
// it (the user's "below 3 brutes"). A Hulk is Health 4, the others 1-2.
const crFogAmbushBudget = 12

// populateFogRoom fills a freshly-laid sensor-fog room, whose contents the fog hides until a
// hero is on or next to it. Half the time it is a loot cache (the Sap Seed and Beacon, which
// place a healing tile and a transporter, plus a couple of random consumables on the floor);
// half the time an ambush: 3-4 robots led by at least one Hulk (the brute), filled out with
// random lighter robots, never a boss, and with their combined Health kept below the budget
// (crFogAmbushBudget) so the room is a scare, not a wipe.
func (g *crawlerGame) populateFogRoom(t *crawlerTile) {
	// Level 1 (the tutorial) is always a cache, never an ambush: no Hulk-led pack in the
	// level that teaches the basics. Level 2+ keeps the 50/50 cache-or-ambush gamble.
	if g.level == 1 || g.rng.IntN(2) == 0 {
		loot := []string{"sap-seed", "beacon"}
		for i := 0; i < 1+g.rng.IntN(2); i++ {
			loot = append(loot, g.rollConsumableDrop())
		}
		for _, slug := range loot {
			g.dropGround(t, 4, slug) // spills to free walkable squares if the centre is taken
		}
		return
	}
	target := 3 + g.rng.IntN(2) // 3 or 4 enemies
	hulk := newRobot("hulk")    // at least one brute leads the pack
	mons := []*crawlerMonster{hulk}
	total := hulk.health
	others := []string{"scrapling", "sentry", "grappler"} // Health 1, 1, 2
	for len(mons) < target {
		// Only the robots that still fit under the budget are eligible.
		var fits []string
		for _, kind := range others {
			if total+newRobot(kind).health < crFogAmbushBudget {
				fits = append(fits, kind)
			}
		}
		if len(fits) == 0 {
			break
		}
		r := newRobot(fits[g.rng.IntN(len(fits))])
		mons = append(mons, r)
		total += r.health
	}
	assignMonsterSquares(t.edges, mons)
	t.monsters = append(t.monsters, mons...)
}

// hazardForRoom maps a hazard-room tile kind to the hazard it seeds, or "" if the kind
// is not a hazard room.
func hazardForRoom(kind string) string {
	switch kind {
	case crKindWiring:
		return hzWiring
	case crKindCoolant:
		return hzCoolant
	case crKindFog:
		return hzFog
	case crKindRust:
		return hzRust // Rusted (one fewer die next turn) when you end on it (Ruins L3+)
	case crKindTrap:
		return hzMine
	case crKindOverheat:
		return hzFire
	}
	return ""
}
