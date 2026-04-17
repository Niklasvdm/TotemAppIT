#!/usr/bin/env python3
"""
Scrape all 311 missing animals from the SGV Totemzoeker website.
Adds them to data/animals.json with nl, alt, slug, traits_nl, desc_nl.
Leaves it, traits_it and desc_it empty for the next step.
"""
import json, re, sys, urllib.request, urllib.parse
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

ROOT = Path(__file__).parent.parent
DATA_FILE = ROOT / "data" / "animals.json"

HEADERS = {
    "User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36",
    "Accept": "text/html,application/xhtml+xml",
    "Accept-Language": "nl-BE,nl;q=0.9",
}

MISSING = [
"buizerd","bultrug","caracara","cavia","chinchilla","cichlide","civetkat","cobra",
"comorenwever","coscoroba","coyote","dagpauwoog","damhert","danio","das","dikdik",
"dikhoornschaap","diksnavelmees","distelvink","doornkruiper","doornsnavel","draaihals",
"drongo","dwerghert","dwergmuis","dziggetai","edelhert","eidereend","eland","ever",
"fazant","fitis","fluithaas","forel","franjeaap","franjepoot","franjeschildpad",
"frankolijn","franse-bulldog","fregatvogel","fret","fuut","fuutkoet","gaai","gaffelbok",
"galago","gaur","gaviaal","geep","geit","gekko","gems","genetkat","gibbon","goendi",
"golden-retriever","goudhaan","goudplevier","goudvink","goudvis","grauwe-gans","griel",
"griend","grijs-bokje","groenling","grondeekhoorn","grondel","guppy","haan","haas",
"hagedis","hazelmuis","hazewind","herdershond","hert","hoenderkoet","hokko","hond",
"hondshaai","hop","houtduif","ibis","ijsduiker","ijsvogel","jabiroe","jacana",
"jachtluipaard","jaguarundi","jak","jako","jan-van-gent","kaaiman","kabeljauw",
"kaketoe","kameel","kamsalamander","kanarie","kapucijnaap","karakol","karekiet",
"katfret","kauw","kea","kemphaan","kempvis","kever","kievit","kikker","klauwier",
"klipdas","kluut","knobbelzwijn","koedoe","koekoek","kogelvis","koi","kokako",
"kolibrie","kongoni","konijn","kookaburra","koolmees","koraalduivel","koraalvlinder",
"kraai","krab","kreeft","krekel","kwartel","kwartelsnip","kwikstaart","lama",
"lampongaap","leeuwaapje","leeuwerik","lepelaar","libel","lieveheersbeestje","lijster",
"linsang","loodsmannetje","lori-aap","lori-papegaai","makreelhaai","maleise-beer",
"mamba","mandril","manenwolf","mangoest","manoel","manta","mara","maraboe","margay",
"marlijn","marmersalamander","marmot","marter","maskerwimpelvis","meerkat","meerkoet",
"meerval","meeuw","merel","mier","monniksrob","muis","murene","mus","mustang",
"nachtaap","nachtegaal","nandoe","narwal","neusbeer","newfoundlander","oeakari",
"opossum","oranje-passiebloemvlinder","oribi","orka","ornaatelfje","oropendola",
"paard","paca","pad","pademelon","paling","palmtortel","papegaai","papegaaivis",
"paradijsvogel","pardelkat","parelhoen","parelkwal","parkiet","patrijs",
"pauwoogstekelrog","pekari","pelikaan","penseelaapje","penseelkrab","pijlinktvis",
"pimpelmees","pinguin","piranha","plevier","poedel","pony","poolvos","potto",
"prairiehond","ree","renvogel","ringstaartmaki","rivierdolfijn","rode-lynx","roek",
"roerdomp","roodborstje","saiga","saki","salangaan","saterhoen","schaap","schaarbek",
"schippertje","scholekster","schorpioen","schrijvertje","schroefhoorngeit",
"secretarisvogel","seriema","serval","sijs","sikahert","sint-bernard","slaapmuis",
"slangehalsvogel","slankbeer","slechtvalk","smelleken","sneeuwhoen","sneeuwstormvogel",
"sneeuwuil","snip","snoek","specht","spiesbok","spinaap","spitsmuis",
"spitssnuitdolfijn","spitsvogel","spoorkoekoek","spotlijster","spreeuw","springhaas",
"sprinkhaan","staartmees","steenarend","steenbok","steenloper","stekelstaarteend",
"stekelstaartzwaluw","steltkluut","steltral","steppelemming","stern","steur",
"stormvogel","strandloper","streepmuis","tagoean","taipan","tarpon","termiet",
"tijgerhaai","tjiftjaf","toekan","toepaja","toerako","torenvalk","vampierinktvis",
"veldmuis","vink","vlinder","vogelbekdier","vuursalamander","vuurstaartlabeo",
"wasbeer","wever","wielewaal","winterkoninkje","wombat","wouw","wulk","wulp","zalm",
"zebramangoest","zee-egel","zee-engel","zeeduivel","zeekat","zeeleeuw","zeepaardje",
"zeester","zeewolf","zonnebaars","zonnevis","zwaardvis","zwaluw",
]


