package game

// Character sheets and dice for Totem Crawler (see crawler.go): the per-totem stat
// lines, the shared die types, and the menu/dice helpers.

type crawlerMenuEntry struct {
	Action string
	Min    int // lowest die value that can pay for this action
}

// Archetype roles. A sheet's behaviour (its signature + Courage unlocks) is keyed on the
// ROLE, not the animal, so several animals can share one archetype (see sheetFor). `totem`
// stays the animal slug (it drives the sprite); `role` drives the mechanics.
const (
	roleGuardian  = "guardian"
	roleMelee     = "melee"
	roleRanged    = "ranged"
	roleHealer    = "healer"
	roleTrickster = "trickster"
	roleCarrier   = "carrier"
	roleGeneric   = "generic"
)

type crawlerSheet struct {
	totem       string
	role        string // archetype (see the role* constants); specials key on this, not totem
	maxHP       int
	diceN       int
	faces       []int // the die's weighted faces (the three types below); roll picks one
	menu        []crawlerMenuEntry
	special     string // the inherent special action's name (brace/mend/carry/guard), or "" for none
	moveRange   int    // squares per Move die (1, or 2 for a fast mover)
	ranged      bool   // may attack a monster on an adjacent tile, not only its own
	objectSlots int    // how many objects this totem can carry (2-3 per the design note)
	unlock0     string // first unlock label
	unlock1     string // second unlock label
}

// The three die types from the design. Every number rolled stays useful: the sheet,
// not the face, decides what it can become. A High die meets Armour far more often; a
// Low die rarely attacks but feeds Move/Special.
var (
	diceLow      = []int{1, 2, 2, 3, 3, 6} // favours Move/Special, weak attack
	diceBalanced = []int{1, 2, 3, 4, 5, 6} // flexible, a plain d6
	diceHigh     = []int{2, 4, 5, 5, 6, 6} // favours Attack/Courage
)

// baseMenu is the default die-value map shared by every sheet, with the sheet's own
// special in the 2+ slot. Move is any die (>=1), the special >=2, Courage >=5, and
// Attack has no minimum (any die; the hit is decided by the monster's Armour).
func baseMenu(special string) []crawlerMenuEntry {
	return []crawlerMenuEntry{{actMove, 1}, {special, 2}, {actAttack, 1}, {actCourage, 5}}
}

// Guardian (tank): tanky, Low dice whose weak numbers become Shields. Brace + Thick Hide
// + Bulwark. Animals: Beaver, Donkey.
func guardianSheet(totem string) crawlerSheet {
	return crawlerSheet{
		totem: totem, role: roleGuardian, maxHP: 7, diceN: 4, faces: diceLow, moveRange: 1, objectSlots: 2,
		menu: baseMenu(actBrace), special: actBrace, unlock0: "Thick Hide", unlock1: "Bulwark",
	}
}

// Striker, ranged: fragile, High dice that meet Armour often. Its signature is RANGED
// damage (attack a monster on an adjacent tile), so no active special-action slot. Barbed
// Shot + Deep Volley. Animals: Arrow Squid, Kite, Hawk.
func rangedSheet(totem string) crawlerSheet {
	return crawlerSheet{
		totem: totem, role: roleRanged, maxHP: 4, diceN: 3, faces: diceHigh, moveRange: 1, ranged: true, objectSlots: 2,
		menu:    []crawlerMenuEntry{{actMove, 1}, {actAttack, 1}, {actCourage, 5}},
		special: "", unlock0: "Barbed Shot", unlock1: "Deep Volley",
	}
}

// Healer / support: staying power, Low dice that become frequent Mends. Spores + Bloom.
// Animals: Treefrog, Fringed turtle.
func healerSheet(totem string) crawlerSheet {
	return crawlerSheet{
		totem: totem, role: roleHealer, maxHP: 6, diceN: 5, faces: diceLow, moveRange: 1, objectSlots: 3,
		menu: baseMenu(actMend), special: actMend, unlock0: "Spores", unlock1: "Bloom",
	}
}

