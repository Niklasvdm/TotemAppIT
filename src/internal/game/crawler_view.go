package game

import "fmt"

// Per-seat view serialisation for Totem Crawler (see crawler.go). The DTOs the
// client receives and viewFor, which projects game state for one seat.

type crawlerMenuView struct {
	Action string `json:"action"`
	Min    int    `json:"min"`
}

type crawlerHeroView struct {
	S            int               `json:"s"`
	N            string            `json:"n"`
	Totem        string            `json:"totem"`
	Health       int               `json:"health"`
	MaxHP        int               `json:"maxHP"`
	Courage      int               `json:"courage"`
	Shields      int               `json:"shields"`
	TileID       int               `json:"tile"`
	Square       int               `json:"square"`
	Unlocked     [2]bool           `json:"unlocked"` // specials currently ACTIVE (auto, by Courage)
	Alive        bool              `json:"alive"`
	Downed       bool              `json:"downed,omitempty"` // at 0 Health, out of the fight until an ally revives
	Special      string            `json:"special"`
	Menu         []crawlerMenuView `json:"menu,omitempty"`         // sheet's action menu (only on your own hero)
	Unlock0      string            `json:"unlock0,omitempty"`      // first special's name
	Unlock1      string            `json:"unlock1,omitempty"`      // second special's name
	DiceType     string            `json:"diceType,omitempty"`     // low/balanced/high, for display
	Ranged       bool              `json:"ranged,omitempty"`       // attack can reach adjacent tiles
	Objects      []string          `json:"objects"`                // object slugs held (public)
	Slots        int               `json:"slots"`                  // object capacity (per totem, 2-3)
	Electrified  bool              `json:"electrified,omitempty"`  // status: loses 1 health each round
	Electrocuted bool              `json:"electrocuted,omitempty"` // status: -1 to each die value
	Snared       bool              `json:"snared,omitempty"`       // status: at most 2 Move dice this turn
	Drained      bool              `json:"drained,omitempty"`      // status: can't gain Courage
	Rusted       bool              `json:"rusted,omitempty"`       // status: one fewer die
	FreeReroll   bool              `json:"freeReroll,omitempty"`   // a free reroll (Glove or Lucky Paws) is available (own hero)
	FreeSrc      string            `json:"freeSrc,omitempty"`      // emoji for the free-reroll source (🧤 / 🦝)
	LuckyReady   bool              `json:"luckyReady,omitempty"`   // Lucky Charm nudge still available this turn (own hero)
	Gone         bool              `json:"gone,omitempty"`
}

type crawlerMonsterView struct {
	ID        int      `json:"id"`
	Kind      string   `json:"kind"`
	Armour    int      `json:"armour"`    // base Armour
	EffArmour int      `json:"effArmour"` // Armour the viewing hero must meet to hit (base + boss plating / jammer, minus Barbed Shot)
	Health    int      `json:"health"`    // current, persists across turns
	MaxHP     int      `json:"maxHP"`
	Damage    int      `json:"damage"`
	Reward    int      `json:"reward"` // Courage granted on defeat (scales with toughness)
	Square    int      `json:"square"`
	Engaged   bool     `json:"engaged"`
	Alive     bool     `json:"alive"`
	IsBoss    bool     `json:"isBoss"`
	Name      string   `json:"name,omitempty"`     // boss display name
	Specials  []string `json:"specials,omitempty"` // boss signature specials
}

type crawlerTileView struct {
	ID        int                  `json:"id"`
	X         int                  `json:"x"`
	Y         int                  `json:"y"`
	Kind      string               `json:"kind"`
	Rot       int                  `json:"rot"`
	Edges     [4]bool              `json:"edges"`
	IsBoss    bool                 `json:"isBoss"`
	WeakPoint bool                 `json:"weakPoint,omitempty"` // a live Hardened-Core weak-point hides here (highlighted, shown through fog)
	Monsters  []crawlerMonsterView `json:"monsters"`
	Loot      []crawlerGroundView  `json:"loot,omitempty"`    // objects lying on the floor
	Hazards   []crawlerHazardView  `json:"hazards,omitempty"` // hazards on this tile's squares
	Fogged    bool                 `json:"fogged,omitempty"`  // sensor fog hides this tile until a hero is on/adjacent
}

