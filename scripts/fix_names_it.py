#!/usr/bin/env python3
"""Apply confident Italian NAME corrections to data/animals.json.

Each fix was verified against the Italian Wikipedia article intro (vernacular
name in the lead sentence) reached via the validated English name's langlink.
Only cases where the current `it` was untranslated (== nl) AND a clear,
distinct Italian vernacular exists are changed. Conservative by design.
"""
import json

PATH = "data/animals.json"

# slug -> (expected_old_it, new_it, source note)
NAME_FIXES = {
    "geep":   ("Geep",   "Aguglia",          "it.wiki Belonidae: 'Le aguglie'"),
    "gibbon": ("Gibbon", "Gibbone",          "it.wiki Hylobatidae: 'detti comunemente gibboni'"),
    "manoel": ("Manoel", "Gatto di Pallas",  "it.wiki Otocolobus manul: 'Il gatto di Pallas'"),
    "serval": ("Serval", "Servalo",          "it.wiki Leptailurus serval: 'Il servalo'"),
}


def main():
    data = json.load(open(PATH, encoding="utf-8"))
    changed = 0
    for a in data:
        fix = NAME_FIXES.get(a["slug"])
        if not fix:
            continue
        old, new, _ = fix
        if a["it"] != old:
            print(f"SKIP {a['slug']}: current it={a['it']!r} != expected {old!r}")
            continue
        a["it"] = new
        changed += 1
        print(f"FIXED {a['slug']}: {old!r} -> {new!r}")
    with open(PATH, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
        f.write("\n")
    print(f"Total it-name fixes applied: {changed}")


if __name__ == "__main__":
    main()
