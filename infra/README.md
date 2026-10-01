---
link: https://github.com/Niklasvdm/TotemAppIT
version: 0.1.0
relate to:
  - "[[Totem Finder (IT)]]"
---

```table-of-contents
```

# Totem — Dev/Test LXC

A **disposable** Proxmox LXC for building and iterating on `totemd`. It installs the toolchain and nothing else — no app, no service — so you SSH in and hack. When it gets messy, destroy it and re-apply for a clean box in a couple of minutes. Same Terraform pattern as the other repos (Telmate provider, unprivileged Debian, SSH-key inject, provisioning script).

## Quick Start

| Goal | Command |
| ---- | ------- |
| First-time setup | `cp env/terraform.tfvars.example env/dev.tfvars` then edit it |
| Create the box | `cd lxc && terraform init && terraform apply -var-file=../env/dev.tfvars` |
| Connect | `ssh root@<container_ip>` (printed as `ssh_command` output) |
| Push local code up | `./sync.sh root@<container_ip>` |
| Run the Phase-1 tests on it | `ssh root@<ip> 'cd /root/totem-it && python3 scripts/db_test.py'` |
| Throw it away | `terraform destroy -var-file=../env/dev.tfvars` |
| Rebuild clean | `destroy` then `apply` again — nothing here is precious |

## What it installs

`configuration/bootstrap-dev.sh` provisions a fresh Debian 12 container with:

| Tool | Source | For |
| ---- | ------ | --- |
| Go (pinned, default 1.26.8) | go.dev tarball → `/usr/local/go` | Phase 2 backend (`ncruces` is pure-Go, so no CGO; needs Go ≥ 1.26) |
| Node.js LTS (default 22) | NodeSource apt repo | Phase 3 React build |
| Python 3 + venv + pip | apt | Phase 1 seeder/tests, ingestion scripts |
| sqlite3 + libsqlite3-dev | apt | Poke at the DB by hand |
| build-essential, git, rsync, jq | apt | General hacking + `sync.sh` |

Versions are variables (`go_version`, `node_major`) — change them in `dev.tfvars`.

**Resource sizing.** Lean defaults: **1 core / 1024 MB / 8 G disk**. That comfortably runs Go builds and the Phase-1 Python tests; the only step that might want more is the Phase-3 Vite build — if it drags or OOMs, bump `cores`/`memory` in `dev.tfvars` and re-apply (no rebuild needed for a resize).

## Files

```
infra/
├── lxc/
│   ├── main.tf            # LXC resource + provisioning (installs toolchain)
│   ├── variables.tf       # Proxmox conn, container, resources, network, tool versions
│   ├── outputs.tf         # ssh_command, sync_hint, rebuild_hint
│   └── .gitignore         # ignores state + real *.tfvars
├── configuration/
│   └── bootstrap-dev.sh   # the toolchain installer (runs once on create)
├── env/
│   └── terraform.tfvars.example
├── sync.sh                # rsync working tree → box (fast edit/build loop)
└── README.md
```

## The iteration loop

```
edit locally  ──►  ./infra/sync.sh root@<ip>  ──►  ssh root@<ip> 'cd /root/totem-it && <build/test>'
     ▲                                                                    │
     └──────────────────────────  repeat  ◄──────────────────────────────┘
```

`sync.sh` uses `rsync --delete` with excludes for `.git`, `build/`, `*.db`, `node_modules`, caches and terraform state — only source crosses the wire, and it's incremental.

## Running a second box in parallel

Bump `vmid` (and `container_ip`) in a second tfvars file and apply — the state is per working directory, so keep them separate:

```bash
cp env/dev.tfvars env/dev2.tfvars   # edit vmid + container_ip
terraform apply -var-file=../env/dev2.tfvars
```

## Notes & security

- **Not production.** `onboot=false`, self-signed nothing, root SSH by key. This box is for building, not for serving public traffic — the real deployment is a static binary + systemd behind the WAF (see the [main README](../README.md)).
- **Unprivileged** container — the pure-Go build needs no extra capabilities.
- **Secrets:** `dev.tfvars` holds the Proxmox token and root password and is git-ignored (only `*.example` is committed). Never commit real tfvars or terraform state.
- **Pre-reqs:** you must be able to reach the Proxmox API (`proxmox_host_ip:8006`) and the container's IP from where you run Terraform, and your SSH key must be in your agent (`ssh-add`). Same reachability caveat as the WireGuard/Actual repos.

## Potential improvements

- Optional `git clone` of the repo in `bootstrap-dev.sh` (needs deploy-key access to the private repo) instead of `sync.sh`.
- A `--with-air` flag to install `air` for live-reloading `totemd` during Phase 2.
- Snapshot the provisioned box as a template so rebuilds skip the toolchain install.
