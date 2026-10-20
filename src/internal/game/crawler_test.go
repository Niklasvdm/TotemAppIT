package game

import (
	"encoding/json"
	"testing"
)

func crLast(t *testing.T, f *fakeOutbox, seat int) crawlerView {
	t.Helper()
	v, ok := f.last[seat].(crawlerView)
	if !ok {
		t.Fatalf("seat %d has no crawlerView", seat)
	}
	return v
}

func crCmd(t *testing.T, g *crawlerGame, seat int, host bool, m map[string]any) {
	t.Helper()
	b, _ := json.Marshal(m)
	g.Command(seat, host, b)
}

// rawCrawler starts a run and leaves it in the opening LAY phase (turn 0 is about
// to place its tile). Used by the lay-phase tests; most other tests use
// startedCrawler, which fast-forwards past the lay phase.
func rawCrawler(t *testing.T, n int) (*crawlerGame, *fakeOutbox) {
	t.Helper()
	seats := make([]int, n)
	for i := range seats {
		seats[i] = i
	}
	fo := newFakeOutbox(seats...)
	g := newCrawlerGame(fo, 7).(*crawlerGame)
	g.AddPlayer("beaver", "bever")
	for i := 1; i < n; i++ {
		g.AddPlayer("ally", "")
	}
	crCmd(t, g, 0, true, map[string]any{"t": crMsgStart})
	if g.phase != crPlaying || g.step != crStepLay {
		t.Fatalf("a run should open in the LAY phase: phase %q step %q", g.phase, g.step)
	}
	return g, fo
}

// startedCrawler starts a run and fast-forwards to the SPEND step on a pristine
// start cross (party on its centre), ready for tests to drive dice directly. A real
// run now opens with the global LAY phase; that phase has its own tests
// (TestCrawlerLayPhase*), so here we discard it and set up the clean spend state the
// dice/combat/movement tests expect.
func startedCrawler(t *testing.T, n int) (*crawlerGame, *fakeOutbox) {
	t.Helper()
	g, fo := rawCrawler(t, n)
	g.tiles = map[int]*crawlerTile{}
	g.at = map[[2]int]int{}
	g.nextID = 0
	g.pending = nil
	start := g.placeTile(0, 0, [4]bool{true, true, true, true}, 0, "cross", false, nil)
	for _, s := range g.order {
		h := g.players[s]
		h.tileID = start.id
		h.square = 4
	}
	g.beginTurn(g.firstAlive())
	if g.phase != crPlaying || g.step != crStepSpend {
		t.Fatalf("after fast-forward: phase %q step %q", g.phase, g.step)
	}
	return g, fo
}

// crLay lays the pending tile for the hero on turn, picking its first option (used
// to drive the LAY phase in tests).
func crLay(t *testing.T, g *crawlerGame) {
	t.Helper()
	if g.pending == nil || len(g.pending.options) == 0 {
		t.Fatalf("no pending tile to lay (step=%q)", g.step)
	}
	o := g.pending.options[0]
	crCmd(t, g, g.turn, false, map[string]any{"t": crMsgLay, "x": o.X, "y": o.Y, "rotation": o.Rotation})
}

// advanceToSpend plays out any pending LAY phase so the next SPEND turn can be
// driven directly. A round now opens with the lay phase, so a test that acts again
// after endTurn must step past it first.
func advanceToSpend(t *testing.T, g *crawlerGame) {
	t.Helper()
	for g.phase == crPlaying && g.step == crStepLay {
		crLay(t, g)
	}
}

func TestCrawlerMenuMinValues(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	s := g.players[0].sheet // Beaver
	// Move >=1, Brace >=2, Courage >=5, and Attack has NO minimum (any die; the hit
	// is decided by the monster's Armour, not the menu).
	for _, tc := range []struct {
		action string
		min    int
	}{{actMove, 1}, {actBrace, 2}, {actAttack, 1}, {actCourage, 5}} {
		if m, _ := s.min(tc.action); m != tc.min {
			t.Fatalf("%s min = %d, want %d", tc.action, m, tc.min)
		}
	}
	// Any die qualifies for Attack now; a 4 can still Move (go down).
	if m, _ := s.min(actAttack); m != 1 {
		t.Fatalf("attack should accept any die (min 1), got %d", m)
	}
	if v := 4; v < func() int { m, _ := s.min(actMove); return m }() {
		t.Fatal("a 4 should qualify for Move")
	}
}

func TestCrawlerWeightedDicePerCharacter(t *testing.T) {
	// Each designed totem rolls its own weighted die faces, not a plain d6.
	for _, tc := range []struct {
		totem   string
		faces   []int
		hp      int
		diceN   int
		attackN int // innate High attack dice on top of diceN (the Healer gets 1)
	}{
		{"bever", diceLow, 7, 4, 0},
		{"pijlinktvis", diceHigh, 4, 3, 0},
		{"boomkikker", diceLow, 6, 4, 1}, // 4 Low + 1 attack die = 5 rolled
		{"wasbeer", diceBalanced, 5, 5, 0},
		{"comorenwever", diceLow, 5, 5, 0},
		{"wolf", diceHigh, 5, 4, 0},
	} {
		s := sheetFor(tc.totem)
		if s.maxHP != tc.hp || s.diceN != tc.diceN || s.attackDiceN != tc.attackN {
			t.Fatalf("%s: hp=%d diceN=%d attackN=%d, want %d/%d/%d", tc.totem, s.maxHP, s.diceN, s.attackDiceN, tc.hp, tc.diceN, tc.attackN)
		}
		if len(s.faces) != len(tc.faces) {
			t.Fatalf("%s: faces=%v want %v", tc.totem, s.faces, tc.faces)
		}
		for i := range s.faces {
			if s.faces[i] != tc.faces[i] {
				t.Fatalf("%s: faces=%v want %v", tc.totem, s.faces, tc.faces)
			}
		}
	}
}

func TestCrawlerAttackTiedToArmourNotDieMin(t *testing.T) {
	// Attack has no fixed menu minimum: the enemy's Armour is the threshold. A die below
	// the Armour cannot land, so it is refused (not a wasted "miss"), and a die that
	// meets the Armour, however low the menu would rank it, lands a hit.
	g, _ := startedCrawler(t, 1)
	m := setupFight(g, 4, 2, 1) // Armour 4
	g.dice = []crawlerDie{{Value: 2}, {Value: 5}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0}) // a 2 vs Armour 4
	if g.dice[0].Spent {
		t.Fatal("a 2 vs Armour 4 cannot land, so attacking with it must be refused (die not spent)")
	}
	if m.health != 2 {
		t.Fatalf("a refused attack must not change Health: %d", m.health)
	}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 1, "action": actAttack, "monster": 0}) // a 5 vs Armour 4
	if !g.dice[1].Spent || m.health != 1 {
		t.Fatalf("a 5 vs Armour 4 should land a hit: spent=%v Health %d", g.dice[1].Spent, m.health)
	}
}

func TestCrawlerMendHeals(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.players[0].sheet = healerSheet("boomkikker") // Mend special
	g.players[0].health = 3
	g.dice = []crawlerDie{{Value: 2}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMend, "ally": 0})
	if g.players[0].health != 5 {
		t.Fatalf("Mend should heal 2 (3 -> 5), got %d", g.players[0].health)
	}
}

// setupFight puts the Beaver on a tile with a single monster so combat can be
// driven directly, bypassing exploration.
func setupFight(g *crawlerGame, a, h, d int) *crawlerMonster {
	m := &crawlerMonster{armour: a, health: h, maxHealth: h, damage: d, square: 4, alive: true, reward: 1}
	t := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, "cross", false, []*crawlerMonster{m})
	hero := g.players[0]
	hero.tileID = t.id
	hero.square = 4
	return m
}

func TestCrawlerPilferGrabsAdjacentLoot(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.sheet = tricksterSheet("wasbeer") // Pilfer die-special
	h.objects = nil
	nt := g.placeTile(1, 0, [4]bool{true, true, true, true}, 0, "cross", false, nil) // east neighbour
	nt.loot = []crawlerGroundItem{{square: 4, slug: "medkit"}}
	g.dice = []crawlerDie{{Value: 3}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actPilfer, "dir": 1})
	if !g.dice[0].Spent {
		t.Fatal("Pilfer should spend the die")
	}
	if len(h.objects) != 1 || h.objects[0] != "medkit" {
		t.Fatalf("Pilfer should pull the medkit into the pack, got %v", h.objects)
	}
	if len(nt.loot) != 0 {
		t.Fatalf("Pilfer should remove the item from the neighbour tile, got %v", nt.loot)
	}
}

func TestCrawlerTricksterLootOnKill(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.sheet = tricksterSheet("wasbeer")
	h.objects = nil
	m := &crawlerMonster{armour: 3, health: 1, maxHealth: 1, damage: 1, square: 0, alive: true, reward: 1, kind: "scrapling"}
	g.tiles[h.tileID].monsters = []*crawlerMonster{m}
	h.square = 4
	g.dice = []crawlerDie{{Value: 5}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	if m.alive {
		t.Fatal("a 5 vs Armour 3 should kill the H1 scrapling")
	}
	// The Trickster's inherent +1 loot draw guarantees at least one consumable on a kill
	// (independent of the drop chance); the killer standing on the tile auto-picks it up.
	if len(h.objects) < 1 {
		t.Fatalf("Trickster should gain at least one consumable on a kill, got %v", h.objects)
	}
}

func TestCrawlerGroundingBootsNegateWiringDamage(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	boots, bare := g.players[0], g.players[1]
	g.level = 2 // hazards only fire from Level 2
	boots.objects = []string{"grounding-boots"}
	boots.health, bare.health = boots.sheet.maxHP, bare.sheet.maxHP
	for _, h := range []*crawlerHero{boots, bare} {
		t2 := g.tiles[h.tileID]
		t2.hazards = []crawlerHazard{{kind: hzWiring, square: h.square}}
		g.enterHazard(h)
	}
	if boots.health != boots.sheet.maxHP {
		t.Fatalf("Grounding Boots should negate live-wiring damage, took %d", boots.sheet.maxHP-boots.health)
	}
	if bare.health != bare.sheet.maxHP-1 {
		t.Fatalf("a bare hero should take 1 from live wiring, health %d", bare.health)
	}
}

func TestCrawlerCombatKillsGor(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	m := setupFight(g, 4, 2, 3) // Gor: A4 H2 D3
	// Rig the dice to 5,4,2,2 and assign the 5 and 4 as attacks.
	g.dice = []crawlerDie{{Value: 5}, {Value: 4}, {Value: 2}, {Value: 2}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 1, "action": actAttack, "monster": 0})
	// Two dice beat Armour 4, so the Health-2 Gor dies immediately (no End Turn).
	if m.alive || m.health != 0 {
		t.Fatalf("the Gor should be dead: alive=%v health=%d", m.alive, m.health)
	}
	if g.players[0].courage != 1 {
		t.Fatalf("a kill grants +1 Courage, got %d", g.players[0].courage)
	}
	hpBefore := g.players[0].health
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	if g.players[0].health != hpBefore {
		t.Fatal("a killed monster deals no damage")
	}
}

func TestCrawlerDamagePersistsAcrossTurns(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	m := setupFight(g, 3, 3, 1) // Health 3; we land one hit per turn
	// Turn 1: one hit -> Health 2 (persists), then take 1 retaliation.
	g.dice = []crawlerDie{{Value: 5}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	if m.health != 2 {
		t.Fatalf("one hit should leave Health 2, got %d", m.health)
	}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn}) // round 2: lay phase, then this hero again
	// The monster's Health must NOT have reset over the round boundary.
	if m.health != 2 {
		t.Fatalf("damage must persist across turns, got Health %d", m.health)
	}
	advanceToSpend(t, g) // play past the new round's mandatory lay phase
	g.dice = []crawlerDie{{Value: 5}, {Value: 5}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 1, "action": actAttack, "monster": 0})
	if m.alive {
		t.Fatal("whittled down over turns, the monster should finally be defeated")
	}
}

func TestCrawlerCombatSurvivesAndHits(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	m := setupFight(g, 4, 2, 3) // needs 2 hits; we land only one
	g.dice = []crawlerDie{{Value: 5}, {Value: 2}, {Value: 2}, {Value: 2}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	hp := g.players[0].health
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	if !m.alive {
		t.Fatal("one hit should not kill a Health-2 monster")
	}
	if g.players[0].health != hp-3 {
		t.Fatalf("a surviving D3 monster deals 3 (no shields): %d -> %d", hp, g.players[0].health)
	}
}

func TestCrawlerShieldsAbsorbDamage(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	m := setupFight(g, 4, 2, 3)
	g.players[0].shields = 2
	g.dice = []crawlerDie{{Value: 5}, {Value: 2}, {Value: 2}, {Value: 2}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	hp := g.players[0].health
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	_ = m
	// D3 minus 2 Shields = 1 damage; shields drop to 0.
	if g.players[0].health != hp-1 || g.players[0].shields != 0 {
		t.Fatalf("shields: hp %d->%d, shields=%d", hp, g.players[0].health, g.players[0].shields)
	}
}

func TestCrawlerEndTurnNextToEnemyTakesDamage(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	setupFight(g, 3, 2, 2) // a live monster shares the hero's tile; we do NOT attack it
	g.players[0].shields = 0
	g.dice = []crawlerDie{{Value: 1}, {Value: 1}, {Value: 1}, {Value: 1}}
	hp := g.players[0].health
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	if g.players[0].health != hp-2 {
		t.Fatalf("ending a turn on a live enemy's tile (even without attacking) should take Damage 2: %d -> %d", hp, g.players[0].health)
	}
}

func TestCrawlerShieldsResetEachRound(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.courage = 3 // Thick Hide is auto-active at >=3 Courage
	h.shields = 4 // e.g. 2 from Thick Hide + 2 from a Brace this round
	g.dice = []crawlerDie{{Value: 1}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	advanceToSpend(t, g) // round end clears shields, then the new round's spend turn
	// Shields do not carry over; the next round starts with just Thick Hide's 2.
	if g.players[0].shields != 2 {
		t.Fatalf("shields must not stack across rounds: want 2 (Thick Hide re-grant), got %d", g.players[0].shields)
	}
}

func TestCrawlerThickHideDeactivatesBelowThreshold(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.courage = 2 // below 3: Thick Hide is NOT active
	g.dice = []crawlerDie{{Value: 1}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	advanceToSpend(t, g)
	if g.players[0].shields != 0 {
		t.Fatalf("Thick Hide should be inactive below 3 Courage, got %d shields", g.players[0].shields)
	}
}

func TestCrawlerGroupFightDamagesAllHeroesOnTileAtRoundEnd(t *testing.T) {
	// Two heroes share a monster's tile. Neither takes damage mid-round; both take it
	// once, at the end of the round, after every hero has acted.
	g, _ := startedCrawler(t, 2)
	m := &crawlerMonster{armour: 3, health: 5, maxHealth: 5, damage: 2, square: 4, alive: true, reward: 1}
	t2 := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, "cross", false, []*crawlerMonster{m})
	for s := 0; s < 2; s++ {
		g.players[s].tileID = t2.id
		g.players[s].square = 4
		g.players[s].shields = 0
	}
	hp0, hp1 := g.players[0].health, g.players[1].health
	g.dice = []crawlerDie{{Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn}) // hero 0 ends; no damage yet
	if g.players[0].health != hp0 || g.players[1].health != hp1 {
		t.Fatal("no hero should take damage before the whole round is over")
	}
	g.dice = []crawlerDie{{Value: 1}}
	crCmd(t, g, 1, false, map[string]any{"t": crMsgEndTurn}) // hero 1 ends: round over, monster strikes both
	if g.players[0].health != hp0-2 || g.players[1].health != hp1-2 {
		t.Fatalf("both heroes on the tile should take Damage 2 at round end: %d, %d", g.players[0].health, g.players[1].health)
	}
	if !m.engaged {
		t.Fatal("a monster that struck should be flagged engaged (for its attack animation)")
	}
}

func TestCrawlerMaxCourageBlocksMoreCourage(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.players[0].courage = crCourageCap
	g.dice = []crawlerDie{{Value: 6}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actCourage})
	if g.dice[0].Spent {
		t.Fatal("Courage at the cap should refuse the action and not consume the die")
	}
	if g.players[0].courage != crCourageCap {
		t.Fatalf("courage should stay at the cap, got %d", g.players[0].courage)
	}
}

func TestCrawlerBraceGrantsShields(t *testing.T) {
	g, _ := startedCrawler(t, 2) // Beaver + ally, both on the start tile
	g.dice = []crawlerDie{{Value: 2}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actBrace, "ally": 1})
	if g.players[1].shields != 2 {
		t.Fatalf("Brace should give an ally 2 shields, got %d", g.players[1].shields)
	}
}

func TestCrawlerCourageEconomy(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.dice = []crawlerDie{{Value: 6}, {Value: 6}, {Value: 6}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actCourage})
	if g.players[0].courage != 1 {
		t.Fatalf("spending a 6 on Courage gives +1, got %d", g.players[0].courage)
	}
	// Reroll costs 1 Courage.
	crCmd(t, g, 0, false, map[string]any{"t": crMsgReroll, "die": 1})
	if g.players[0].courage != 0 {
		t.Fatalf("a reroll should cost 1 Courage, got %d", g.players[0].courage)
	}
}

