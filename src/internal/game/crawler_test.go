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
		totem string
		faces []int
		hp    int
		diceN int
	}{
		{"bever", diceLow, 7, 4},
		{"pijlinktvis", diceHigh, 4, 4},
		{"boomkikker", diceLow, 6, 4},
		{"wasbeer", diceBalanced, 5, 5},
		{"comorenwever", diceLow, 5, 5},
	} {
		s := sheetFor(tc.totem)
		if s.maxHP != tc.hp || s.diceN != tc.diceN {
			t.Fatalf("%s: hp=%d diceN=%d, want %d/%d", tc.totem, s.maxHP, s.diceN, tc.hp, tc.diceN)
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
	g.players[0].sheet = treefrogSheet() // Mend special
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
	h := &crawlerHero{sheet: beaverSheet()}
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

// A tile can hold several monsters (a swarm), each laid on its own walkable square.
func TestCrawlerMultipleMonstersPerTile(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	mons := []*crawlerMonster{
		{armour: 3, health: 1, maxHealth: 1, damage: 1, kind: "drone", alive: true, reward: 1},
		{armour: 3, health: 1, maxHealth: 1, damage: 1, kind: "drone", alive: true, reward: 1},
		{armour: 3, health: 1, maxHealth: 1, damage: 1, kind: "drone", alive: true, reward: 1},
	}
	edges := [4]bool{true, true, true, true} // a cross: centre + four doorways walkable
	assignMonsterSquares(edges, mons)
	seen := map[int]bool{}
	for _, m := range mons {
		if seen[m.square] {
			t.Fatalf("two monsters share square %d: a swarm must spread out", m.square)
		}
		if !crawlerWalkable(edges)[m.square] {
			t.Fatalf("monster placed on non-walkable square %d", m.square)
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

// The boss's Health scales with the party size (~3 per hero) rather than a flat value.
func TestCrawlerBossHealthScalesWithPlayers(t *testing.T) {
	for _, n := range []int{1, 2, 4, 6} {
		g := newCrawlerGame(newFakeOutbox(0), 7).(*crawlerGame)
		for i := 0; i < n; i++ {
			g.AddPlayer("hero", "")
		}
		g.bands = 3
		g.buildDeck()
		want := 3 * n
		if want < 3 {
			want = 3
		}
		found := false
		for _, s := range g.deck {
			if !s.isBoss {
				continue
			}
			found = true
			if len(s.monsters) != 1 || s.monsters[0].maxHealth != want {
				t.Fatalf("%d players: boss Health = %v, want %d", n, s.monsters, want)
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
	g.players[0].sheet = comorosSheet() // moveRange 2
	g.players[0].square = 3             // W doorway of the start cross
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
	s := squidSheet()
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

// A healing spring heals the moment a hero steps onto its centre, costs no die, and is
// not consumed: stepping off and back on heals again.
func TestCrawlerHealingSpringHealsOnEntryRepeatable(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	spring := g.placeTile(5, 5, [4]bool{true, true, true, true}, 0, crKindHealing, false, nil)
	h := g.players[0]
	h.tileID = spring.id
	h.square = 3 // W doorway, not yet on the spring
	h.health = 2
	g.dice = []crawlerDie{{Value: 1}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMove, "dir": 1}) // East -> centre
	if h.square != 4 || h.health != 2+crSpringHeal {
		t.Fatalf("stepping onto the spring centre should heal with no extra die: sq=%d hp=%d", h.square, h.health)
	}
	// Step off to the N doorway and back onto the centre: it heals again (not depleted).
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 1, "action": actMove, "dir": 0}) // North -> N door
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 2, "action": actMove, "dir": 2}) // South -> centre
	if h.square != 4 || h.health != 2+2*crSpringHeal {
		t.Fatalf("re-entering the spring centre should heal again (not depleted): sq=%d hp=%d", h.square, h.health)
	}
}

// Arrow Squid specials: Barbed Shot (Courage >= 3) lets a lower die land; Deep Volley
// (Courage >= 5) makes an attacking 6 count as two hits, once a turn.
func TestCrawlerSquidBarbedShotAndDeepVolley(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	g.players[0].sheet = squidSheet()
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

func TestCrawlerBossDefeatWins(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	boss := &crawlerMonster{armour: 5, health: 1, maxHealth: 1, damage: 4, square: 4, alive: true, isBoss: true, reward: 3}
	bt := g.placeTile(9, 9, [4]bool{true, false, false, false}, 2, "boss", true, []*crawlerMonster{boss})
	g.players[0].tileID = bt.id
	g.dice = []crawlerDie{{Value: 6}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actAttack, "monster": 0})
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	if g.phase != crWon {
		t.Fatalf("defeating the boss should win, phase=%q", g.phase)
	}
}
