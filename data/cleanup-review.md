# Cleanup review — trait translations & Italian names

Two bounded quality passes over `data/animals.json` (471 animals). Dutch (`nl`)
is authoritative and was never touched; `traits_nl`, all `desc_*`, `alt`, `slug`,
and the `nl`/`en` names were left unchanged. A backup is at
`data/animals.json.bak`. Reproducible scripts: `scripts/fix_traits.py` and
`scripts/fix_names_it.py`.

## 1. Trait-translation quality pass

Collapsed the per-animal parallel arrays into a `nl -> {it, en}` map:
**321 distinct Dutch traits**. The it/en were consistent per nl trait across all
animals (one exception: `zorgvuldig` had both `careful`/`carefully` in en —
resolved). Corrections were applied back mechanically to every animal:
**394 `traits_en` cells** and **45 `traits_it` cells** rewritten.

### English fixes (29 distinct traits)

| nl trait | old en | new en | reason |
| --- | --- | --- | --- |
| `druk` | print | **busy** | MT homonym blunder ("druk" = press/print) |
| `plantrekker` | plant puller | **resourceful** | nonsense literal of "plan-trekker" |
| `honkvast` | base | **home-loving** | MT homonym ("honk" = base) |
| `driftig` | adrift | **irascible** | nonsense; means hot-tempered |
| `beweeglijk` | movable | **agile** | wrong meaning |
| `monter` | monter | **chipper** | untranslated |
| `aangepast` | custom | **adapted** | wrong meaning |
| `onberekenbaar` | unaccountable | **unpredictable** | wrong meaning |
| `doelgericht` | targeted | **goal-oriented** | wrong meaning |
| `apart` | separate | **distinctive** | wrong meaning |
| `eetlustig` | appetising | **hearty eater** | wrong meaning (good appetite) |
| `verstrooid` | scattered | **absent-minded** | wrong meaning |
| `vrijgevochten` | liberal | **free-spirited** | off (political connotation) |
| `nachtelijk` | night | **nocturnal** | noun → adjective |
| `instinctief` | instinctively | **instinctive** | adverb → adjective |
| `respectvol` | respectfully | **respectful** | adverb → adjective |
| `vlug` | Quick | **quick** | stray capital |
| `aanpassend` | customising | **adaptable** | wrong meaning |
| `afwachtend` | waiting | **wait-and-see** | not a natural trait word |
| `familiaal` | family | **family-oriented** | bare noun |
| `fel` | bright | **fierce** | off |
| `onstuimig` | heady | **impetuous** | off |
| `bedaard` | subdued | **composed** | off |
| `gezapig` | dull | **placid** | off |
| `gezellig` | cozy | **convivial** | "cozy" describes a place, not a character |
| `potsierlijk` | ludicrous | **comical** | softened to neutral trait |
| `zorgvuldig` | careful/carefully | **meticulous** | resolves conflict + distinguishes from `voorzichtig` (careful) |
| `zanglustig` | vocal | **songful** | clearer (see it below) |
| `bouwlustig` | building | **builder** | bare gerund → noun-trait |

### Italian fixes (7 distinct traits)

| nl trait | old it | new it | reason |
| --- | --- | --- | --- |
| `eigenwijze` | eigenwijze | **testardo** | untranslated (Dutch left in) |
| `flamboyant` | flamboyante | **sgargiante** | not an Italian word (French) |
| `probleemoplossend` | risoluto | **risolutivo** | "risoluto" = resolute, wrong meaning |
| `slaapgraag` | pigro | **dormiglione** | "pigro" = lazy; now "sleepy" |
| `opvrolijkend` | rallegra gli altri | **rallegrante** | clunky sentence → clean adjective |
| `zanglustig` | melodioso | **canterino** | proper word for a bird fond of singing (also de-dups `melodieus`) |
| `bouwlustig` | costruttivo | **costruttore** | "costruttivo" = constructive; means builder |

Note: distinct Dutch traits legitimately sharing one it/en word (synonyms, e.g.
`trots`/`fier` → "proud", `gracieus`/`sierlijk`/`bevallig` → "graceful") were
left as-is per instructions.

## 2. Italian NAME validation

English names were already validated (not redone). Italian names were checked
against **Italian Wikipedia** lead sentences (vernacular name in the article
intro), reached via the validated English name's `it` langlink. Italian
Wikipedia titles most species under their scientific binomial, so the vast
majority of `it` names could not be *confidently* contradicted and were left
untouched. Scanned: 63 `it == nl` and 17 `it == en` candidates — almost all are
legitimate international names in Italian (zebra, gorilla, koala, cobra, orca,
armadillo, yak, kudu, caracal, termite, …).

**4 confident fixes** (each was `it == nl`, untranslated, with a clear distinct
Italian vernacular):

| slug | old it | new it | source |
| --- | --- | --- | --- |
| `geep` | Geep | **Aguglia** | it.wiki Belonidae: "Le aguglie" |
| `gibbon` | Gibbon | **Gibbone** | it.wiki Hylobatidae: "detti comunemente gibboni" |
| `manoel` | Manoel | **Gatto di Pallas** | it.wiki Otocolobus manul: "Il gatto di Pallas" |
| `serval` | Serval | **Servalo** | it.wiki Leptailurus serval: "Il servàlo" ("serval" is a listed variant) |

### Left for human review (not changed — not confident)

- `kongoni` (it = "Kongoni"): Italian for hartebeest is *alcelafo* (Coke's →
  *alcelafo di Coke*), but "kongoni" is used internationally for the specific
  animal — left as-is.
- `hokko` (it = "Hokko"): Italian for curassow is *hocco*; could not reach the
  it.wiki article via langlink to verify the exact species — left as-is.
- `dziggetai` (it = "Dziggetai"): Asian wild ass; Italian vernacular (*chiang* /
  *emione*) is ambiguous for this exact form — left as-is.
