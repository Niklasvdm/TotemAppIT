#!/usr/bin/env python3
"""
Fetch ONE freely-licensed photo per animal species for the totem finder.

For each animal (keyed by `slug`) it tries, in order:
  1. Wikidata image (P18) resolved via the Dutch Wikipedia article for `nl`,
     then the Wikimedia Commons file + its licence/author (extmetadata).
  2. Wikipedia REST summary (nl, then en) -> originalimage/thumbnail.
  3. iNaturalist taxa search -> default_photo (free licences only).

Only free licences are accepted: public domain / CC0 / CC-BY / CC-BY-SA.
Anything non-free or unknown-licence is SKIPPED and recorded for manual sourcing.

Usage:
    python3 scripts/scrape_images.py                 # full run (all slugs)
    python3 scripts/scrape_images.py --sample         # 12-animal sanity sample
    python3 scripts/scrape_images.py --slugs wolf,vos # specific slugs
    python3 scripts/scrape_images.py --limit 20       # first N missing

Reads:  data/animals.json
Writes: data/images/<slug>.webp          (max ~600px long edge, WebP)
        data/images/attributions.json    {slug: {author, license, source_url}}
        data/images/review.md            low-confidence / failed matches

Idempotent: skips any slug that already has data/images/<slug>.webp.
"""

import argparse
import io
import json
import re
import sys
import threading
import time
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).parent.parent
DATA_FILE = ROOT / "data" / "animals.json"
IMAGES_DIR = ROOT / "data" / "images"
ATTR_FILE = IMAGES_DIR / "attributions.json"
REVIEW_FILE = IMAGES_DIR / "review.md"

MAX_EDGE = 600
MAX_WORKERS = 4  # polite concurrency across several public APIs (they rate-limit hard)

HEADERS = {
    "User-Agent": (
        "TotemsIT-image-scraper/1.0 (https://github.com/niklasvdm/Totems-IT; "
        "Scout totem finder; contact Niklas@vandermersch.eu) Python-urllib"
    ),
    "Accept": "application/json,*/*;q=0.8",
    "Accept-Language": "nl;q=0.9,en;q=0.8",
}

# Accepted free licences. Match is substring/regex against licence text/shortnames
# AND the LicenseUrl (e.g. creativecommons.org/licenses/by-sa/4.0), which is the
# most reliable signal.
FREE_LICENSE_PATTERNS = [
    r"\bcc0\b",
    r"publicdomain/zero",
    r"public domain",
    r"\bpd\b",
    r"cc[\s\-]?by([\s\-]?sa)?",           # CC-BY, CC-BY-SA
    r"licenses/by(-sa)?/",                  # CC licence URL
    r"creative commons attribution",
    r"\battribution\b",                     # Commons shortname "Attribution" == CC-BY
]
# Explicitly non-free markers (NC = non-commercial, ND = no-derivs we still accept? -> ND is fine
# for display but we keep it simple: reject NC. ND is acceptable, we only forbid NC and all-rights).
NONFREE_PATTERNS = [
    r"non[\s\-]?commercial",
    r"[\s\-]nc[\s\-]",
    r"cc[\s\-]?by[\s\-]?nc",
    r"all rights reserved",
    r"copyright",
    r"fair use",
    r"\bgfdl only\b",
]

# iNaturalist license_code -> human label (only free ones listed; others rejected)
INAT_FREE = {
    "cc0": "CC0",
    "cc-by": "CC-BY",
    "cc-by-sa": "CC-BY-SA",
}

_print_lock = threading.Lock()


def log(msg):
    with _print_lock:
        print(msg, flush=True)


def http_get(url, timeout=25):
    req = urllib.request.Request(url, headers=HEADERS)
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return resp.read()


def http_json(url, timeout=25, retries=3):
    for attempt in range(retries + 1):
        try:
            return json.loads(http_get(url, timeout=timeout).decode("utf-8", "replace"))
        except urllib.error.HTTPError as e:
            if e.code in (429, 503) and attempt < retries:
                time.sleep(5 + attempt * 5)
                continue
            raise
        except Exception:
            if attempt < retries:
                time.sleep(1.5 + attempt * 1.5)
                continue
            raise