func TestCrawlerSpecialsAutoActivateByCourage(t *testing.T) {
	// No manual unlock: a special is active exactly while Courage is at/above its
	// threshold (3, then 5), and switches off again when Courage drops.
	h := &crawlerHero{sheet: guardianSheet("bever")}
	for _, tc := range []struct {
		courage int
		a0, a1  bool
	}{
		{2, false, false}, {3, true, false}, {4, true, false}, {5, true, true}, {4, true, false}, {0, false, false},
	} {
		h.courage = tc.courage
		if h.specialActive(0) != tc.a0 || h.specialActive(1) != tc.a1 {
			t.Fatalf("courage %d: active=%v/%v want %v/%v", tc.courage, h.specialActive(0), h.specialActive(1), tc.a0, tc.a1)
		}
	}
}

func TestCrawlerMoveWithinTile(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.players[0].square = 4 // centre
	g.dice = []crawlerDie{{Value: 1}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMove, "dir": 0}) // N
	if g.players[0].square != 1 {
		t.Fatalf("moving N from centre should land on square 1, got %d", g.players[0].square)
	}
}

func TestCrawlerStraightBlocksSideMove(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	// Lay a straight (N-S) tile under the hero and stand on its centre.
	st := g.placeTile(0, 1, [4]bool{true, false, true, false}, 0, "straight", false, nil)
	g.players[0].tileID = st.id
	g.players[0].square = 4
	g.dice = []crawlerDie{{Value: 6}, {Value: 6}, {Value: 6}, {Value: 6}}
	// Moving E (dir 1) off the corridor must fail: the side square is off the path.
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMove, "dir": 1})
	if g.players[0].square != 4 {
		t.Fatalf("a side move on a straight tile should be blocked; square=%d", g.players[0].square)
	}
	if g.dice[0].Spent {
		t.Fatal("a blocked move must not consume the die")
	}
	// Moving N (dir 0) along the corridor works.
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMove, "dir": 0})
	if g.players[0].square != crawlerDoor[0] {
		t.Fatalf("moving N along the corridor should reach the north doorway; square=%d", g.players[0].square)
	}
}

func TestCrawlerMoveLaysTileAtFrontier(t *testing.T) {
	// Stepping out of the start tile onto a frontier draws a tile, pauses to lay it
	// (rotation only), and placing it steps the hero onto the new tile.
	g, fo := startedCrawler(t, 1)
	g.players[0].square = crawlerDoor[0] // north doorway
	g.dice = []crawlerDie{{Value: 1}, {Value: 1}, {Value: 1}, {Value: 1}}
	before := len(g.tiles)
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMove, "dir": 0})
	if g.step != crStepLay || g.pending == nil {
		t.Fatalf("stepping onto a frontier should pause to lay; step=%q pending=%v", g.step, g.pending)
	}
	v := crLast(t, fo, 0)
	if v.Pending == nil || len(v.Pending.Options) == 0 {
		t.Fatal("the view should offer rotation options")
	}
	for _, o := range v.Pending.Options {
		if o.X != 0 || o.Y != -1 {
			t.Fatalf("lay options must all be at the entered frontier (0,-1); got (%d,%d)", o.X, o.Y)
		}
		if !o.Edges[crawlerOpp(0)] {
			t.Fatalf("option %+v has no opening on the entry (south) side", o)
		}
	}
	o := v.Pending.Options[0]
	crCmd(t, g, 0, false, map[string]any{"t": crMsgLay, "x": o.X, "y": o.Y, "rotation": o.Rotation})
	if len(g.tiles) != before+1 {
		t.Fatalf("exactly one tile should be laid: %d -> %d", before, len(g.tiles))
	}
	if g.step != crStepSpend || g.pending != nil {
		t.Fatalf("after laying, play returns to the spend step; step=%q", g.step)
	}
	if g.players[0].tileID != g.at[[2]int{0, -1}] {
		t.Fatal("the hero should have stepped onto the newly laid tile")
	}
}

func TestCrawlerLayOnAllFourSides(t *testing.T) {
	// The start cross is open on all four sides, so stepping out each doorway lays a
	// tile at the frontier in that direction.
	for d := 0; d < 4; d++ {
		g, _ := startedCrawler(t, 1)
		g.players[0].square = crawlerDoor[d]
		g.dice = []crawlerDie{{Value: 1}, {Value: 1}, {Value: 1}, {Value: 1}}
		crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMove, "dir": d})
		if g.step != crStepLay || g.pending == nil || len(g.pending.options) == 0 {
			t.Fatalf("dir %d: expected a lay with options, step=%q", d, g.step)
		}
		o := g.pending.options[0]
		wantX, wantY := crawlerDelta[d][0], crawlerDelta[d][1]
		if o.X != wantX || o.Y != wantY {
			t.Fatalf("dir %d: tile laid at (%d,%d), want (%d,%d)", d, o.X, o.Y, wantX, wantY)
		}
		crCmd(t, g, 0, false, map[string]any{"t": crMsgLay, "x": o.X, "y": o.Y, "rotation": o.Rotation})
		if _, ok := g.at[[2]int{wantX, wantY}]; !ok {
			t.Fatalf("dir %d: no tile placed at the frontier", d)
		}
	}
}

func TestCrawlerLayOptionsAlwaysConnect(t *testing.T) {
	// Across many seeds, every offered lay option opens on the entry side (so the
	// path connects) and matches its placed neighbours.
	for seed := uint64(1); seed <= 60; seed++ {
		g := newCrawlerGame(newFakeOutbox(0), seed).(*crawlerGame)
		g.AddPlayer("beaver", "bever")
		crCmd(t, g, 0, true, map[string]any{"t": crMsgStart})
		for d := 0; d < 4; d++ {
			if g.deckPos >= len(g.deck) {
				break
			}
			nx, ny := crawlerDelta[d][0], crawlerDelta[d][1]
			for _, o := range g.layOptions(g.deck[g.deckPos], d, nx, ny) {
				if !o.Edges[crawlerOpp(d)] {
					t.Fatalf("seed %d dir %d: option without an opening on the entry side", seed, d)
				}
				if !g.edgesFit(o.X, o.Y, o.Edges) {
					t.Fatalf("seed %d dir %d: option does not fit neighbours", seed, d)
				}
			}
		}
	}
}

func TestCrawlerLayPhaseOffersConnectedPlacementsNoMove(t *testing.T) {
	// A run opens in the LAY phase offering placements across the frontier (not one
	// server-chosen cell): the hero picks a cell AND a rotation. Every option fits its
	// neighbours and joins the map. Laying one does NOT move the hero, and for a single
	// hero it then opens the SPEND phase.
	g, _ := rawCrawler(t, 1)
	if g.pending == nil || !g.pending.phaseLay {
		t.Fatalf("the lay phase should offer a proactive (phaseLay) tile; pending=%v", g.pending)
	}
	cells := map[[2]int]bool{}
	for _, o := range g.pending.options {
		cells[[2]int{o.X, o.Y}] = true
		if !g.edgesFit(o.X, o.Y, o.Edges) {
			t.Fatalf("lay-phase option %+v does not fit its neighbours", o)
		}
		if !g.connects(o.X, o.Y, o.Edges) {
			t.Fatalf("lay-phase option %+v does not join the map", o)
		}
	}
	// The start cross is open on all four sides, so the frontier (and the placements)
	// span more than one cell: the player really does get a choice of where.
	if len(cells) < 2 {
		t.Fatalf("the lay phase should offer a choice of cells, got %d", len(cells))
	}
	startTile := g.players[0].tileID
	before := len(g.tiles)
	crLay(t, g)
	if g.step != crStepSpend {
		t.Fatalf("after the only hero lays, the spend phase should open; step=%q", g.step)
	}
	if len(g.tiles) != before+1 {
		t.Fatalf("exactly one tile should be laid: %d -> %d", before, len(g.tiles))
	}
	if g.players[0].tileID != startTile {
		t.Fatal("a proactive lay must not move the hero")
	}
}

func TestCrawlerLayPhaseBoxedHeroStillLays(t *testing.T) {
	// Regression: a hero whose own tile is fully surrounded by placed tiles must still
	// get to lay, at a frontier elsewhere on the map. The old single-cell lay only
	// looked next to the hero's tile and skipped such a hero entirely ("can't lay").
	g := newCrawlerGame(newFakeOutbox(0), 7).(*crawlerGame)
	g.AddPlayer("beaver", "bever")
	g.phase = crPlaying
	g.bands = 3
	g.grace = -1 // no collapse countdown yet (startLevel sets this in a real run)
	g.buildDeck()
	c := g.placeTile(0, 0, [4]bool{true, true, true, true}, 0, "cross", false, nil) // hero's tile, open all sides
	g.players[0].tileID = c.id
	g.players[0].square = 4
	// Box the centre in: three dead-ends that only open back toward it, so it has no
	// adjacent empty cell; the west neighbour is a straight that also opens on to a
	// real frontier at (-2,0).
	g.placeTile(0, -1, [4]bool{false, false, true, false}, 0, "dead-end", false, nil) // opens S (to centre)
	g.placeTile(1, 0, [4]bool{false, false, false, true}, 0, "dead-end", false, nil)  // opens W (to centre)
	g.placeTile(0, 1, [4]bool{true, false, false, false}, 0, "dead-end", false, nil)  // opens N (to centre)
	g.placeTile(-1, 0, [4]bool{false, true, false, true}, 0, "straight", false, nil)  // opens E (centre) + W (frontier)
	g.turn = 0
	g.beginLayPhase()
	if g.step != crStepLay || g.pending == nil || len(g.pending.options) == 0 {
		t.Fatalf("a boxed-in hero should still be offered a lay elsewhere; step=%q pending=%v", g.step, g.pending)
	}
	for _, o := range g.pending.options {
		if o.X != -2 || o.Y != 0 {
			t.Fatalf("the only frontier is (-2,0); got an option at (%d,%d)", o.X, o.Y)
		}
	}
}

func TestCrawlerLayPhaseSweepsPartyThenSpend(t *testing.T) {
	// Every living hero lays once, in seat order, before anyone spends.
	g, _ := rawCrawler(t, 3)
	for seat := 0; seat < 3; seat++ {
		if g.step != crStepLay {
			t.Fatalf("hero %d: expected the lay phase, step=%q", seat, g.step)
		}
		if g.turn != seat {
			t.Fatalf("lay phase should reach seat %d in order, turn=%d", seat, g.turn)
		}
		crLay(t, g)
	}
	if g.step != crStepSpend || g.turn != 0 {
		t.Fatalf("after all three lay, the spend phase opens on seat 0; step=%q turn=%d", g.step, g.turn)
	}
	if len(g.tiles) != 4 { // start cross + one per hero
		t.Fatalf("three heroes should have laid three tiles (plus the start): %d", len(g.tiles))
	}
}

func TestCrawlerNewRoundReturnsToLay(t *testing.T) {
	// After the last hero spends, the next round opens with the LAY phase again.
	g, _ := startedCrawler(t, 1)
	g.dice = []crawlerDie{{Value: 1}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	if g.step != crStepLay || g.pending == nil || !g.pending.phaseLay {
		t.Fatalf("ending the last hero's turn should open a new round's lay phase; step=%q pending=%v", g.step, g.pending)
	}
}

func TestCrawlerStackEmptyGivesGraceThenCollapses(t *testing.T) {
	// Running the tile stack out is no longer an instant loss: it starts a collapse
	// countdown (crGraceTurns) and only loses when that reaches zero.
	g, _ := startedCrawler(t, 1)
	g.deckPos = len(g.deck) // stack exhausted
	g.grace = -1
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	if g.phase != crPlaying {
		t.Fatalf("an empty stack must not instantly lose; phase=%q", g.phase)
	}
	if g.grace != crGraceTurns {
		t.Fatalf("the collapse countdown should start at %d, got %d", crGraceTurns, g.grace)
	}
	for i := 0; i < crGraceTurns-1; i++ {
		crCmd(t, g, g.turn, false, map[string]any{"t": crMsgEndTurn})
		if g.phase != crPlaying {
			t.Fatalf("collapsed too early (turn %d, grace=%d)", i, g.grace)
		}
	}
	crCmd(t, g, g.turn, false, map[string]any{"t": crMsgEndTurn})
	if g.phase != crLost {
		t.Fatalf("after %d turns the dungeon should collapse; phase=%q grace=%d", crGraceTurns, g.phase, g.grace)
	}
}

func TestCrawlerAutoEndsWhenAllDiceSpent(t *testing.T) {
	// Spending your last die ends the turn automatically (no need to click End turn).
	g, _ := startedCrawler(t, 2)
	g.dice = []crawlerDie{{Value: 6}} // a single die; spending it should pass the turn
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actCourage})
	if g.turn != 1 || g.step != crStepSpend {
		t.Fatalf("spending the last die should auto-end to the next hero; turn=%d step=%q", g.turn, g.step)
	}
}

func TestCrawlerBossInFinalBand(t *testing.T) {
	for seed := uint64(1); seed <= 40; seed++ {
		g := newCrawlerGame(newFakeOutbox(0), seed).(*crawlerGame)
		g.AddPlayer("beaver", "bever")
		g.bands = 3
		g.buildDeck()
		bossIdx := -1
		for i, s := range g.deck {
			if s.isBoss {
				bossIdx = i
			}
		}
		if bossIdx < 0 {
			t.Fatalf("seed %d: no boss in the deck", seed)
		}
		if g.deck[bossIdx].band != g.bands-1 {
			t.Fatalf("seed %d: boss in band %d, want final band %d", seed, g.deck[bossIdx].band, g.bands-1)
		}
	}
}

// A service tile (healing spring, transporter, shop) must NEVER be buried behind the boss:
// laying stops the moment the boss lair is laid, so a service after it is unreachable. The
// essential tiles are dealt into the stack above the boss, which this locks in over many
// seeds and every level.
func TestCrawlerEssentialServicesAlwaysBeforeBoss(t *testing.T) {
	for seed := uint64(1); seed <= 60; seed++ {
		for level := 1; level <= crMaxLevel; level++ {
			g := newCrawlerGame(newFakeOutbox(0), seed).(*crawlerGame)
			g.AddPlayer("beaver", "bever")
			g.bands = 3
			g.level = level
			g.buildDeck()
			bi := g.bossIndex()
			if bi < 0 {
				t.Fatalf("level %d seed %d: no boss in the deck", level, seed)
			}
			shop, heal, portal := 0, 0, 0
			for i, s := range g.deck {
				switch s.kind {
				case crKindShop:
					shop++
					if i >= bi {
						t.Fatalf("level %d seed %d: shop at %d is not before the boss at %d", level, seed, i, bi)
					}
				case crKindHealing:
					heal++
					if i >= bi {
						t.Fatalf("level %d seed %d: healing at %d is not before the boss at %d", level, seed, i, bi)
					}
				case crKindPortal:
					portal++
					if i >= bi {
						t.Fatalf("level %d seed %d: portal at %d is not before the boss at %d", level, seed, i, bi)
					}
				}
			}
			wantPortal := 1
			if level == 3 {
				wantPortal = 2
			}
			if shop != 1 || heal != 2 || portal != wantPortal {
				t.Fatalf("level %d seed %d: essentials shop=%d heal=%d portal=%d (want 1/2/%d)", level, seed, shop, heal, portal, wantPortal)
			}
		}
	}
}

// A tile can hold several monsters (a swarm), each on its own hero-UNREACHABLE square (so
// they never stack with a hero): corners / closed-edge midpoints, never a walkable one.
func TestCrawlerMultipleMonstersPerTile(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	mons := []*crawlerMonster{
		{armour: 3, health: 1, maxHealth: 1, damage: 1, kind: "scrapling", alive: true, reward: 1},
		{armour: 3, health: 1, maxHealth: 1, damage: 1, kind: "scrapling", alive: true, reward: 1},
		{armour: 3, health: 1, maxHealth: 1, damage: 1, kind: "scrapling", alive: true, reward: 1},
	}
	edges := [4]bool{true, true, true, true} // a cross: centre + four doorways walkable
	assignMonsterSquares(edges, mons)
	seen := map[int]bool{}
	for _, m := range mons {
		if seen[m.square] {
			t.Fatalf("two monsters share square %d: a swarm must spread out", m.square)
		}
		if crawlerWalkable(edges)[m.square] {
			t.Fatalf("robot placed on a walkable (hero) square %d", m.square)
		}
		seen[m.square] = true
	}
	// And they all surface in the tile view so the client can draw them.
	bt := g.placeTile(5, 5, edges, 1, "cross", false, mons)
	g.players[0].tileID = bt.id
	g.broadcastViews()
	v := crLast(t, g.out.(*fakeOutbox), 0)
	var tv *crawlerTileView
	for i := range v.Tiles {
		if v.Tiles[i].ID == bt.id {
			tv = &v.Tiles[i]
		}
	}
	if tv == nil || len(tv.Monsters) != 3 {
		t.Fatalf("tile view should carry 3 monsters, got %v", tv)
	}
	if tv.Monsters[0].Reward != 1 {
		t.Fatalf("monster view should carry its Courage reward, got %d", tv.Monsters[0].Reward)
	}
}

