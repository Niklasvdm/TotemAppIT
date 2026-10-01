#!/usr/bin/env python3
"""Apply reviewed trait-translation corrections to data/animals.json.

Builds the nl -> {it,en} map, applies the corrections below (keyed by the
Dutch trait), then rewrites every animal's traits_it / traits_en in place by
mapping each traits_nl[i] through the corrected map. traits_nl is never touched.

Run from the repo root. Writes with indent=2 / ensure_ascii=False to preserve
the original formatting.
"""
import json
import sys

PATH = "data/animals.json"

# nl trait -> partial override {"it": ..., "en": ...}
CORRECTIONS = {
    # --- English fixes (clear mistranslations / non-words / wrong POS) ---
    "aangepast":        {"en": "adapted"},        # was "custom" (wrong meaning)
    "aanpassend":       {"en": "adaptable"},      # was "customising"
    "afwachtend":       {"en": "wait-and-see"},   # was "waiting"
    "bedaard":          {"en": "composed"},       # was "subdued"
    "beweeglijk":       {"en": "agile"},          # was "movable" (wrong)
    "driftig":          {"en": "irascible"},      # was "adrift" (nonsense)
    "druk":             {"en": "busy"},           # was "print" (MT homonym error)
    "fel":              {"en": "fierce"},         # was "bright"
    "gezapig":          {"en": "placid"},         # was "dull"
    "gezellig":         {"en": "convivial"},      # was "cozy"
    "honkvast":         {"en": "home-loving"},    # was "base" (MT homonym error)
    "instinctief":      {"en": "instinctive"},    # was "instinctively" (adverb)
    "monter":           {"en": "chipper"},        # was "monter" (untranslated)
    "nachtelijk":       {"en": "nocturnal"},      # was "night" (noun)
    "plantrekker":      {"en": "resourceful"},    # was "plant puller" (nonsense)
    "potsierlijk":      {"en": "comical"},        # was "ludicrous"
    "respectvol":       {"en": "respectful"},     # was "respectfully" (adverb)
    "vlug":             {"en": "quick"},          # was "Quick" (capitalisation)
    "vrijgevochten":    {"en": "free-spirited"},  # was "liberal"
    "apart":            {"en": "distinctive"},    # was "separate" (wrong meaning)
    "familiaal":        {"en": "family-oriented"},# was "family" (noun)
    "onstuimig":        {"en": "impetuous"},      # was "heady"
    "verstrooid":       {"en": "absent-minded"},  # was "scattered"
    "onberekenbaar":    {"en": "unpredictable"},  # was "unaccountable"
    "doelgericht":      {"en": "goal-oriented"},  # was "targeted" (wrong meaning)
    "eetlustig":        {"en": "hearty eater"},   # was "appetising" (wrong meaning)
    "zorgvuldig":       {"en": "meticulous"},     # was "careful"/"carefully" (conflict + dup of voorzichtig)

    # --- Italian fixes ---
    "eigenwijze":       {"it": "testardo"},       # was "eigenwijze" (untranslated)
    "flamboyant":       {"it": "sgargiante"},     # was "flamboyante" (not Italian)
    "slaapgraag":       {"it": "dormiglione"},    # was "pigro" (= lazy; now sleepy)
    "probleemoplossend":{"it": "risolutivo"},     # was "risoluto" (= resolute)
    "opvrolijkend":     {"it": "rallegrante"},    # was "rallegra gli altri" (clunky phrase)

    # --- Both it + en ---
    "zanglustig":       {"it": "canterino", "en": "songful"},   # was melodioso (dup) / "vocal"
    "bouwlustig":       {"it": "costruttore", "en": "builder"}, # was costruttivo / "building"
}


def build_map(data):
    m = {}
    for a in data:
        for i in range(len(a["traits_nl"])):
            nl = a["traits_nl"][i]
            if nl not in m:
                m[nl] = {"it": a["traits_it"][i], "en": a["traits_en"][i]}
    return m


def main():
    data = json.load(open(PATH, encoding="utf-8"))
    m = build_map(data)

    # Validate corrections reference existing keys
    for k in CORRECTIONS:
        if k not in m:
            print(f"WARNING: correction key not found in data: {k!r}", file=sys.stderr)

    # Apply corrections to the map
    for k, ov in CORRECTIONS.items():
        if k in m:
            m[k].update(ov)

    # Rewrite every animal's parallel it/en arrays from the corrected map
    it_changes = en_changes = 0
    for a in data:
        for i in range(len(a["traits_nl"])):
            nl = a["traits_nl"][i]
            new_it = m[nl]["it"]
            new_en = m[nl]["en"]
            if a["traits_it"][i] != new_it:
                a["traits_it"][i] = new_it
                it_changes += 1
            if a["traits_en"][i] != new_en:
                a["traits_en"][i] = new_en
                en_changes += 1

    with open(PATH, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
        f.write("\n")

    print(f"Applied. it cells changed: {it_changes}, en cells changed: {en_changes}")
    distinct_it = sum(1 for k, ov in CORRECTIONS.items() if "it" in ov)
    distinct_en = sum(1 for k, ov in CORRECTIONS.items() if "en" in ov)
    print(f"Distinct nl traits with it fix: {distinct_it}, with en fix: {distinct_en}")


if __name__ == "__main__":
    main()
