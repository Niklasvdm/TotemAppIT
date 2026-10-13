package game

import "strconv"

// Combat for Totem Crawler (see crawler.go): attacks, the end-of-round group fight,
// shields, healing and the Courage rewards.

// clearEngaged drops the "attacked" flag on every monster, done at the start of each
// round so a monster's attack animation shows for exactly the round after it struck.
func (g *crawlerGame) clearEngaged() {
	for _, t := range g.tiles {
		for _, m := range t.monsters {
			m.engaged = false
		}
	}
}

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
	// A boss cannot be sniped from an adjacent tile: you must stand on its tile to hit it
	// (ranged reach works on ordinary robots, not the boss).
	if m.isBoss && t.id != h.tileID {
		return false
	}
	// Hardened Core (boss special): immune while its weak-point still stands on the tile.
	if hasSpecial(m, bsHardcore) && g.weakPointAlive(t) {
		return false
	}
	// A die below the monster's (effective) Armour cannot land, so it is not a legal
	// attack: refuse it rather than burning the die on a guaranteed miss. The player
	// spends low dice on Move/Special instead, which is the point of the die map.
	if value < g.effectiveArmour(h, m) {
		return false
	}
	m.engaged = true
	// Adaptive Plating: a landed 6 resets the climbing Armour bonus.
	if value >= 6 && hasSpecial(m, bsAdaptive) && m.plating > 0 {
		m.plating = 0
		g.log = append(g.log, m.bossName+"'s plating is cracked.")
	}
	hits := 1
	// Arrow Squid, Deep Volley (second special): once a turn a 6 lands two hits.
	if h.sheet.totem == "pijlinktvis" && h.specialActive(1) && !h.volleyUsed && value >= 6 {
		hits = 2
		h.volleyUsed = true
		g.log = append(g.log, h.name+"'s Deep Volley strikes twice.")
	}
	// Whetstone (object): your 6s land an extra hit.
	if value >= 6 && heroHas(h, "whetstone") {
		hits++
	}
	// Pack Tactics (Wolf, first special): +1 hit while a living ally shares your tile.
	if h.sheet.totem == "wolf" && h.specialActive(0) && g.allyOnTile(h) {
		hits++
	}
	m.health -= hits
	// Widowframe's Split: the first time it drops to half Health it breaks off a second,
	// weaker core (a tough fragment that must also be cleared).
	if m.health > 0 && hasSpecial(m, bsSplit) && !m.didSplit && m.health*2 <= m.maxHealth {
		m.didSplit = true
		g.spawnCore(t, m)
		g.log = append(g.log, m.bossName+" splits into a second core!")
	}
	if m.health <= 0 {
		m.health = 0
		m.alive = false
		g.awardKillCourage(t, h, m.reward)
		g.dropFrom(t, m, h) // loot drops on the floor; the killer on-tile grabs it
		if m.isBoss {
			if g.defeatedBosses == nil {
				g.defeatedBosses = map[string]bool{}
			}
			g.defeatedBosses[m.kind] = true // he's gone: never drawn again this run
			if g.level >= crMaxLevel {
				g.phase = crWon
				g.log = append(g.log, m.bossName+" falls! You have cleared the dungeon.")
			} else {
				g.levelCleared = true // descend at end of turn (crawler.go endTurn)
				g.log = append(g.log, m.bossName+" falls! The way down opens.")
			}
		} else {
			g.log = append(g.log, h.name+" defeats a monster.")
		}
		// Frenzy (Wolf, second special): each kill on your turn grants a free attack die
		// (a fresh High die added unspent to the pool), so kills can chain.
		if h.sheet.totem == "wolf" && h.specialActive(1) {
			g.dice = append(g.dice, crawlerDie{Value: diceHigh[g.rng.IntN(len(diceHigh))], Bonus: true})
			g.log = append(g.log, h.name+"'s Frenzy grants a free attack die.")
		}
	}
	return true
}

// awardKillCourage grants a defeated monster's Courage reward to EVERY living hero on
// the monster's tile (the group that fought it shares the spoils), plus the hero who
// landed the killing blow in case they struck from range and are not on that tile. Each
// hero is capped at the Courage cap.
func (g *crawlerGame) awardKillCourage(t *crawlerTile, killer *crawlerHero, reward int) {
	if reward <= 0 {
		return
	}
	done := map[*crawlerHero]bool{}
	grant := func(hr *crawlerHero) {
		if hr == nil || !hr.alive || done[hr] || hr.drained {
			return // Drained (status) blocks Courage gain
		}
		done[hr] = true
		hr.courage += reward
		if hr.courage > crCourageCap {
			hr.courage = crCourageCap
		}
	}
	for _, s := range g.order {
		if hr := g.players[s]; hr.tileID == t.id {
			grant(hr)
		}
	}
	grant(killer)
}

