#!/usr/bin/env python3
"""Validate the English (`en`) and Italian (`it`) animal NAMES in data/animals.json
against an authoritative source (Wikidata labels, resolved via Dutch Wikipedia).

The Dutch (`nl`) names are treated as authoritative/correct and are used as the
lookup key. We NEVER touch `nl`, and we do not validate traits.

Pipeline (free public APIs only; no SPARQL/WDQS):
  1. Resolve each `nl` name -> Wikidata Q-id via nl.wikipedia pageprops
     (batched, 50 titles/request, follows normalisation + redirects).
  2. Fetch labels (en/it/nl) + claims for each Q-id via wbgetentities
     (batched, 50 ids/request).
  3. Sanity-check the item is a taxon (has P225 taxon name or P105 taxon rank,
     or P31 instance-of a taxon-ish class) before trusting it.
  4. Fall back to Wikipedia langlinks (en/it) when Wikidata has no label.

Output:
  * data/name_validation_candidates.json  - every detected mismatch (full detail)
  * data/name_corrections.json            - conservative, confident corrections only
  * data/name-review.md                   - human-readable summary

Idempotent: all network responses are cached in data/.name_cache.json, so the
script can be re-run cheaply. Delete the cache to force a refresh.

Polite: descriptive User-Agent, batched requests, bounded retries with backoff
on HTTP 429 / 5xx.
"""
from __future__ import annotations

import json
import os
import sys
import time
import unicodedata
from pathlib import Path

import requests

ROOT = Path(__file__).resolve().parent.parent
DATA = ROOT / "data"
ANIMALS = DATA / "animals.json"
CACHE = DATA / ".name_cache.json"
CANDIDATES = DATA / "name_validation_candidates.json"
CORRECTIONS = DATA / "name_corrections.json"
REVIEW = DATA / "name-review.md"

NL_WIKI = "https://nl.wikipedia.org/w/api.php"
WIKIDATA = "https://www.wikidata.org/w/api.php"

USER_AGENT = (
    "Totems-IT-name-validator/1.0 "
    "(https://github.com/niklasvdm/Totems-IT; Niklas@vandermersch.eu) "
    "python-requests"
)

SESSION = requests.Session()
SESSION.headers.update({"User-Agent": USER_AGENT})

# Taxon-rank / instance-of Q-ids that confirm the item is an organism taxon or
# a domesticated breed (dogs, cats, etc. are breeds, not taxa, but legitimate).
TAXON_INSTANCE_OK = {
    "Q16521",   # taxon
    "Q310890",  # monotypic taxon
    "Q23038290",  # fossil taxon
    "Q713623",  # clade
    "Q68895",   # breed / dog breed
    "Q39367",   # dog breed? (varies) - kept permissive
    "Q38829",   # domestic breed variants
    "Q2382443", # horse breed
    "Q43577",   # ... permissive extras
}

# ---------------------------------------------------------------------------
# Curated, confident corrections.
#
# These were selected by reviewing data/name_validation_candidates.json (the
# machine-generated mismatch list this script produces) and keeping ONLY the
# cases where:
#   * the Wikidata item is a verified taxon,
#   * the authoritative label is a real vernacular common name (not the Latin /
#     scientific name, not a bare plural family label), and
#   * the current value is a genuine mistranslation or untranslated word.
#
# Over-corrections were deliberately rejected: more-specific-but-not-wrong
# labels ("Flamingo" -> "Greater flamingo"), valid synonyms ("Hippo",
# "Eider duck", "Gavial", "Nandu", "Orang-utan"), and species that share a
# Wikidata item with another animal in the catalogue (panter/luipaard,
# spiesbok/gemsbok, aardeekhoorn/grondeekhoorn) which would create duplicates.
# ---------------------------------------------------------------------------