// Trickster / loot: 5 Balanced dice, Pilfer as its die-special (grab a ground item from an
// adjacent tile), plus an inherent +1 loot draw on every kill. Lucky Paws (free reroll) +
// Big Score (a 4th slot and an extra drop). Animals: Raccoon, Squirrel, Spider Monkey,
// Chimpanzee, Koala.
func tricksterSheet(totem string) crawlerSheet {
	return crawlerSheet{
		totem: totem, role: roleTrickster, maxHP: 5, diceN: 5, faces: diceBalanced, moveRange: 1, objectSlots: 3,
		menu: baseMenu(actPilfer), special: actPilfer, unlock0: "Lucky Paws", unlock1: "Big Score",
	}
}

// Carrier / mobility: 5 Low dice, the fast mover (2 squares per Move die). Carry gives an
// ally a free move; Ferry crosses a tile edge, Flock makes it two squares. Animals:
// Comoros Weaver, Bullfinch.
func carrierSheet(totem string) crawlerSheet {
	return crawlerSheet{
		totem: totem, role: roleCarrier, maxHP: 5, diceN: 5, faces: diceLow, moveRange: 2, objectSlots: 2,
		menu: baseMenu(actCarry), special: actCarry, unlock0: "Ferry", unlock1: "Flock",
	}
}

// Striker, melee: 4 High dice, fights up close (takes the hits the ranged striker avoids).
// Passive Group strength (+1 heal with an ally on your tile); unlocks reward fighting in a
// pack. No die-special. Animals: Wolf, Houndshark, Swordfish.
func meleeSheet(totem string) crawlerSheet {
	return crawlerSheet{
		totem: totem, role: roleMelee, maxHP: 5, diceN: 4, faces: diceHigh, moveRange: 1, objectSlots: 3,
		menu:    []crawlerMenuEntry{{actMove, 1}, {actAttack, 1}, {actCourage, 5}},
		special: "", unlock0: "Pack Tactics", unlock1: "Frenzy",
	}
}

// genericSheet is the fallback for totems without a bespoke design: a standard 4
// Balanced dice, Guard special.
func genericSheet(totem string) crawlerSheet {
	return crawlerSheet{
		totem: totem, role: roleGeneric, maxHP: 5, diceN: 4, faces: diceBalanced, moveRange: 1, objectSlots: 3,
		menu: baseMenu(actGuard), special: actGuard, unlock0: "Resolve", unlock1: "Second Wind",
	}
}

// sheetFor maps a totem slug to its archetype sheet (the design note's archetype->animal
// table), falling back to the generic one. Each archetype carries several animals; they
// share the sheet's mechanics and differ only in sprite (the `totem` slug).
func sheetFor(totem string) crawlerSheet {
	switch totem {
	case "bever", "ezel": // Guardian
		return guardianSheet(totem)
	case "wolf", "hondshaai", "zwaardvis": // Striker, melee
		return meleeSheet(totem)
	case "pijlinktvis", "wouw", "havik": // Striker, ranged
		return rangedSheet(totem)
	case "boomkikker", "franjeschildpad": // Healer
		return healerSheet(totem)
	case "wasbeer", "eekhoorn", "spinaap", "chimpansee", "koala": // Trickster
		return tricksterSheet(totem)
	case "comorenwever", "goudvink": // Carrier
		return carrierSheet(totem)
	case "":
		return genericSheet("totem")
	default:
		return genericSheet(totem)
	}
}

func (s crawlerSheet) min(action string) (int, bool) {
	for _, m := range s.menu {
		if m.Action == action {
			return m.Min, true
		}
	}
	return 0, false
}

// diceTypeName labels a face set as low/balanced/high for the character sheet.
func diceTypeName(faces []int) string {
	sum := 0
	for _, f := range faces {
		sum += f
	}
	switch {
	case len(faces) == 0:
		return "balanced"
	case sum <= 17: // {1,2,2,3,3,6}=17
		return "low"
	case sum >= 28: // {2,4,5,5,6,6}=28
		return "high"
	default:
		return "balanced"
	}
}

// rollFace rolls one weighted die face for the sheet's die type (falls back to a
// plain d6 for a sheet with no faces set).
func (g *crawlerGame) rollFace(s crawlerSheet) int {
	if len(s.faces) == 0 {
		return 1 + g.rng.IntN(6)
	}
	return s.faces[g.rng.IntN(len(s.faces))]
}