// effectiveArmour is the die value an attack needs to land on m: the monster's Armour,
// lowered by 1 while the Arrow Squid's Barbed Shot (first special) is active (minimum
// 1). The client mirrors this so the Attack buttons it offers match what the server
// will accept.
func (g *crawlerGame) effectiveArmour(h *crawlerHero, m *crawlerMonster) int {
	a := m.armour + m.plating // Adaptive Plating's climbing Armour bonus
	if h.sheet.totem == "pijlinktvis" && h.specialActive(0) {
		a--
	}
	// Pylon's Jammer Field (boss special): while it lives, every OTHER monster is +1 Armour.
	if !hasSpecial(m, bsJammer) && g.jammerActive() {
		a++
	}
	// A Jammer ROBOT buffs the other robots sharing its tile by +1 Armour (kill it first).
	a += g.tileJammerBonus(m)
	if a < 1 {
		a = 1
	}
	return a
}

// tileJammerBonus is +1 if a living Jammer robot shares m's tile (and is not m itself),
// else 0: the Jammer support bot hardens the robots around it.
func (g *crawlerGame) tileJammerBonus(m *crawlerMonster) int {
	for _, t := range g.tiles {
		onTile := false
		for _, x := range t.monsters {
			if x == m {
				onTile = true
				break
			}
		}
		if !onTile {
			continue
		}
		for _, x := range t.monsters {
			if x != m && x.alive && x.kind == "jammer" {
				return 1
			}
		}
		return 0
	}
	return 0
}

// weakPointAlive reports whether tile t still holds a live Hardened-Core weak-point.
func (g *crawlerGame) weakPointAlive(t *crawlerTile) bool {
	for _, m := range t.monsters {
		if m.alive && m.weakPoint {
			return true
		}
	}
	return false
}

// jammerActive reports whether a live Jammer-Field boss (Pylon) is on the board.
func (g *crawlerGame) jammerActive() bool {
	for _, t := range g.tiles {
		for _, m := range t.monsters {
			if m.alive && hasSpecial(m, bsJammer) {
				return true
			}
		}
	}
	return false
}

// monsterDamage is a monster's strike Damage after Enrage (+1 per round it has
// survived). Overload is a status applier, not a damage boost (see resolveRound).
func (g *crawlerGame) monsterDamage(m *crawlerMonster) int {
	return m.damage + m.enrage
}

// freeSquares lists the walkable squares of tile t not held by a live monster.
func (g *crawlerGame) freeSquares(t *crawlerTile) []int {
	w := crawlerWalkable(t.edges)
	taken := map[int]bool{}
	for _, m := range t.monsters {
		if m.alive {
			taken[m.square] = true
		}
	}
	var out []int
	for _, s := range crawlerSquareOrder {
		if w[s] && !taken[s] {
			out = append(out, s)
		}
	}
	return out
}

// spawnMonsters places the given adds on tile t's free squares; extras are dropped when
// the tile is full.
func (g *crawlerGame) spawnMonsters(t *crawlerTile, adds []*crawlerMonster) {
	free := g.freeSquares(t)
	for i, a := range adds {
		if i >= len(free) {
			return
		}
		a.square = free[i]
		t.monsters = append(t.monsters, a)
	}
}

// spawnCore breaks a splitting boss into a second, weaker core (half the boss's max
// Health) on a free square of its tile; it must also be cleared.
func (g *crawlerGame) spawnCore(t *crawlerTile, boss *crawlerMonster) {
	hp := boss.maxHealth / 2
	if hp < 1 {
		hp = 1
	}
	g.spawnMonsters(t, []*crawlerMonster{{
		armour: boss.armour, health: hp, maxHealth: hp, damage: boss.damage, kind: "hulk", alive: true, reward: 2,
	}})
}

// electrifyRandom applies Electrified to one random living hero (Overload). Grounding
// Boots make a hero immune.
func (g *crawlerGame) electrifyRandom(name string) {
	var cand []*crawlerHero
	for _, s := range g.order {
		h := g.players[s]
		if h.alive && !h.electrified && !heroHas(h, "grounding-boots") {
			cand = append(cand, h)
		}
	}
	if len(cand) == 0 {
		return
	}
	tgt := cand[g.rng.IntN(len(cand))]
	tgt.electrified = true
	g.log = append(g.log, name+" electrifies "+tgt.name+".")
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
	if h.health >= h.sheet.maxHP && !heroHasStatus(h) {
		return // nothing to heal and no status to wash off
	}
	h.health = h.sheet.maxHP // a spring restores the hero to FULL health on entry
	heroClearStatuses(h)     // and washes off every status
	g.log = append(g.log, h.name+" is restored to full health by the spring.")
}