type crawlerGroundView struct {
	Square int    `json:"square"`
	Slug   string `json:"slug"`
}

type crawlerHazardView struct {
	Square int    `json:"square"` // -1 = whole tile (fog)
	Kind   string `json:"kind"`
}

type crawlerStatusRollView struct {
	Roll   int    `json:"roll"`
	Need   int    `json:"need"`
	Status string `json:"status"`
	Hit    bool   `json:"hit"`
	Target string `json:"target,omitempty"`
}

type crawlerLootRollView struct {
	Roll   int    `json:"roll"`
	Chance int    `json:"chance"`
	Hit    bool   `json:"hit"`
	Name   string `json:"name,omitempty"`
	Killer string `json:"killer,omitempty"`
}

type crawlerFrontierView struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type crawlerView struct {
	T          string                 `json:"t"`
	Phase      string                 `json:"ph"`
	Step       string                 `json:"step"`
	Turn       int                    `json:"turn"`
	You        int                    `json:"you"`
	Dice       []crawlerDie           `json:"dice"`
	Heroes     []crawlerHeroView      `json:"heroes"`
	Tiles      []crawlerTileView      `json:"tiles"`
	Frontiers  []crawlerFrontierView  `json:"frontiers"`
	Pending    *crawlerPendingView    `json:"pending"`
	DeckLeft   int                    `json:"deckLeft"`
	Bands      int                    `json:"bands"`
	BossNear   bool                   `json:"bossNear"`
	BonusMoves int                    `json:"bonusMoves"` // directed steps left in the active hero's multi-move
	Grace      int                    `json:"grace"`      // collapse countdown once the stack is empty; -1 while tiles remain
	Cleared    bool                   `json:"cleared"`    // boss is down: the post-boss victory lap, with End turn / End level
	PostBoss   int                    `json:"postBoss"`   // post-boss turns the CURRENT player has left (0..crPostBossTurns), when cleared
	Level      int                    `json:"level"`      // 1..crMaxLevel
	LevelTiles int                    `json:"levelTiles"` // total tiles in this level's stack (for the start-of-level title card)
	BossName   string                 `json:"bossName"`   // this level's boss display name (for the title card)
	Biome      string                 `json:"biome"`      // Woodland / Ruins / Machine (display label)
	ShopStock  []crawlerShopView      `json:"shopStock"`  // the shop's offer this level (buy when on a shop tile)
	BossPool   []crawlerBossView      `json:"bossPool"`   // the full boss roster, for the debug start picker
	LootRoll   *crawlerLootRollView   `json:"lootRoll"`   // the most recent kill's drop roll (a visible die)
	StatusRoll *crawlerStatusRollView `json:"statusRoll"` // the most recent robot sting's status die
	Log        []string               `json:"log"`
	Debug      *crawlerDebugView      `json:"debug,omitempty"` // only when the run is in debug mode (F3 overlay)
}

// crawlerDebugView is the extra payload the debug overlay reads: the RNG seed, run state,
// the remaining deck (what will spawn) and the per-band spawn odds. Sent only when the run
// was started in debug mode, so normal players never receive it.
type crawlerDebugView struct {
	Seed       uint64            `json:"seed"`
	Level      int               `json:"level"`
	Biome      string            `json:"biome"`
	Phase      string            `json:"phase"`
	Step       string            `json:"step"`
	Turn       int               `json:"turn"`
	Grace      int               `json:"grace"`
	DeckPos    int               `json:"deckPos"`
	DeckLen    int               `json:"deckLen"`
	ForcedBoss string            `json:"forcedBoss"`
	Defeated   []string          `json:"defeated"` // boss slugs already beaten this run
	Upcoming   []crawlerDeckPeek `json:"upcoming"` // the rest of the deck, top first
	Odds       []string          `json:"odds"`     // per-band spawn-weight summary
}

type crawlerDeckPeek struct {
	Idx      int      `json:"idx"`
	Kind     string   `json:"kind"`
	IsBoss   bool     `json:"isBoss"`
	Monsters []string `json:"monsters"` // e.g. "hulk A3 H4 D3"
}

type crawlerBossView struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	MinLvl int    `json:"minLvl"`
	MaxLvl int    `json:"maxLvl"`
}

type crawlerShopView struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	Cost int    `json:"cost"`
}