def classify_license(*texts):
    """Return (is_free, normalized_label) from arbitrary licence text fragments."""
    blob = " ".join(t for t in texts if t).lower()
    if not blob.strip():
        return False, "unknown"
    for pat in NONFREE_PATTERNS:
        if re.search(pat, blob):
            return False, "non-free"
    for pat in FREE_LICENSE_PATTERNS:
        if re.search(pat, blob):
            # Produce a tidy label
            if re.search(r"cc0|publicdomain/zero", blob):
                return True, "CC0"
            if re.search(r"public domain|\bpd\b", blob):
                return True, "Public Domain"
            if re.search(r"cc[\s\-]?by[\s\-]?sa|licenses/by-sa/|attribution.?share", blob):
                return True, "CC-BY-SA"
            return True, "CC-BY"
    return False, "unknown"


# ---------------------------------------------------------------------------
# Source 1: Wikidata P18 via Dutch Wikipedia -> Commons imageinfo
# ---------------------------------------------------------------------------

def wiki_page_to_qid(lang, title):
    """Resolve a Wikipedia article title to a Wikidata QID.

    Returns (qid, resolved_title, is_disambiguation).
    """
    url = (
        f"https://{lang}.wikipedia.org/w/api.php?action=query&redirects=1&format=json"
        f"&prop=pageprops&titles={urllib.parse.quote(title)}"
    )
    data = http_json(url)
    pages = data.get("query", {}).get("pages", {})
    for _, page in pages.items():
        if "missing" in page:
            continue
        pp = page.get("pageprops", {})
        qid = pp.get("wikibase_item")
        is_disambig = "disambiguation" in pp
        if qid:
            return qid, page.get("title", title), is_disambig
    return None, None, False


def wikidata_entity(qid):
    url = f"https://www.wikidata.org/wiki/Special:EntityData/{qid}.json"
    data = http_json(url)
    return data.get("entities", {}).get(qid, {})


def is_taxon_entity(entity):
    """Confidence signal: does the Wikidata item look like an organism/taxon?"""
    claims = entity.get("claims", {})
    # P225 = taxon name, P105 = taxon rank, P31 instance-of taxon(Q16521)
    if "P225" in claims or "P105" in claims:
        return True
    for st in claims.get("P31", []):
        try:
            qid = st["mainsnak"]["datavalue"]["value"]["id"]
            if qid in ("Q16521", "Q713623", "Q310890", "Q23038290"):  # taxon, clade, monotypic, fossil taxon
                return True
        except (KeyError, TypeError):
            continue
    return False


def wikidata_p18(entity):
    claims = entity.get("claims", {})
    for st in claims.get("P18", []):
        try:
            return st["mainsnak"]["datavalue"]["value"]  # a Commons filename
        except (KeyError, TypeError):
            continue
    return None


def commons_imageinfo(filename):
    """Fetch URL + extmetadata (licence/author) for a Commons file title."""
    title = "File:" + filename
    url = (
        "https://commons.wikimedia.org/w/api.php?action=query&format=json&prop=imageinfo"
        "&iiprop=url|extmetadata&iiurlwidth=800&titles=" + urllib.parse.quote(title)
    )
    data = http_json(url)
    pages = data.get("query", {}).get("pages", {})
    for _, page in pages.items():
        infos = page.get("imageinfo")
        if not infos:
            continue
        info = infos[0]
        ext = info.get("extmetadata", {})
        license_short = ext.get("LicenseShortName", {}).get("value", "")
        license_name = ext.get("License", {}).get("value", "")
        license_url = ext.get("LicenseUrl", {}).get("value", "")
        usage_terms = ext.get("UsageTerms", {}).get("value", "")
        artist_html = ext.get("Artist", {}).get("value", "")
        artist = re.sub(r"<[^>]+>", "", artist_html).strip()
        artist = re.sub(r"\s+", " ", artist)
        img_url = info.get("thumburl") or info.get("url")
        desc_url = info.get("descriptionurl") or (
            "https://commons.wikimedia.org/wiki/" + urllib.parse.quote(title)
        )
        return {
            "url": img_url,
            "license_text": " ".join([license_short, license_name, license_url, usage_terms]),
            "author": artist or "Unknown",
            "source_url": desc_url,
        }
    return None