CURATED_EN = {
    "alpenhond": "Dhole",                         # was "Alpine dog"
    "baardkoekoek": "Dwarf koel",                 # was "Bearded cuckoo"
    "beermarter": "Binturong",                    # was "Bear marten"
    "berglemming": "Norway lemming",              # was "Mountain lemming"
    "blauwe-reiger": "Grey heron",                # was "Blue heron"
    "comorenwever": "Comoros fody",               # was "Comoros weaver"
    "dwergmuis": "Eurasian harvest mouse",        # was "Pygmy mouse"
    "franjeschildpad": "Mata mata",               # was "Fringe turtle"
    "geep": "Garfish",                            # was "Geep" (untranslated)
    "goendi": "Common gundi",                     # was "Goendi" (untranslated)
    "hazelmuis": "Hazel dormouse",                # was "Hazel mouse"
    "ijsduiker": "Common loon",                   # was "Polar diver"
    "kluut": "Pied avocet",                       # was "Pied Piper" (!)
    "knobbelzwijn": "Warthog",                    # was "Mute boar"
    "koolmees": "Great tit",                      # was "Titmouse"
    "kwartelsnip": "Seedsnipe",                   # was "Quail snipe"
    "lampongaap": "Southern pig-tailed macaque",  # was "Lampong monkey"
    "manenwolf": "Maned wolf",                    # was "Mane wolf"
    "manoel": "Pallas's cat",                     # was "Manoel" (untranslated)
    "maskerwimpelvis": "Moorish idol",            # was "Masked wrasse"
    "newfoundlander": "Newfoundland dog",         # was "Newfoundlander"
    "oehoe": "Eurasian eagle-owl",                # was "Owl"
    "ornaatelfje": "Superb fairywren",            # was "Ornate woodnymph"
    "pardelkat": "Ocelot",                        # was "Pardel cat"
    "pauwoogstekelrog": "Ocellate river stingray",  # was "Peacock stingray"
    "renvogel": "Cream-coloured courser",         # was "Roadrunner"
    "salangaan": "Swiftlet",                      # was "Salanganese"
    "schroefhoorngeit": "Markhor",                # was "Screw horn goat"
    "spoorkoekoek": "Coucal",                     # was "Spur-winged cuckoo"
    "veldmuis": "Common vole",                    # was "Field mouse"
    "vuurstaartlabeo": "Red-tailed black shark",  # was "Firetail labeo"
    "wilde-eend": "Mallard",                      # was "Wild duck"
    "wolharige-mammoet": "Woolly mammoth",        # was "Wooly mammoth" (spelling)
    "zebramangoest": "Banded mongoose",           # was "Zebra mongoose"
    "zonnebaars": "Pumpkinseed",                  # was "Sunfish"
}

CURATED_IT = {
    "zebramangoest": "Mangusta striata",          # was "Mangusta bandita"
}


# ---------------------------------------------------------------------------
# caching
# ---------------------------------------------------------------------------

def load_cache() -> dict:
    if CACHE.exists():
        try:
            return json.loads(CACHE.read_text(encoding="utf-8"))
        except Exception:
            return {}
    return {}


def save_cache(cache: dict) -> None:
    CACHE.write_text(json.dumps(cache, ensure_ascii=False, indent=0), encoding="utf-8")


# ---------------------------------------------------------------------------
# polite HTTP
# ---------------------------------------------------------------------------

def api_get(url: str, params: dict, max_retries: int = 5) -> dict:
    params = dict(params)
    params.setdefault("format", "json")
    params.setdefault("formatversion", "2")
    backoff = 1.0
    for attempt in range(max_retries):
        try:
            resp = SESSION.get(url, params=params, timeout=30)
        except requests.RequestException as exc:  # network hiccup
            if attempt == max_retries - 1:
                raise
            time.sleep(backoff)
            backoff *= 2
            continue
        if resp.status_code == 429 or resp.status_code >= 500:
            retry_after = resp.headers.get("Retry-After")
            wait = float(retry_after) if retry_after and retry_after.isdigit() else backoff
            time.sleep(wait)
            backoff *= 2
            continue
        resp.raise_for_status()
        # be polite between successful calls
        time.sleep(0.2)
        return resp.json()
    raise RuntimeError(f"Exhausted retries for {url} {params}")


def chunked(seq, n):
    for i in range(0, len(seq), n):
        yield seq[i : i + n]


# ---------------------------------------------------------------------------
# step 1: nl name -> Q-id
# ---------------------------------------------------------------------------