// The boss's Health scales with the party size (hpPerHero per hero), so it is always a
// positive multiple of the player count.
func TestCrawlerBossHealthScalesWithPlayers(t *testing.T) {
	for _, n := range []int{1, 2, 4, 6} {
		g := newCrawlerGame(newFakeOutbox(0), 7).(*crawlerGame)
		for i := 0; i < n; i++ {
			g.AddPlayer("hero", "")
		}
		g.bands = 3
		g.level = 1
		g.buildDeck()
		found := false
		for _, s := range g.deck {
			if !s.isBoss {
				continue
			}
			found = true
			boss := s.monsters[0] // the boss is always the first monster of its tile
			// Health = base + perHero*(players-1); bases are 5-6 and perHero 2-3 across the
			// pool, so any drawn boss sits within these bounds (the note's "base + per extra").
			lo, hi := 5+2*(n-1), 6+3*(n-1)
			if !boss.isBoss || boss.maxHealth < lo || boss.maxHealth > hi {
				t.Fatalf("%d players: boss Health %d should be base+perHero*(players-1) (%d..%d)", n, boss.maxHealth, lo, hi)
			}
		}
		if !found {
			t.Fatalf("%d players: no boss in the deck", n)
		}
	}
}

// A fast mover's Move die gives a second step the player directs separately, so the
// two squares can turn a corner (east, then north) instead of going straight.
func TestCrawlerFastMoveIsDirected(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.players[0].sheet = carrierSheet("comorenwever") // moveRange 2
	g.players[0].square = 3                           // W doorway of the start cross
	g.dice = []crawlerDie{{Value: 1}, {Value: 1}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMove, "dir": 1}) // East -> centre
	if g.players[0].square != 4 || g.bonusMoves != 1 {
		t.Fatalf("first step East should reach centre with 1 bonus move: sq=%d bonus=%d", g.players[0].square, g.bonusMoves)
	}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgStep, "dir": 0}) // North -> N doorway (a turn)
	if g.players[0].square != 1 || g.bonusMoves != 0 {
		t.Fatalf("directed second step North should reach the N doorway: sq=%d bonus=%d", g.players[0].square, g.bonusMoves)
	}
}

// The Arrow Squid has no dash: its signature is ranged attack, not movement, so there
// is no Jet action on its sheet (only Move, Attack, Courage).
func TestCrawlerSquidHasNoSpecialAction(t *testing.T) {
	s := rangedSheet("pijlinktvis")
	if s.special != "" {
		t.Fatalf("the Arrow Squid should have no active special action, got %q", s.special)
	}
	for _, m := range s.menu {
		if m.Action != actMove && m.Action != actAttack && m.Action != actCourage {
			t.Fatalf("the Squid menu should be Move/Attack/Courage only, found %q", m.Action)
		}
	}
	if !s.ranged {
		t.Fatal("the Arrow Squid should be ranged")
	}
}

// A hero on a portal's centre can teleport to another portal; a non-portal target is
// refused.
func TestCrawlerPortalTeleport(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	pa := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, crKindPortal, false, nil)
	pb := g.placeTile(8, 8, [4]bool{true, true, true, true}, 0, crKindPortal, false, nil)
	plain := g.placeTile(2, 2, [4]bool{true, true, true, true}, 0, "cross", false, nil)
	g.players[0].tileID = pa.id
	g.players[0].square = 4
	crCmd(t, g, 0, false, map[string]any{"t": crMsgPortal, "tile": plain.id})
	if g.players[0].tileID != pa.id {
		t.Fatal("teleporting to a non-portal tile must be refused")
	}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgPortal, "tile": pb.id})
	if g.players[0].tileID != pb.id || g.players[0].square != 4 {
		t.Fatalf("portal should move the hero to the other portal's centre: tile=%d sq=%d", g.players[0].tileID, g.players[0].square)
	}
}

// A healing spring restores a hero to FULL health the moment it steps onto the centre,
// costs no die, and is not consumed: stepping off and back on heals again.
func TestCrawlerHealingSpringHealsToFullOnEntryRepeatable(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	spring := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, crKindHealing, false, nil)
	h := g.players[0]
	h.tileID = spring.id
	h.square = 3 // W doorway, not yet on the spring
	h.health = 1
	g.dice = []crawlerDie{{Value: 1}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMove, "dir": 1}) // East -> centre
	if h.square != 4 || h.health != h.sheet.maxHP {
		t.Fatalf("stepping onto the spring centre should restore full health with no die: sq=%d hp=%d/%d", h.square, h.health, h.sheet.maxHP)
	}
	// Take damage, step off and back onto the centre: it heals to full again (not depleted).
	h.health = 2
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 1, "action": actMove, "dir": 0}) // North -> N door
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 2, "action": actMove, "dir": 2}) // South -> centre
	if h.square != 4 || h.health != h.sheet.maxHP {
		t.Fatalf("re-entering the spring centre should heal to full again: sq=%d hp=%d/%d", h.square, h.health, h.sheet.maxHP)
	}
}

// Defeating a monster grants its Courage reward to every living hero on its tile (the
// group that fought it), not only the one who struck the killing blow.
func TestCrawlerKillCourageSharedOnTile(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	m := &crawlerMonster{armour: 3, health: 1, maxHealth: 1, damage: 1, square: 4, alive: true, reward: 2}
	tile := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, "cross", false, []*crawlerMonster{m})
	g.players[0].tileID = tile.id
	g.players[1].tileID = tile.id // ally sharing the tile, did not attack
	g.players[0].courage = 0
	g.players[1].courage = 0
	g.turn = 0
	g.dice = []crawlerDie{{Value: 6}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	if g.players[0].courage != 2 || g.players[1].courage != 2 {
		t.Fatalf("both heroes on the tile should gain the %d reward: p0=%d p1=%d", m.reward, g.players[0].courage, g.players[1].courage)
	}
}

// Arrow Squid specials: Barbed Shot (Courage >= 3) lets a lower die land; Deep Volley
// (Courage >= 5) makes an attacking 6 count as two hits, once a turn.
func TestCrawlerSquidBarbedShotAndDeepVolley(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.players[0].sheet = rangedSheet("pijlinktvis")
	m := setupFight(g, 4, 6, 1) // Armour 4, Health 6
	g.players[0].courage = 0
	g.dice = []crawlerDie{{Value: 3}, {Value: 3}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0}) // 3 vs Armour 4, no Barbed
	if g.dice[0].Spent || m.health != 6 {
		t.Fatalf("without Barbed Shot a 3 vs Armour 4 is refused: spent=%v hp=%d", g.dice[0].Spent, m.health)
	}
	g.players[0].courage = crUnlock0Courage // Barbed Shot active: Armour effectively 3
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 1, "action": actAttack, "monster": 0})
	if !g.dice[1].Spent || m.health != 5 {
		t.Fatalf("Barbed Shot should let a 3 land on Armour 4: spent=%v hp=%d", g.dice[1].Spent, m.health)
	}
	g.players[0].courage = crUnlock1Courage // Deep Volley active
	g.dice = []crawlerDie{{Value: 6}, {Value: 6}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	if m.health != 3 {
		t.Fatalf("Deep Volley: an attacking 6 should land two hits (5 -> 3): hp=%d", m.health)
	}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 1, "action": actAttack, "monster": 0})
	if m.health != 2 {
		t.Fatalf("Deep Volley is once a turn: the second 6 lands one hit (3 -> 2): hp=%d", m.health)
	}
}

// Defeating the FINAL level's boss wins the run.
func TestCrawlerBossDefeatWins(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.level = crMaxLevel // final boss: its defeat ends the run
	boss := &crawlerMonster{armour: 5, health: 1, maxHealth: 1, damage: 4, square: 4, alive: true, isBoss: true, reward: 3, bossName: "The Core"}
	bt := g.placeTile(9, 9, [4]bool{true, false, false, false}, 2, "boss", true, []*crawlerMonster{boss})
	g.players[0].tileID = bt.id
	g.dice = []crawlerDie{{Value: 6}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	if g.phase != crWon {
		t.Fatalf("defeating the final boss should win, phase=%q", g.phase)
	}
}

// Defeating a non-final boss descends to the next level with a fresh map; beating the
// last Forest level (2) crosses into the Ruins biome (level 3).
func TestCrawlerBossDefeatDescendsLevel(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.level = 2
	g.biome = crWoodland
	boss := &crawlerMonster{armour: 5, health: 1, maxHealth: 1, damage: 4, square: 4, alive: true, isBoss: true, reward: 3, bossName: "Vespex"}
	bt := g.placeTile(9, 9, [4]bool{true, false, false, false}, 2, "boss", true, []*crawlerMonster{boss})
	g.players[0].tileID = bt.id
	g.players[0].courage = 5 // will be reset by the level transition
	g.dice = []crawlerDie{{Value: 6}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	// Boss down: a solo hero gets crPostBossTurns (2) victory-lap turns before the level
	// descends, so the first End turn stays on level 2 and the second drops to level 3.
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	if g.level != 2 {
		t.Fatalf("the first post-boss turn should not descend yet, got level %d", g.level)
	}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	if g.phase != crPlaying || g.level != 3 || g.biome != crRuins {
		t.Fatalf("beating level 2 should start level 3 (Ruins), got phase=%q level=%d biome=%q", g.phase, g.level, g.biome)
	}
	if g.players[0].courage != 0 || g.players[0].health != g.players[0].sheet.maxHP {
		t.Fatalf("a new level should reset Courage and refill health: courage=%d hp=%d", g.players[0].courage, g.players[0].health)
	}
}

// --- boss pool & signature specials ----------------------------------------

func crBossFight(g *crawlerGame, m *crawlerMonster) *crawlerTile {
	t := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, "boss", true, []*crawlerMonster{m})
	g.players[0].tileID = t.id
	g.players[0].square = 4
	return t
}

func TestCrawlerRollBossIsLevelGated(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	// Level 1 only draws Mossback, Vespex or Pylon (no raw-damage specials).
	g.level = 1
	ok := map[string]bool{"Mossback": true, "Vespex": true, "Pylon": true}
	for i := 0; i < 30; i++ {
		m := g.rollBossMonsters()[0]
		if !m.isBoss || !ok[m.bossName] {
			t.Fatalf("level 1 boss should be a Woodland opener, got %q", m.bossName)
		}
	}
	// Level 6 can draw The Core (the finale, two specials).
	g.level = 6
	sawCore := false
	for i := 0; i < 60; i++ {
		if g.rollBossMonsters()[0].bossName == "The Core" {
			sawCore = true
		}
	}
	if !sawCore {
		t.Fatal("level 6 should be able to draw The Core")
	}
}

func TestCrawlerBossAdaptivePlatingClimbsResetOnSix(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	m := &crawlerMonster{armour: 3, health: 6, maxHealth: 6, damage: 1, square: 4, alive: true, isBoss: true, reward: 3, bossName: "Mossback", specials: []string{bsAdaptive}}
	crBossFight(g, m)
	m.plating = 2 // two rounds of climbing Armour
	h := g.players[0]
	if got := g.effectiveArmour(h, m); got != 5 {
		t.Fatalf("Adaptive Plating should add to Armour (3+2), got %d", got)
	}
	// A landed 6 cracks the plating back to 0.
	g.dice = []crawlerDie{{Value: 6}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	if m.plating != 0 {
		t.Fatalf("a 6 should reset the plating, got %d", m.plating)
	}
}

func TestCrawlerBossJammerRaisesOthersArmour(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	jammer := &crawlerMonster{armour: 6, health: 3, maxHealth: 3, damage: 3, square: 4, alive: true, isBoss: true, bossName: "Pylon", specials: []string{bsJammer}}
	g.placeTile(7, 7, [4]bool{true, true, true, true}, 0, "boss", true, []*crawlerMonster{jammer})
	drone := &crawlerMonster{armour: 3, health: 1, maxHealth: 1, damage: 1, square: 4, alive: true, kind: "scrapling"}
	crBossFight(g, drone)
	h := g.players[0]
	if got := g.effectiveArmour(h, drone); got != 4 {
		t.Fatalf("jammer should raise a drone's Armour from 3 to 4, got %d", got)
	}
	jammer.alive = false
	if got := g.effectiveArmour(h, drone); got != 3 {
		t.Fatalf("with the jammer dead the drone's Armour returns to 3, got %d", got)
	}
}

func TestCrawlerBossShockwaveHitsTileOnly(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	m := &crawlerMonster{armour: 5, health: 5, maxHealth: 5, damage: 0, square: 4, alive: true, isBoss: true, bossName: "Thornstag", specials: []string{bsShockwave}}
	bt := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, "boss", true, []*crawlerMonster{m})
	far := g.placeTile(9, 9, [4]bool{true, true, true, true}, 0, "cross", false, nil)
	// Seat 0 on the boss tile, seat 1 far away.
	g.players[0].tileID = bt.id
	g.players[0].health = 5
	g.players[1].tileID = far.id
	g.players[1].health = 5
	g.resolveRound()
	if g.players[0].health != 4 {
		t.Fatalf("shockwave should hit a hero on the boss tile for 1, got %d", g.players[0].health)
	}
	if g.players[1].health != 5 {
		t.Fatalf("shockwave must NOT reach a hero on another tile, got %d", g.players[1].health)
	}
}

func TestCrawlerBossSummonerISpawnsScrapling(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	m := &crawlerMonster{armour: 5, health: 6, maxHealth: 6, damage: 3, square: 4, alive: true, isBoss: true, bossName: "Vespex", specials: []string{bsSummon1}}
	bt := crBossFight(g, m)
	g.players[0].tileID = g.placeTile(9, 9, [4]bool{true, true, true, true}, 0, "cross", false, nil).id
	g.resolveRound()
	drones := 0
	for _, mm := range bt.monsters {
		if mm.alive && mm.kind == "scrapling" {
			drones++
		}
	}
	if drones != 1 {
		t.Fatalf("Vespex (Summoner I) should spawn 1 Scrapling each round, got %d", drones)
	}
}

func TestCrawlerBossSummonerIIISpawnsSentryAndDrone(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	m := &crawlerMonster{armour: 6, health: 8, maxHealth: 8, damage: 4, square: 4, alive: true, isBoss: true, bossName: "The Core", specials: []string{bsSummon3}}
	bt := crBossFight(g, m)
	g.players[0].tileID = g.placeTile(9, 9, [4]bool{true, true, true, true}, 0, "cross", false, nil).id
	g.resolveRound()
	sentries, drones := 0, 0
	for _, mm := range bt.monsters {
		if !mm.alive {
			continue
		}
		if mm.kind == "sentry" {
			sentries++
		}
		if mm.kind == "scrapling" {
			drones++
		}
	}
	if sentries != 1 || drones != 1 {
		t.Fatalf("Summoner III should add a Sentry and a Scrapling, got sentries=%d drones=%d", sentries, drones)
	}
}

func TestCrawlerBossHardenedCoreImmuneUntilWeakPoint(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	boss := &crawlerMonster{armour: 3, health: 6, maxHealth: 6, damage: 4, square: 4, alive: true, isBoss: true, bossName: "Hauler", specials: []string{bsHardcore}}
	wp := &crawlerMonster{armour: 3, health: 1, maxHealth: 1, alive: true, weakPoint: true}
	t2 := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, "boss", true, []*crawlerMonster{boss, wp})
	g.players[0].tileID = t2.id
	g.players[0].square = 4
	g.dice = []crawlerDie{{Value: 6}, {Value: 6}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0}) // boss (idx 0): immune
	if boss.health != 6 {
		t.Fatalf("the boss should be immune while its weak-point stands, hp=%d", boss.health)
	}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 1, "action": actAttack, "monster": 1}) // kill weak-point (idx 1)
	g.dice = []crawlerDie{{Value: 6}, {Value: 6}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0}) // now the boss takes it
	if wp.alive || boss.health != 5 {
		t.Fatalf("with the weak-point gone the boss takes damage: wpAlive=%v hp=%d", wp.alive, boss.health)
	}
}

func TestCrawlerMonsterDamageEnrage(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	base := &crawlerMonster{damage: 4, specials: []string{bsEnrage}}
	if d := g.monsterDamage(base); d != 4 {
		t.Fatalf("no enrage stacks yet -> base damage 4, got %d", d)
	}
	base.enrage = 2 // two rounds survived
	if d := g.monsterDamage(base); d != 6 {
		t.Fatalf("Enrage adds +1 per round survived (4+2), got %d", d)
	}
}

