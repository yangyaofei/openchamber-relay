# OpenChamber Relay

[English](README.md) | [中文](README.zh.md)

自托管 [OpenChamber](https://github.com/openchamber/openchamber) 的部署编排与配套组件：
一个自研的 Go WebSocket Relay（`relay/`），加 Docker Compose 编排和
GitHub Actions CI/CD（上游 `@openchamber/web` 发版自动构建镜像）。

**目的**：让公网上的 OpenChamber 客户端（手机 App、其他设备上的浏览器）
穿透 NAT/防火墙，连到任意 OpenChamber 实例——无论是自托管 server 还是
你桌面上的 App。

## 两种用法

| | 用法 A：合体模式 | 用法 B：纯 Relay |
|---|---|---|
| 编排文件 | `docker-compose.yml` | `docker-compose.relay.yml` |
| 跑什么 | OpenChamber 服务 + Relay 同机 | 只有 Relay |
| 适合谁 | 自己家里/内网服务器一体化部署 | 公网 VPS 上给所有设备做中继 |
| 谁接入 | 本机的 OpenChamber（server 进程） | 任意 OpenChamber 实例（桌面 App / server），把 Relay 地址填进它的设置即可 |

```
用法 A（合体）                          用法 B（纯 Relay）
公网客户端                              公网客户端
│ wss                                  │ wss
▼                                      ▼
路由器 9527 ─► 内网 443                 Relay（公网 VPS）
▼                                        │ relay 只做配对+透传
宿主机反向代理 (:443)                    │ 流量端到端加密
├─ /relay/* ─► relay 容器 (23001)        ▼
└─ /*       ─► openchamber 容器        任意 OpenChamber 实例
                 │                       （桌面 App 或自托管 server，
                 │ OPENCODE_HOST=        各自作为 host 接入）
                 ▼
         宿主机 OpenCode server (:4096)
```

要点：

- **两个容器默认只绑回环**（`127.0.0.1:23000/23001`），对外暴露交给反向代理。
- **Relay 不看业务数据**：远程客户端与 OpenChamber 之间的流量端到端加密
  （ECDH + AES-GCM，配对时协商密钥），relay 只做 socket 配对与透传。
  详见 [relay/README.md](relay/README.md)。
- 用法 A 中 **OpenCode server 跑在宿主机**，容器通过
  `host.docker.internal:4096` 访问（compose 已自动注入）。
- **公网端口随意**：示例里 9527 只是路由器映射到本机 443 的公网侧端口，
  换成任何端口只需让 `OPENCHAMBER_RELAY_URL` 与之一致。

## 快速开始

### 用法 A：合体模式

前置：Docker + Compose v2；宿主机已运行 `opencode serve`（默认 `:4096`）。

```bash
git clone https://github.com/yangyaofei/openchamber-relay.git
cd openchamber-relay

cp .env.example .env
$EDITOR .env        # 至少改两个密码；按需改 hostname / relay url

docker compose up -d
curl http://127.0.0.1:23001/health    # relay 健康检查
```

浏览器打开 `http://127.0.0.1:23000`（密码见 `.env` 的
`OPENCHAMBER_UI_PASSWORD`）即可使用。

### 用法 B：纯 Relay

前置：一台公网机器（VPS）+ Docker。

```bash
git clone https://github.com/yangyaofei/openchamber-relay.git
cd openchamber-relay

cp .env.example .env
$EDITOR .env        # 按需改 RELAY_BIND / RELAY_PORT / RELAY_VERIFY_AUTH

docker compose -f docker-compose.relay.yml up -d
```

然后在任意 OpenChamber 实例（桌面 App 的远程配对设置、或用法 A 部署的
server 的 `OPENCHAMBER_RELAY_URL`）填入：

```
wss://relay.your.domain.example/relay/ws
```

### 对外暴露（两种用法通用，自行选择）

容器默认只绑回环地址。Relay 的 WebSocket 端点是 `/ws`（健康检查
`/health`），对外怎么暴露由你自己决定：

- **反向代理 + TLS（推荐）**：用任意支持 WebSocket 的代理（Nginx、Caddy、
  Traefik 等）把流量转到 `${RELAY_PORT}`，客户端即可用 `wss://` 接入；
- **裸跑**：`.env` 里 `RELAY_BIND=0.0.0.0`，客户端用 `ws://` 直连
  （明文，仅测试用）。

客户端填的 Relay 地址由你的暴露方式决定（域名/路径/端口自定）。

## 仓库结构

```
openchamber-relay/
├── docker-compose.yml          用法 A：OpenChamber + Relay 合体
├── docker-compose.relay.yml    用法 B：纯 Relay
├── .env.example                全部可配置项与注释
├── docker/
│   ├── openchamber.Dockerfile  OpenChamber 运行时镜像(ARG 版本参数化)
│   └── relay.Dockerfile        Relay 镜像(多阶段 Go 构建)
├── relay/                      自研 Go WebSocket Relay
│   ├── main.go / relay.go / auth.go / sweeper.go
│   ├── test/smoke.mjs          冒烟测试(不依赖真实密钥)
│   └── README.md
└── .github/workflows/
    ├── openchamber-image.yml   跟踪上游发版,自动构建 OpenChamber 镜像
    └── relay-image.yml         relay 代码变更即构建镜像
```

## CI / CD

| Workflow | 触发 | 产物 |
|---|---|---|
| `openchamber-image` | 每 6 小时轮询 npm `@openchamber/web`；支持手动指定版本 | `ghcr.io/<repo>/openchamber:vX.Y.Z` + `:latest`，并打 tag `openchamber-vX.Y.Z` 防重复构建 |
| `relay-image` | `relay/**` 变更合入 main；或打 `v*` tag | `ghcr.io/<repo>/relay:latest` / `:vX.Y.Z` |

部署机更新：

```bash
docker compose pull && docker compose up -d
# 用法 B 换成: docker compose -f docker-compose.relay.yml pull && docker compose -f docker-compose.relay.yml up -d
```

## 本地构建（不用 GHCR）

把 compose 里对应服务的 `image:` 换成注释掉的 `build:` 块即可；
OpenChamber 镜像支持 `OPENCHAMBER_VERSION` build arg 指定 npm 版本。

## 已知取舍

- `RELAY_VERIFY_AUTH=false`（默认）：协议已支持 host 端 ECDSA P-256 签名
  校验，但客户端侧尚未全面启用，当前依赖端到端加密保证安全。
- 容器以 UID 1000 运行（复用 node 基础镜像用户），`./data` 目录权限需匹配。
