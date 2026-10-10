# CLAUDE.md — working guide for Totems-IT

Operating notes for Claude sessions on this repo. The authoritative docs are the three
READMEs and the architecture docs (see [Where the real docs are](#where-the-real-docs-are));
this file is the short version plus the rules that are easy to get wrong.

## What we are building

An Italian translation of the Dutch **Scouts en Gidsen Vlaanderen** *Totemzoeker*: a searchable
catalogue of 472 animal totems (traits + descriptions in it/en/nl, similarity + exact-filter
search), plus a suite of **multiplayer games** on a shared realtime engine. It ships as a **single
Go binary** (`totemd`) that serves both the JSON API and the embedded React SPA, behind the
existing NGINX+ModSecurity WAF. No Docker: static binary + systemd on an LXC.

- **Backend:** Go (`chi` router), pure-Go SQLite (`ncruces/go-sqlite3` + `vfs/adiantum`,
  encrypted at rest), layered transport -> domain -> data.
- **Frontend:** React 18 + Vite 7 + TypeScript + react-router 7 + TanStack Query + Zustand +
  react-i18next (it/en/nl).
- **Games engine:** `internal/game` — a `Game` interface + `Outbox`, one goroutine per room,
  per-seat private views. Nine games today (Bomberman is the only realtime one) plus the Totem
  Crawler (V0). See [docs/games-architecture.md](docs/games-architecture.md).

## Hard rules (do not break these)

1. **Never run git.** The user handles all git personally (commit/merge/checkout/push/branch/…).
   Do not stage, commit, or push. Not ever.
2. **Never materialize or print the DB encryption key.** The dev key is the non-secret string
   `dev-only-key`; prod uses a TPM-sealed systemd credential. The key is env-only
   (`TOTEM_DB_KEY` / `TOTEM_DB_KEY_FILE`), never on disk beside the DB, never echoed.
3. **No em-dashes in prose.** Use normal punctuation (colons, parentheses, periods). This applies
   to docs, changelog entries and chat copy that you author.
4. **Ask before big or destructive actions.** Confirm before anything hard to reverse or
   outward-facing. Approval in one context does not carry to the next.
5. **Minimal Go imports.** Keep import sets tight; prefer stdlib patterns that reduce imports
   (e.g. `signal.NotifyContext`). Centralize I/O so fewer layers import `os` (it lives in
   `internal/config` and `internal/ingest`, basically nowhere else).
6. **Translate via the DeepL scripts.** Use `scripts/translate_descriptions.py` (nl -> it/en),
   not inline/hand translation.
7. **Weigh options genuinely.** Give a real recommendation with trade-offs, not validation of the
   first idea.
8. **Testing is first-class.** Every backend change ships with tests; data integrity has its own
   gate. This is a self-identified weak spot, so do not skip it.

## Servers and environments

| Env | Host | Notes |
| --- | --- | --- |
| **Dev LXC** | `root@192.168.10.195` | The build box. Repo at `~/totem-it`. **Go toolchain lives here, not locally.** Disposable (Terraform, `onboot=false`). DNS `dev.totem.nvdm.eu`. |
| **Prod** | `root@192.168.10.200` (example) | Separate container + Terraform workspace `prod`. Shipped binary + systemd. DNS `totem.nvdm.eu`. |
| **Edge** | NGINX + ModSecurity WAF | Terminates TLS, rate-limits, reverse-proxies to `127.0.0.1:8683`. Lives in the `[[ReverseProxyWAF]]` project. |
| **Proxmox host** | `192.168.10.55:8006` | Not reachable from Claude's session; the user runs `terraform apply` from their LAN. |

`totemd` default bind is `127.0.0.1:8683`. Config is env-only: `TOTEM_ADDR`, `TOTEM_DB_PATH`,
`TOTEM_DB_KEY`/`TOTEM_DB_KEY_FILE`, `TOTEM_IMAGE_DIR`, `TOTEM_EMOJI_FILE`,
`TOTEM_ALLOWED_ORIGINS` (dev proxy only, empty in prod).

## Dev workflow (there is no git on the box)

The dev LXC is an **rsync mirror** of the local working tree, not a checkout.

```bash
# 1. edit locally
# 2. push the tree to the box (rsync --delete; excludes .git, node_modules, *.db, dist, tf state)
./infra/sync.sh root@192.168.10.195
# 3. rebuild Go + restart both API and Vite on the box (also applies DB migrations on startup)
ssh root@192.168.10.195 'cd ~/totem-it && ./run-dev.sh'
```

- **Go lives on the box only.** Build/vet/test over SSH:
  `ssh root@192.168.10.195 'cd ~/totem-it/src && source /etc/profile.d/go.sh && go test ./...'`
  (run `go` from `src/`). **gofmt on the box**, then rsync the formatted files back.
- `sync.sh` does `rsync --delete`, so it clobbers the box's `go.mod`/`go.sum` with the repo's.
  Keep the repo copies current (re-`go mod tidy` on the box, scp back, if deps change).
- A new `package-lock.json` (e.g. after a pull) needs a one-off `npm install` on the box;
  `run-dev.sh` only installs when `node_modules` is missing.
- `run-dev.sh` health-checks the backend before starting Vite and prints a `:5173` URL.

### Running totemd over SSH (gotchas that have bitten before)

- **Never `pkill -f totemd`** over SSH: the pattern matches the ssh command's own argv and kills
  the session (exit 255). Use `pkill -x totemd` (exact name).
- Stale `go run` children linger on `:8683`. Before a smoke test:
  `fuser -k 8683/tcp; pkill -x totemd`. Otherwise curls hit the old binary and "my change didn't
  apply" is a red herring. `run-dev.sh` already pkills at start.
- A backgrounded `go build -o /tmp/x && … &` backgrounds the whole chain, so curls can hit a
  stale binary. Prefer `go run ./cmd/totemd` for smoke tests, or build synchronously.
- A dead proxied backend shows up as a silently empty UI, not an error. Always health-check.

## Documentation standards

The user's READMEs double as **Obsidian vault notes** and follow a fixed house style (see the
`readme-style-reference` memory). When writing or editing a README:

- YAML frontmatter at the top (`link:`, `version:`, `relate to:` with `[[Backlinks]]`), then a
  ` ```table-of-contents ``` ` fenced block, then an H1 and a dense non-marketing intro.
- Tables over prose for anything structured. A **Concepts** section defining bold domain terms.
  Annotated code blocks (inline `#` comments saying *why*). A **Security** section using the
  **Design Decision / Risk / Mitigation** pattern. End with Issues/Roadmap -> **Changelog**
  (semver, dated) -> References.
- Explain *why* a non-obvious choice was made; never explain a self-evident command.
- **Prose is single long lines per paragraph, not hard-wrapped.** Hard-wrapping at ~80 cols
  renders as a narrow column in Obsidian. If you ever need to unwrap, `scratchpad/reflow.py`
  does it (preserves fences, tables, frontmatter, lists, blockquotes).

### PlantUML

Diagrams go **in the Markdown** (Obsidian renders them inline), not in chat. Every diagram starts
with these skinparams so boxes are white with black text under a dark theme:

```
skinparam shadowing false
skinparam backgroundColor #FFFFFF
skinparam monochrome true
skinparam defaultFontColor #111111
```

`monochrome true` is what forces white boxes / black text (it overrides the dark theme; without it
you get black-on-black). Keep component labels **unique** within a diagram or PlantUML merges nodes.
Class bodies must be multi-line (inline `;`-separated members do not parse).

### Obsidian README sync

READMEs are symlinked into the vault via `~/projects/Obsidian-projects-README-sync/`
(`sync-pairs.conf` maps SOURCE|DEST, `readme-sync.sh` creates the symlinks under
`~/Obsidian/Elegost/4. Home Lab/Applications/`). Current pairs: `TotemApp-IT` (root README),
`TotemApp-IT-Games` (games-architecture.md), `TotemApp-IT-Crawler` (crawler-architecture.md).
Cross-link notes with the frontmatter `relate to: - "[[NoteName]]"`.

## Games engine essentials

- A game is one file in `internal/game/<game>.go` implementing the `Game` interface, registered by
  slug in `gameFactories` (`engine.go`). Turn games are **event-driven** (`TickHz()==0`); only
  Bomberman is realtime (60Hz).
- **Per-seat private views** are the core trick: on a change, loop `Outbox.Seats()` and send each
  seat `viewFor(seat)` via `Outbox.Send(seat, v)` (a `broadcastViews()` wrapper). Bomberman uses
  `Outbox.Broadcast`.
- **The nil-slice gotcha (recurring white-screen bug):** a view slice must never be nil.
  `encoding/json` serialises `nil` as `null` and the client does `.length`/`.map` on it. Build
  slices with `append([]T{}, …)` (never `append([]T(nil), …)`), and guard the client with `?? []`.
- Client side: `useRoom<V>()` transport hook + `RoomBar` (shared), one page component per game, a
  `rules.ts` entry, i18n keys in all three languages, CSS in `styles.css`, a route in `App.tsx`,
  and a card in `GamesPage.tsx`.
- **Report a problem** exists for games too (migration `0004_game_reports.sql`,
  `POST /api/v1/games/reports`, `game.Exists()`, `totem-admin game-reports`, `GameReportButton.tsx`
  in every room bar).
- DB migrations are embedded SQL in `internal/store/migrations/*.sql`, applied in filename order on
  startup, tracked in `schema_migrations`.

## Versioning

`VERSION` at the repo root is the single source of truth (currently `0.4.2`). `deploy.sh` stamps it
into the Go binaries (`-ldflags`) and the SPA (Vite `define`); the `F3` HUD shows client vs server
stamps. Bump `VERSION` and add a dated semver entry to the README **Changelog** when cutting work.

## Where the real docs are

- [README.md](README.md) — the whole system: architecture, API, data model, Bomberman netcode,
  deployment, security, roadmap, changelog. The source of truth.
- [infra/README.md](infra/README.md) — the disposable dev/test LXC (Terraform, bootstrap, sync.sh)
  and the dev-vs-prod workspace split.
- [docs/games-architecture.md](docs/games-architecture.md) — the shared engine, multi-tenancy /
  isolation knobs, the `Game`/`Outbox` types, networking, and the catalog of all nine games.
- [docs/crawler-architecture.md](docs/crawler-architecture.md) — the Totem Crawler spec (V0
  implemented, refined in 0.4.2).
- [docs/openapi.yaml](docs/openapi.yaml) — the OpenAPI 3.1 contract.

Claude's own cross-session memory lives in
`~/.claude/projects/-home-niklasvdm-projects-Totems-IT/memory/` (indexed by `MEMORY.md`); the
project-state memories there (`project_totems_architecture`, `project_totem_crawler`) carry the
detailed history.