func TestCrawlerBossOverloadElectrifies(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	m := &crawlerMonster{armour: 6, health: 6, maxHealth: 6, damage: 0, square: 4, alive: true, isBoss: true, bossName: "Teslacrane", specials: []string{bsOverload}}
	crBossFight(g, m)
	g.players[0].health = 6
	g.resolveRound() // overload electrifies, then the tick deals 1
	if !g.players[0].electrified {
		t.Fatal("Overload should apply Electrified to a hero")
	}
	hp := g.players[0].health
	g.resolveRound() // still electrified -> another tick
	if g.players[0].health >= hp {
		t.Fatalf("Electrified should tick 1 damage each round: %d -> %d", hp, g.players[0].health)
	}
	// Grounding Boots make a hero immune.
	g.players[0].electrified = false
	g.players[0].objects = []string{"grounding-boots"}
	g.resolveRound()
	if g.players[0].electrified {
		t.Fatal("Grounding Boots should prevent Electrified")
	}
}

func TestCrawlerBossSplitBreaksOffCore(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	m := &crawlerMonster{armour: 3, health: 4, maxHealth: 4, damage: 4, square: 4, alive: true, isBoss: true, bossName: "Widowframe", specials: []string{bsSplit}}
	bt := crBossFight(g, m)
	g.dice = []crawlerDie{{Value: 4}, {Value: 4}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0}) // 4 -> 3
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 1, "action": actAttack, "monster": 0}) // 3 -> 2 (half) -> split
	if !m.didSplit {
		t.Fatal("Widowframe should split at half health")
	}
	adds := 0
	for _, mm := range bt.monsters {
		if mm.alive && !mm.isBoss {
			adds++
		}
	}
	if adds != 1 {
		t.Fatalf("Split should break off one weaker core, got %d adds", adds)
	}
}

// --- objects / loot economy ------------------------------------------------

func TestCrawlerObjectCatalogueMatchesNote(t *testing.T) {
	perms, cons := 0, 0
	weight := 0
	for _, o := range crObjects {
		if o.perm {
			perms++
		} else {
			cons++
			weight += o.weight
		}
	}
	if perms != 8 || cons != 8 {
		t.Fatalf("catalogue should be 8 permanents + 8 consumables, got %d/%d", perms, cons)
	}
	if weight != 100 {
		t.Fatalf("consumable drop weights should sum to 100, got %d", weight)
	}
}

func TestCrawlerShopBuySpendsCourage(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	shop := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, crKindShop, false, nil)
	h := g.players[0]
	h.tileID = shop.id
	h.square = 4
	h.courage = 10
	h.objects = nil
	g.shopStock = []string{"medkit", "scope"}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgBuy, "obj": "scope"})
	if h.courage != 7 || !heroHas(h, "scope") {
		t.Fatalf("buying Scope (cost 3) should spend Courage and add it: courage=%d objs=%v", h.courage, h.objects)
	}
	// Off a shop tile, buying is refused.
	h.tileID = g.placeTile(6, 6, [4]bool{true, true, true, true}, 0, "cross", false, nil).id
	crCmd(t, g, 0, false, map[string]any{"t": crMsgBuy, "obj": "medkit"})
	if heroHas(h, "medkit") {
		t.Fatal("buying away from a shop must be refused")
	}
}

func TestCrawlerShopSellGivesCourage(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	shop := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, crKindShop, false, nil)
	h := g.players[0]
	h.tileID = shop.id
	h.square = 4
	h.courage = 4
	h.objects = []string{"medkit", "scope"}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSell, "obj": "medkit"})
	if h.courage != 5 || heroHas(h, "medkit") {
		t.Fatalf("selling should give +1 Courage and drop the item: courage=%d objs=%v", h.courage, h.objects)
	}
	// At the Courage cap, selling is refused (it would be wasted).
	h.courage = crCourageCap
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSell, "obj": "scope"})
	if h.courage != crCourageCap || !heroHas(h, "scope") {
		t.Fatalf("selling at the cap must be refused: courage=%d objs=%v", h.courage, h.objects)
	}
	// Off a shop tile, selling is refused.
	h.courage = 4
	h.tileID = g.placeTile(6, 6, [4]bool{true, true, true, true}, 0, "cross", false, nil).id
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSell, "obj": "scope"})
	if h.courage != 4 || !heroHas(h, "scope") {
		t.Fatal("selling away from a shop must be refused")
	}
}

func TestCrawlerShopStocksMajorityPermanents(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.rollShop()
	if len(g.shopStock) != crShopSize {
		t.Fatalf("shop should hold %d objects: %v", crShopSize, g.shopStock)
	}
	perms := 0
	for _, s := range g.shopStock {
		if d := objBySlug(s); d != nil && d.perm {
			perms++
		}
	}
	if perms < crShopPerms {
		t.Fatalf("shop should stock a majority (>=%d) permanents, got %d: %v", crShopPerms, perms, g.shopStock)
	}
}

func TestCrawlerKillDropPoolHasBothRarities(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	sawPerm, sawConsumable := false, false
	for i := 0; i < 3000; i++ {
		d := objBySlug(g.rollDrop())
		if d == nil {
			t.Fatal("rollDrop returned an unknown slug")
		}
		if d.perm {
			sawPerm = true
		} else {
			sawConsumable = true
		}
	}
	if !sawPerm || !sawConsumable {
		t.Fatalf("kill drops should include both permanents and consumables over many rolls: perm=%v consumable=%v", sawPerm, sawConsumable)
	}
}

func TestCrawlerHealerHasInnateAttackDie(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.players[0].sheet = healerSheet("boomkikker")
	g.beginTurn(0)
	if len(g.dice) != 5 {
		t.Fatalf("healer should roll 4 Low + 1 attack die = 5 dice: got %d", len(g.dice))
	}
	bonus := 0
	for _, d := range g.dice {
		if d.Bonus {
			bonus++
		}
	}
	if bonus != 1 {
		t.Fatalf("healer should have exactly 1 Bonus (attack) die: got %d", bonus)
	}
}

func TestCrawlerWeakPointSpawnsNearBoss(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	boss := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, "boss", true, nil)
	near := g.placeTile(5, 6, [4]bool{true, true, true, true}, 0, crKindRust, false, nil)
	g.placeTile(5, 12, [4]bool{true, true, true, true}, 0, crKindRust, false, nil) // far
	g.placeWeakPoint(boss.id)
	wpTile := -1
	for id, ti := range g.tiles {
		for _, m := range ti.monsters {
			if m.weakPoint {
				wpTile = id
			}
		}
	}
	if wpTile != near.id {
		t.Fatalf("weak-point should land on the NEAR hazard tile (%d), got %d", near.id, wpTile)
	}
}

func TestCrawlerHaulerDoesNotLockInWhileImmune(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	boss := &crawlerMonster{armour: 5, health: 6, maxHealth: 6, damage: 4, kind: "hauler", isBoss: true, alive: true, square: 4, specials: []string{bsHardcore}}
	lair := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, "boss", true, []*crawlerMonster{boss})
	g.placeTile(5, 4, [4]bool{true, true, true, true}, 0, "cross", false, nil) // a tile to the North to leave onto
	wpTile := g.placeTile(5, 6, [4]bool{true, true, true, true}, 0, crKindRust, false, nil)
	wp := newWeakPoint()
	wpTile.monsters = append(wpTile.monsters, wp)
	h := g.players[0]
	h.tileID, h.square = lair.id, crawlerDoor[0] // on the North doorway of the lair
	if !g.tryMove(0, 0) {
		t.Fatal("Hauler must not lock heroes in while its weak-point still stands")
	}
	// weak-point destroyed: now it locks in like any boss
	wp.alive = false
	h.tileID, h.square = lair.id, crawlerDoor[0]
	if g.tryMove(0, 0) {
		t.Fatal("once the weak-point is gone, Hauler locks heroes in like any boss")
	}
}

func TestCrawlerRetryLevelReplaysWithoutObjects(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.startLevel(3)
	h := g.players[0]
	h.objects = []string{"medkit", "scope"}
	h.health = 1
	h.courage = 5
	g.phase = crLost
	crCmd(t, g, 0, true, map[string]any{"t": crMsgRetry})
	if g.phase != crPlaying {
		t.Fatalf("retry should resume play: phase=%q", g.phase)
	}
	if g.level != 3 {
		t.Fatalf("retry should stay on the same level: %d", g.level)
	}
	if len(h.objects) != 0 {
		t.Fatalf("retry should empty the pack: %v", h.objects)
	}
	if h.health != h.sheet.maxHP {
		t.Fatalf("retry should refill health: %d/%d", h.health, h.sheet.maxHP)
	}
	// A non-host, or outside a loss, is refused.
	g.phase = crLost
	crCmd(t, g, 0, false, map[string]any{"t": crMsgRetry})
	if g.phase != crLost {
		t.Fatal("a non-host retry must be refused")
	}
}

func TestCrawlerSpareDieAddsHighDie(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	base := h.sheet.diceN
	h.objects = []string{"spare-die"}
	g.beginTurn(0)
	if len(g.dice) != base+1 {
		t.Fatalf("Spare Die should add one die: %d, want %d", len(g.dice), base+1)
	}
}

func TestCrawlerWhetstoneDoublesSixes(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	m := setupFight(g, 3, 5, 1) // Armour 3, Health 5
	g.players[0].objects = []string{"whetstone"}
	g.dice = []crawlerDie{{Value: 6}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	if m.health != 3 {
		t.Fatalf("Whetstone should make a 6 land two hits (5 -> 3): %d", m.health)
	}
}

func TestCrawlerMedkitHeals(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.health = 2
	h.objects = []string{"medkit"}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgUse, "obj": "medkit"})
	if h.health != 5 || heroHas(h, "medkit") {
		t.Fatalf("Medkit should heal 3 and be consumed: hp=%d objs=%v", h.health, h.objects)
	}
}

func TestCrawlerBatteryGivesCourage(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.courage = 1
	h.objects = []string{"battery"}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgUse, "obj": "battery"})
	if h.courage != 3 || heroHas(h, "battery") {
		t.Fatalf("Battery should give +2 Courage and be consumed: courage=%d", h.courage)
	}
}

func TestCrawlerGloveFreeRerollThenCourage(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.objects = []string{"gamblers-glove"}
	h.courage = 1
	g.beginTurn(0)
	g.reroll(0, 0) // free via the glove
	if h.courage != 1 || !h.gloveUsed {
		t.Fatalf("first reroll should be free via the glove: courage=%d used=%v", h.courage, h.gloveUsed)
	}
	g.reroll(0, 0) // now costs Courage
	if h.courage != 0 {
		t.Fatalf("the second reroll should spend Courage: courage=%d", h.courage)
	}
}

func TestCrawlerObjectsCarryAcrossLevels(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.players[0].objects = []string{"scope", "medkit"}
	g.players[0].courage = 5
	g.startLevel(2)
	if len(g.players[0].objects) != 2 {
		t.Fatalf("objects should carry across levels, got %v", g.players[0].objects)
	}
	if g.players[0].courage != 0 {
		t.Fatalf("Courage should reset across levels, got %d", g.players[0].courage)
	}
}

func TestCrawlerObjectCapIsPerTotem(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0] // the Beaver carries only 2
	if heroSlots(h) != 2 {
		t.Fatalf("the Beaver should have 2 object slots, got %d", heroSlots(h))
	}
	h.objects = []string{"medkit", "grenade"} // full at 2
	if g.giveObject(h, "scope") {
		t.Fatal("a full pack should refuse a new object")
	}
	if len(h.objects) != heroSlots(h) {
		t.Fatalf("pack should stay at the cap, got %d", len(h.objects))
	}
	// The Arrow Squid (Striker, ranged) carries 2; the Treefrog (Healer) carries 3.
	g.players[0].sheet = rangedSheet("pijlinktvis")
	if heroSlots(g.players[0]) != 2 {
		t.Fatalf("the Arrow Squid should have 2 object slots, got %d", heroSlots(g.players[0]))
	}
	g.players[0].sheet = healerSheet("boomkikker")
	if heroSlots(g.players[0]) != 3 {
		t.Fatalf("the Treefrog should have 3 object slots, got %d", heroSlots(g.players[0]))
	}
}

func TestCrawlerVaultDropsVisibleLootWhenLaid(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.level = 2
	vault := g.placeTile(5, 5, crSpecialBase[crKindVault], 0, crKindVault, false, nil)
	g.populateVault(vault) // the drop that layTile performs when a Vault is laid
	if len(vault.loot) != 1 || vault.loot[0].square != 4 {
		t.Fatalf("a laid Vault should drop one consumable on its centre floor, got %v", vault.loot)
	}
	// The loot is a real floor object, grabbed like any drop (not auto-pocketed).
	h := g.players[0]
	h.tileID = vault.id
	h.square = 4
	h.objects = nil
	g.pickup(0, 4)
	if len(h.objects) != 1 || len(vault.loot) != 0 {
		t.Fatalf("a hero on the Vault's centre should pick up its loot: objs=%v floor=%v", h.objects, vault.loot)
	}
}

func TestCrawlerVaultOnlyFromLevel2(t *testing.T) {
	for _, lvl := range []int{1, 2} {
		g := newCrawlerGame(newFakeOutbox(0), 9).(*crawlerGame)
		g.AddPlayer("a", "")
		g.bands = 3
		g.level = lvl
		g.buildDeck()
		has := false
		for _, s := range g.deck {
			if s.kind == crKindVault {
				has = true
			}
		}
		if lvl == 1 && has {
			t.Fatal("no Vault should appear at Level 1")
		}
		if lvl == 2 && !has {
			t.Fatal("a Vault should appear from Level 2")
		}
	}
}

// --- loot on the floor ------------------------------------------------------

func TestCrawlerBossDropsLandAndKillerOnTilePicksUp(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	m := setupFight(g, 1, 1, 1)
	m.isBoss = true
	m.bossName = "Test Boss"
	h := g.players[0]
	tile := g.tiles[h.tileID] // setupFight puts the hero on the monster's tile
	h.objects = nil
	g.dropFrom(tile, m, h)
	// A boss drops 3 items; the killer on the tile auto-picks up to its slot cap.
	if len(h.objects) == 0 {
		t.Fatalf("killer on the boss tile should auto-pick its drops, got %v", h.objects)
	}
	picked := len(h.objects)
	left := len(tile.loot)
	if picked+left != 3 {
		t.Fatalf("a boss drops 3 items (picked %d + floor %d != 3)", picked, left)
	}
	if picked != heroSlots(h) && left != 0 {
		t.Fatalf("auto-pickup should fill the pack or clear the floor: picked=%d slots=%d floor=%d", picked, heroSlots(h), left)
	}
}

func TestCrawlerPickupOnlyFromOwnTile(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.objects = nil
	// Loot sits on a DIFFERENT tile from the hero.
	other := g.placeTile(9, 9, [4]bool{true, true, true, true}, 0, "cross", false, nil)
	g.dropGround(other, 4, "medkit")
	// Hero stands elsewhere: a pickup must be refused (no sniping the adjacent tile).
	g.pickup(0, 4)
	if heroHas(h, "medkit") || len(other.loot) != 1 {
		t.Fatalf("pickup from another tile must fail: objs=%v floor=%d", h.objects, len(other.loot))
	}
	// Move onto that tile, then the pickup succeeds.
	h.tileID = other.id
	h.square = 4
	g.pickup(0, 4)
	if !heroHas(h, "medkit") || len(other.loot) != 0 {
		t.Fatalf("pickup from your own tile should work: objs=%v floor=%d", h.objects, len(other.loot))
	}
}

func TestCrawlerDropLeavesObjectOnTheSquare(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.objects = []string{"scope"}
	h.square = 2
	g.dropObject(0, "scope")
	if heroHas(h, "scope") {
		t.Fatal("dropping should remove the object from the pack")
	}
	tile := g.tiles[h.tileID]
	if len(tile.loot) != 1 || tile.loot[0].square != 2 || tile.loot[0].slug != "scope" {
		t.Fatalf("the dropped object should stay on square 2: %+v", tile.loot)
	}
}

func TestCrawlerBossCannotBeSnipedFromAdjacentTile(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.sheet.ranged = true // give reach so only the boss rule blocks it
	m := &crawlerMonster{armour: 1, health: 5, maxHealth: 5, damage: 1, square: 4, alive: true, reward: 1, isBoss: true, bossName: "Boss"}
	bossTile := g.placeTile(1, 0, [4]bool{true, true, true, true}, 0, "boss", true, []*crawlerMonster{m})
	// Hero stands on the adjacent start tile, not the boss tile.
	if g.tryAttack(0, bossTile.id, 0, 6) {
		t.Fatal("a boss must not be hittable from an adjacent tile")
	}
	// Standing on the boss tile, the same attack lands.
	h.tileID = bossTile.id
	if !g.tryAttack(0, bossTile.id, 0, 6) {
		t.Fatal("a boss on your own tile should be hittable")
	}
}