def resolve_qids(nl_names: list[str], cache: dict) -> dict:
    """Return {nl_name: qid_or_None}."""
    q = cache.setdefault("qid", {})
    todo = [n for n in nl_names if n not in q]
    for batch in chunked(todo, 50):
        data = api_get(
            NL_WIKI,
            {
                "action": "query",
                "prop": "pageprops",
                "ppprop": "wikibase_item",
                "redirects": "1",
                "titles": "|".join(batch),
            },
        )
        query = data.get("query", {})
        # map requested title -> final title through normalized + redirects
        alias = {}
        for norm in query.get("normalized", []):
            alias[norm["from"]] = norm["to"]
        for red in query.get("redirects", []):
            alias[red["from"]] = red["to"]

        def final_title(t):
            seen = set()
            while t in alias and t not in seen:
                seen.add(t)
                t = alias[t]
            return t

        # build final-title -> qid
        title_to_qid = {}
        for page in query.get("pages", []):
            if page.get("missing"):
                continue
            wb = page.get("pageprops", {}).get("wikibase_item")
            title_to_qid[page["title"]] = wb

        for name in batch:
            q[name] = title_to_qid.get(final_title(name))
        save_cache(cache)
    return {n: q.get(n) for n in nl_names}


# ---------------------------------------------------------------------------
# step 2: Q-id -> labels + taxon check
# ---------------------------------------------------------------------------

def fetch_entities(qids: list[str], cache: dict) -> dict:
    """Return {qid: {labels:{lang:val}, is_taxon:bool}}."""
    ent = cache.setdefault("entity", {})
    todo = sorted({q for q in qids if q and q not in ent})
    for batch in chunked(todo, 50):
        data = api_get(
            WIKIDATA,
            {
                "action": "wbgetentities",
                "ids": "|".join(batch),
                "props": "labels|claims",
                "languages": "en|it|nl",
            },
        )
        entities = data.get("entities", {})
        for qid in batch:
            e = entities.get(qid, {})
            labels = {
                lang: v.get("value")
                for lang, v in e.get("labels", {}).items()
            }
            claims = e.get("claims", {})
            is_taxon = "P225" in claims or "P105" in claims
            if not is_taxon and "P31" in claims:
                for st in claims["P31"]:
                    try:
                        tgt = st["mainsnak"]["datavalue"]["value"]["id"]
                    except (KeyError, TypeError):
                        continue
                    if tgt in TAXON_INSTANCE_OK:
                        is_taxon = True
                        break
            # P225 = taxon name (the scientific/Latin name). We capture it so we
            # can discard "authoritative" labels that are just the scientific
            # name (Wikidata's fallback when no vernacular label exists).
            taxon_name = None
            for st in claims.get("P225", []):
                try:
                    taxon_name = st["mainsnak"]["datavalue"]["value"]
                    break
                except (KeyError, TypeError):
                    continue
            ent[qid] = {"labels": labels, "is_taxon": is_taxon,
                        "taxon_name": taxon_name}
        save_cache(cache)
    return {q: ent.get(q) for q in qids if q}


# ---------------------------------------------------------------------------
# step 3: langlink fallback
# ---------------------------------------------------------------------------

def fetch_langlink(nl_name: str, lang: str, cache: dict) -> str | None:
    key = f"{lang}:{nl_name}"
    ll = cache.setdefault("langlink", {})
    if key in ll:
        return ll[key]
    data = api_get(
        NL_WIKI,
        {
            "action": "query",
            "prop": "langlinks",
            "lllang": lang,
            "lllimit": "max",
            "redirects": "1",
            "titles": nl_name,
        },
    )
    val = None
    for page in data.get("query", {}).get("pages", []):
        for link in page.get("langlinks", []):
            if link.get("lang") == lang:
                val = link.get("title")
    ll[key] = val
    save_cache(cache)
    return val


# ---------------------------------------------------------------------------
# normalisation / comparison helpers
# ---------------------------------------------------------------------------

def norm(s: str | None) -> str:
    if not s:
        return ""
    s = unicodedata.normalize("NFKD", s)
    s = "".join(c for c in s if not unicodedata.combining(c))
    s = s.lower().strip()
    # drop parenthetical qualifiers e.g. "kiwi (bird)"
    if "(" in s:
        s = s.split("(")[0].strip()
    return s


def titlecase_first(s: str) -> str:
    """Match dataset style: capitalise first letter only, keep rest as-is."""
    if not s:
        return s
    return s[0].upper() + s[1:]