def try_wikidata(animal):
    """Resolve a species image via Wikidata P18.

    Tries the Dutch Wikipedia article for `nl` first, then the English article
    for `en`. Disambiguation pages (e.g. "Vos", "Kat", "Uil") carry no useful
    P18, so we fall through to the other language, which usually points at the
    correct single concept/species. Returns (result_dict, reason).
    """
    candidates = []  # (lang, title) to try, in priority order
    nl = animal.get("nl", "").strip()
    en = animal.get("en", "").strip()
    if nl:
        candidates.append(("nl", nl))
    if en:
        candidates.append(("en", en))
    if not candidates:
        return None, "no nl/en name"

    reasons = []
    best = None  # prefer a taxon hit over a non-taxon concept hit
    for lang, title in candidates:
        try:
            qid, resolved, is_disambig = wiki_page_to_qid(lang, title)
        except Exception as e:
            reasons.append(f"{lang} resolve error: {e}")
            continue
        if not qid:
            reasons.append(f"no {lang} article")
            continue
        if is_disambig:
            reasons.append(f"{lang}:{resolved} is disambiguation")
            continue
        try:
            entity = wikidata_entity(qid)
        except Exception as e:
            reasons.append(f"{lang} entity error: {e}")
            continue
        taxon = is_taxon_entity(entity)
        filename = wikidata_p18(entity)
        if not filename:
            reasons.append(f"{lang}:{resolved} no P18 (taxon={taxon})")
            continue
        try:
            info = commons_imageinfo(filename)
        except Exception as e:
            reasons.append(f"commons error: {e}")
            continue
        if not info or not info.get("url"):
            reasons.append(f"commons fetch failed for {filename}")
            continue
        is_free, lic = classify_license(info["license_text"])
        if not is_free:
            reasons.append(f"{lang} non-free '{info['license_text'].strip()}'")
            continue
        result = {
            "url": info["url"],
            "author": info["author"],
            "license": lic,
            "source_url": info["source_url"],
            "source": f"wikidata-p18-{lang}",
            "qid": qid,
            "taxon": taxon,
            "resolved": f"{lang}:{resolved}",
        }
        if taxon:
            return result, None  # high-confidence: stop immediately
        if best is None:
            best = result  # keep concept-article hit, but let a later taxon win
    if best:
        return best, None
    return None, "; ".join(reasons) if reasons else "no wikidata image"


# ---------------------------------------------------------------------------
# Source 2: Wikipedia REST summary
# ---------------------------------------------------------------------------

def try_wikipedia_summary(animal):
    for lang, key in (("nl", "nl"), ("en", "en")):
        name = animal.get(key, "").strip()
        if not name:
            continue
        url = (
            f"https://{lang}.wikipedia.org/api/rest_v1/page/summary/"
            + urllib.parse.quote(name.replace(" ", "_"))
        )
        try:
            data = http_json(url)
        except Exception:
            continue
        dtype = data.get("type", "")
        if dtype.endswith("not_found") or dtype == "disambiguation":
            continue
        img = data.get("originalimage") or data.get("thumbnail")
        if not img or not img.get("source"):
            continue
        # REST summaries don't carry licence; Wikipedia lead images are on Commons
        # but licence is unknown here -> treat as low confidence, verify via Commons name.
        # We only accept if we can confirm a free licence from the file page.
        src = img["source"]
        m = re.search(r"/commons/(?:thumb/)?[0-9a-f]/[0-9a-f]{2}/([^/]+)", src)
        if m:
            fname = urllib.parse.unquote(m.group(1))
            info = commons_imageinfo(fname)
            if info and info.get("url"):
                is_free, label = classify_license(info["license_text"])
                if is_free:
                    return {
                        "url": info["url"],
                        "author": info["author"],
                        "license": label,
                        "source_url": info["source_url"],
                        "source": f"wikipedia-summary-{lang}",
                        "taxon": None,
                        "resolved": f"{lang}:{data.get('title', name)}",
                    }, None
                return None, f"summary image non-free '{info['license_text'].strip()}'"
    return None, "no wikipedia summary image"


# ---------------------------------------------------------------------------
# Source 3: iNaturalist
# ---------------------------------------------------------------------------

ANIMAL_ICONIC = {
    "Animalia", "Mammalia", "Aves", "Reptilia", "Amphibia", "Actinopterygii",
    "Insecta", "Arachnida", "Mollusca", "Chromista",
}