func TestCrawlerLootRollRecordedOnOrdinaryKill(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	m := setupFight(g, 1, 1, 1)
	m.kind = "hulk" // highest drop chance
	g.dice = []crawlerDie{{Value: 6}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	if m.alive {
		t.Fatal("the monster should be dead")
	}
	if g.lootRoll == nil {
		t.Fatal("an ordinary kill should record a visible drop roll")
	}
	if g.lootRoll.chance != robotDropChance("hulk") || g.lootRoll.roll < 1 || g.lootRoll.roll > 100 {
		t.Fatalf("drop roll should be 1..100 vs the kind's chance: %+v", g.lootRoll)
	}
}

func TestCrawlerLootRollClearsAtTurnStart(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.lootRoll = &crawlerLootRoll{roll: 10, chance: 35}
	g.beginTurn(0)
	if g.lootRoll != nil {
		t.Fatal("the drop roll should clear at the start of a turn")
	}
}

// --- character ability coverage ---------------------------------------------

func TestCrawlerRaccoonLuckyPawsFreeReroll(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.sheet = tricksterSheet("wasbeer")
	h.courage = 3 // Lucky Paws (first unlock) active at 3
	h.objects = nil
	g.beginTurn(0)
	h.courage = 3 // beginTurn does not change courage; keep it explicit
	g.reroll(0, 0)
	if h.courage != 3 || !h.pawsUsed {
		t.Fatalf("Lucky Paws should give one free reroll: courage=%d used=%v", h.courage, h.pawsUsed)
	}
	g.reroll(0, 0) // the second reroll now costs Courage
	if h.courage != 2 {
		t.Fatalf("the second reroll should spend Courage: %d", h.courage)
	}
}

func TestCrawlerTreefrogBloomHealsParty(t *testing.T) {
	g, _ := startedCrawler(t, 3)
	heal := g.players[0]
	heal.sheet = healerSheet("boomkikker")
	heal.courage = 5
	a, b := g.players[1], g.players[2]
	a.health, b.health = 1, 1
	g.dice = []crawlerDie{{Value: 3}, {Value: 3}, {Value: 3}, {Value: 3}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMend, "ally": 1})
	if a.health != 1+2+1 {
		t.Fatalf("Bloom: mend target should heal 2 + 1 Bloom = +3, got %d", a.health-1)
	}
	if b.health != 1+1 {
		t.Fatalf("Bloom: the other ally on the tile should heal 1, got %d", b.health-1)
	}
	if !heal.bloomUsed {
		t.Fatal("Bloom should be marked used for the round")
	}
}

func TestCrawlerWeaverFerryCarriesAcrossTileEdge(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	w := g.players[0]
	w.sheet = carrierSheet("comorenwever")
	w.courage = 3 // Ferry (first unlock) active at 3
	// Two adjacent cross tiles already placed; the ally sits at the east doorway.
	a := g.tiles[w.tileID]                                                          // at (0,0)
	b := g.placeTile(1, 0, [4]bool{true, true, true, true}, 0, "cross", false, nil) // east neighbour
	ally := g.players[1]
	ally.tileID = a.id
	ally.square = 5 // east doorway
	g.dice = []crawlerDie{{Value: 3}, {Value: 3}, {Value: 3}, {Value: 3}, {Value: 3}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actCarry, "ally": 1, "dir": 1})
	if ally.tileID != b.id {
		t.Fatalf("Ferry should carry the ally into the east tile, got tile %d (want %d)", ally.tileID, b.id)
	}
}

func TestCrawlerWeaverFlockCarriesTwoSquares(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	w := g.players[0]
	w.sheet = carrierSheet("comorenwever")
	w.courage = 5 // Flock (second unlock) active at 5
	ally := g.players[1]
	ally.tileID = w.tileID
	ally.square = 3 // west doorway on the cross
	g.dice = []crawlerDie{{Value: 3}, {Value: 3}, {Value: 3}, {Value: 3}, {Value: 3}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actCarry, "ally": 1, "dir": 1})
	if ally.square != 5 {
		t.Fatalf("Flock should carry two squares W-door -> centre -> E-door (5), got %d", ally.square)
	}
}

func TestCrawlerRaccoonBigScoreAddsSlot(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.sheet = tricksterSheet("wasbeer") // 3 object slots
	h.courage = 0
	if heroSlots(h) != 3 {
		t.Fatalf("Raccoon base slots should be 3, got %d", heroSlots(h))
	}
	h.courage = 5 // Big Score active
	if heroSlots(h) != 4 {
		t.Fatalf("Big Score should grant a 4th slot, got %d", heroSlots(h))
	}
}

func TestCrawlerShopStocksFiveAndRefillsOnBuy(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.rollShop()
	if len(g.shopStock) != crShopSize {
		t.Fatalf("the shop should stock %d objects, got %d", crShopSize, len(g.shopStock))
	}
	shop := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, crKindShop, false, nil)
	h := g.players[0]
	h.tileID = shop.id
	h.square = 4
	h.courage = 10
	h.objects = nil
	bought := g.shopStock[0]
	crCmd(t, g, 0, false, map[string]any{"t": crMsgBuy, "obj": bought})
	if !heroHas(h, bought) {
		t.Fatalf("the hero should hold the bought object %q", bought)
	}
	if len(g.shopStock) != crShopSize {
		t.Fatalf("the shop should refill back to %d after a purchase, got %d", crShopSize, len(g.shopStock))
	}
}

// --- statuses and hazards (Level 2+) ----------------------------------------

func TestCrawlerElectrocutedLowersEachDie(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.electrocuted = true
	g.beginTurn(0)
	for i, d := range g.dice {
		if d.Value < 1 {
			t.Fatalf("die %d below 1: %d", i, d.Value)
		}
	}
	// A fresh roll with electrocuted off should (on average) read higher, but the key
	// invariant is: no die exceeds the sheet's max face minus the shock, and min is 1.
	max := 0
	for _, f := range h.sheet.faces {
		if f > max {
			max = f
		}
	}
	for _, d := range g.dice {
		if d.Value > max {
			t.Fatalf("electrocuted die %d exceeds max face %d", d.Value, max)
		}
	}
}

func TestCrawlerRustedDropsADie(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	base := h.sheet.diceN
	h.rusted = true
	g.beginTurn(0)
	if len(g.dice) != base-1 {
		t.Fatalf("Rusted should drop one die (%d -> %d), got %d", base, base-1, len(g.dice))
	}
}

func TestCrawlerSnaredCapsMoveDice(t *testing.T) {
	g, _ := startedCrawler(t, 1) // hero at the start cross centre; E/W shuffles stay in-tile
	h := g.players[0]
	dirs := []int{1, 3, 1, 3} // E, W, E, W — never crosses a tile edge, so each is a plain move
	countMoves := func() int {
		h.square = 4
		h.moveDiceUsed = 0
		g.dice = []crawlerDie{{Value: 3}, {Value: 3}, {Value: 3}, {Value: 3}}
		moves := 0
		for i, dir := range dirs {
			before := h.square
			crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": i, "action": actMove, "dir": dir})
			if h.square != before {
				moves++
			}
		}
		return moves
	}
	h.snared = true
	if got := countMoves(); got != crSnareMoveDice {
		t.Fatalf("Snared should allow exactly %d Move dice, got %d", crSnareMoveDice, got)
	}
	h.snared = false
	if got := countMoves(); got != 4 {
		t.Fatalf("without Snared all four dice may Move, got %d", got)
	}
}

func TestCrawlerDrainedBlocksCourage(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.drained = true
	h.courage = 2
	g.dice = []crawlerDie{{Value: 6}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actCourage})
	if h.courage != 2 || g.dice[0].Spent {
		t.Fatalf("Drained must block the Courage action: courage=%d spent=%v", h.courage, g.dice[0].Spent)
	}
}

func TestCrawlerWiringHazardEntryAndEndOfTurn(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.level = 2 // hazards begin at Level 2
	h := g.players[0]
	t0 := g.tiles[h.tileID]
	t0.hazards = []crawlerHazard{{square: h.square, kind: hzWiring}}
	hp := h.health
	g.enterHazard(h)
	if h.health != hp-1 {
		t.Fatalf("live wiring should chip 1 on entry (%d -> %d), got %d", hp, hp-1, h.health)
	}
	g.endTurnHazards(h)
	if !h.electrocuted {
		t.Fatal("ending a turn on live wiring should Electrocute")
	}
}

func TestCrawlerMineFiresOnce(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.level = 2
	h := g.players[0]
	t0 := g.tiles[h.tileID]
	t0.hazards = []crawlerHazard{{square: h.square, kind: hzMine}}
	hp := h.health
	g.enterHazard(h)
	if h.health != hp-3 {
		t.Fatalf("a mine should burst for 3 (%d -> %d), got %d", hp, hp-3, h.health)
	}
	hp = h.health
	g.enterHazard(h) // spent now
	if h.health != hp {
		t.Fatal("a spent mine should not fire again")
	}
}

func TestCrawlerCoolantSnaresOnEntry(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.level = 2
	h := g.players[0]
	t0 := g.tiles[h.tileID]
	t0.hazards = []crawlerHazard{{square: h.square, kind: hzCoolant}}
	g.enterHazard(h)
	if !h.snared {
		t.Fatal("wading into coolant should Snare")
	}
}

func TestCrawlerFireEndOfTurn(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.level = 2
	h := g.players[0]
	t0 := g.tiles[h.tileID]
	t0.hazards = []crawlerHazard{{square: h.square, kind: hzFire}}
	hp := h.health
	g.endTurnHazards(h)
	if h.health != hp-2 || !h.electrified {
		t.Fatalf("fire should deal 2 + Electrify: hp %d->%d electrified=%v", hp, h.health, h.electrified)
	}
}

func TestCrawlerFogHidesTileContents(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	g.level = 2
	// Put the party far away, then a fog tile with a monster off on its own.
	far := g.placeTile(9, 9, [4]bool{true, true, true, true}, 0, crKindFog, false,
		[]*crawlerMonster{{armour: 3, health: 1, maxHealth: 1, damage: 1, square: 4, alive: true}})
	far.hazards = []crawlerHazard{{square: -1, kind: hzFog}}
	v := g.viewFor(0)
	var tv *crawlerTileView
	for i := range v.Tiles {
		if v.Tiles[i].ID == far.id {
			tv = &v.Tiles[i]
		}
	}
	if tv == nil || !tv.Fogged || len(tv.Monsters) != 0 {
		t.Fatalf("a distant fog tile should hide its monsters: fogged=%v monsters=%d", tv != nil && tv.Fogged, len(tv.Monsters))
	}
	// A hero merely adjacent does NOT reveal it (fog lifts only from the centre).
	g.players[0].tileID = g.placeTile(9, 8, [4]bool{true, true, true, true}, 0, "cross", false, nil).id
	v = g.viewFor(0)
	for i := range v.Tiles {
		if v.Tiles[i].ID == far.id && (!v.Tiles[i].Fogged || len(v.Tiles[i].Monsters) != 0) {
			t.Fatalf("an adjacent hero should NOT reveal the fog tile (centre-only), got monsters=%d", len(v.Tiles[i].Monsters))
		}
	}
	// Push a hero onto the fog tile's centre: revealFog (fired on landing) uncovers it.
	g.players[0].tileID = far.id
	g.players[0].square = 4
	g.revealFog(g.players[0])
	v = g.viewFor(0)
	for i := range v.Tiles {
		if v.Tiles[i].ID == far.id && len(v.Tiles[i].Monsters) != 1 {
			t.Fatalf("a hero on the fog tile's centre should reveal its monster, got %d", len(v.Tiles[i].Monsters))
		}
	}
	// The reveal STICKS: everyone leaves, the tile stays uncovered.
	g.players[0].tileID = g.placeTile(9, 8, [4]bool{true, true, true, true}, 0, "cross", false, nil).id
	g.players[0].square = 4
	v = g.viewFor(0)
	for i := range v.Tiles {
		if v.Tiles[i].ID == far.id && (v.Tiles[i].Fogged || len(v.Tiles[i].Monsters) != 1) {
			t.Fatalf("an uncovered fog tile should stay revealed after heroes leave: fogged=%v monsters=%d", v.Tiles[i].Fogged, len(v.Tiles[i].Monsters))
		}
	}
}

func TestCrawlerStatusDieTablePure(t *testing.T) {
	if monsterStatus("scrapling", 2) != "electrocuted" || monsterStatus("scrapling", 5) != "rusted" {
		t.Fatal("Scrapling sting should be Electrocuted early, Rusted in the machine rooms")
	}
	if monsterStatus("hulk", 3) != "" {
		t.Fatal("the Hulk carries no status")
	}
	if monsterStatus("sentry", 2) != "electrocuted" || monsterStatus("grappler", 2) != "snared" || monsterStatus("jammer", 3) != "drained" {
		t.Fatal("Sentry = Electrocuted, Grappler = Snared, Jammer = Drained")
	}
	if statusThreshold("scrapling", 1) != 7 || statusThreshold("scrapling", 4) != 4 {
		t.Fatalf("thresholds: L1 never(7), L4 4; got %d/%d", statusThreshold("scrapling", 1), statusThreshold("scrapling", 4))
	}
}

func TestCrawlerGroundingBootsImmuneToShock(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.objects = []string{"grounding-boots"}
	if g.applyStatus(h, "electrocuted") || g.applyStatus(h, "electrified") {
		t.Fatal("Grounding Boots should shrug off the electrical statuses")
	}
	if !g.applyStatus(h, "snared") {
		t.Fatal("Grounding Boots do not block Snared")
	}
}

func TestCrawlerSpringClearsStatuses(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.electrocuted, h.snared, h.drained = true, true, true
	h.health = h.sheet.maxHP // full health, but still afflicted
	spring := g.placeTile(2, 0, crSpecialBase[crKindHealing], 0, crKindHealing, false, nil)
	h.tileID = spring.id
	h.square = 4
	g.maybeSpring(0)
	if heroHasStatus(h) {
		t.Fatal("a spring should wash off every status")
	}
}

func TestCrawlerSporesClearsOnMend(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	heal := g.players[0]
	heal.sheet = healerSheet("boomkikker")
	heal.courage = 3 // Spores (first unlock) active at 3
	a := g.players[1]
	a.electrocuted, a.snared = true, true
	a.health = a.sheet.maxHP // full health: only Spores' cleanse should justify the Mend
	g.dice = []crawlerDie{{Value: 3}, {Value: 3}, {Value: 3}, {Value: 3}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMend, "ally": 1})
	if heroHasStatus(a) {
		t.Fatalf("Spores should clear the Mend target's statuses")
	}
	if !g.dice[0].Spent {
		t.Fatal("a Spores cleanse should consume the die even at full health")
	}
}

func TestCrawlerHazardRoomsAppearFromLevel2(t *testing.T) {
	for _, lvl := range []int{1, 2} {
		g := newCrawlerGame(newFakeOutbox(0), 9).(*crawlerGame)
		g.AddPlayer("a", "")
		g.bands = 3
		g.level = lvl
		g.buildDeck()
		seen := map[string]bool{}
		for _, s := range g.deck {
			seen[s.kind] = true
		}
		// Fog (the sensor/hidden room) appears from Level 1; the other Woodland hazard rooms
		// (live-wiring, coolant-flood) start at Level 2.
		if !seen[crKindFog] {
			t.Fatalf("L%d: the sensor-fog room should appear from Level 1", lvl)
		}
		otherHaz := seen[crKindWiring] || seen[crKindCoolant]
		if lvl == 1 && otherHaz {
			t.Fatal("wiring/coolant hazard rooms should not appear at Level 1")
		}
		if lvl == 2 && !otherHaz {
			t.Fatal("wiring/coolant hazard rooms should appear from Level 2")
		}
	}
}

func TestCrawlerSeedHazardsOnWiringRoom(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	room := g.placeTile(3, 0, crSpecialBase[crKindWiring], 0, crKindWiring, false, nil)
	g.seedHazards(room)
	if len(room.hazards) == 0 {
		t.Fatal("a wiring room should seed wiring hazards on its doorways")
	}
	for _, hz := range room.hazards {
		if hz.kind != hzWiring {
			t.Fatalf("a wiring room should seed only wiring, got %q", hz.kind)
		}
	}
}

func TestCrawlerRustRoomInflictsRusted(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.level = 3 // the rust room is a Ruins hazard (L3+)
	room := g.placeTile(3, 0, crSpecialBase[crKindRust], 0, crKindRust, false, nil)
	g.seedHazards(room)
	if len(room.hazards) == 0 {
		t.Fatal("a rust room should seed hazards on its doorways")
	}
	for _, hz := range room.hazards {
		if hz.kind != hzRust {
			t.Fatalf("a rust room should seed rust, got %q", hz.kind)
		}
	}
	h := g.players[0]
	h.tileID = room.id
	h.square = room.hazards[0].square
	hp := h.health
	g.enterHazard(h) // rust does no damage on entry
	if h.health != hp {
		t.Fatalf("rust should not damage on entry: %d -> %d", hp, h.health)
	}
	g.endTurnHazards(h) // ending on rust inflicts Rusted
	if !h.rusted {
		t.Fatal("ending a turn on a rust square should inflict Rusted")
	}
}

