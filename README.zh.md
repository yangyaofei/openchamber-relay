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
- **Tailscale Funnel**：机器已在你的 tailnet 里的话，`tailscale funnel 8080`
  一条命令发布端口并自带 TLS——免域名、免证书、免公网 IP，客户端用分配的
  `https://<机器名>.<tailnet>.ts.net` 地址即可；
- **裸跑**：`.env` 里 `RELAY_BIND=0.0.0.0`，客户端用 `ws://` 直连
  （明文，仅测试用）。

客户端填的 Relay 地址由你的暴露方式决定（域名/路径/端口自定）。

## 把 OpenChamber 实例指向你的 Relay

远程客户端只是接入方，OpenChamber 实例本身（上面的 **server** 部署，
或**桌面 App**——它内嵌同一个 server 进程）也要作为 host 拨到你的
Relay。OpenChamber 默认使用官方 `wss://relay.openchamber.dev/ws`，
切换到自建 Relay 有两种方式：

**方式一：OpenChamber 设置文件**（`~/.config/openchamber/settings.json`，
桌面与 server 共用；若设置过 `OPENCHAMBER_DATA_DIR` 则以它为准）：

```json
{
  "privateRelay": {
    "enabled": true,
    "relayUrl": "wss://relay.your.domain.example/relay/ws"
  }
}
```

改完重启 OpenChamber（或在远程配对设置里把 Relay 关/开一次，效果相同，
UI 写的就是这个文件）。

**方式二：环境变量 `OPENCHAMBER_RELAY_URL`** —— 部署级锁定端点，完全
覆盖设置文件里的值（UI 中显示为锁定）。上面用法 A 的 compose 传给
openchamber 容器的就是它。

Host 侧设置好后，手机客户端照常扫码配对即可：配对 offer 会自动带上
你的 Relay 地址，客户端自动继承，无需在每台设备上单独配 Relay。

## 裸机部署（不用 Docker）

Relay 是单个静态二进制，Docker 完全可选。从
[releases](https://github.com/yangyaofei/openchamber-relay/releases) 下载
（每个 tar.gz 里同时带二进制**和两个 systemd unit 文件**），或
`go install github.com/yangyaofei/openchamber-relay/relay@latest`。

### 方式一：deb / rpm（装时一次 sudo，system / user scope 自选）

```bash
# deb
sudo apt install ./openchamber-relay_<version>_linux_amd64.deb
# rpm
sudo dnf install ./openchamber-relay_<version>_linux_amd64.rpm
```

包内容：二进制装到 `/usr/bin/openchamber-relay`、两套 systemd unit、
`/etc/openchamber-relay/env`。安装后不会自动启动，自选 scope：

```bash
# system scope（以动态临时用户运行，开机自启）
sudo systemctl enable --now openchamber-relay

# user scope（以当前用户运行，之后全程免 sudo）
systemctl --user enable --now openchamber-relay
loginctl enable-linger "$USER"   # 开机自启 / 登出不停止
```

### 方式二：tar.gz（完全无 sudo）

```bash
# 1. 解压，二进制放到 PATH
tar -xzf openchamber-relay_<version>_linux_amd64.tar.gz
mkdir -p ~/.local/bin && mv openchamber-relay ~/.local/bin/

# 2. 安装 user unit（两个 unit 文件都在 tar 包里）
mkdir -p ~/.config/systemd/user
mv openchamber-relay-user.service ~/.config/systemd/user/openchamber-relay.service
rm openchamber-relay.service openchamber-relay.env   # system scope 的文件，这里用不上

# 3. 启用
systemctl --user daemon-reload
systemctl --user enable --now openchamber-relay
loginctl enable-linger "$USER"   # 开机自启 / 登出不停止
```

如果还是要 system scope：`sudo cp openchamber-relay.service /etc/systemd/system/`、
`sudo cp openchamber-relay.env /etc/openchamber-relay/env`，然后
`sudo systemctl daemon-reload && sudo systemctl enable --now openchamber-relay`。

### 改配置

user unit 默认 `PORT=8080` / `RELAY_VERIFY_AUTH=false`，用 drop-in 覆盖，
不要直接改 unit：

```bash
mkdir -p ~/.config/systemd/user/openchamber-relay.service.d
printf '[Service]\nEnvironment=PORT=9000\n' \
  > ~/.config/systemd/user/openchamber-relay.service.d/override.conf
systemctl --user daemon-reload && systemctl --user restart openchamber-relay
```

system scope：编辑 `/etc/openchamber-relay/env` 后
`sudo systemctl restart openchamber-relay`。

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

两条互相独立的发布节奏，各管各的产物：

**Relay（本仓库自己的 release）**

| 触发 | 产物 |
|---|---|
| push 到 `master`（relay 有变更） | 多架构镜像 `relay:sha-<commit>` 推 GHCR，不带版本 tag |
| 打 tag `vX.Y.Z` | 镜像 `relay:vX.Y.Z` + `relay:latest` 推 GHCR，并由 GoReleaser 创建 GitHub Release：交叉编译二进制 `linux/{amd64,arm64,armv7,386}` + `darwin/{amd64,arm64}`（tar.gz，内含两个 systemd unit，附 checksums）及 `deb`/`rpm` 包（system + user 双 unit、`/etc/openchamber-relay/env`） |

**OpenChamber 镜像（跟踪上游，与本仓库 git tag 完全解耦）**

| 触发 | 产物 |
|---|---|
| 每 6 小时轮询 npm `@openchamber/web` | 把 npm 版本列表与 GHCR 已有镜像 tag 做差集，缺失的逐个构建（默认回补最近 20 个版本，新到旧；手动触发可指定版本/窗口） |

上游每个 release 版本恰好对应一个 `openchamber:vX.Y.Z` 镜像 tag；`openchamber:latest` 始终指向最新上游版本；上游发多少版本都不会在本仓库产生 git tag。过老的 npm 版本若已无法正常安装，构建失败会自动跳过——只有最新版本是必须成功的。

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