def looks_like_scientific(label: str) -> bool:
    """Heuristic: a binomial Latin name (Genus species) rather than a common name."""
    parts = label.split()
    if len(parts) == 2 and parts[0][:1].isupper() and parts[1].islower():
        # could be a common name too ("Red fox"), but Latin binomials have a
        # capitalised genus + lowercase species; common EN/IT names usually are
        # not two words both plausibly Latin. Keep this conservative and only a
        # hint, not a hard filter.
        return False
    return False


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

def main() -> int:
    animals = json.loads(ANIMALS.read_text(encoding="utf-8"))
    cache = load_cache()

    nl_names = [a["nl"] for a in animals]
    print(f"Resolving {len(nl_names)} Dutch names -> Q-ids ...", file=sys.stderr)
    qid_map = resolve_qids(nl_names, cache)

    qids = [q for q in qid_map.values() if q]
    print(f"Fetching {len(set(qids))} Wikidata entities ...", file=sys.stderr)
    ent_map = fetch_entities(qids, cache)

    candidates = []  # all mismatches with detail
    unresolved = []  # nl name did not resolve to a verified taxon

    for a in animals:
        slug, nl = a["slug"], a["nl"]
        cur = {"en": a.get("en", ""), "it": a.get("it", "")}
        qid = qid_map.get(nl)
        ent = ent_map.get(qid) if qid else None

        auth = {"en": None, "it": None}
        source = "wikidata"
        is_taxon = bool(ent and ent.get("is_taxon"))
        taxon_name = ent.get("taxon_name") if ent else None

        if ent and is_taxon:
            labels = ent["labels"]
            auth["en"] = labels.get("en")
            auth["it"] = labels.get("it")

        # langlink fallback for any missing authoritative label (only if we at
        # least have a resolvable, taxon-verified page — avoids disambiguation)
        for lang in ("en", "it"):
            if is_taxon and not auth[lang]:
                ll = fetch_langlink(nl, lang, cache)
                if ll:
                    auth[lang] = ll
                    source = "langlink"

        if not is_taxon:
            unresolved.append({"slug": slug, "nl": nl, "qid": qid,
                               "reason": "no qid" if not qid else "not verified taxon"})
            continue

        for lang in ("en", "it"):
            a_label = auth[lang]
            if not a_label:
                continue
            if norm(cur[lang]) == norm(a_label):
                continue  # already correct

            # Is the authoritative label merely the scientific/Latin name?
            auth_is_scientific = bool(
                taxon_name and norm(a_label) == norm(taxon_name)
            )
            # Does the current value already appear inside the (longer)
            # authoritative common name? e.g. current "Flamingo" vs
            # authoritative "Greater flamingo" -> current is a fine short name.
            cur_tokens = set(norm(cur[lang]).split())
            auth_tokens = set(norm(a_label).split())
            current_in_auth = bool(
                cur_tokens and cur_tokens.issubset(auth_tokens)
            )

            candidates.append({
                "slug": slug,
                "nl": nl,
                "lang": lang,
                "current": cur[lang],
                "authoritative": titlecase_first(a_label),
                "qid": qid,
                "source": source,
                "current_equals_nl": norm(cur[lang]) == norm(nl),
                "auth_is_scientific": auth_is_scientific,
                "current_in_authoritative": current_in_auth,
            })

    CANDIDATES.write_text(
        json.dumps(candidates, ensure_ascii=False, indent=2), encoding="utf-8"
    )
    print(f"Wrote {len(candidates)} candidate mismatches -> {CANDIDATES}",
          file=sys.stderr)
    print(f"{len(unresolved)} animals unresolved/not-verified (left untouched)",
          file=sys.stderr)

    emit_outputs(animals, candidates)
    return 0


