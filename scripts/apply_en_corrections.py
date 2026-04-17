#!/usr/bin/env python3
"""
Manually correct English (and some Italian) animal names that DeepL translated badly.

DeepL lacks animal context for short names, causing errors like:
  Bij (bee) → "At"   |  Wouw (red kite) → "Wow"  |  Steenloper → "Stonehenge"

Add corrections here as you spot them. Safe to re-run at any time.

Usage:
    python3 scripts/apply_en_corrections.py
    python3 scripts/generate_pages.py   # rebuild pages after
"""
import json
from pathlib import Path

ROOT = Path(__file__).parent.parent
DATA_FILE = ROOT / "data" / "animals.json"

# slug -> correct English name
EN_CORRECTIONS = {
    # ── Already-wrong translations from first batch ──────────────────────────
    "aardeekhoorn":  "Chipmunk",              # IT=Tamia; NL aardeekhoorn = chipmunk
    "agame":         "Agama",
    "agoeti":        "Agouti",
    "alk":           "Razorbill",
    "ara":           "Macaw",
    "arassari":      "Aracari",
    "arend":         "Eagle",
    "beensnoek":     "Longnose gar",
    "beerdiertje":   "Tardigrade",
    "beloega":       "Beluga",
    "beo":           "Hill myna",
    "berglemming":   "Mountain lemming",
    "bij":           "Bee",
    "bosbok":        "Bushbuck",
    "bosduiker":     "Bush duiker",
    "dagpauwoog":    "Peacock butterfly",
    "damhert":       "Fallow deer",
    "das":           "Badger",
    "dikdik":        "Dik-dik",
    "distelvink":    "Goldfinch",
    "doornkruiper":  "Treecreeper",
    "draaihals":     "Wryneck",
    "ever":          "Boar",
    "fennek":        "Fennec fox",
    "fitis":         "Willow warbler",
    "fluithaas":     "Pika",
    "franjepoot":    "Phalarope",
    "frankolijn":    "Francolin",

    # ── New corrections from full-run review ─────────────────────────────────
    "fosse":                "Fossa",
    "fuutkoet":             "Little grebe",
    "gaffelbok":            "Pronghorn",
    "gems":                 "Chamois",
    "genetkat":             "Genet",
    "griend":               "Pilot whale",
    "haas":                 "Hare",
    "hazewind":             "Greyhound",
    "hedendaagse-wolf":     "Grey wolf",
    "hoenderkoet":          "Purple swamphen",
    "hop":                  "Hoopoe",
    "jabiroe":              "Jabiru",
    "jak":                  "Yak",
    "jako":                 "African grey parrot",
    "karakol":              "Caracal",
    "karekiet":             "Reed warbler",
    "katfret":              "Asian wild cat",
    "kauw":                 "Jackdaw",
    "kemphaan":             "Ruff",            # NOT kestrel; kemphaan = ruff (bird)
    "kempvis":              "Siamese fighting fish",
    "kwal":                 "Jellyfish",
    "leeuw":                "Lion",
    "leeuwaapje":           "Lion tamarin",
    "lepelaar":             "Spoonbill",
    "loodsmannetje":        "Pilot fish",
    "lori-aap":             "Slow loris",
    "lori-papegaai":        "Lory",
    "maki":                 "Lemur",
    "meerkat":              "Vervet monkey",   # NL meerkat = vervet; stokstaartje = EN meerkat
    "meerkoet":             "Coot",
    "merel":                "Blackbird",
    "moeflon":              "Mouflon",
    "murene":               "Moray eel",
    "nachtaap":             "Owl monkey",
    "neusbeer":             "Coati",
    "oeakari":              "Uakari",
    "ornaatelfje":          "Ornate woodnymph",
    "pad":                  "Toad",
    "palmtortel":           "Laughing dove",
    "ree":                  "Roe deer",
    "renvogel":             "Roadrunner",
    "roofvogel-algemeen":   "Bird of prey",
    "roodborstje":          "Robin",
    "schaarbek":            "Skimmer",
    "schrijvertje":         "Whirligig beetle",
    "schorpioen":           "Scorpion",
    "sikahert":             "Sika deer",
    "slang-algemeen":       "Snake",
    "slankbeer":            "Sloth bear",
    "snip":                 "Snipe",
    "spiesbok":             "Oryx",
    "spitsvogel":           "Red-backed shrike",  # NOT shrew; spitsvogel = shrike
    "spoorkoekoek":         "Spur-winged cuckoo",
    "steenbok":             "Ibex",
    "steenloper":           "Ruddy turnstone",
    "steltkluut":           "Black-winged stilt",
    "steltral":             "Stilt rail",
    "steppelemming":        "Steppe lemming",
    "stern":                "Tern",
    "strandloper":          "Sandpiper",
    "toepaja":              "Treeshrew",
    "toerako":              "Turaco",
    "vos":                  "Fox",
    "wouw":                 "Red kite",
    "zeewolf":              "Wolffish",
    "zonnevis":             "Ocean sunfish",
}

# slug -> correct Italian name (only for known errors)
IT_CORRECTIONS = {
    "honingdas":  "Tasso del miele",   # was "Ratel" (Afrikaans name)
    "alpenhond":  "Cane da montagna",  # was "Cane selvatico asiatico" (wrong animal entirely)
}


def main():
    with open(DATA_FILE, encoding="utf-8") as f:
        animals = json.load(f)

    en_updated = it_updated = 0

    for a in animals:
        slug = a.get("slug", "")

        if slug in EN_CORRECTIONS:
            old, new = a.get("en", ""), EN_CORRECTIONS[slug]
            if old != new:
                a["en"] = new
                en_updated += 1
                print(f"  EN  {slug:30s}  {old!r:28s} -> {new!r}")

        if slug in IT_CORRECTIONS:
            old, new = a.get("it", ""), IT_CORRECTIONS[slug]
            if old != new:
                a["it"] = new
                it_updated += 1
                print(f"  IT  {slug:30s}  {old!r:28s} -> {new!r}")

    with open(DATA_FILE, "w", encoding="utf-8") as f:
        json.dump(animals, f, ensure_ascii=False, indent=2)

    print(f"\nApplied {en_updated} EN correction(s), {it_updated} IT correction(s).")
    print("Next: python3 scripts/generate_pages.py")


if __name__ == "__main__":
    main()
