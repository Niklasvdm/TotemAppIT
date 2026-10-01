#!/usr/bin/env python3
"""Build data/emoji.json: map every animal slug -> best-fitting Unicode animal emoji.

Strategy (in priority order, first match wins):
  1. Per-slug OVERRIDES for tricky cases (odd translations, false-positive traps,
     proper names with no English keyword).
  2. Ordered KEYWORD rules matched against the English name (whole-word) and the
     Dutch name (substring, which catches Dutch compound nouns like
     "boomkikker" -> "kikker" -> frog).
  3. Final fallback: paw prints.

Each resulting emoji is tagged "specific" (a dedicated / very-close emoji) or
"category" (a broad family fallback) or "paw" so a human can later hand-tune the
broad ones (see data/emoji-review.md).

Reproducible: re-run `python3 scripts/build_emoji_map.py` to regenerate both
data/emoji.json and data/emoji-review.md.
"""
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ANIMALS = ROOT / "data" / "animals.json"
OUT_JSON = ROOT / "data" / "emoji.json"
OUT_REVIEW = ROOT / "data" / "emoji-review.md"

# ---------------------------------------------------------------------------
# 1. Per-slug overrides. Value = (emoji, tag).
#    These win over the keyword engine and fix mistranslations / trap words.
# ---------------------------------------------------------------------------
OVERRIDES = {
    # --- marine "fish-named-after-something-else" traps ---
    "zeewolf": ("\U0001F41F", "category"),          # wolffish -> fish (not wolf)
    "zeekat": ("\U0001F991", "category"),           # cuttlefish -> squid (not cat)
    "zeeleeuw": ("\U0001F9AD", "specific"),         # sea lion -> seal (not lion)
    "koraalduivel": ("\U0001F420", "category"),     # lionfish -> tropical fish
    "koraalvlinder": ("\U0001F420", "category"),    # butterflyfish -> tropical fish
    "papegaaivis": ("\U0001F420", "category"),      # parrotfish -> tropical fish
    "maskerwimpelvis": ("\U0001F420", "category"),  # wrasse/bannerfish -> tropical fish
    "anemoonvis": ("\U0001F420", "category"),       # clownfish -> tropical fish
    "kempvis": ("\U0001F420", "category"),          # betta -> tropical fish
    "zeeduivel": ("\U0001F41F", "category"),        # monkfish -> fish
    "zonnevis": ("\U0001F41F", "category"),         # ocean sunfish -> fish
    "zonnebaars": ("\U0001F41F", "category"),       # sunfish -> fish
    "zeepaardje": ("\U0001F41F", "category"),       # seahorse -> fish (no seahorse emoji)
    "bladschaap": ("\U0001F40C", "category"),       # leaf sheep (sea slug) -> snail/slug
    "blauwe-zeeslak": ("\U0001F40C", "category"),   # blue sea slug -> slug
    "zee-engel": ("\U0001F41A", "category"),        # sea angel (pteropod) -> shell
    "zee-egel": ("\U0001F41A", "category"),         # sea urchin -> shell (not hedgehog)
    "zeester": ("\U0001F41A", "category"),          # starfish -> shell (no starfish emoji)
    "loodsmannetje": ("\U0001F41F", "category"),    # pilot fish -> fish
    "vuurstaartlabeo": ("\U0001F420", "category"),  # firetail labeo -> tropical fish
    "beensnoek": ("\U0001F41F", "category"),        # longnose gar -> fish
    "murene": ("\U0001F41F", "category"),           # moray eel -> fish
    "geep": ("\U0001F41F", "category"),             # garfish -> fish
    "grondel": ("\U0001F41F", "category"),          # goby -> fish
    "manta": ("\U0001F41F", "category"),            # manta ray -> fish
    "pauwoogstekelrog": ("\U0001F41F", "category"), # stingray -> fish
    "koi": ("\U0001F420", "category"),              # koi -> tropical/ornamental fish
    "guppy": ("\U0001F420", "category"),            # guppy -> aquarium fish
    "danio": ("\U0001F420", "category"),            # danio -> aquarium fish
    "cichlide": ("\U0001F420", "category"),         # cichlid -> aquarium fish
    "goudvis": ("\U0001F41F", "specific"),          # goldfish -> fish
    "mantiskreeft": ("\U0001F990", "category"),     # mantis shrimp -> shrimp
    # --- bear-named non-bears ---
    "neusbeer": ("\U0001F99D", "category"),         # coati -> raccoon (not bear)
    "beermarter": ("\U0001F43E", "paw"),            # binturong (bearcat) -> paw
    "beerdiertje": ("\U0001F9A0", "category"),      # tardigrade -> microbe
    # --- badgers / mustelids / small carnivores ---
    "honingdas": ("\U0001F9A1", "specific"),        # honey badger -> badger
    "klipdas": ("\U0001F43E", "paw"),               # hyrax -> paw (not badger)
    "marter": ("\U0001F43E", "paw"),                # marten
    "hermelijn": ("\U0001F43E", "paw"),             # ermine/stoat
    "wezel": ("\U0001F43E", "paw"),                 # weasel
    "bunzing": ("\U0001F43E", "paw"),               # polecat
    "mink": ("\U0001F43E", "paw"),                  # mink
    "fret": ("\U0001F43E", "paw"),                  # ferret
    "katfret": ("\U0001F43E", "paw"),               # asian wild cat / marbled polecat
    # --- primates without a dedicated emoji -> monkey family ---
    "lemur": ("\U0001F412", "category"),
    "maki": ("\U0001F412", "category"),
    "sifaka": ("\U0001F412", "category"),
    "galago": ("\U0001F412", "category"),
    "potto": ("\U0001F412", "category"),
    "lori-aap": ("\U0001F412", "category"),
    "ringstaartmaki": ("\U0001F412", "category"),
    "oeakari": ("\U0001F412", "category"),
    "saki": ("\U0001F412", "category"),
    "manoel": ("\U0001F412", "category"),           # (not a monkey, but obscure) -> paw? keep monkey off
    # --- proper-name animals the keyword engine can't parse ---
    "agoeti": ("\U0001F400", "category"),           # agouti -> rat/rodent
    "paca": ("\U0001F400", "category"),             # paca -> rodent
    "mara": ("\U0001F400", "category"),             # patagonian mara -> rodent
    "coypu": ("\U0001F400", "category"),            # nutria -> rodent
    "goendi": ("\U0001F400", "category"),           # gundi -> rodent
    "tagoean": ("\U0001F43F️", "category"),    # giant flying squirrel -> squirrel
    "stekelstaarteekhoorn": ("\U0001F43F️", "category"),
    "gaur": ("\U0001F403", "category"),             # gaur -> ox
    "banteng": ("\U0001F403", "category"),          # banteng -> ox
    "jak": ("\U0001F403", "category"),              # yak -> ox
    "kongoni": ("\U0001F98C", "category"),          # hartebeest -> deer-like antelope
    "oribi": ("\U0001F98C", "category"),
    "saiga": ("\U0001F98C", "category"),
    "dikdik": ("\U0001F98C", "category"),
    "gemsbok": ("\U0001F98C", "category"),
    "koedoe": ("\U0001F98C", "category"),           # kudu
    "impala": ("\U0001F98C", "category"),
    "gnoe": ("\U0001F98C", "category"),             # wildebeest
    "dziggetai": ("\U0001F40E", "category"),        # wild ass -> horse
    "okapi": ("\U0001F992", "category"),            # okapi -> giraffe family
    "tapir": ("\U0001F43E", "paw"),
    "gordeldier": ("\U0001F43E", "paw"),            # armadillo
    "miereneter": ("\U0001F43E", "paw"),            # anteater
    "mol": ("\U0001F43E", "paw"),                   # mole
    "vogelbekdier": ("\U0001F986", "category"),     # platypus -> duck (duck-billed)
    "wombat": ("\U0001F43E", "paw"),
    "opossum": ("\U0001F43E", "paw"),
    "tasmaanse-duivel": ("\U0001F43E", "paw"),
    "stokstaartje": ("\U0001F43E", "paw"),          # meerkat (animal) -> paw
    "mangoest": ("\U0001F43E", "paw"),
    "zebramangoest": ("\U0001F43E", "paw"),
    "hyena": ("\U0001F43E", "paw"),
    "linsang": ("\U0001F43E", "paw"),
    "toepaja": ("\U0001F43E", "paw"),               # treeshrew
    "fluithaas": ("\U0001F430", "category"),        # pika -> rabbit family
    "springhaas": ("\U0001F400", "category"),       # springhare -> rodent
    "stekelvarken": ("\U0001F994", "category"),     # porcupine -> hedgehog (spiny)
    # small wild cats / viverrids -> cat
    "serval": ("\U0001F408", "category"),
    "karakol": ("\U0001F408", "category"),          # caracal
    "pardelkat": ("\U0001F408", "category"),        # ocelot-like
    "jaguarundi": ("\U0001F408", "category"),
    "margay": ("\U0001F408", "category"),
    "genetkat": ("\U0001F408", "category"),
    "civetkat": ("\U0001F408", "category"),
    "fosse": ("\U0001F408", "category"),            # fossa
    "manoel-cat": ("\U0001F408", "category"),
    "lynx": ("\U0001F408", "category"),
    "rode-lynx": ("\U0001F408", "category"),        # bobcat
    # canids without own emoji -> wolf/dog family
    "coyote": ("\U0001F43A", "category"),
    "jakhals": ("\U0001F43A", "category"),          # jackal
    "dingo": ("\U0001F43A", "category"),
    # misc birds that trip keywords
    "secretarisvogel": ("\U0001F985", "category"),  # secretary bird -> raptor
    "slangehalsvogel": ("\U0001F426", "category"),  # anhinga/darter -> bird
    "spitsvogel": ("\U0001F426", "category"),       # red-backed shrike -> bird
    "renvogel": ("\U0001F426", "category"),         # roadrunner/courser -> bird
    "kiwi": ("\U0001F426", "category"),             # kiwi bird -> bird
    "kakapo": ("\U0001F99C", "category"),           # flightless parrot -> parrot
    "kea": ("\U0001F99C", "category"),              # alpine parrot -> parrot
    "jako": ("\U0001F99C", "category"),             # african grey parrot
    "lori-papegaai": ("\U0001F99C", "category"),    # lory -> parrot
    "kaketoe": ("\U0001F99C", "category"),          # cockatoo -> parrot
    "ara": ("\U0001F99C", "category"),              # macaw -> parrot
    "parkiet": ("\U0001F99C", "category"),          # parakeet -> parrot
    "beo": ("\U0001F426", "category"),              # hill myna -> bird
    "toekan": ("\U0001F426", "category"),           # toucan -> bird
    "arassari": ("\U0001F426", "category"),         # aracari -> bird
    "kolibrie": ("\U0001F426", "category"),         # hummingbird -> bird
    "ornaatelfje": ("\U0001F426", "category"),      # woodnymph (hummingbird) -> bird
    "emoe": ("\U0001F426", "category"),             # emu -> bird
    "nandoe": ("\U0001F426", "category"),           # rhea -> bird
    "struisvogel": ("\U0001F426", "category"),      # ostrich -> bird
    "kaaiman": ("\U0001F40A", "category"),          # cayman -> crocodile
    "gaviaal": ("\U0001F40A", "category"),          # gavial -> crocodile
    "komodovaran": ("\U0001F98E", "category"),      # komodo dragon -> lizard
    "agame": ("\U0001F98E", "category"),            # agama -> lizard
    "axolotl": ("\U0001F98E", "category"),          # axolotl -> lizard-ish
    # amphibians (salamanders/newts) -> lizard shape
    "kamsalamander": ("\U0001F98E", "category"),
    "marmersalamander": ("\U0001F98E", "category"),
    "vuursalamander": ("\U0001F98E", "category"),
    # whales / cetaceans without own emoji
    "narwal": ("\U0001F40B", "category"),           # narwhal -> whale
    "beloega": ("\U0001F40B", "category"),          # beluga -> whale
    "griend": ("\U0001F40B", "category"),           # pilot whale -> whale
    "bultrug": ("\U0001F40B", "category"),          # humpback -> whale
    "blauwe-vinvis": ("\U0001F40B", "category"),    # blue whale -> whale
    "spitssnuitdolfijn": ("\U0001F42C", "category"),# beaked whale -> dolphin
    "bruinvis": ("\U0001F42C", "category"),         # porpoise -> dolphin
    "rivierdolfijn": ("\U0001F42C", "category"),    # river dolphin -> dolphin
    "walrus": ("\U0001F9AD", "category"),           # walrus -> seal
    "monniksrob": ("\U0001F9AD", "specific"),       # monk seal -> seal
    # cephalopods / inverts
    "pijlinktvis": ("\U0001F991", "category"),      # squid
    "vampierinktvis": ("\U0001F991", "category"),   # vampire squid
    "wulk": ("\U0001F41A", "category"),             # whelk -> shell
    "kwal": ("\U0001FABC", "specific"),             # jellyfish
    "parelkwal": ("\U0001FABC", "specific"),        # comb jelly -> jellyfish
    "libel": ("\U0001F41B", "category"),            # dragonfly -> bug
    "schrijvertje": ("\U0001FAB2", "category"),     # whirligig beetle -> beetle
    "kever": ("\U0001FAB2", "category"),            # beetle
    "termiet": ("\U0001F41C", "category"),          # termite -> ant
    "schorpioen": ("\U0001F982", "specific"),       # scorpion
    # dog breeds -> dog
    "alpenhond": ("\U0001F415", "category"),
    "berner-sennenhond": ("\U0001F415", "category"),
    "boxer": ("\U0001F415", "category"),
    "collie": ("\U0001F415", "category"),
    "franse-bulldog": ("\U0001F415", "category"),
    "golden-retriever": ("\U0001F415", "category"),
    "herdershond": ("\U0001F415", "category"),
    "newfoundlander": ("\U0001F415", "category"),
    "sint-bernard": ("\U0001F415", "category"),
    "hazewind": ("\U0001F415", "category"),         # greyhound
    "poedel": ("\U0001F429", "specific"),           # poodle
    # equines
    "mustang": ("\U0001F40E", "specific"),
    "pony": ("\U0001F40E", "specific"),
    "ezel": ("\U0001FACF", "specific"),             # donkey
    # misc mammals
    "chinchilla": ("\U0001F42D", "category"),
    "cavia": ("\U0001F439", "category"),            # guinea pig -> hamster
    "prairiehond": ("\U0001F43F️", "category"),# prairie dog -> squirrel/rodent
    "berglemming": ("\U0001F42D", "category"),
    "steppelemming": ("\U0001F42D", "category"),
    "spitsmuis": ("\U0001F42D", "category"),        # shrew -> mouse
    "gaffelbok": ("\U0001F98C", "category"),        # pronghorn -> deer-like
    "spiesbok": ("\U0001F98C", "category"),         # oryx -> deer-like
    "bosbok": ("\U0001F98C", "category"),           # bushbuck
    "bosduiker": ("\U0001F98C", "category"),        # duiker
    "grijs-bokje": ("\U0001F98C", "category"),      # grey buck
    "springbok": ("\U0001F98C", "category"),
    "gazelle": ("\U0001F98C", "category"),
    "antilope": ("\U0001F98C", "category"),
    "pekari": ("\U0001F417", "category"),           # peccary -> boar
    "knobbelzwijn": ("\U0001F417", "category"),     # warthog -> boar
    "wolharige-mammoet": ("\U0001FA99", "specific"),# mammoth
    "dwergneushoorn": ("\U0001F98F", "category"),   # dwarf rhino
}