def try_inaturalist(animal):
    """Last resort. iNaturalist's `q=` is a FUZZY search that happily returns the
    wrong taxon (e.g. "vos" -> "vosy" -> wasps), so we only accept a hit whose
    matched common name EXACTLY equals one of our names and whose iconic taxon is
    an animal. Even then the result is treated as low confidence by the caller.
    """
    names = {animal.get(k, "").strip().lower() for k in ("nl", "en", "it")}
    names.discard("")
    for key in ("en", "nl", "it"):
        name = animal.get(key, "").strip()
        if not name:
            continue
        url = (
            "https://api.inaturalist.org/v1/taxa?per_page=8&all_names=true&q="
            + urllib.parse.quote(name)
        )
        try:
            data = http_json(url)
        except Exception:
            continue
        for taxon in data.get("results", []):
            if taxon.get("iconic_taxon_name") not in ANIMAL_ICONIC:
                continue
            # Require an exact (case-insensitive) common-name/matched-term match
            # against one of our names to guard against fuzzy mismatches.
            cand = {
                (taxon.get("preferred_common_name") or "").lower(),
                (taxon.get("matched_term") or "").lower(),
                (taxon.get("name") or "").lower(),
            }
            if not (cand & names):
                continue
            photo = taxon.get("default_photo")
            if not photo:
                continue
            lic = (photo.get("license_code") or "").lower()
            if lic not in INAT_FREE:
                continue
            img_url = photo.get("medium_url") or photo.get("url")
            if img_url:
                img_url = img_url.replace("/square.", "/medium.").replace("/small.", "/medium.")
            attribution = photo.get("attribution") or "iNaturalist contributor"
            author = re.sub(r"\(c\)\s*", "", attribution)
            author = re.split(r",| some rights| all rights| no rights", author)[0].strip()
            return {
                "url": img_url,
                "author": author or "iNaturalist contributor",
                "license": INAT_FREE[lic],
                "source_url": f"https://www.inaturalist.org/taxa/{taxon.get('id')}",
                "source": "inaturalist",
                "taxon": False,  # treat as low-confidence — flag for human review
                "resolved": f"inat:{taxon.get('name')} ({key}={name})",
            }, None
    return None, "no exact-match free iNaturalist photo"


# ---------------------------------------------------------------------------
# Download + convert
# ---------------------------------------------------------------------------

def download_and_save(url, dest):
    raw = http_get(url, timeout=40)
    img = Image.open(io.BytesIO(raw))
    img = img.convert("RGB")
    w, h = img.size
    if max(w, h) > MAX_EDGE:
        if w >= h:
            nw, nh = MAX_EDGE, round(h * MAX_EDGE / w)
        else:
            nw, nh = round(w * MAX_EDGE / h), MAX_EDGE
        img = img.resize((nw, nh), Image.LANCZOS)
    img.save(dest, "WEBP", quality=82, method=6)


# ---------------------------------------------------------------------------
# Orchestration
# ---------------------------------------------------------------------------

def process(animal):
    """Returns a dict describing the outcome for this slug."""
    slug = animal["slug"]
    dest = IMAGES_DIR / f"{slug}.webp"
    if dest.exists():
        return {"slug": slug, "status": "skip-exists"}

    reasons = []
    result = None
    for fn in (try_wikidata, try_wikipedia_summary, try_inaturalist):
        try:
            result, reason = fn(animal)
        except Exception as e:
            result, reason = None, f"{fn.__name__} error: {e}"
        if result:
            break
        reasons.append(reason)
        time.sleep(0.2)

    if not result:
        return {
            "slug": slug,
            "status": "flagged",
            "nl": animal.get("nl", ""),
            "reasons": reasons,
        }

    try:
        download_and_save(result["url"], dest)
    except Exception as e:
        return {
            "slug": slug,
            "status": "flagged",
            "nl": animal.get("nl", ""),
            "reasons": reasons + [f"download/convert failed: {e}"],
        }

    # Confidence: low if not confirmed as a taxon/animal via Wikidata
    low_conf = result.get("taxon") is not True
    return {
        "slug": slug,
        "status": "ok",
        "nl": animal.get("nl", ""),
        "attribution": {
            "author": result["author"],
            "license": result["license"],
            "source_url": result["source_url"],
        },
        "source": result["source"],
        "resolved": result.get("resolved", ""),
        "low_confidence": low_conf,
    }


def load_json(path, default):
    if path.exists():
        try:
            return json.loads(path.read_text(encoding="utf-8"))
        except Exception:
            return default
    return default