func TestCrawlerTorchClearsTileHazards(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.level = 2
	h := g.players[0]
	h.objects = []string{"torch"}
	t0 := g.tiles[h.tileID]
	t0.hazards = []crawlerHazard{{square: 1, kind: hzWiring}, {square: 5, kind: hzWiring}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgUse, "obj": "torch"})
	if len(t0.hazards) != 0 || heroHas(h, "torch") {
		t.Fatalf("a Torch should clear the tile's hazards and be consumed: hazards=%d held=%v", len(t0.hazards), h.objects)
	}
}

// --- archetypes, Wolf, and renamed enemies ----------------------------------

func TestCrawlerWolfGroupStrengthHealsWithAlly(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	w := g.players[0]
	w.sheet = meleeSheet("wolf")
	w.health = 3
	ally := g.players[1]
	ally.tileID = w.tileID // share the tile
	g.beginTurn(0)
	if w.health != 4 {
		t.Fatalf("Group strength should heal 1 with an ally on the tile (3 -> 4), got %d", w.health)
	}
	// Alone, no heal.
	w.health = 3
	ally.tileID = g.placeTile(9, 9, [4]bool{true, true, true, true}, 0, "cross", false, nil).id
	g.beginTurn(0)
	if w.health != 3 {
		t.Fatalf("Group strength should not heal when alone, got %d", w.health)
	}
}

func TestCrawlerWolfPackTacticsExtraHit(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	w := g.players[0]
	w.sheet = meleeSheet("wolf")
	w.courage = 3                             // Pack Tactics active
	m := setupFight(g, 3, 5, 1)               // Armour 3, Health 5; hero 0 is on its tile
	g.players[1].tileID = g.players[0].tileID // ally shares the Wolf's tile
	g.dice = []crawlerDie{{Value: 6}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	if m.health != 3 { // 1 base + 1 Pack Tactics = 2 hits, 5 -> 3
		t.Fatalf("Pack Tactics should add a hit with an ally on the tile (5 -> 3), got %d", m.health)
	}
}

func TestCrawlerWolfFrenzyFreeDieOnKill(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	w := g.players[0]
	w.sheet = meleeSheet("wolf")
	w.courage = 5 // Frenzy active
	m := setupFight(g, 3, 1, 1)
	g.dice = []crawlerDie{{Value: 6}, {Value: 1}, {Value: 1}, {Value: 1}}
	before := len(g.dice)
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	if m.alive {
		t.Fatal("the Health-1 monster should die")
	}
	if len(g.dice) != before+1 {
		t.Fatalf("Frenzy should add a free attack die on a kill (%d -> %d), got %d", before, before+1, len(g.dice))
	}
}

func TestCrawlerJammerRobotHardensAllies(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	jammer := &crawlerMonster{armour: 3, health: 2, maxHealth: 2, damage: 1, square: 1, kind: "jammer", alive: true, reward: 2}
	scrap := &crawlerMonster{armour: 3, health: 1, maxHealth: 1, damage: 1, square: 5, kind: "scrapling", alive: true, reward: 1}
	t0 := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, "cross", false, []*crawlerMonster{jammer, scrap})
	h.tileID = t0.id
	if g.effectiveArmour(h, scrap) != 4 {
		t.Fatalf("a Jammer robot should harden the Scrapling to Armour 4, got %d", g.effectiveArmour(h, scrap))
	}
	// Kill the Jammer: the aura lifts.
	jammer.alive = false
	if g.effectiveArmour(h, scrap) != 3 {
		t.Fatalf("with the Jammer dead the Scrapling is back to Armour 3, got %d", g.effectiveArmour(h, scrap))
	}
}

func TestCrawlerBossHealthIsBasePlusPerPlayer(t *testing.T) {
	for _, n := range []int{1, 3, 5} {
		g := newCrawlerGame(newFakeOutbox(0), 7).(*crawlerGame)
		for i := 0; i < n; i++ {
			g.AddPlayer("h", "")
		}
		g.level = 1
		mons := g.rollBossMonsters()
		boss := mons[0]
		var def *bossDef
		for i := range bossRoster {
			if bossRoster[i].name == boss.bossName {
				def = &bossRoster[i]
			}
		}
		if def == nil {
			t.Fatalf("drawn boss %q not in the roster", boss.bossName)
		}
		want := def.baseHP + def.hpPerHero*(n-1)
		if boss.maxHealth != want {
			t.Fatalf("%s with %d players: Health %d, want base+perHero*(players-1) = %d", boss.bossName, n, boss.maxHealth, want)
		}
	}
}

func TestCrawlerRobotDropChancesMatchNote(t *testing.T) {
	for kind, want := range map[string]int{"scrapling": 15, "sentry": 25, "jammer": 25, "grappler": 25, "hulk": 35} {
		if got := robotDropChance(kind); got != want {
			t.Fatalf("%s drop chance %d, want %d", kind, got, want)
		}
	}
}

func TestCrawlerPickupAllowedOffTurn(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	// It is seat 0's turn; seat 1 stands on a tile with loot and should still grab it.
	g.turn = 0
	h := g.players[1]
	t0 := g.tiles[h.tileID]
	h.objects = nil
	g.dropGround(t0, h.square, "medkit")
	g.pickup(1, h.square)
	if !heroHas(h, "medkit") || len(t0.loot) != 0 {
		t.Fatalf("a hero on the tile should pick up loot even off-turn: objs=%v floor=%d", h.objects, len(t0.loot))
	}
}

func TestCrawlerBossHoldsTheCentre(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	// A crossroads boss lair with the boss on the centre square.
	boss := &crawlerMonster{armour: 5, health: 9, maxHealth: 9, damage: 4, square: 4, kind: "boss", alive: true, isBoss: true, bossName: "Mossback"}
	lair := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, "boss", true, []*crawlerMonster{boss})
	h := g.players[0]
	h.tileID = lair.id
	h.square = crawlerDoor[0] // the north doorway (a side square)
	// Moving inward toward the centre is refused: the boss holds it.
	if g.tryMove(0, 2) || h.square != crawlerDoor[0] {
		t.Fatalf("a hero must not step onto the boss's centre square (square=%d)", h.square)
	}
	// With the boss dead the centre frees up.
	boss.alive = false
	if !g.tryMove(0, 2) || h.square != 4 {
		t.Fatalf("with the boss gone the centre is walkable again (square=%d)", h.square)
	}
}

// --- start options: level, forced boss, starting loot ------------------------

func newLobbyCrawler(t *testing.T, n int) *crawlerGame {
	t.Helper()
	g := newCrawlerGame(newFakeOutbox(0), 7).(*crawlerGame)
	g.AddPlayer("a", "bever")
	for i := 1; i < n; i++ {
		g.AddPlayer("ally", "")
	}
	return g
}

func TestCrawlerStartAtChosenLevel(t *testing.T) {
	g := newLobbyCrawler(t, 1)
	crCmd(t, g, 0, true, map[string]any{"t": crMsgStart, "level": 3})
	if g.level != 3 || g.biome != crRuins {
		t.Fatalf("start at level 3 should begin in the Ruins: level=%d biome=%q", g.level, g.biome)
	}
}

func TestCrawlerStartForcedBoss(t *testing.T) {
	g := newLobbyCrawler(t, 1)
	crCmd(t, g, 0, true, map[string]any{"t": crMsgStart, "level": 1, "boss": "pylon"})
	found := ""
	for _, s := range g.deck {
		if s.isBoss && len(s.monsters) > 0 {
			found = s.monsters[0].bossName
		}
	}
	if found != "Pylon" {
		t.Fatalf("forcing boss \"pylon\" should put Pylon in the deck, got %q", found)
	}
}

func TestCrawlerStartDevLoot(t *testing.T) {
	g := newLobbyCrawler(t, 2)
	crCmd(t, g, 0, true, map[string]any{"t": crMsgStart, "level": 1, "loot": []any{"medkit", "scope"}})
	for _, s := range g.order {
		h := g.players[s]
		if !heroHas(h, "medkit") || !heroHas(h, "scope") {
			t.Fatalf("every hero should start with the dev loot, %s has %v", h.name, h.objects)
		}
	}
}

func TestCrawlerElectrocutedReducesRollsAndRerolls(t *testing.T) {
	// electro() is the single reducer used by the opening roll, Spare Die and rerolls.
	h := &crawlerHero{electrocuted: true}
	if electro(h, 6) != 5 || electro(h, 2) != 1 || electro(h, 1) != 1 {
		t.Fatal("electrocuted should drop a value by 1, floored at 1")
	}
	h.electrocuted = false
	if electro(h, 6) != 6 {
		t.Fatal("no penalty without the status")
	}
	// Integration: every die is reduced on the opening roll AND after a reroll, so no die
	// ever shows the max face (6) while electrocuted.
	g, _ := startedCrawler(t, 1)
	hero := g.players[0]
	hero.electrocuted = true
	hero.courage = 9 // pay for rerolls
	g.beginTurn(0)
	hero.electrocuted = true // beginTurn does not clear statuses
	for i := range g.dice {
		if g.dice[i].Value > 5 {
			t.Fatalf("opening roll die %d = %d should be <= 5 while electrocuted", i, g.dice[i].Value)
		}
	}
	for i := range g.dice {
		g.reroll(0, i)
		if g.dice[i].Value > 5 {
			t.Fatalf("rerolled die %d = %d should still be reduced while electrocuted", i, g.dice[i].Value)
		}
	}
}

// --- fog room content & the gate ---------------------------------------------

// A sensor-fog room is never empty: over many seeds it lays either a loot cache (the Sap
// Seed + Beacon plus random consumables on the floor) or an ambush (a Hulk and one or two
// random robots, never a boss). Both outcomes must occur, and neither leaves the tile bare.
func TestCrawlerFogRoomAlwaysHasContent(t *testing.T) {
	sawLoot, sawAmbush := false, false
	for seed := uint64(1); seed <= 60; seed++ {
		g := newCrawlerGame(newFakeOutbox(0), seed).(*crawlerGame)
		g.AddPlayer("beaver", "bever")
		g.level = 2
		tile := g.placeTile(3, 3, crSpecialBase[crKindFog], 0, crKindFog, false, nil)
		g.populateFogRoom(tile)
		hasLoot := len(tile.loot) > 0
		hasMons := len(tile.monsters) > 0
		if hasLoot == hasMons {
			t.Fatalf("seed %d: a fog room is exactly one of loot/ambush, got loot=%v mons=%v", seed, hasLoot, hasMons)
		}
		if hasLoot {
			sawLoot = true
			var slugs []string
			for _, gr := range tile.loot {
				slugs = append(slugs, gr.slug)
			}
			if !containsStr(slugs, "sap-seed") || !containsStr(slugs, "beacon") {
				t.Fatalf("seed %d: a loot cache must include the Sap Seed and Beacon, got %v", seed, slugs)
			}
		}
		if hasMons {
			sawAmbush = true
			if tile.monsters[0].kind != "hulk" {
				t.Fatalf("seed %d: a fog ambush leads with a Hulk, got %q", seed, tile.monsters[0].kind)
			}
			if n := len(tile.monsters); n < 3 || n > 4 {
				t.Fatalf("seed %d: a fog ambush is 3-4 enemies, got %d", seed, n)
			}
			total := 0
			for _, m := range tile.monsters {
				if m.isBoss {
					t.Fatalf("seed %d: a fog ambush never spawns a boss", seed)
				}
				total += m.health
			}
			if total >= crFogAmbushBudget {
				t.Fatalf("seed %d: a fog ambush's combined Health %d must stay below the budget %d", seed, total, crFogAmbushBudget)
			}
		}
	}
	if !sawLoot || !sawAmbush {
		t.Fatalf("over 60 seeds a fog room should show both a loot cache and an ambush (loot=%v ambush=%v)", sawLoot, sawAmbush)
	}
}

// The boss lair is the generic crossroads EXCEPT at the biome seams: Level 2's boss guards
// the city gate and Level 4's boss guards the stairwell down (the descent), both one-door
// dead-ends. Every other level keeps the four-way crossroads lair.
func TestCrawlerBossLairPerLevel(t *testing.T) {
	seam := map[int]string{2: crKindGate, 4: crKindDescent}
	for lvl := 1; lvl <= 6; lvl++ {
		g := newCrawlerGame(newFakeOutbox(0), 5).(*crawlerGame)
		g.AddPlayer("beaver", "bever")
		g.bands = 3
		g.level = lvl
		g.buildDeck()
		var boss *tileSpec
		for i := range g.deck {
			if g.deck[i].isBoss {
				boss = &g.deck[i]
			}
		}
		if boss == nil {
			t.Fatalf("level %d: no boss in the deck", lvl)
		}
		if want, ok := seam[lvl]; ok {
			if boss.kind != want {
				t.Fatalf("level %d boss lair should be %q, got %q", lvl, want, boss.kind)
			}
			if openEdgeCount(boss.base) != 1 {
				t.Fatalf("the level %d seam boss lair must be a one-door dead-end, got %d open edges", lvl, openEdgeCount(boss.base))
			}
		} else if boss.kind != "boss" {
			t.Fatalf("level %d boss lair should be the generic crossroads, got kind %q", lvl, boss.kind)
		}
	}
}

// Level 3 descends through that same broken gate: its start tile is the gate dead-end, not
// the crossroads portal every other level opens on.
func TestCrawlerLevel3StartsOnTheGate(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.startLevel(3)
	start := g.tiles[g.players[0].tileID]
	if start.kind != crKindGate {
		t.Fatalf("level 3 should start on the gate, got kind %q", start.kind)
	}
	if openEdgeCount(start.edges) != 1 {
		t.Fatalf("the level 3 gate start must be a one-door dead-end, got %d open edges", openEdgeCount(start.edges))
	}
	g.startLevel(1)
	if s := g.tiles[g.players[0].tileID]; s.kind != crKindPortal {
		t.Fatalf("other levels still open on the portal, got kind %q", s.kind)
	}
}

// Every level keeps a usable transporter pair: a portal can only teleport to another
// portal, so there must be at least two on the map. Level 3 starts on the gate (not a
// portal), so it carries a second deck portal to make up the pair.
func TestCrawlerTransporterPairEachLevel(t *testing.T) {
	for lvl := 1; lvl <= 6; lvl++ {
		g := newCrawlerGame(newFakeOutbox(0), 3).(*crawlerGame)
		g.AddPlayer("beaver", "bever")
		g.bands = 3
		g.level = lvl
		g.buildDeck()
		portals := 0
		for _, s := range g.deck {
			if s.kind == crKindPortal {
				portals++
			}
		}
		if lvl != 3 {
			portals++ // the start tile is a portal on every level but 3 (the gate)
		}
		if portals < 2 {
			t.Fatalf("level %d needs a transporter pair (>=2 portals), got %d", lvl, portals)
		}
	}
}

// The kangaroo merchant holds the shop centre (non-walkable), and the shop opens the moment
// a hero steps onto the tile (from the doorway), not only on the centre: statuses clear and
// buying works from the doorway.
func TestCrawlerShopBlocksCentreAndOpensOnEntry(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	shop := g.placeTile(5, 5, crSpecialBase[crKindShop], 0, crKindShop, false, nil)
	h := g.players[0]
	h.tileID = shop.id
	if !centreBlocked(shop, 4) {
		t.Fatal("the shop centre should be blocked (the merchant stands there)")
	}
	h.square = crawlerDoor[0] // the single open-edge doorway
	if g.tryMove(0, 2) {      // step from the N doorway toward the centre (S)
		t.Fatal("a hero must not be able to step onto the shop centre")
	}
	// Stepping onto the tile opens the shop: statuses clear from the doorway.
	h.electrocuted = true
	g.maybeShop(0)
	if h.electrocuted {
		t.Fatal("the shop should patch up statuses from the doorway, not require the centre")
	}
	// And buying works from the doorway.
	g.shopStock = []string{"medkit"}
	h.courage = 9
	before := len(h.objects)
	g.buyObject(0, "medkit")
	if len(h.objects) != before+1 {
		t.Fatalf("should buy from the shop doorway: objects %d -> %d", before, len(h.objects))
	}
}

// On the Level 3 gate the party emerges on the city side: the single open-edge street
// square, not the centre. Other levels still start on the centre.
func TestCrawlerLevel3SpawnsOnCitySide(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.startLevel(3)
	h := g.players[0]
	start := g.tiles[h.tileID]
	if start.kind != crKindGate {
		t.Fatalf("L3 start should be the gate, got %q", start.kind)
	}
	open := -1
	for d := 0; d < 4; d++ {
		if start.edges[d] {
			open = d
			break
		}
	}
	if h.square == 4 || h.square != crawlerDoor[open] {
		t.Fatalf("L3 should spawn on the open-edge street square %d (city side), got %d", crawlerDoor[open], h.square)
	}
	g.startLevel(1)
	if g.players[0].square != 4 {
		t.Fatalf("non-gate levels still spawn on the centre, got square %d", g.players[0].square)
	}
}