# ---------------------------------------------------------------------------
# 2. Ordered keyword rules: (keywords, emoji, tag).
#    Checked top-to-bottom; first hit wins. A keyword matches if it appears as a
#    whole word in the English name OR as a substring in the Dutch name.
# ---------------------------------------------------------------------------
RULES = [
    # --- primates (specific emoji) ---
    (["gorilla"], "\U0001F98D", "specific"),
    (["orang", "orangutan", "orang-utan"], "\U0001F9A7", "specific"),
    # --- primates (category: monkey) ---
    (["monkey", "baboon", "mandrill", "chimpanzee", "chimp", "gibbon",
      "macaque", "tamarin", "marmoset", "capuchin", "aap", "baviaan"],
     "\U0001F412", "category"),
    # --- canids ---
    (["fox", "vos", "fennec", "fennek"], "\U0001F98A", "specific"),
    (["wolf"], "\U0001F43A", "specific"),
    (["dog", "hond", "puppy", "retriever", "bulldog", "collie", "boxer",
      "terrier", "spaniel", "mastiff"], "\U0001F415", "category"),
    # --- cats ---
    (["lion"], "\U0001F981", "specific"),
    (["tiger", "tijger"], "\U0001F405", "specific"),
    (["cheetah", "leopard", "jaguar", "panther", "cougar", "puma", "luipaard",
      "panter", "jachtluipaard"], "\U0001F406", "specific"),
    (["cat", "kat", "serval", "caracal", "lynx", "ocelot", "wildcat"],
     "\U0001F408", "category"),
    # --- bears ---
    (["polar bear"], "\U0001F43B‍❄️", "specific"),
    (["panda"], "\U0001F43C", "specific"),
    (["koala"], "\U0001F428", "specific"),
    (["sloth", "luiaard"], "\U0001F9A5", "specific"),
    (["bear", "beer"], "\U0001F43B", "specific"),
    # --- other carnivores ---
    (["raccoon", "wasbeer", "coati"], "\U0001F99D", "specific"),
    (["otter"], "\U0001F9A6", "specific"),
    (["skunk", "stinkdier"], "\U0001F9A8", "specific"),
    (["badger", "das"], "\U0001F9A1", "specific"),
    (["hyena"], "\U0001F43E", "paw"),
    # --- hedgehog / bat / rodents ---
    (["hedgehog", "egel"], "\U0001F994", "specific"),
    (["bat", "vleermuis"], "\U0001F987", "specific"),
    (["beaver", "bever"], "\U0001F9AB", "specific"),
    (["chipmunk", "squirrel", "eekhoorn"], "\U0001F43F️", "specific"),
    (["hamster"], "\U0001F439", "specific"),
    (["rat"], "\U0001F400", "specific"),
    (["mouse", "muis", "vole", "lemming", "dormouse", "gerbil"],
     "\U0001F42D", "specific"),
    (["rabbit", "hare", "konijn", "haas", "bunny"], "\U0001F430", "specific"),
    (["porcupine", "stekelvarken"], "\U0001F994", "category"),
    (["marmot", "gopher", "prairie"], "\U0001F43F️", "category"),
    # --- large mammals ---
    (["elephant", "olifant"], "\U0001F418", "specific"),
    (["mammoth", "mammoet"], "\U0001FA99", "specific"),
    (["rhino", "rhinoceros", "neushoorn"], "\U0001F98F", "specific"),
    (["hippo", "hippopotamus", "nijlpaard"], "\U0001F99B", "specific"),
    (["giraffe", "giraf", "okapi"], "\U0001F992", "specific"),
    (["zebra"], "\U0001F993", "specific"),
    (["bison", "bizon"], "\U0001F9AC", "specific"),
    (["buffalo", "buffel", "banteng", "gaur", "yak", "ox"], "\U0001F403", "category"),
    (["cow", "cattle", "koe"], "\U0001F404", "category"),
    (["moose", "elk", "eland"], "\U0001FACE", "specific"),
    (["reindeer", "rendier", "caribou"], "\U0001F98C", "specific"),
    (["deer", "hert", "ree"], "\U0001F98C", "specific"),
    (["antelope", "gazelle", "impala", "springbok", "oryx", "kudu", "gnu",
      "wildebeest", "saiga", "duiker", "dik-dik", "bushbuck", "pronghorn",
      "antilope", "bok"], "\U0001F98C", "category"),
    # --- horses / donkeys / camels ---
    (["horse", "pony", "mustang", "stallion", "paard"], "\U0001F40E", "specific"),
    (["donkey", "ass", "ezel", "mule"], "\U0001FACF", "specific"),
    (["camel", "dromedary", "kameel", "dromedaris"], "\U0001F42A", "specific"),
    (["llama", "alpaca", "lama", "vicuna", "guanaco"], "\U0001F999", "specific"),
    # --- sheep / goats / pigs ---
    (["ram", "bighorn", "mouflon", "dikhoorn"], "\U0001F40F", "specific"),
    (["sheep", "ewe", "lamb", "schaap"], "\U0001F411", "specific"),
    (["goat", "ibex", "chamois", "geit", "gems", "steenbok"], "\U0001F410", "specific"),
    (["boar", "warthog", "peccary", "ever", "zwijn"], "\U0001F417", "specific"),
    (["pig", "hog", "swine", "varken"], "\U0001F416", "specific"),
    # --- marsupials ---
    (["kangaroo", "wallaby", "pademelon", "kangoeroe"], "\U0001F998", "specific"),
    # --- marine mammals ---
    (["dolphin", "porpoise", "dolfijn"], "\U0001F42C", "specific"),
    (["whale", "orca", "walvis", "vinvis", "narwhal", "beluga"], "\U0001F40B", "specific"),
    (["seal", "sea lion", "walrus", "zeehond", "rob"], "\U0001F9AD", "specific"),
    (["sea otter", "zeeotter"], "\U0001F9A6", "specific"),
    # --- reptiles ---
    (["crocodile", "alligator", "cayman", "caiman", "gavial", "krokodil", "kaaiman"],
     "\U0001F40A", "specific"),
    (["tortoise", "turtle", "schildpad", "terrapin"], "\U0001F422", "specific"),
    (["cobra", "mamba", "python", "anaconda", "adder", "viper", "taipan",
      "boa", "snake", "slang", "serpent"], "\U0001F40D", "specific"),
    (["gecko", "agama", "iguana", "chameleon", "monitor", "varaan", "kameleon",
      "lizard", "hagedis", "skink", "komodo"], "\U0001F98E", "specific"),
    (["salamander", "newt", "axolotl"], "\U0001F98E", "category"),
    # --- amphibians ---
    (["frog", "kikker"], "\U0001F438", "specific"),
    (["toad", "pad"], "\U0001F438", "category"),
    # --- sharks & big fish ---
    (["shark", "haai", "dogfish"], "\U0001F988", "specific"),
    (["puffer", "blowfish", "kogelvis"], "\U0001F421", "specific"),
    # --- insects & bugs ---
    (["bee", "bij", "wasp", "hornet"], "\U0001F41D", "specific"),
    (["ant", "termite", "mier"], "\U0001F41C", "specific"),
    (["butterfly", "vlinder"], "\U0001F98B", "specific"),
    (["caterpillar", "rups"], "\U0001F41B", "specific"),
    (["ladybird", "ladybug", "lieveheers"], "\U0001F41E", "specific"),
    (["cricket", "grasshopper", "locust", "krekel", "sprinkhaan"], "\U0001F997", "specific"),
    (["beetle", "kever"], "\U0001FAB2", "specific"),
    (["spider", "spin"], "\U0001F577️", "specific"),
    (["scorpion", "schorpioen"], "\U0001F982", "specific"),
    (["dragonfly", "libel"], "\U0001F41B", "category"),
    (["snail", "slug", "slak"], "\U0001F40C", "specific"),
    (["worm", "worm"], "\U0001FAB1", "category"),
    # --- crustaceans / cephalopods ---
    (["crab", "krab"], "\U0001F980", "specific"),
    (["lobster", "kreeft"], "\U0001F99E", "specific"),
    (["shrimp", "prawn", "garnaal"], "\U0001F990", "specific"),
    (["octopus", "octopus"], "\U0001F419", "specific"),
    (["squid", "cuttlefish", "inktvis"], "\U0001F991", "specific"),
    (["jellyfish", "kwal"], "\U0001FABC", "specific"),
    (["oyster", "clam", "mussel", "whelk", "mossel"], "\U0001F9AA", "category"),
    # --- birds: specific ---
    (["penguin", "pinguin", "pingu"], "\U0001F427", "specific"),
    (["owl", "uil", "oehoe"], "\U0001F989", "specific"),
    (["flamingo"], "\U0001F9A9", "specific"),
    (["peacock", "pauw"], "\U0001F99A", "specific"),
    (["parrot", "macaw", "cockatoo", "parakeet", "lory", "lorikeet", "papegaai",
      "kaketoe", "parkiet"], "\U0001F99C", "specific"),
    (["swan", "zwaan"], "\U0001F9A2", "specific"),
    (["goose", "gans"], "\U0001FABF", "specific"),
    (["duck", "eend", "mallard", "teal", "widgeon", "eider"], "\U0001F986", "specific"),
    (["rooster", "cock", "haan"], "\U0001F413", "specific"),
    (["chicken", "hen", "fowl", "kip", "hoen"], "\U0001F414", "category"),
    (["turkey", "kalkoen"], "\U0001F983", "specific"),
    (["dove", "pigeon", "duif", "tortel"], "\U0001F54A️", "specific"),
    # --- raptors -> eagle ---
    (["eagle", "arend", "zeearend"], "\U0001F985", "specific"),
    (["hawk", "falcon", "kestrel", "buzzard", "kite", "harrier", "vulture",
      "condor", "osprey", "caracara", "havik", "valk", "gier", "buizerd",
      "sperwer", "wouw", "roofvogel", "bird of prey", "merlin", "smelleken"],
     "\U0001F985", "category"),
    # --- corvids -> black bird ---
    (["crow", "raven", "rook", "jackdaw", "kraai", "raaf", "roek", "kauw"],
     "\U0001F426‍⬛", "specific"),
    # --- generic birds (catch-all) ---
    (["bird", "vogel", "finch", "vink", "sparrow", "mus", "tit", "mees",
      "warbler", "robin", "wren", "lark", "leeuwerik", "thrush", "lijster",
      "starling", "spreeuw", "swallow", "zwaluw", "swift", "martin",
      "woodpecker", "specht", "kingfisher", "ijsvogel", "heron", "reiger",
      "stork", "ooievaar", "crane", "kraanvogel", "ibis", "spoonbill",
      "lepelaar", "pelican", "pelikaan", "gull", "meeuw", "tern", "stern",
      "plover", "plevier", "sandpiper", "snipe", "snip", "curlew", "wulp",
      "lapwing", "kievit", "cuckoo", "koekoek", "magpie", "ekster", "jay",
      "gaai", "oriole", "wielewaal", "nightingale", "nachtegaal", "pheasant",
      "fazant", "quail", "kwartel", "partridge", "patrijs", "grouse", "hoen",
      "gannet", "cormorant", "aalscholver", "albatross", "albatros", "petrel",
      "stormvogel", "grebe", "fuut", "coot", "koet", "rail", "ral",
      "hummingbird", "toucan", "hoopoe", "hop", "shrike", "klauwier",
      "weaver", "wever", "canary", "kanarie", "siskin", "sijs",
      "goldfinch", "greenfinch", "bullfinch", "goldcrest", "nuthatch",
      "treecreeper", "wagtail", "kwikstaart", "pipit", "bunting", "myna",
      "drongo", "turaco", "seriema", "jacana", "jabiru", "frigatebird",
      "fregatvogel", "oystercatcher", "scholekster", "avocet", "kluut",
      "stilt", "phalarope", "turnstone", "ruff", "kemphaan", "bittern",
      "roerdomp", "chiffchaff", "tjiftjaf", "wryneck", "razorbill", "alk",
      "skimmer", "roadrunner", "kiwi", "emu", "rhea", "ostrich", "kookaburra",
      "mockingbird", "oropendola", "shoebill", "secretary", "francolin",
      "ani", "aracari", "thornbill"], "\U0001F426", "category"),
    # --- generic fish (catch-all) ---
    (["fish", "vis", "trout", "forel", "salmon", "zalm", "carp", "karper",
      "pike", "snoek", "cod", "kabeljauw", "catfish", "meerval", "eel",
      "paling", "sturgeon", "steur", "gar", "arapaima", "tarpon", "piranha",
      "barracuda", "marlin", "marlijn", "swordfish", "zwaardvis", "ray",
      "stingray", "rog", "perch", "baars", "guppy", "koi", "goldfish",
      "sunfish", "danio", "cichlid", "cichlide", "labeo", "gudgeon",
      "minnow", "herring", "mackerel", "makreel", "anchovy", "sardine"],
     "\U0001F41F", "category"),
    # --- rodent-ish catch-all ---
    (["guinea pig", "cavia", "chinchilla", "agouti", "paca", "coypu", "nutria",
      "capybara", "springhare", "rodent", "knaagdier"], "\U0001F400", "category"),
]