def main():
    ap = argparse.ArgumentParser(description="Fetch free-licensed animal photos.")
    ap.add_argument("--sample", action="store_true", help="run the 12-animal sanity sample")
    ap.add_argument("--slugs", help="comma-separated slugs to process")
    ap.add_argument("--limit", type=int, help="process only first N missing slugs")
    ap.add_argument("--workers", type=int, default=MAX_WORKERS)
    args = ap.parse_args()

    IMAGES_DIR.mkdir(parents=True, exist_ok=True)

    with open(DATA_FILE, encoding="utf-8") as f:
        animals = json.load(f)
    by_slug = {a["slug"]: a for a in animals}

    SAMPLE = ["wolf", "adder", "aalscholver", "leeuw", "mier", "vos",
              "olifant", "haai", "uil", "egel", "pauw", "kat"]

    if args.slugs:
        wanted = [s.strip() for s in args.slugs.split(",") if s.strip()]
        targets = [by_slug[s] for s in wanted if s in by_slug]
    elif args.sample:
        targets = [by_slug[s] for s in SAMPLE if s in by_slug]
    else:
        targets = list(animals)

    # idempotent: drop ones already done
    targets = [a for a in targets if not (IMAGES_DIR / f"{a['slug']}.webp").exists()]
    if args.limit:
        targets = targets[: args.limit]

    log(f"Animals total: {len(animals)}")
    log(f"To process (missing): {len(targets)}  | workers={args.workers}")
    if not targets:
        log("Nothing to do — all targeted images already exist.")
        return

    attributions = load_json(ATTR_FILE, {})
    results = []
    ok = flagged = 0

    with ThreadPoolExecutor(max_workers=args.workers) as ex:
        futures = {ex.submit(process, a): a["slug"] for a in targets}
        for i, fut in enumerate(as_completed(futures), 1):
            r = fut.result()
            results.append(r)
            if r["status"] == "ok":
                ok += 1
                attributions[r["slug"]] = r["attribution"]
                flag = " [LOW-CONF]" if r["low_confidence"] else ""
                log(f"  [{i}/{len(targets)}] OK   {r['slug']:<22} {r['source']:<20} "
                    f"{r['attribution']['license']:<12}{flag}")
            elif r["status"] == "skip-exists":
                log(f"  [{i}/{len(targets)}] SKIP {r['slug']} (exists)")
            else:
                flagged += 1
                log(f"  [{i}/{len(targets)}] FLAG {r['slug']:<22} {'; '.join(r.get('reasons', []))[:90]}")

    # persist attributions (merge)
    ATTR_FILE.write_text(
        json.dumps(attributions, ensure_ascii=False, indent=2, sort_keys=True),
        encoding="utf-8",
    )

    # write review.md (merge-aware: regenerate from current run's flags + low-conf)
    low_conf = [r for r in results if r["status"] == "ok" and r["low_confidence"]]
    flags = [r for r in results if r["status"] == "flagged"]
    lines = ["# Image sourcing review\n",
             f"_Generated for {len(targets)} processed slug(s). "
             f"OK: {ok}, flagged: {flagged}, low-confidence: {len(low_conf)}._\n"]
    lines.append("\n## Needs manual sourcing (no free image found / download failed)\n")
    if flags:
        lines.append("| slug | nl name | reasons |")
        lines.append("|------|---------|---------|")
        for r in sorted(flags, key=lambda x: x["slug"]):
            reasons = "; ".join(r.get("reasons", [])).replace("|", "/")
            lines.append(f"| `{r['slug']}` | {r.get('nl','')} | {reasons} |")
    else:
        lines.append("_None in this run._")
    lines.append("\n## Low-confidence matches (image found, but species not confirmed as a taxon — eyeball these)\n")
    if low_conf:
        lines.append("| slug | nl name | source | resolved as | license |")
        lines.append("|------|---------|--------|-------------|---------|")
        for r in sorted(low_conf, key=lambda x: x["slug"]):
            lines.append(
                f"| `{r['slug']}` | {r.get('nl','')} | {r['source']} | "
                f"{r.get('resolved','')} | {r['attribution']['license']} |"
            )
    else:
        lines.append("_None in this run._")
    lines.append("")
    REVIEW_FILE.write_text("\n".join(lines), encoding="utf-8")

    log(f"\nDone. OK={ok}  flagged={flagged}  low-confidence={len(low_conf)}")
    log(f"Images:       {IMAGES_DIR}/<slug>.webp")
    log(f"Attributions: {ATTR_FILE}")
    log(f"Review:       {REVIEW_FILE}")


if __name__ == "__main__":
    main()