type crawlerPendingView struct {
	Kind     string             `json:"kind"`
	EntryDir int                `json:"entryDir"`
	Options  []crawlerLayOption `json:"options"`
	PhaseLay bool               `json:"phaseLay"` // true = proactive LAY phase, false = move-triggered
}

func (g *crawlerGame) viewFor(seat int) crawlerView {
	v := crawlerView{
		T: MsgState, Phase: g.phase, Step: g.step, Turn: g.turn, You: seat,
		Dice: append([]crawlerDie{}, g.dice...), DeckLeft: len(g.deck) - g.deckPos,
		Bands: g.bands, Grace: g.grace, BonusMoves: g.bonusMoves, Log: lastLog(g.log, 12),
		Level: g.level, Biome: biomeLabel(g.biome), Cleared: g.levelCleared,
		LevelTiles: len(g.deck), BossName: g.levelBossName(),
		ShopStock: []crawlerShopView{},
		Heroes:    []crawlerHeroView{}, Tiles: []crawlerTileView{}, Frontiers: []crawlerFrontierView{},
	}
	// On the victory lap, tell the client how many post-boss turns the current player has
	// left so it can show "End turn (X left)" alongside "End level".
	if g.levelCleared {
		if h, ok := g.players[g.turn]; ok {
			if left := crPostBossTurns - h.postBossTurns; left > 0 {
				v.PostBoss = left
			}
		}
	}
	for _, slug := range g.shopStock {
		if d := objBySlug(slug); d != nil {
			v.ShopStock = append(v.ShopStock, crawlerShopView{Slug: d.slug, Name: d.name, Cost: d.cost})
		}
	}
	for _, d := range bossRoster {
		v.BossPool = append(v.BossPool, crawlerBossView{Slug: d.slug, Name: d.name, MinLvl: d.minLvl, MaxLvl: d.maxLvl})
	}
	if lr := g.lootRoll; lr != nil {
		v.LootRoll = &crawlerLootRollView{Roll: lr.roll, Chance: lr.chance, Hit: lr.hit, Name: lr.name, Killer: lr.killer}
	}
	if sr := g.statusRoll; sr != nil {
		v.StatusRoll = &crawlerStatusRollView{Roll: sr.roll, Need: sr.need, Status: sr.status, Hit: sr.hit, Target: sr.target}
	}
	// Boss band reached once the final band is the only thing left in the stack.
	if g.phase == crPlaying && g.deckPos >= len(g.deck)-perBandSize() {
		v.BossNear = true
	}
	if g.debug {
		v.Debug = g.debugView()
	}
	connected := map[int]bool{}
	for _, s := range g.out.Seats() {
		connected[s] = true
	}
	for _, s := range g.order {
		h := g.players[s]
		hv := crawlerHeroView{
			S: s, N: h.name, Totem: h.sheet.totem, Health: h.health, MaxHP: h.sheet.maxHP,
			Courage: h.courage, Shields: h.shields, TileID: h.tileID, Square: h.square,
			Unlocked: [2]bool{h.specialActive(0), h.specialActive(1)}, Alive: h.alive, Downed: h.downed, Special: h.sheet.special, Gone: !connected[s],
			Objects: append([]string{}, h.objects...), Slots: heroSlots(h),
			Electrified: h.electrified, Electrocuted: h.electrocuted, Snared: h.snared, Drained: h.drained, Rusted: h.rusted,
		}
		if s == seat { // the viewer's own sheet carries its menu, specials and die type
			hv.Menu = make([]crawlerMenuView, 0, len(h.sheet.menu))
			for _, m := range h.sheet.menu {
				hv.Menu = append(hv.Menu, crawlerMenuView{Action: m.Action, Min: m.Min})
			}
			hv.Unlock0, hv.Unlock1 = h.sheet.unlock0, h.sheet.unlock1
			hv.DiceType = diceTypeName(h.sheet.faces)
			hv.Ranged = h.sheet.ranged
			switch {
			case heroHas(h, "gamblers-glove") && !h.gloveUsed:
				hv.FreeReroll, hv.FreeSrc = true, "🧤"
			case h.sheet.role == roleTrickster && h.specialActive(0) && !h.pawsUsed:
				hv.FreeReroll, hv.FreeSrc = true, "🦝"
			}
			hv.LuckyReady = heroHas(h, "lucky-charm") && !h.luckyUsed
		}
		v.Heroes = append(v.Heroes, hv)
	}
	for _, t := range g.tiles {
		tv := crawlerTileView{ID: t.id, X: t.x, Y: t.y, Kind: t.kind, Rot: t.rot, Edges: t.edges, IsBoss: t.isBoss, Monsters: []crawlerMonsterView{}}
		for _, m := range t.monsters { // the weak-point's TILE is always flagged (highlight shows through fog)
			if m.alive && m.weakPoint {
				tv.WeakPoint = true
				break
			}
		}
		for _, hz := range t.hazards {
			if hz.spent {
				continue
			}
			tv.Hazards = append(tv.Hazards, crawlerHazardView{Square: hz.square, Kind: hz.kind})
		}
		// Sensor fog hides a tile's robots and loot until a hero has stood on its CENTRE
		// (you push into the middle of the room to uncover it); once uncovered it stays
		// revealed (fogRevealed, set on landing) rather than re-hiding when heroes leave.
		fogged := tileHasFog(t) && !t.fogRevealed
		tv.Fogged = fogged
		if !fogged {
			for _, l := range t.loot {
				tv.Loot = append(tv.Loot, crawlerGroundView{Square: l.square, Slug: l.slug})
			}
			for i, m := range t.monsters {
				eff := m.armour
				if vh := g.players[seat]; vh != nil {
					eff = g.effectiveArmour(vh, m) // what THIS hero's die must meet (plating/jammer/barbed)
				}
				tv.Monsters = append(tv.Monsters, crawlerMonsterView{
					ID: i, Kind: m.kind, Armour: m.armour, EffArmour: eff, Health: m.health, MaxHP: m.maxHealth, Damage: m.damage,
					Reward: m.reward, Square: m.square, Engaged: m.engaged, Alive: m.alive, IsBoss: m.isBoss,
					Name: m.bossName, Specials: append([]string{}, m.specials...),
				})
			}
		}
		v.Tiles = append(v.Tiles, tv)
	}
	v.Frontiers = g.frontierViews()
	if g.pending != nil {
		v.Pending = &crawlerPendingView{Kind: g.pending.spec.kind, EntryDir: g.pending.entryDir, Options: append([]crawlerLayOption{}, g.pending.options...), PhaseLay: g.pending.phaseLay}
	}
	return v
}