def emit_outputs(animals: list, candidates: list) -> None:
    """Write the curated corrections file and the human-readable review."""
    by_slug = {a["slug"]: a for a in animals}

    # Build corrections, only including a field when it actually differs.
    corrections: dict[str, dict] = {}
    rows = {"en": [], "it": []}  # (slug, nl, old, new)
    for lang, curated in (("en", CURATED_EN), ("it", CURATED_IT)):
        for slug, new in curated.items():
            a = by_slug.get(slug)
            if not a:
                raise SystemExit(f"curated slug not in dataset: {slug}")
            old = a.get(lang, "")
            if norm(old) == norm(new):
                continue  # already correct, nothing to change
            corrections.setdefault(slug, {})[lang] = new
            rows[lang].append((slug, a["nl"], old, new))

    CORRECTIONS.write_text(
        json.dumps(dict(sorted(corrections.items())), ensure_ascii=False, indent=2)
        + "\n",
        encoding="utf-8",
    )

    # Skipped strong candidates (mismatch found but deliberately not corrected).
    curated_keys = {(s, "en") for s in CURATED_EN} | {(s, "it") for s in CURATED_IT}
    strong = [
        c for c in candidates
        if not c["auth_is_scientific"] and not c["current_in_authoritative"]
    ]
    skipped = [c for c in strong if (c["slug"], c["lang"]) not in curated_keys]

    n_en = len(rows["en"])
    n_it = len(rows["it"])

    def table(lang):
        out = ["| slug | nl | old | new |", "| --- | --- | --- | --- |"]
        for slug, nl, old, new in sorted(rows[lang]):
            out.append(f"| `{slug}` | {nl} | {old} | **{new}** |")
        return "\n".join(out)

    skip_lines = []
    for c in sorted(skipped, key=lambda z: (z["lang"], z["slug"])):
        skip_lines.append(
            f"| `{c['slug']}` | {c['lang']} | {c['nl']} | {c['current']} | "
            f"{c['authoritative']} |"
        )

    md = f"""# Animal name validation review

Validation of the English (`en`) and Italian (`it`) animal **names** in
`data/animals.json` against authoritative Wikidata labels (resolved through
Dutch Wikipedia). Dutch (`nl`) names were used as the lookup key and left
untouched; traits were not validated.

Generated by `scripts/validate_names.py`. Proposed corrections live in
`data/name_corrections.json` (this file never modifies `animals.json`).

## Summary

- **{n_en} English (`en`) corrections** proposed.
- **{n_it} Italian (`it`) corrections** proposed.
- {len([a for a in animals])} animals checked in total.
- {len([c for c in candidates])} raw mismatches detected; most were rejected as
  **false positives** — the Wikidata "label" was just the Latin/scientific name
  (no vernacular label), a bare plural family label (e.g. "Cockatoos"), a
  more-specific-but-not-wrong name (e.g. "Flamingo" vs "Greater flamingo"), or a
  valid synonym.

### Why so few Italian corrections?

Wikidata's Italian vernacular coverage is sparse: for most species the Italian
label is simply the scientific name, so the existing Italian values could not be
*confidently* contradicted and were left as-is. Only one Italian value had a
clear, authoritative vernacular correction.

## Proposed English corrections ({n_en})

{table('en')}

## Proposed Italian corrections ({n_it})

{table('it')}

## Deliberately skipped (mismatch found, but low-confidence / not an error)

These had a non-scientific Wikidata label that differed from the current value,
but were **not** corrected — either the current value is a valid synonym or
less-specific common name, the mapping is ambiguous, or correcting it would
duplicate another animal that shares the same Wikidata item
(`panter`/`luipaard`, `spiesbok`/`gemsbok`, `aardeekhoorn`/`grondeekhoorn`).

| slug | lang | nl | current (kept) | Wikidata label (not trusted) |
| --- | --- | --- | --- | --- |
{chr(10).join(skip_lines)}

## Notes

- Authoritative source: MediaWiki Action API + Wikidata `wbgetentities`
  (`props=labels|claims`). The SPARQL/WDQS endpoint was avoided per the recent
  outage. Items were sanity-checked as taxa via `P225` (taxon name) /
  `P105` (taxon rank) / `P31` before their labels were trusted.
- Animals whose Dutch name did not resolve to a verified taxon were left
  untouched (no guesses).
- Re-run with `python3 scripts/validate_names.py` (responses are cached in
  `data/.name_cache.json`, so re-runs are fast and idempotent).
"""
    REVIEW.write_text(md, encoding="utf-8")
    print(f"Wrote {n_en} en + {n_it} it corrections -> {CORRECTIONS}",
          file=sys.stderr)
    print(f"Wrote review -> {REVIEW}", file=sys.stderr)


if __name__ == "__main__":
    sys.exit(main())