def fetch_animal(slug):
    url = "https://www.scoutsengidsenvlaanderen.be/totemzoeker/" + urllib.parse.quote(slug, safe="")
    try:
        req = urllib.request.Request(url, headers=HEADERS)
        html = urllib.request.urlopen(req, timeout=20).read().decode("utf-8", errors="replace")
    except Exception as e:
        print(f"  ERROR {slug}: {e}", file=sys.stderr)
        return None

    # Dutch name
    m = re.search(r"<h1[^>]*><span>(.*?)</span>", html)
    nl = m.group(1).strip() if m else slug

    # Alt name
    m2 = re.search(r'<h2 class="h3 mb-3 mb-md-4 mb-xl-5 text-muted">(.*?)</h2>', html)
    alt = m2.group(1).strip() if m2 else ""

    # Traits
    traits_section = re.search(r'totem-detail__characteristics__items(.*?)(?:</div></div></div>)', html, re.DOTALL)
    traits_nl = []
    if traits_section:
        traits_nl = re.findall(r'aria-label="([^"]+)"', traits_section.group(1))
        traits_nl = [t.strip() for t in traits_nl]

    # Description
    m3 = re.search(r'<meta property="og:description" content="([^"]+)"', html)
    desc_nl = m3.group(1).strip() if m3 else ""

    return {
        "nl": nl,
        "it": "",
        "alt": alt,
        "traits_nl": traits_nl,
        "traits_it": [],
        "slug": slug,
        "desc_nl": desc_nl,
        "desc_it": "",
    }


def main():
    with open(DATA_FILE, encoding="utf-8") as f:
        animals = json.load(f)

    existing_slugs = {a["slug"] for a in animals}
    to_add = [s for s in MISSING if s not in existing_slugs]
    print(f"Scraping {len(to_add)} missing animals …")

    results = {}
    with ThreadPoolExecutor(max_workers=20) as ex:
        futures = {ex.submit(fetch_animal, s): s for s in to_add}
        for i, f in enumerate(as_completed(futures), 1):
            slug = futures[f]
            data = f.result()
            if data:
                results[slug] = data
                print(f"  [{i}/{len(to_add)}] OK  {slug} — {data['nl']}  traits={len(data['traits_nl'])}")
            else:
                print(f"  [{i}/{len(to_add)}] --  {slug}")

    # Append new animals (keep existing ones intact)
    for slug in to_add:
        if slug in results:
            animals.append(results[slug])

    # Sort alphabetically by slug
    animals.sort(key=lambda a: a["slug"])

    with open(DATA_FILE, "w", encoding="utf-8") as f:
        json.dump(animals, f, ensure_ascii=False, indent=2)

    print(f"\nDone. animals.json now has {len(animals)} entries.")
    print(f"New animals added: {len(results)}")
    print("Next step: fill in 'it', 'traits_it', 'desc_it' fields, then run generate_pages.py")


if __name__ == "__main__":
    main()
