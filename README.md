# OpenChamber Relay

[English](README.md) | [中文](README.zh.md)

A self-contained Go WebSocket relay that lets remote [OpenChamber](https://github.com/openchamber/openchamber)
clients (mobile apps, browsers on other machines) reach your OpenChamber
instance through NAT/firewalls. The relay never inspects application
traffic: clients and OpenChamber exchange end-to-end encrypted frames
(ECDH + AES-GCM, negotiated at pairing); the relay only pairs sockets and
forwards opaque messages. Protocol details in [relay/README.md](relay/README.md).

## Step 1: pick a deployment shape

Two shapes, independent of how you install things:

| | Shape A: Relay + OpenChamber on one box | Shape B: relay-only |
|---|---|---|
| Runs | OpenChamber service + relay, same machine | Just the relay |
| Fits | All-in-one on a home/lan machine | Public VPS serving all your devices |
| Who connects | The local OpenChamber | Any OpenChamber instance (desktop app / server), configured with your relay URL |

```
Shape A (all-in-one)                     Shape B (relay-only)
remote clients                           remote clients
│ wss                                    │ wss
▼                                        ▼
host reverse proxy (:443)                Relay (public VPS)
├─ /relay/* ─► relay                     │ pairing + forwarding only
└─ /*       ─► openchamber               │ traffic is E2E encrypted
                  │                      ▼
                  ▼                    any OpenChamber instance
          OpenCode server (:4096)       (desktop app or server,
                                        each dials in as host)
```

## Step 2: pick an install & run method

| Install | Shape A | Shape B | Runs as |
|---|---|---|---|
| [Docker Compose](#method-1-docker-compose-recommended) | `docker-compose.yml` | `docker-compose.relay.yml` | container |
| [Package / binary](#method-2-package--binary-bare-metal) | relay via package + OpenChamber via official npm/desktop app | just the relay | systemd or plain CLI |
| [Build from source](#method-3-build-from-source) | same | `go build` | same |

Every install method supports both shapes — the relay itself installs the
same way; the only difference is the OpenChamber half, which has a ready
image under Docker and the official npm package / desktop app on bare metal.

### Method 1: Docker Compose (recommended)

Requires Docker + Compose v2.

```bash
git clone https://github.com/yangyaofei/openchamber-relay.git
cd openchamber-relay
cp .env.example .env
$EDITOR .env
```

- **Shape A**: the host must already run `opencode serve` (default `:4096`;
  the container reaches it via `host.docker.internal`). Set at least the two
  passwords in `.env`, then:

  ```bash
  docker compose up -d
  curl http://127.0.0.1:23001/health    # relay health check
  ```

  Open `http://127.0.0.1:23000` in a browser (password:
  `OPENCHAMBER_UI_PASSWORD`).
- **Shape B**: set `RELAY_BIND=0.0.0.0` (or run behind a reverse proxy), then:

  ```bash
  docker compose -f docker-compose.relay.yml up -d
  ```

Images come from GHCR and update with this repo's releases; upgrade a
deployment with `docker compose pull && docker compose up -d`.

### Method 2: package / binary (bare metal)

The relay is a single static binary with no runtime dependencies. Get it
from [releases](https://github.com/yangyaofei/openchamber-relay/releases)
(each tar.gz carries the binary **and** both systemd unit files), or
`go install github.com/yangyaofei/openchamber-relay/relay@latest`.

**deb / rpm** (one sudo at install; system or user scope, your choice):

```bash
sudo apt install ./openchamber-relay_<version>_linux_amd64.deb   # or dnf install the .rpm
```

Package contents: binary at `/usr/bin/openchamber-relay`, both systemd
units, `/etc/openchamber-relay/env`. Nothing autostarts; pick a scope:

```bash
sudo systemctl enable --now openchamber-relay          # system scope
# or
systemctl --user enable --now openchamber-relay        # user scope (sudo-free)
loginctl enable-linger "$USER"                         # autostart for user scope
```

**tar.gz** (fully sudo-free):

```bash
tar -xzf openchamber-relay_<version>_linux_amd64.tar.gz
mkdir -p ~/.local/bin && mv openchamber-relay ~/.local/bin/
mkdir -p ~/.config/systemd/user
mv openchamber-relay-user.service ~/.config/systemd/user/openchamber-relay.service
systemctl --user daemon-reload && systemctl --user enable --now openchamber-relay
loginctl enable-linger "$USER"
```

**Run it directly** (no systemd, foreground):

```bash
PORT=8080 ./openchamber-relay
```

**Configuration**: for the user unit use a drop-in override
(`~/.config/systemd/user/openchamber-relay.service.d/override.conf` with
`[Service]\nEnvironment=PORT=9000`, then daemon-reload + restart); for
system scope edit `/etc/openchamber-relay/env` and restart.

**Shape A on bare metal**: run the relay as above; the co-located
OpenChamber is simply the official npm package
(`npm install -g @openchamber/web`, then `openchamber serve`) or the
desktop app acting as host — point it at your relay per the next section.

### Method 3: build from source

```bash
git clone https://github.com/yangyaofei/openchamber-relay.git
cd openchamber-relay/relay
go build -o openchamber-relay .
PORT=8080 ./openchamber-relay
```

The built binary is equivalent to the release one; run it via the systemd /
CLI patterns of Method 2 (unit files live in [packaging/](packaging/)).
The Docker images can be built from this repo too: swap a service's
`image:` line for the commented `build:` block in the compose file; the
OpenChamber image accepts an `OPENCHAMBER_VERSION` build arg to pin the
npm version.

## Public exposure (any method)

Containers bind loopback by default. The relay's WebSocket endpoint is
`/ws` (health check at `/health`); how you expose it is up to you:

- **Reverse proxy + TLS (recommended)**: any WebSocket-capable proxy
  (Nginx, Caddy, Traefik, ...) forwarding to the relay port; clients then
  connect via `wss://`;
- **Tailscale Funnel**: if the machine is already in your tailnet,
  `tailscale funnel 8080` publishes the port with automatic TLS — no
  domain, certificate or public IP needed;
- **Bare**: set `RELAY_BIND=0.0.0.0`; clients connect with `ws://` in
  plaintext — testing only.

The relay URL clients use follows from your exposure choice
(domain/path/port are yours).

## Pointing an OpenChamber instance at your relay

Remote clients are just consumers; the OpenChamber instance itself (a
server deployment, or the desktop app — it embeds the same server process)
also dials your relay as a host. OpenChamber defaults to the official
`wss://relay.openchamber.dev/ws`; two ways to switch:

**Option 1: OpenChamber settings file** (`~/.config/openchamber/settings.json`,
shared by desktop and server; `$OPENCHAMBER_DATA_DIR` wins if set):

```json
{
  "privateRelay": {
    "enabled": true,
    "relayUrl": "wss://relay.your.domain.example/relay/ws"
  }
}
```

Restart OpenChamber after editing (or toggle the relay off/on in the
remote-pairing settings — same effect, the UI writes this file).

**Option 2: environment variable `OPENCHAMBER_RELAY_URL`** — a
deployment-level pin that overrides the stored setting entirely (shown as
locked in the UI). Shape A's compose passes exactly this to the
openchamber container.

Once the host side is set, phone clients pair by QR code as usual: the
pairing offer carries your relay URL automatically, clients inherit it,
no per-device relay configuration needed.

## Configuration reference

The relay's entire configuration is three environment variables (no config
file, no CLI flags):

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | listen port |
| `RELAY_VERIFY_AUTH` | `false` | verify host ECDSA P-256 signatures (see trade-offs) |
| `RELAY_SERVICE_NAME` | `openchamber-relay` | `service` field in the `/health` JSON |

Remaining behavior knobs are compile-time constants (64MB max frame, 128
clients per host, 30s ping / 90s pong, 60s host-disconnect grace, ...),
documented atop `relay/relay.go`. All Docker Compose knobs (OpenChamber
passwords, ports, mounts, ...) live in `.env.example`.

## Repository layout

```
openchamber-relay/
├── docker-compose.yml          Shape A: OpenChamber + relay, one box
├── docker-compose.relay.yml    Shape B: relay-only
├── .env.example                every compose knob, annotated
├── docker/                     Dockerfiles for both images
├── packaging/                  systemd units (system / user) + env file
└── relay/                      Go WebSocket relay source + tests
```

## Known trade-offs

- `RELAY_VERIFY_AUTH=false` (default): when enabled, host connections
  (host-control / host-data) must carry an ECDSA P-256 signature over
  `{ts}.{serverId}.{role}.{connectionId}` with `serverId` cryptographically
  bound to the signing key (same scheme as the official relay), blocking
  serverId squatting on your relay. Kept off by default since traffic is
  end-to-end encrypted anyway; official hosts always send the signature,
  so it can be turned on freely.
- Containers run as UID 1000 (inherited from the node base image); `./data`
  directory permissions must match.