WORD_RE_CACHE = {}


def word_match(keyword, text):
    """Whole-word (or phrase) match of keyword in text."""
    kw = keyword.lower()
    if " " in kw or "-" in kw:
        return kw in text
    pat = WORD_RE_CACHE.get(kw)
    if pat is None:
        pat = re.compile(r"\b" + re.escape(kw) + r"\b")
        WORD_RE_CACHE[kw] = pat
    return pat.search(text) is not None


def classify(animal):
    slug = animal["slug"]
    if slug in OVERRIDES:
        return OVERRIDES[slug]
    en = (animal.get("en") or "").lower()
    nl = (animal.get("nl") or "").lower()
    for keywords, emoji, tag in RULES:
        for kw in keywords:
            if word_match(kw, en) or kw in nl:
                return emoji, tag
    return "\U0001F43E", "paw"  # paw prints fallback


def main():
    animals = json.loads(ANIMALS.read_text(encoding="utf-8"))
    mapping = {}
    tags = {}
    for a in animals:
        emoji, tag = classify(a)
        mapping[a["slug"]] = emoji
        tags[a["slug"]] = tag

    OUT_JSON.write_text(
        json.dumps(mapping, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )

    specific = [s for s, t in tags.items() if t == "specific"]
    category = [s for s, t in tags.items() if t == "category"]
    paw = [s for s, t in tags.items() if t == "paw"]

    by_slug = {a["slug"]: a for a in animals}
    lines = []
    lines.append("# Emoji mapping review\n")
    lines.append(
        "Auto-generated by `scripts/build_emoji_map.py`. "
        "The slugs below got a **broad-category** emoji or the **paw** fallback "
        "and are the best candidates for human hand-tuning. "
        "Slugs mapped to a specific-species emoji are not listed.\n"
    )
    lines.append("## Summary\n")
    lines.append(f"- Total slugs: **{len(animals)}**")
    lines.append(f"- Specific-species emoji: **{len(specific)}**")
    lines.append(f"- Broad-category emoji: **{len(category)}**")
    lines.append(f"- Paw fallback (🐾): **{len(paw)}**\n")

    lines.append("## Paw fallback (🐾) — no sensible animal emoji\n")
    for s in sorted(paw):
        a = by_slug[s]
        lines.append(f"- `{s}` — {a['en']} / {a['nl']} → {mapping[s]}")
    lines.append("")

    lines.append("## Category emoji — close family, could be refined\n")
    for s in sorted(category):
        a = by_slug[s]
        lines.append(f"- `{s}` — {a['en']} / {a['nl']} → {mapping[s]}")
    lines.append("")

    OUT_REVIEW.write_text("\n".join(lines), encoding="utf-8")

    print(f"Wrote {OUT_JSON} ({len(mapping)} slugs)")
    print(f"  specific={len(specific)} category={len(category)} paw={len(paw)}")


if __name__ == "__main__":
    main()