// Hazard-room shapes: the rust room is an open crossroads (like coolant/fog), the trap
// room is a straight corridor (a street between tall buildings), so their open-edge counts
// differ. The drift guard keeps these in sync with the client art.
func TestCrawlerHazardRoomShapes(t *testing.T) {
	if got := openEdgeCount(crSpecialBase[crKindRust]); got != 4 {
		t.Fatalf("rust room should be a crossroads (4 open edges), got %d", got)
	}
	if got := openEdgeCount(crSpecialBase[crKindTrap]); got != 2 {
		t.Fatalf("trap room should be a straight corridor (2 open edges), got %d", got)
	}
	if got := openEdgeCount(crSpecialBase[crKindDescent]); got != 1 {
		t.Fatalf("the descent should be a one-door dead-end, got %d open edges", got)
	}
}

// The shrine landmark is Woodland Level 2 only for now; no other level draws it.
func TestCrawlerShrineOnlyLevel2(t *testing.T) {
	for lvl := 1; lvl <= 6; lvl++ {
		g := newCrawlerGame(newFakeOutbox(0), 3).(*crawlerGame)
		g.AddPlayer("beaver", "bever")
		g.bands = 3
		g.level = lvl
		g.buildDeck()
		shrines := 0
		for _, s := range g.deck {
			if s.kind == crKindShrine {
				shrines++
			}
		}
		want := 0
		if lvl == 2 {
			want = 1
		}
		if shrines != want {
			t.Fatalf("level %d shrine count = %d, want %d (shrine is L2-only for now)", lvl, shrines, want)
		}
	}
}

func openEdgeCount(e [4]bool) int {
	n := 0
	for _, o := range e {
		if o {
			n++
		}
	}
	return n
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// --- 0.14.0 behaviour changes -----------------------------------------------

func TestCrawlerBossLaidStartsCollapse(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.grace = -1
	spec := tileSpec{band: g.bands - 1, kind: "boss", base: ttCross.base, isBoss: true}
	opts := g.allLayOptions(spec)
	if len(opts) == 0 {
		t.Fatal("need a legal boss placement to test")
	}
	g.pending = &crawlerPending{spec: spec, options: opts, phaseLay: true}
	g.step = crStepLay
	o := opts[0]
	g.layTile(0, o.X, o.Y, o.Rotation)
	if g.grace != crGraceTurns {
		t.Fatalf("laying the boss lair should start the collapse countdown, grace=%d", g.grace)
	}
}

func TestCrawlerForcedBossConsumedAfterOneLevel(t *testing.T) {
	g, _ := rawCrawler(t, 1)
	g.defeatedBosses = map[string]bool{}
	g.level = 3
	g.forcedBoss = "teslacrane"
	mons := g.rollBossMonsters()
	if mons[0].kind != "teslacrane" {
		t.Fatalf("the forced boss should be drawn, got %q", mons[0].kind)
	}
	if g.forcedBoss != "" {
		t.Fatal("the forced boss should be consumed after its one level")
	}
}

func TestCrawlerDefeatedBossNotRedrawn(t *testing.T) {
	g, _ := rawCrawler(t, 1)
	g.defeatedBosses = map[string]bool{"hauler": true, "vultrix": true}
	g.level = 4
	for i := 0; i < 20; i++ {
		if k := g.rollBossMonsters()[0].kind; k == "hauler" || k == "vultrix" {
			t.Fatalf("a beaten boss must never be redrawn, got %q", k)
		}
	}
}

func TestCrawlerSetTotemLockedOncePlaying(t *testing.T) {
	g := newCrawlerGame(newFakeOutbox(0), 7).(*crawlerGame)
	g.AddPlayer("beaver", "bever") // seat 0, still in the lobby
	crCmd(t, g, 0, false, map[string]any{"t": crMsgTotem, "totem": "wolf"})
	if g.players[0].sheet.totem != "wolf" {
		t.Fatalf("a lobby totem change should switch the sheet, got %q", g.players[0].sheet.totem)
	}
	g.phase = crPlaying
	crCmd(t, g, 0, false, map[string]any{"t": crMsgTotem, "totem": "bever"})
	if g.players[0].sheet.totem != "wolf" {
		t.Fatal("the totem must be locked once the run starts")
	}
}

func TestCrawlerSpareDieIsBonus(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.objects = []string{"spare-die"}
	g.beginTurn(0)
	bonus := 0
	for _, d := range g.dice {
		if d.Bonus {
			bonus++
		}
	}
	if len(g.dice) != h.sheet.diceN+1 || bonus != 1 {
		t.Fatalf("a Spare Die should add exactly one bonus die: dice=%d bonus=%d", len(g.dice), bonus)
	}
}

func TestCrawlerRuinsHealingIsCross(t *testing.T) {
	// Woodland (L1-2) healing is a 3-way T-junction; Ruins (L3-4) and Machine (L5-6) are crosses.
	for _, tc := range []struct{ level, want int }{{1, 3}, {2, 3}, {3, 4}, {4, 4}, {5, 4}, {6, 4}} {
		g, _ := rawCrawler(t, 1)
		g.level = tc.level
		g.biome = biomeForLevel(tc.level)
		g.buildDeck()
		found := false
		for _, s := range g.deck {
			if s.kind == crKindHealing {
				found = true
				if got := openEdgeCount(s.base); got != tc.want {
					t.Fatalf("L%d healing should have %d open edges, got %d", tc.level, tc.want, got)
				}
			}
		}
		if !found {
			t.Fatalf("L%d: no healing tile found in the deck", tc.level)
		}
	}
}

func TestCrawlerArchetypeAnimalMapping(t *testing.T) {
	// The design note's archetype->animal table: each animal gets its archetype's sheet
	// (role + stats), differing only in the totem slug (sprite).
	cases := map[string][]string{
		roleGuardian:  {"bever", "ezel"},
		roleMelee:     {"wolf", "hondshaai", "zwaardvis"},
		roleRanged:    {"pijlinktvis", "wouw", "havik"},
		roleHealer:    {"boomkikker", "franjeschildpad"},
		roleTrickster: {"wasbeer", "eekhoorn", "spinaap", "chimpansee", "koala"},
		roleCarrier:   {"comorenwever", "goudvink"},
	}
	for role, animals := range cases {
		lead := sheetFor(animals[0])
		for _, slug := range animals {
			s := sheetFor(slug)
			if s.role != role {
				t.Fatalf("%s should be role %q, got %q", slug, role, s.role)
			}
			if s.totem != slug {
				t.Fatalf("%s sheet should keep its own totem slug, got %q", slug, s.totem)
			}
			if s.maxHP != lead.maxHP || s.diceN != lead.diceN || s.special != lead.special {
				t.Fatalf("%s should share the %s stat line (HP%d/dice%d/special %q), got HP%d/dice%d/special %q",
					slug, role, lead.maxHP, lead.diceN, lead.special, s.maxHP, s.diceN, s.special)
			}
		}
	}
	if sheetFor("some-unknown-slug").role != roleGeneric {
		t.Fatal("an unmapped animal should fall back to the generic sheet")
	}
}

func TestCrawlerDownedNotDeadAndRevive(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	h0, h1 := g.players[0], g.players[1]
	h1.tileID, h1.square = h0.tileID, h0.square // same tile, so h1 can revive h0
	g.hurt(h0, h0.health+5)                     // lethal
	if !h0.downed || !h0.alive {
		t.Fatalf("a hero at 0 should be downed, not dead: downed=%v alive=%v", h0.downed, h0.alive)
	}
	if g.actingHeroes() != 1 {
		t.Fatalf("one down, one up: actingHeroes=%d", g.actingHeroes())
	}
	g.turn, g.step, g.dice = 1, crStepSpend, []crawlerDie{{Value: 3}, {Value: 3}} // two dice: don't auto-end
	g.revive(1, 0)
	if h0.downed || h0.health != 2 {
		t.Fatalf("revive should bring the ally back on 2 Health: downed=%v health=%d", h0.downed, h0.health)
	}
	if !g.dice[0].Spent || g.dice[1].Spent {
		t.Fatalf("revive should spend exactly one die: %+v", g.dice)
	}
}

func TestCrawlerPartyLostOnlyWhenAllDown(t *testing.T) {
	g, _ := startedCrawler(t, 2)
	g.hurt(g.players[0], 99)
	if g.actingHeroes() == 0 {
		t.Fatal("one hero still up: the party is not lost yet")
	}
	g.hurt(g.players[1], 99)
	if g.actingHeroes() != 0 {
		t.Fatal("both down: no hero can act")
	}
}

func TestCrawlerCourageHeal(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.health, h.courage = 1, 5
	g.turn, g.step = 0, crStepSpend
	g.courageHeal(0)
	if h.health != 2 || h.courage != 3 {
		t.Fatalf("courage-heal should trade 2 Courage for 1 Health: health=%d courage=%d", h.health, h.courage)
	}
	h.health = h.sheet.maxHP
	c := h.courage
	g.courageHeal(0)
	if h.courage != c {
		t.Fatal("courage-heal at full Health should be refused")
	}
}

func TestCrawlerWeakPointOnHazardTile(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.placeTile(1, 0, [4]bool{true, true, true, true}, 0, "cross", false, nil) // a plain tile (not preferred)
	rust := g.placeTile(0, 1, crSpecialBase[crKindRust], 0, crKindRust, false, nil)
	boss := &crawlerMonster{armour: 5, health: 6, maxHealth: 6, damage: 4, square: 4, alive: true, isBoss: true, bossName: "Hauler", specials: []string{bsHardcore}}
	bt := g.placeTile(2, 0, crSpecialBase[crKindGate], 0, "boss", true, []*crawlerMonster{boss})
	g.placeWeakPoint(bt.id)
	// It must land on the rust (hazard) room, never the boss tile.
	var host int = -1
	for _, tl := range g.tiles {
		for _, m := range tl.monsters {
			if m.weakPoint && m.alive {
				host = tl.id
			}
		}
	}
	if host != rust.id {
		t.Fatalf("the weak-point should prefer the hazard (rust) room %d, got tile %d", rust.id, host)
	}
	if host == bt.id || !g.weakPointAlive() {
		t.Fatalf("weak-point must be off the boss tile and visible map-wide (host=%d)", host)
	}
}

func TestCrawlerLayNeverSealsTheMap(t *testing.T) {
	// Mapmaking Rule 4: a tile whose only placements would seal the map (leave no open
	// edge for any future tile) is never laid. Here the sole frontier is one cell and a
	// dead-end drawn on top would close it; the engine buries the dead-end and lays the
	// straight instead, which keeps an exit open.
	g := newCrawlerGame(newFakeOutbox(0), 7).(*crawlerGame)
	g.AddPlayer("beaver", "bever")
	g.phase = crPlaying
	g.bands = 3
	g.grace = -1
	// A dead-end start tile opening only East: the lone frontier is (1,0).
	a := g.placeTile(0, 0, [4]bool{false, true, false, false}, 0, "dead-end", false, nil)
	g.players[0].tileID = a.id
	g.players[0].square = 4
	deadEnd := tileSpec{band: 0, kind: "dead-end", base: [4]bool{false, true, false, false}}
	straight := tileSpec{band: 0, kind: "straight", base: [4]bool{false, true, false, true}}
	g.deck = []tileSpec{deadEnd, straight}
	g.deckPos = 0
	g.turn = 0
	g.beginLayPhase()
	if g.pending == nil || len(g.pending.options) == 0 {
		t.Fatalf("a non-sealing tile should be offered; pending=%v", g.pending)
	}
	if g.pending.spec.kind != "straight" {
		t.Fatalf("the sealing dead-end must be cycled past, the straight laid; got %q", g.pending.spec.kind)
	}
	for _, o := range g.pending.options {
		if !g.keepsForestOpen(o.X, o.Y, o.Edges) {
			t.Fatalf("offered a map-sealing placement at (%d,%d)", o.X, o.Y)
		}
	}
	if len(g.deck) != 2 || g.deck[g.deckPos].kind != "dead-end" {
		t.Fatalf("the sealing dead-end must be CYCLED (kept in the stack, retried next), not discarded; deck=%v deckPos=%d", g.deck, g.deckPos)
	}
}

func TestCrawlerLayCyclesUnconnectableTile(t *testing.T) {
	// Mapmaking Rule 3: a drawn tile that cannot legally connect anywhere is CYCLED past
	// (kept in the stack, retried as the map grows), never discarded. A tile with no
	// openings can never join the map, so the straight behind it is laid instead.
	g := newCrawlerGame(newFakeOutbox(0), 7).(*crawlerGame)
	g.AddPlayer("beaver", "bever")
	g.phase = crPlaying
	g.bands = 3
	g.grace = -1
	a := g.placeTile(0, 0, [4]bool{true, true, true, true}, 0, "cross", false, nil) // open all sides
	g.players[0].tileID = a.id
	g.players[0].square = 4
	blank := tileSpec{band: 0, kind: "blank", base: [4]bool{}} // no openings: never connects
	straight := tileSpec{band: 0, kind: "straight", base: [4]bool{false, true, false, true}}
	g.deck = []tileSpec{blank, straight}
	g.deckPos = 0
	g.turn = 0
	g.beginLayPhase()
	if g.pending == nil || g.pending.spec.kind != "straight" {
		t.Fatalf("the unconnectable blank must be cycled past and the straight laid; pending=%v", g.pending)
	}
	if len(g.deck) != 2 || g.deck[g.deckPos].kind != "blank" {
		t.Fatalf("the unconnectable blank must be kept (cycled), not discarded; deck=%v deckPos=%d", g.deck, g.deckPos)
	}
}

func TestCrawlerLayKeepsMapOpenUntilBoss(t *testing.T) {
	// Across many seeds, proactively laying every tile never seals the map while forest
	// tiles remain: there is always a frontier for the next tile until the boss lair
	// (which alone is allowed to end the forest) is placed.
	for seed := uint64(1); seed <= 40; seed++ {
		g := newCrawlerGame(newFakeOutbox(0), seed).(*crawlerGame)
		g.AddPlayer("beaver", "bever")
		crCmd(t, g, 0, true, map[string]any{"t": crMsgStart}) // opens in the LAY phase
		for i := 0; i < 200; i++ {
			g.step = crStepLay
			g.pending = nil
			g.grace = -1 // keep laying (ignore the collapse clock for this placement sweep)
			g.beginLayTurn(0)
			if g.pending == nil || len(g.pending.options) == 0 {
				break // nothing left the stack can lay
			}
			o := g.pending.options[0]
			boss := g.pending.spec.isBoss
			g.layTile(0, o.X, o.Y, o.Rotation)
			if boss {
				break // the boss lair may seal; stop checking once it is down
			}
			if len(g.frontierCoords()) == 0 {
				t.Fatalf("seed %d, step %d: the map sealed shut before the boss with tiles still to lay", seed, i)
			}
		}
	}
}

func TestCrawlerArmourCappedSoASixAlwaysLands(t *testing.T) {
	// Regression (the "can't hurt Mossback even with a 6" bug): a boss whose Armour climbs
	// (Adaptive Plating) or is buffed must never exceed the max die face, or a 6 could not
	// land and the boss would be permanently unkillable. Here the plating has run away to
	// +5 (effective Armour 10 uncapped); a 6 must still land and crack the plating.
	g, _ := startedCrawler(t, 1)
	m := setupFight(g, 5, 3, 0)
	m.specials = []string{bsAdaptive}
	m.plating = 5
	m.bossName = "Mossback"
	if ea := g.effectiveArmour(g.players[0], m); ea != crMaxDieFace {
		t.Fatalf("effective Armour must cap at %d (a 6 can always land), got %d", crMaxDieFace, ea)
	}
	g.dice = []crawlerDie{{Value: 6}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	if !g.dice[0].Spent {
		t.Fatal("a 6 must land against capped Armour")
	}
	if m.health != 2 {
		t.Fatalf("the landed 6 should have dealt its hit (3 -> 2), got Health %d", m.health)
	}
	if m.plating != 0 {
		t.Fatalf("a landed 6 must crack Adaptive Plating back to 0, got %d", m.plating)
	}
}

func TestCrawlerHaulerGraceScalesWithParty(t *testing.T) {
	// Hauler (the weak-point boss) gets a longer collapse countdown that scales down with
	// party size: 10 solo, 7 at two, 6 at three, 5 at four or more. A boss without the
	// weak-point special keeps the flat crGraceTurns.
	hardcore := []struct {
		players, want int
	}{{1, 10}, {2, 7}, {3, 6}, {4, 5}, {5, 5}}
	for _, tc := range hardcore {
		g := newCrawlerGame(newFakeOutbox(0, 1, 2, 3, 4), 7).(*crawlerGame)
		g.AddPlayer("beaver", "bever")
		for i := 1; i < tc.players; i++ {
			g.AddPlayer("ally", "")
		}
		g.grace = -1
		g.placeTile(0, 0, [4]bool{true, true, true, true}, 0, "boss", true,
			[]*crawlerMonster{{armour: 5, health: 6, maxHealth: 6, alive: true, specials: []string{bsHardcore}}})
		g.startGrace()
		if g.grace != tc.want {
			t.Fatalf("Hauler with %d player(s): want grace %d, got %d", tc.players, tc.want, g.grace)
		}
	}

	// A plain boss (no weak-point) uses crGraceTurns regardless of party size.
	g := newCrawlerGame(newFakeOutbox(0), 7).(*crawlerGame)
	g.AddPlayer("beaver", "bever")
	g.grace = -1
	g.placeTile(0, 0, [4]bool{true, true, true, true}, 0, "boss", true,
		[]*crawlerMonster{{armour: 5, health: 6, maxHealth: 6, alive: true, specials: []string{bsAdaptive}}})
	g.startGrace()
	if g.grace != crGraceTurns {
		t.Fatalf("a plain boss should use crGraceTurns (%d), got %d", crGraceTurns, g.grace)
	}
}

func TestCrawlerPostBossVictoryLap(t *testing.T) {
	// After the boss falls the level does not descend immediately: each hero gets up to
	// crPostBossTurns (2) more turns, then it descends. Here two players each take two
	// End turns, and only the fourth End turn drops to the next level.
	g, _ := startedCrawler(t, 2)
	g.levelCleared = true
	lvl := g.level

	g.endTurn(0) // p0 turn 1/2
	if g.level != lvl {
		t.Fatalf("should not descend after one post-boss turn (level %d)", g.level)
	}
	if g.turn != 1 {
		t.Fatalf("turn should pass to seat 1, got %d", g.turn)
	}
	g.endTurn(1) // p1 turn 1/2
	if g.level != lvl || g.turn != 0 {
		t.Fatalf("should pass back to seat 0 without descending (level %d, turn %d)", g.level, g.turn)
	}
	g.endTurn(0) // p0 turn 2/2 (now at cap)
	if g.level != lvl || g.turn != 1 {
		t.Fatalf("seat 0 at cap, seat 1 still has a turn (level %d, turn %d)", g.level, g.turn)
	}
	g.endTurn(1) // p1 turn 2/2: everyone capped -> descend
	if g.level != lvl+1 {
		t.Fatalf("after both heroes used their 2 post-boss turns, should descend to level %d, got %d", lvl+1, g.level)
	}
	if g.levelCleared {
		t.Fatal("descending should clear the cleared flag")
	}
}

func TestCrawlerEndLevelCurrentPlayerOnly(t *testing.T) {
	// End level descends at once, but only when invoked by the current player.
	g, _ := startedCrawler(t, 2)
	g.levelCleared = true
	lvl := g.level // turn is seat 0

	crCmd(t, g, 1, false, map[string]any{"t": crMsgEndLevel}) // not their turn
	if g.level != lvl {
		t.Fatal("a non-current player must not be able to End level")
	}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndLevel}) // current player
	if g.level != lvl+1 {
		t.Fatalf("the current player's End level should descend to %d, got %d", lvl+1, g.level)
	}
}