func perBandSize() int { return 4 }

func (g *crawlerGame) frontierViews() []crawlerFrontierView {
	out := []crawlerFrontierView{}
	for _, f := range g.frontierCoords() {
		out = append(out, crawlerFrontierView{X: f[0], Y: f[1]})
	}
	return out
}

func lastLog(xs []string, n int) []string {
	if len(xs) > n {
		xs = xs[len(xs)-n:]
	}
	return append([]string{}, xs...)
}

func (g *crawlerGame) broadcastViews() {
	for _, seat := range g.out.Seats() {
		g.out.Send(seat, g.viewFor(seat))
	}
}

// debugView assembles the debug overlay payload: the seed and run state, the remaining
// deck (so you can see exactly what tiles and robots will spawn), and the per-band spawn
// odds. Built only when g.debug is set.
func (g *crawlerGame) debugView() *crawlerDebugView {
	d := &crawlerDebugView{
		Seed: g.seed, Level: g.level, Biome: biomeLabel(g.biome),
		Phase: g.phase, Step: g.step, Turn: g.turn, Grace: g.grace,
		DeckPos: g.deckPos, DeckLen: len(g.deck), ForcedBoss: g.forcedBoss,
		Odds: bandOddsText(g.level),
	}
	for _, bd := range bossRoster { // stable order
		if g.defeatedBosses[bd.slug] {
			d.Defeated = append(d.Defeated, bd.slug)
		}
	}
	for i := g.deckPos; i < len(g.deck); i++ {
		s := g.deck[i]
		peek := crawlerDeckPeek{Idx: i - g.deckPos, Kind: s.kind, IsBoss: s.isBoss}
		for _, m := range s.monsters {
			peek.Monsters = append(peek.Monsters, fmt.Sprintf("%s A%d H%d D%d", m.kind, m.armour, m.maxHealth, m.damage))
		}
		d.Upcoming = append(d.Upcoming, peek)
	}
	return d
}