// maybeShop clears a hero's statuses the moment they STEP ONTO a shop tile (the kangaroo
// merchant holds the centre, so a hero reaches the tile via its doorway; the shop opens on
// arrival, no need to reach the middle). The note lists "shop" as a status-repair. Free,
// repeatable.
func (g *crawlerGame) maybeShop(seat int) {
	h := g.players[seat]
	t := g.tiles[h.tileID]
	if t == nil || t.kind != crKindShop || !h.alive {
		return
	}
	if heroClearStatuses(h) {
		g.log = append(g.log, h.name+" is patched up at the shop.")
	}
}

// attackTarget resolves which tile an attack lands on. tileID 0 or the hero's own
// tile means melee (your tile). Any other tile is allowed only for a ranged totem and
// only if it is directly reachable through an open edge of the hero's tile.
func (g *crawlerGame) attackTarget(h *crawlerHero, tileID int) *crawlerTile {
	own := g.tiles[h.tileID]
	if tileID == 0 || tileID == h.tileID || own == nil {
		return own
	}
	// Ranged reach: the Arrow Squid's sheet, or anyone carrying a Scope (object).
	if !h.sheet.ranged && !heroHas(h, "scope") {
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
	tgt.electrified = false // a heal clears Electrified
	g.log = append(g.log, src.name+" mends "+tgt.name+".")
	return true
}

// bloomHeal is the Treefrog's Bloom: heal 1 to every OTHER living hero sharing the
// Treefrog's tile (the primary Mend target is already healed). Caps at each hero's max.
func (g *crawlerGame) bloomHeal(src *crawlerHero) {
	n := 0
	for _, s := range g.order {
		a := g.players[s]
		if a == src || !a.alive || a.tileID != src.tileID || a.health >= a.sheet.maxHP {
			continue
		}
		a.health++
		a.electrified = false
		n++
	}
	if n > 0 {
		g.log = append(g.log, src.name+"'s Bloom heals the party.")
	}
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
	// Boss round-end specials, each on a surviving boss.
	for _, t := range g.tiles {
		for _, m := range t.monsters {
			if !m.alive || !m.isBoss {
				continue
			}
			m.engaged = true
			// Shockwave: an extra 1 to every hero on its tile.
			if hasSpecial(m, bsShockwave) {
				for _, s := range g.order {
					if h := g.players[s]; h.alive && h.tileID == t.id {
						g.hurt(h, 1)
					}
				}
			}
			// Overload: Electrified to a random hero.
			if hasSpecial(m, bsOverload) {
				g.electrifyRandom(m.bossName)
			}
			// Adaptive Plating climbs; Enrage's Damage climbs.
			if hasSpecial(m, bsAdaptive) {
				m.plating++
			}
			if hasSpecial(m, bsEnrage) {
				m.enrage++
			}
		}
	}
	// Electrified ticks for 1 on each afflicted hero.
	for _, s := range g.order {
		if h := g.players[s]; h.alive && h.electrified {
			g.hurt(h, 1)
		}
	}
	for _, s := range g.order {
		g.players[s].shields = 0
	}
	if g.aliveHeroes() == 0 {
		g.phase = crLost
		g.log = append(g.log, "The party has fallen. The dungeon wins.")
		return
	}
	// Summoners reinforce at round end, so the adds are present next round.
	for _, t := range g.tiles {
		for _, m := range t.monsters {
			if m.alive {
				if adds := summonAdds(m); len(adds) > 0 {
					g.spawnMonsters(t, adds)
					g.log = append(g.log, m.bossName+" summons reinforcements.")
				}
			}
		}
	}
}

// hurt applies raw damage to a hero (through shields), used by board-wide effects like
// Shockwave that are not a single monster-on-tile strike.
func (g *crawlerGame) hurt(h *crawlerHero, dmg int) {
	if h.shields >= dmg {
		h.shields -= dmg
		return
	}
	dmg -= h.shields
	h.shields = 0
	h.health -= dmg
	if h.health <= 0 {
		h.health = 0
		h.alive = false
		g.log = append(g.log, h.name+" falls.")
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
	base := g.monsterDamage(m)
	dmg := base - tgt.shields
	if tgt.shields > base {
		tgt.shields -= base
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
		return
	}
	// The robot's sting rolls its status die (Level 2+): a Scrapling can Electrocute
	// (Rusted deep in the machine rooms). Bosses inflict via their own specials instead.
	g.rollStatusOnHit(m, tgt)
}

// allyOnTile reports whether a living hero other than h shares h's tile.
func (g *crawlerGame) allyOnTile(h *crawlerHero) bool {
	for _, s := range g.order {
		a := g.players[s]
		if a != h && a.alive && a.tileID == h.tileID {
			return true
		}
	}
	return false
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
