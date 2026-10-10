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

func startedCrawler(t *testing.T, n int) (*crawlerGame, *fakeOutbox) {
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
	if g.phase != crPlaying {
		t.Fatalf("after start: phase %q", g.phase)
	}
	return g, fo
}

func TestCrawlerMenuMinValues(t *testing.T) {
	g, _ := startedCrawler(t, 1)
	s := g.players[0].sheet // Beaver
	// Move >=1, Brace >=2, Attack >=4, Courage >=5.
	for _, tc := range []struct {
		action string
		min    int
	}{{actMove, 1}, {actBrace, 2}, {actAttack, 4}, {actCourage, 5}} {
		if m, _ := s.min(tc.action); m != tc.min {
			t.Fatalf("%s min = %d, want %d", tc.action, m, tc.min)
		}
	}
	// A die must meet the minimum: a 1 cannot Attack, a 4 can Move (go down).
	if v := 1; v >= func() int { m, _ := s.min(actAttack); return m }() {
		t.Fatal("a 1 should not qualify for Attack")
	}
	if v := 4; v < func() int { m, _ := s.min(actMove); return m }() {
		t.Fatal("a 4 should qualify for Move")
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
	crCmd(t, g, 0, false, map[string]any{"t": crMsgEndTurn})
	// Turn 2 begins (same hero, solo). Health must NOT have reset.
	if m.health != 2 {
		t.Fatalf("damage must persist across turns, got Health %d", m.health)
	}
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
	// Unlock needs 3; grant some Courage and unlock the first special.
	g.players[0].courage = 3
	crCmd(t, g, 0, false, map[string]any{"t": crMsgUnlock, "which": 0})
	if !g.players[0].unlocked[0] || g.players[0].courage != 0 {
		t.Fatalf("unlock 0 costs 3: unlocked=%v courage=%d", g.players[0].unlocked, g.players[0].courage)
	}
	// The second unlock needs the first and costs 5 more.
	g.players[0].courage = 4
	crCmd(t, g, 0, false, map[string]any{"t": crMsgUnlock, "which": 1})
	if g.players[0].unlocked[1] {
		t.Fatal("unlock 1 should be refused without enough Courage")
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

func TestCrawlerLayTileOnFrontier(t *testing.T) {
	g, fo := startedCrawler(t, 1)
	// Walk to the north doorway of the start tile and step out into a frontier.
	g.players[0].square = crawlerDoor[0] // north doorway (square 1)
	g.dice = []crawlerDie{{Value: 1}, {Value: 1}, {Value: 1}, {Value: 1}}
	crCmd(t, g, 0, false, map[string]any{"t": crMsgSpend, "die": 0, "action": actMove, "dir": 0})
	if g.step != crStepLay || g.pending == nil {
		t.Fatalf("stepping onto a frontier should pause to lay a tile; step=%q pending=%v", g.step, g.pending)
	}
	v := crLast(t, fo, 0)
	if v.Pending == nil || len(v.Pending.Options) == 0 {
		t.Fatal("the view should offer legal lay options")
	}
	// Every offered option has an opening on the entry side (south, since we left north).
	for _, o := range v.Pending.Options {
		if !o.Edges[crawlerOpp(0)] {
			t.Fatalf("option %+v has no opening on the entry side", o)
		}
	}
	// Choose the first option; the tile is laid and the move completes.
	o := v.Pending.Options[0]
	tilesBefore := len(g.tiles)
	crCmd(t, g, 0, false, map[string]any{"t": crMsgLay, "x": o.X, "y": o.Y, "rotation": o.Rotation})
	if g.step != crStepSpend || g.pending != nil {
		t.Fatal("after laying, play returns to the spend step")
	}
	if len(g.tiles) != tilesBefore+1 {
		t.Fatalf("exactly one tile should be laid: %d -> %d", tilesBefore, len(g.tiles))
	}
	if _, ok := g.at[[2]int{o.X, o.Y}]; !ok {
		t.Fatal("the new tile should be registered at its coord")
	}
}

func TestCrawlerLayOnAllFourSides(t *testing.T) {
	// From the start cross (open on all four sides) the player can lay a tile in
	// every direction: step out each doorway and place.
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
	// Across many seeds, every offered lay option connects on the entry side and
	// matches its placed neighbours.
	for seed := uint64(1); seed <= 60; seed++ {
		g := newCrawlerGame(newFakeOutbox(0), seed).(*crawlerGame)
		g.AddPlayer("beaver", "bever")
		crCmd(t, g, 0, true, map[string]any{"t": crMsgStart})
		for d := 0; d < 4; d++ {
			nx, ny := crawlerDelta[d][0], crawlerDelta[d][1]
			if g.deckPos >= len(g.deck) {
				break
			}
			spec := g.deck[g.deckPos]
			for _, o := range g.layOptions(spec, d, nx, ny) {
				if !o.Edges[crawlerOpp(d)] {
					t.Fatalf("seed %d dir %d: option without entry opening", seed, d)
				}
				if !g.edgesFit(o.X, o.Y, o.Edges) {
					t.Fatalf("seed %d dir %d: option does not fit neighbours", seed, d)
				}
			}
		}
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