func TestCrawlerRustedDieReturnsWhenClearedMidTurn(t *testing.T) {
	// Clearing Rusted during your turn (spring/shop/Spores) gives back the die it took,
	// immediately, rather than only next turn.
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.rusted = true
	g.beginTurn(0) // rolls with Rusted -> one fewer die
	before := len(g.dice)
	if g.clearHeroStatuses(h); len(g.dice) != before+1 {
		t.Fatalf("clearing Rusted mid-turn should restore a die: %d -> %d", before, len(g.dice))
	}
	if h.rusted {
		t.Fatal("Rusted should be cleared")
	}
}

func TestCrawlerLockedInBossLair(t *testing.T) {
	// You cannot leave a boss lair while the boss lives; once it falls, you can.
	g, _ := startedCrawler(t, 1)
	boss := &crawlerMonster{armour: 5, health: 3, maxHealth: 3, alive: true, isBoss: true, square: 4}
	bt := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, "boss", true, []*crawlerMonster{boss})
	g.placeTile(6, 5, [4]bool{false, false, false, true}, 0, "dead-end", false, nil) // east neighbour, open W
	h := g.players[0]
	h.tileID = bt.id
	h.square = crawlerDoor[1] // the east doorway of the boss tile

	if g.tryMove(0, 1) || h.tileID != bt.id {
		t.Fatal("must not be able to leave the boss lair while the boss is alive")
	}
	boss.alive = false
	if !g.tryMove(0, 1) {
		t.Fatal("should be able to leave once the boss is dead")
	}
}

func TestCrawlerFireBitesOnEntry(t *testing.T) {
	// Fire now bites when you step onto it (like live wiring), not only at end of turn.
	g, _ := startedCrawler(t, 1)
	g.level = 5
	ft := g.placeTile(3, 3, [4]bool{true, true, true, true}, 0, "overheat", false, nil)
	ft.hazards = []crawlerHazard{{square: 4, kind: hzFire}}
	h := g.players[0]
	h.tileID = ft.id
	h.square = 4
	h.health = h.sheet.maxHP
	before := h.health
	g.enterHazard(h)
	if h.health != before-2 {
		t.Fatalf("stepping onto fire should deal 2 damage (%d -> %d)", before, h.health)
	}
}

func TestCrawlerRobotsOnHeroUnreachableSquares(t *testing.T) {
	// Robots land on hero-unreachable squares (corners / closed-edge mids), never a walkable
	// one; the boss keeps the centre. So heroes and robots never share a square.
	edges := [4]bool{true, true, true, true} // crossroads: walkable = {4,1,3,5,7}
	boss := &crawlerMonster{isBoss: true}
	r1, r2, r3 := &crawlerMonster{}, &crawlerMonster{}, &crawlerMonster{}
	assignMonsterSquares(edges, []*crawlerMonster{boss, r1, r2, r3})
	if boss.square != 4 {
		t.Fatalf("boss should hold the centre, got %d", boss.square)
	}
	w := crawlerWalkable(edges)
	seen := map[int]bool{}
	for _, m := range []*crawlerMonster{r1, r2, r3} {
		if w[m.square] {
			t.Fatalf("a robot landed on a walkable (hero) square %d", m.square)
		}
		if seen[m.square] {
			t.Fatalf("robots stacked on square %d", m.square)
		}
		seen[m.square] = true
	}
}

func TestCrawlerMoveLayCyclesToFittingTile(t *testing.T) {
	// Moving to a frontier spawns a tile that fits that doorway, cycling past a top tile that
	// cannot connect there (before, the move was silently blocked).
	g, _ := startedCrawler(t, 1)
	g.grace = -1
	g.deck = []tileSpec{
		{kind: "blank", base: [4]bool{}},                            // no openings: never connects
		{kind: "straight", base: [4]bool{false, true, false, true}}, // E-W corridor
	}
	g.deckPos = 0
	if !g.beginLay(0, 1, 1, 0) { // step east onto frontier (1,0)
		t.Fatal("moving to a frontier should spawn a fitting tile")
	}
	if g.pending == nil || g.pending.spec.kind != "straight" {
		t.Fatalf("should have cycled past the unconnectable blank to the straight; pending=%v", g.pending)
	}
}

func TestCrawlerExploreLootChance(t *testing.T) {
	// Any explored room (enemies or not) has a chance at 1-2 loot, capped at 2; the Vault /
	// fog / boss run their own loot and never get this.
	g, _ := startedCrawler(t, 1)
	vault := g.placeTile(5, 5, [4]bool{false, false, true, false}, 0, crKindVault, false, nil)
	g.seedTileLoot(vault, true)
	if len(vault.loot) != 0 {
		t.Fatalf("the Vault runs its own loot; seedTileLoot must skip it, got %d", len(vault.loot))
	}
	sawLoot := false
	for seed := uint64(1); seed <= 80; seed++ {
		g2 := newCrawlerGame(newFakeOutbox(0), seed).(*crawlerGame)
		g2.tiles = map[int]*crawlerTile{}
		g2.at = map[[2]int]int{}
		// An EMPTY ordinary room (no enemies) should still sometimes get loot.
		et := g2.placeTile(5, 5, [4]bool{true, true, true, true}, 0, "cross", false, nil)
		g2.seedTileLoot(et, true)
		if len(et.loot) > 2 {
			t.Fatalf("seed %d: more than 2 loot pieces (%d)", seed, len(et.loot))
		}
		if len(et.loot) > 0 {
			sawLoot = true
		}
	}
	if !sawLoot {
		t.Fatal("explored rooms should sometimes get loot even with no enemies")
	}
}

func TestCrawlerFogL1CacheOnly(t *testing.T) {
	// Level 1 (the tutorial) fog is always a loot cache, never a Hulk-led ambush.
	for seed := uint64(1); seed <= 40; seed++ {
		g := newCrawlerGame(newFakeOutbox(0), seed).(*crawlerGame)
		g.AddPlayer("beaver", "bever")
		g.level = 1
		tile := g.placeTile(3, 3, crSpecialBase[crKindFog], 0, crKindFog, false, nil)
		g.populateFogRoom(tile)
		if len(tile.monsters) != 0 {
			t.Fatalf("seed %d: Level 1 fog must be cache-only, got %d monsters", seed, len(tile.monsters))
		}
		if len(tile.loot) == 0 {
			t.Fatalf("seed %d: Level 1 fog should still drop a cache", seed)
		}
	}
}

func TestCrawlerFoggedAmbushHoldsUntilRevealed(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.level = 2
	h := g.players[0]
	m := &crawlerMonster{armour: 3, health: 4, maxHealth: 4, damage: 3, square: 0, alive: true, kind: "hulk", reward: 2}
	ft := g.placeTile(4, 4, crSpecialBase[crKindFog], 0, crKindFog, false, []*crawlerMonster{m})
	g.seedHazards(ft) // lays the fog hazard, so the tile reads as fogged
	h.tileID = ft.id
	h.square = crawlerDoor[0] // a doorway, not the centre: the tile is not yet revealed
	h.health = h.sheet.maxHP
	g.resolveRound()
	if h.health != h.sheet.maxHP {
		t.Fatalf("a hidden fog ambush must not strike before the tile is revealed, lost %d", h.sheet.maxHP-h.health)
	}
	ft.fogRevealed = true // a hero reached the centre and lifted the fog
	g.resolveRound()
	if h.health != h.sheet.maxHP-m.damage {
		t.Fatalf("a revealed ambush should strike for %d, got health %d", m.damage, h.health)
	}
}

func TestCrawlerFloorLootGating(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	// Proactive LAY-phase lays seed at ~1-in-6 (where enemies spawn), not zero.
	pt := g.placeTile(6, 6, [4]bool{true, true, true, true}, 0, "cross", false, nil)
	for i := 0; i < 120; i++ {
		g.seedTileLoot(pt, false)
	}
	if len(pt.loot) == 0 {
		t.Fatalf("LAY-phase tiles seed floor loot at 1/6; expected some over 120 rolls, got 0")
	}
	// Service tiles never seed, even on exploration.
	for i, kind := range []string{crKindShop, crKindHealing, crKindPortal} {
		st := g.placeTile(10+i, 10+i, crSpecialBase[kind], 0, kind, false, nil)
		for j := 0; j < 50; j++ {
			g.seedTileLoot(st, true)
		}
		if len(st.loot) != 0 {
			t.Fatalf("service tile %q must not seed floor loot, got %d", kind, len(st.loot))
		}
	}
}

func TestCrawlerFastMoverKeepsStepAfterLayingTile(t *testing.T) {
	// A moveRange-2 hero whose move LAID a new tile still gets its remaining bonus step
	// (before, laying a tile swallowed the whole die).
	g, _ := startedCrawler(t, 1)
	h := g.players[0]
	h.sheet.moveRange = 2
	h.square = crawlerDoor[1] // east doorway, facing the frontier
	g.deck = []tileSpec{{kind: "straight", base: [4]bool{false, true, false, true}}}
	g.deckPos = 0
	g.grace = -1
	g.dice = []crawlerDie{{Value: 6}}
	g.turn = 0

	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMove, "dir": 1})
	if g.pending == nil {
		t.Fatal("stepping east off the edge should start a lay")
	}
	o := g.pending.options[0]
	crCmd(t, g, 0, false, map[string]any{"t": crMsgLay, "x": o.X, "y": o.Y, "rotation": o.Rotation})
	if g.bonusMoves != 1 {
		t.Fatalf("a fast mover should keep 1 bonus step after laying a tile, got %d", g.bonusMoves)
	}
}

func TestCrawlerLayPhaseLaysForestNoLoss(t *testing.T) {
	// Regression (the "level 1 had very few tiles" bug): the proactive lay phase must NEVER
	// discard a tile (cycling keeps the stack length constant), and must lay the forest up
	// to the boss. The logged count is the real number of forest tiles for a level-1 deck.
	g := newCrawlerGame(newFakeOutbox(0), 7).(*crawlerGame)
	g.AddPlayer("beaver", "bever")
	g.phase = crPlaying
	g.bands = 3
	g.grace = -1
	g.level = 1
	g.biome = crWoodland
	g.buildDeck()
	deckLen := len(g.deck)
	start := g.placeTile(0, 0, [4]bool{true, true, true, true}, 0, "cross", false, nil)
	g.players[0].tileID = start.id
	g.players[0].square = 4
	g.turn = 0

	laid, sawBoss := 0, false
	for i := 0; i < 200; i++ {
		g.pending = nil
		g.step = crStepLay
		g.grace = -1 // keep laying for this count (ignore the collapse clock)
		g.beginLayTurn(0)
		if len(g.deck) != deckLen {
			t.Fatalf("a tile was DISCARDED: deck length %d -> %d", deckLen, len(g.deck))
		}
		if g.pending == nil || len(g.pending.options) == 0 {
			break
		}
		if g.pending.spec.isBoss {
			sawBoss = true
			break
		}
		o := g.pending.options[0]
		g.layTile(0, o.X, o.Y, o.Rotation)
		laid++
	}
	t.Logf("level 1: laid %d forest tiles of a %d-tile deck before the boss (sawBoss=%v)", laid, deckLen, sawBoss)
	if laid < 8 {
		t.Fatalf("only %d tiles laid of a %d deck: the lay phase is thinning the level", laid, deckLen)
	}
}

func TestCrawlerDeckMatchesLevelTable(t *testing.T) {
	// The deck must contain EXACTLY the per-level tile table (path counts, specials, boss).
	totals := [6]int{20, 25, 21, 26, 22, 27}
	want := map[string][6]int{
		"straight": {4, 4, 2, 5, 2, 3},
		"bend":     {4, 4, 3, 3, 2, 2},
		"tee":      {3, 3, 1, 2, 1, 3},
		"cross":    {2, 2, 1, 2, 1, 2},
		"dead-end": {1, 1, 1, 1, 1, 1},
		"healing":  {2, 2, 2, 2, 2, 2},
		"shop":     {1, 1, 1, 1, 1, 1},
		"portal":   {1, 1, 2, 1, 1, 1},
		"vault":    {0, 1, 1, 1, 1, 1},
		"shrine":   {0, 1, 0, 0, 0, 0},
		"wiring":   {0, 1, 1, 1, 1, 1},
		"coolant":  {0, 1, 1, 1, 1, 1},
		"fog":      {1, 2, 2, 3, 3, 4},
		"rust":     {0, 0, 1, 1, 1, 1},
		"trap":     {0, 0, 1, 1, 2, 2},
		"overheat": {0, 0, 0, 0, 1, 1},
	}
	bossKind := [6]string{"boss", "gate", "boss", "descent", "boss", "boss"}
	for lvl := 1; lvl <= 6; lvl++ {
		g := newCrawlerGame(newFakeOutbox(0), uint64(lvl*7+1)).(*crawlerGame)
		g.AddPlayer("beaver", "bever")
		g.bands = 3
		g.level = lvl
		g.biome = biomeForLevel(lvl)
		g.buildDeck()
		if len(g.deck) != totals[lvl-1] {
			t.Errorf("L%d: deck has %d tiles, table wants %d", lvl, len(g.deck), totals[lvl-1])
		}
		got := map[string]int{}
		bosses := 0
		for _, s := range g.deck {
			if s.isBoss {
				bosses++
				if s.kind != bossKind[lvl-1] {
					t.Errorf("L%d: boss kind %q, want %q", lvl, s.kind, bossKind[lvl-1])
				}
				continue
			}
			got[s.kind]++
		}
		if bosses != 1 {
			t.Errorf("L%d: %d boss tiles, want exactly 1", lvl, bosses)
		}
		for kind, counts := range want {
			if got[kind] != counts[lvl-1] {
				t.Errorf("L%d: %s = %d, table wants %d", lvl, kind, got[kind], counts[lvl-1])
			}
		}
	}
}
