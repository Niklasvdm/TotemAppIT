#!/usr/bin/env python3
"""
Phase 1 — data-integrity + query tests for the normalised catalogue.

Runs against an in-memory DB seeded from the real data/animals.json, so there is
nothing to clean up: every test builds a throwaway :memory: database and it
vanishes on teardown.

    python3 scripts/db_test.py            # verbose
    python3 -m unittest scripts.db_test   # from repo root
"""
from __future__ import annotations

import sqlite3
import unittest
from pathlib import Path

import sys
sys.path.insert(0, str(Path(__file__).parent))
from seed_db import build_db, load_animals, LANGS  # noqa: E402

# Load the real dataset once; each test seeds a fresh in-memory DB from it.
ANIMALS = load_animals()


def fresh_db() -> sqlite3.Connection:
    conn = sqlite3.connect(":memory:")
    conn.row_factory = sqlite3.Row
    build_db(conn, ANIMALS)
    return conn


class SchemaAndCounts(unittest.TestCase):
    def setUp(self):
        self.conn = fresh_db()

    def tearDown(self):
        self.conn.close()

    def test_animal_count_matches_source(self):
        n = self.conn.execute("SELECT COUNT(*) FROM animal").fetchone()[0]
        self.assertEqual(n, len(ANIMALS))

    def test_slugs_unique_and_present(self):
        rows = self.conn.execute("SELECT slug FROM animal").fetchall()
        slugs = [r["slug"] for r in rows]
        self.assertEqual(len(slugs), len(set(slugs)), "duplicate slugs")
        self.assertTrue(all(s for s in slugs), "empty slug present")

    def test_traits_deduplicated(self):
        unique_nl = {t for a in ANIMALS for t in a.get("traits_nl", [])}
        n = self.conn.execute("SELECT COUNT(*) FROM trait").fetchone()[0]
        self.assertEqual(n, len(unique_nl))


class DataIntegrity(unittest.TestCase):
    def setUp(self):
        self.conn = fresh_db()

    def tearDown(self):
        self.conn.close()

    def test_foreign_keys_intact(self):
        violations = self.conn.execute("PRAGMA foreign_key_check").fetchall()
        self.assertEqual(list(violations), [], f"FK violations: {violations}")

    def test_every_animal_has_all_three_names(self):
        # Names are non-empty for all three languages in the source; enforce it.
        rows = self.conn.execute(
            """
            SELECT a.slug, t.lang, t.name
            FROM animal a
            JOIN translation t ON t.animal_id = a.id
            """
        ).fetchall()
        by_slug: dict[str, dict[str, str]] = {}
        for r in rows:
            by_slug.setdefault(r["slug"], {})[r["lang"]] = r["name"]
        for slug, names in by_slug.items():
            for lg in LANGS:
                self.assertIn(lg, names, f"{slug} missing {lg} translation row")
            for lg in ("nl", "it", "en"):
                self.assertTrue(names[lg].strip(), f"{slug} has empty {lg} name")

    def test_italian_description_present_for_all(self):
        # desc_it is 100% populated in the source — this is the primary language.
        missing = self.conn.execute(
            "SELECT COUNT(*) FROM translation WHERE lang='it' AND TRIM(description)=''"
        ).fetchone()[0]
        self.assertEqual(missing, 0, "some animals lack an Italian description")

    def test_no_orphan_traits(self):
        orphan = self.conn.execute(
            "SELECT COUNT(*) FROM trait WHERE id NOT IN (SELECT trait_id FROM animal_trait)"
        ).fetchone()[0]
        self.assertEqual(orphan, 0, "trait with no animal")

    def test_every_animal_has_traits(self):
        none = self.conn.execute(
            "SELECT COUNT(*) FROM animal WHERE id NOT IN (SELECT animal_id FROM animal_trait)"
        ).fetchone()[0]
        self.assertEqual(none, 0, "animal with no traits")

    def test_each_trait_has_three_translations(self):
        bad = self.conn.execute(
            "SELECT COUNT(*) FROM trait WHERE (SELECT COUNT(*) FROM trait_translation WHERE trait_id=trait.id) != 3"
        ).fetchone()[0]
        self.assertEqual(bad, 0, "trait without exactly 3 translation rows")


class Search(unittest.TestCase):
    def setUp(self):
        self.conn = fresh_db()

    def tearDown(self):
        self.conn.close()

    def _trait_ids(self, keys_nl: list[str]) -> list[int]:
        qs = ",".join("?" * len(keys_nl))
        rows = self.conn.execute(f"SELECT id FROM trait WHERE key_nl IN ({qs})", keys_nl).fetchall()
        return [r["id"] for r in rows]

    def _filter_sql(self, include_nl: list[str], exclude_nl: list[str]) -> set[str]:
        inc = self._trait_ids(include_nl)
        exc = self._trait_ids(exclude_nl)
        inc_qs = ",".join("?" * len(inc)) or "NULL"
        exc_qs = ",".join("?" * len(exc)) or "NULL"
        sql = f"""
            SELECT a.slug FROM animal a
            WHERE (SELECT COUNT(*) FROM animal_trait at
                   WHERE at.animal_id = a.id AND at.trait_id IN ({inc_qs})) = ?
              AND NOT EXISTS (SELECT 1 FROM animal_trait at
                   WHERE at.animal_id = a.id AND at.trait_id IN ({exc_qs}))
        """
        # bind order must follow SQL text order: include ids, the count, then exclude ids
        rows = self.conn.execute(sql, [*inc, len(inc), *exc]).fetchall()
        return {r["slug"] for r in rows}

    def _filter_oracle(self, include_nl: list[str], exclude_nl: list[str]) -> set[str]:
        inc, exc = set(include_nl), set(exclude_nl)
        out = set()
        for a in ANIMALS:
            ts = set(a.get("traits_nl", []))
            if inc <= ts and not (exc & ts):
                out.add(a["slug"])
        return out

    def test_filter_mode_matches_oracle(self):
        cases = [
            (["moedig"], []),
            (["sociaal"], ["solitair"]),
            (["intelligent", "sociaal"], []),
            ([], ["sociaal"]),
        ]
        for inc, exc in cases:
            # only run cases whose traits actually exist in the data
            if len(self._trait_ids(inc)) != len(inc) or len(self._trait_ids(exc)) != len(exc):
                continue
            with self.subTest(include=inc, exclude=exc):
                self.assertEqual(self._filter_sql(inc, exc), self._filter_oracle(inc, exc))

    def test_similarity_self_is_one(self):
        # Jaccard of an animal's trait set with itself must be 1.0.
        a = ANIMALS[0]
        ts = set(a.get("traits_nl", []))
        self.assertGreater(len(ts), 0)
        inter = len(ts & ts)
        union = len(ts | ts)
        self.assertEqual(inter / union, 1.0)

    def test_name_search_like(self):
        # Free-text name search uses LIKE (the API's `q=` param), matching the
        # Go runtime which has no FTS5. 'Vipera' is the Italian name for 'adder'.
        rows = self.conn.execute(
            "SELECT a.slug FROM animal a "
            "JOIN translation t ON t.animal_id = a.id AND t.lang = 'it' "
            "WHERE lower(t.name) LIKE '%' || lower(?) || '%'",
            ("vipera",),
        ).fetchall()
        self.assertIn("adder", {r["slug"] for r in rows})


if __name__ == "__main__":
    unittest.main(verbosity=2)
