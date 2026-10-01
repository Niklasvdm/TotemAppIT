#!/usr/bin/env python3
"""
Phase 1 — build and seed the normalised SQLite catalogue from data/animals.json.

This is schema-first tooling: it validates the data model against the real
dataset before any Go exists. The DB it produces is *unencrypted* — file-level
Adiantum encryption is a how-you-open-it concern added in Phase 2 (Go/ncruces)
and does not change a single table.

Usage:
    python3 scripts/seed_db.py                 # -> build/totem.db
    python3 scripts/seed_db.py --db out.db --force
    python3 scripts/seed_db.py --check         # build in-memory, report stats, write nothing

The build_db() function is imported by scripts/db_test.py to seed an in-memory DB.
"""
from __future__ import annotations

import argparse
import json
import sqlite3
import sys
from pathlib import Path

ROOT = Path(__file__).parent.parent
DATA_FILE = ROOT / "data" / "animals.json"
MIGRATIONS_DIR = ROOT / "src" / "internal" / "store" / "migrations"
DEFAULT_DB = ROOT / "build" / "totem.db"

LANGS = ("nl", "it", "en")


def load_animals(data_file: Path = DATA_FILE) -> list[dict]:
    with data_file.open(encoding="utf-8") as fh:
        return json.load(fh)


def apply_migrations(conn: sqlite3.Connection, migrations_dir: Path = MIGRATIONS_DIR) -> None:
    for sql_file in sorted(migrations_dir.glob("*.sql")):
        conn.executescript(sql_file.read_text(encoding="utf-8"))


def seed(conn: sqlite3.Connection, animals: list[dict]) -> None:
    """Populate an already-migrated connection. Idempotent only on a fresh DB."""
    cur = conn.cursor()

    # trait dedupe: canonical Dutch key -> trait id, remembering first-seen it/en.
    trait_id: dict[str, int] = {}
    trait_tr: dict[str, dict[str, str]] = {}

    for a in animals:
        cur.execute(
            "INSERT INTO animal (slug, name_nl, alt_names, source_url) VALUES (?, ?, ?, ?)",
            (a["slug"], a.get("nl", ""), a.get("alt", "") or "", a.get("source_url", "") or ""),
        )
        aid = cur.lastrowid

        # translations (name + description) per language
        names = {"nl": a.get("nl", ""), "it": a.get("it", ""), "en": a.get("en", "")}
        descs = {"nl": a.get("desc_nl", ""), "it": a.get("desc_it", ""), "en": a.get("desc_en", "")}
        cur.executemany(
            "INSERT INTO translation (animal_id, lang, name, description) VALUES (?, ?, ?, ?)",
            [(aid, lg, names[lg] or "", descs[lg] or "") for lg in LANGS],
        )

        # traits: aligned nl/it/en arrays (verified aligned in the dataset)
        tnl = a.get("traits_nl", [])
        tit = a.get("traits_it", [])
        ten = a.get("traits_en", [])
        for i, key in enumerate(tnl):
            if key not in trait_id:
                cur.execute("INSERT INTO trait (key_nl) VALUES (?)", (key,))
                trait_id[key] = cur.lastrowid
                trait_tr[key] = {
                    "nl": key,
                    "it": tit[i] if i < len(tit) else "",
                    "en": ten[i] if i < len(ten) else "",
                }
            cur.execute(
                "INSERT OR IGNORE INTO animal_trait (animal_id, trait_id) VALUES (?, ?)",
                (aid, trait_id[key]),
            )

    # trait translations (one row per trait per language)
    for key, tid in trait_id.items():
        tr = trait_tr[key]
        cur.executemany(
            "INSERT INTO trait_translation (trait_id, lang, value) VALUES (?, ?, ?)",
            [(tid, lg, tr[lg]) for lg in LANGS],
        )

    # Free-text search is done with LIKE at query time (FTS5 is not in the Go
    # runtime's pure-Go SQLite build; at ~471 rows a LIKE scan is sub-millisecond).
    conn.commit()


def build_db(conn: sqlite3.Connection, animals: list[dict] | None = None,
             migrations_dir: Path = MIGRATIONS_DIR) -> sqlite3.Connection:
    """Migrate + seed a connection. Used by both main() and the test suite."""
    conn.execute("PRAGMA foreign_keys = ON")
    apply_migrations(conn, migrations_dir)
    seed(conn, animals if animals is not None else load_animals())
    return conn


def _stats(conn: sqlite3.Connection) -> dict[str, int]:
    q = conn.execute
    return {
        "animals": q("SELECT COUNT(*) FROM animal").fetchone()[0],
        "traits": q("SELECT COUNT(*) FROM trait").fetchone()[0],
        "animal_trait": q("SELECT COUNT(*) FROM animal_trait").fetchone()[0],
        "translations": q("SELECT COUNT(*) FROM translation").fetchone()[0],
        "trait_translations": q("SELECT COUNT(*) FROM trait_translation").fetchone()[0],
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--db", type=Path, default=DEFAULT_DB, help="output DB path")
    ap.add_argument("--data", type=Path, default=DATA_FILE, help="animals.json path")
    ap.add_argument("--force", action="store_true", help="overwrite an existing DB file")
    ap.add_argument("--check", action="store_true", help="build in-memory, print stats, write nothing")
    args = ap.parse_args()

    animals = load_animals(args.data)

    if args.check:
        conn = sqlite3.connect(":memory:")
        build_db(conn, animals)
        for k, v in _stats(conn).items():
            print(f"  {k:20} {v}")
        return 0

    if args.db.exists():
        if not args.force:
            print(f"refusing to overwrite {args.db} (pass --force)", file=sys.stderr)
            return 1
        args.db.unlink()
    args.db.parent.mkdir(parents=True, exist_ok=True)

    conn = sqlite3.connect(args.db)
    build_db(conn, animals)
    stats = _stats(conn)
    conn.close()
    print(f"wrote {args.db}")
    for k, v in stats.items():
        print(f"  {k:20} {v}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
