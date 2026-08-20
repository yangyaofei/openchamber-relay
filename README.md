# OpenChamber Relay

[English](README.md) | [中文](README.zh.md)

Deployment orchestration and companion components for self-hosting
[OpenChamber](https://github.com/openchamber/openchamber): a small Go
WebSocket relay (`relay/`), Docker Compose stacks, and GitHub Actions CI/CD
that auto-builds images whenever upstream `@openchamber/web` publishes a
release.

**Goal**: let OpenChamber clients on the public internet (mobile apps,
browsers on other machines) reach any OpenChamber instance through NAT or
firewalls — whether it is a self-hosted server or the app on your desktop.

## Two deployment modes

| | Mode A: combined | Mode B: relay-only |
|---|---|---|
| Compose file | `docker-compose.yml` | `docker-compose.relay.yml` |
| What runs | OpenChamber server + relay on one host | Just the relay |
| For whom | All-in-one deployment on your home/LAN server | A public VPS relaying for all your devices |
| Who connects | The local OpenChamber (server process) | Any OpenChamber instance (desktop app / server) — just point its relay setting at this relay |

```
Mode A (combined)                       Mode B (relay-only)
Remote client                           Remote client
│ wss                                   │ wss
▼                                       ▼
Router 9527 ─► LAN 443                  Relay (public VPS)
▼                                         │ pairs + forwards opaque
Host reverse proxy (:443)                 │ frames; traffic is E2E
├─ /relay/* ─► relay container (23001)    ▼ encrypted
└─ /*       ─► openchamber container    Any OpenChamber instance
                 │                        (desktop app or self-hosted
                 │ OPENCODE_HOST=        server, each joins as host)
                 ▼
         Host OpenCode server (:4096)
```

Key points:

- **Both containers bind loopback by default** (`127.0.0.1:23000/23001`);
  public exposure is up to your own reverse proxy setup.
- **The relay never sees your data**: frames between remote clients and
  OpenChamber are end-to-end encrypted (ECDH + AES-GCM, keys negotiated at
  pairing); the relay only pairs sockets and forwards opaque messages.
  See [relay/README.md](relay/README.md).
- In Mode A, the **OpenCode server runs on the host** and the container
  reaches it via `host.docker.internal:4096` (injected automatically by
  compose).
- **Any public port works**: 9527 in the diagram is just the router-side
  port mapped to the local 443 — use whatever port you like and keep
  `OPENCHAMBER_RELAY_URL` consistent with it.

## Quick start

### Mode A: combined

Prerequisites: Docker + Compose v2; `opencode serve` already running on the
host (default `:4096`).

```bash
git clone https://github.com/yangyaofei/openchamber-relay.git
cd openchamber-relay

cp .env.example .env
$EDITOR .env        # at least change the two passwords; hostname / relay url as needed

docker compose up -d
curl http://127.0.0.1:23001/health    # relay health check
```

Open `http://127.0.0.1:23000` in a browser (password: `OPENCHAMBER_UI_PASSWORD`
from `.env`) and start using it.

### Mode B: relay-only

Prerequisites: a public machine (VPS) + Docker.

```bash
git clone https://github.com/yangyaofei/openchamber-relay.git
cd openchamber-relay

cp .env.example .env
$EDITOR .env        # RELAY_BIND / RELAY_PORT / RELAY_VERIFY_AUTH as needed

docker compose -f docker-compose.relay.yml up -d
```

Then point any OpenChamber instance at it (the remote pairing settings of a
desktop app, or `OPENCHAMBER_RELAY_URL` of a Mode A server):

```
wss://relay.your.domain.example/relay/ws
```

### Public exposure (either mode, your choice)

Containers bind loopback by default. The relay's WebSocket endpoint is
`/ws` (health check at `/health`); how you expose it is up to you:

- **Reverse proxy + TLS (recommended)**: any WebSocket-capable proxy
  (Nginx, Caddy, Traefik, ...) forwarding to `${RELAY_PORT}`; clients then
  connect via `wss://`;
- **Tailscale Funnel**: if the machine is already in your tailnet,
  `tailscale funnel 8080` publishes the port with automatic TLS — no
  domain, certificate or public IP needed; clients use the provided
  `https://<machine>.<tailnet>.ts.net` URL;
- **Bare**: set `RELAY_BIND=0.0.0.0` in `.env`; clients connect with
  `ws://` in plaintext — testing only.

The relay address clients use depends on your exposure setup (domain /
path / port of your choosing).

## Bare-metal deployment (no Docker)

The relay is a single static binary — Docker is entirely optional. Grab a
release from [releases](https://github.com/yangyaofei/openchamber-relay/releases)
(each tar.gz ships the binary **plus both systemd units**), or use
`go install github.com/yangyaofei/openchamber-relay/relay@latest`.

### Option 1: deb / rpm (needs sudo once, system or user scope your pick)

```bash
# deb
sudo apt install ./openchamber-relay_<version>_linux_amd64.deb
# rpm
sudo dnf install ./openchamber-relay_<version>_linux_amd64.rpm
```

The package installs the binary to `/usr/bin/openchamber-relay`, both
systemd units and `/etc/openchamber-relay/env`. Nothing is started
automatically — pick a scope:

```bash
# system scope (runs as a transient user, starts at boot)
sudo systemctl enable --now openchamber-relay

# user scope (runs as you, no sudo needed from here on)
systemctl --user enable --now openchamber-relay
loginctl enable-linger "$USER"   # keep it running after logout / start at boot
```

### Option 2: tar.gz (no sudo at all)

```bash
# 1. unpack and put the binary on PATH
tar -xzf openchamber-relay_<version>_linux_amd64.tar.gz
mkdir -p ~/.local/bin && mv openchamber-relay ~/.local/bin/

# 2. install the user unit (both unit files are inside the tarball)
mkdir -p ~/.config/systemd/user
mv openchamber-relay-user.service ~/.config/systemd/user/openchamber-relay.service
rm openchamber-relay.service openchamber-relay.env   # system-scope files, not needed here

# 3. enable
systemctl --user daemon-reload
systemctl --user enable --now openchamber-relay
loginctl enable-linger "$USER"   # autostart at boot / survive logout
```

Prefer the system scope anyway? `sudo cp openchamber-relay.service /etc/systemd/system/`,
`sudo cp openchamber-relay.env /etc/openchamber-relay/env`, then
`sudo systemctl daemon-reload && sudo systemctl enable --now openchamber-relay`.

### Adjusting settings

The user unit defaults to `PORT=8080` / `RELAY_VERIFY_AUTH=false`; use a
drop-in override instead of editing the unit:

```bash
mkdir -p ~/.config/systemd/user/openchamber-relay.service.d
printf '[Service]\nEnvironment=PORT=9000\n' \
  > ~/.config/systemd/user/openchamber-relay.service.d/override.conf
systemctl --user daemon-reload && systemctl --user restart openchamber-relay
```

System scope: edit `/etc/openchamber-relay/env` and
`sudo systemctl restart openchamber-relay`.

## Repository layout

```
openchamber-relay/
├── docker-compose.yml          Mode A: OpenChamber + relay combined
├── docker-compose.relay.yml    Mode B: relay-only
├── .env.example                All configurable options, annotated
├── docker/
│   ├── openchamber.Dockerfile  OpenChamber runtime image (version ARG)
│   └── relay.Dockerfile        Relay image (multi-stage Go build)
├── relay/                      The Go WebSocket relay
│   ├── main.go / relay.go / auth.go / sweeper.go
│   ├── test/smoke.mjs          Smoke test (no real keys required)
│   └── README.md
└── .github/workflows/
    ├── openchamber-image.yml   Track upstream releases, auto-build image
    └── relay-image.yml         Build image on relay changes
```

## CI / CD

Two independent release cadences, one per artifact:

**Relay (this repo's own releases)**

| Trigger | What happens |
|---|---|
| push to `master` (relay changes) | multi-arch image `relay:sha-<commit>` on GHCR — no version tag |
| push tag `vX.Y.Z` | image `relay:vX.Y.Z` + `relay:latest` on GHCR, plus a GoReleaser GitHub Release: cross-compiled binaries `linux/{amd64,arm64,armv7,386}` + `darwin/{amd64,arm64}` (tar.gz, both systemd units included, checksums) and `deb`/`rpm` packages (system & user units, `/etc/openchamber-relay/env`) |

**OpenChamber image (tracks upstream, decoupled from this repo's git tags)**

| Trigger | What happens |
|---|---|
| every 6h poll of npm `@openchamber/web` | diffs npm versions against the image tags already in GHCR and builds every missing one, newest first (rolling window of 20 recent versions; manual dispatch can override version/window) |

Every upstream release version gets exactly one `openchamber:vX.Y.Z` image tag; `openchamber:latest` always points at the newest upstream version. This repo's git tag space stays untouched by upstream. Very old npm versions that no longer install cleanly fail and are skipped automatically — only the latest version is mandatory.

Updating a deployment:

```bash
docker compose pull && docker compose up -d
# Mode B: docker compose -f docker-compose.relay.yml pull && docker compose -f docker-compose.relay.yml up -d
```

## Building locally (without GHCR)

Swap a service's `image:` line for the commented `build:` block in the
compose file; the OpenChamber image accepts an `OPENCHAMBER_VERSION` build
arg to pin the npm version.

## Known trade-offs

- `RELAY_VERIFY_AUTH=false` (default): the protocol supports host-side
  ECDSA P-256 signature verification, but clients have not adopted it
  broadly yet; security currently rests on end-to-end encryption.
- Containers run as UID 1000 (inherited from the node base image); `./data`
  directory permissions must match.
