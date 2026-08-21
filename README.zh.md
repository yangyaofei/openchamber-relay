# OpenChamber Relay

[English](README.md) | [中文](README.zh.md)

自研的 Go WebSocket Relay，让公网上的 [OpenChamber](https://github.com/openchamber/openchamber)
客户端（手机 App、其他设备上的浏览器）穿透 NAT/防火墙，连到你的
OpenChamber 实例。Relay 不看业务数据：客户端与 OpenChamber 之间端到端
加密（ECDH + AES-GCM，配对时协商），Relay 只做 socket 配对与透传。
协议细节见 [relay/README.md](relay/README.md)。

## 第一步：选部署形态

两种形态，跟安装方式无关，先选定：

| | 形态 A：Relay + OpenChamber 一体 | 形态 B：纯 Relay |
|---|---|---|
| 跑什么 | OpenChamber 服务 + Relay 同机 | 只有 Relay |
| 适合谁 | 家里/内网一台机器全包 | 公网 VPS，给所有设备做公共中继 |
| 谁接入 | 本机的 OpenChamber | 任意 OpenChamber 实例（桌面 App / server），把 Relay 地址填进它的设置 |

```
形态 A（一体）                            形态 B（纯 Relay）
公网客户端                                公网客户端
│ wss                                    │ wss
▼                                        ▼
宿主机反向代理 (:443)                     Relay（公网 VPS）
├─ /relay/* ─► relay                      │ 只做配对+透传
└─ /*       ─► openchamber                │ 流量端到端加密
                  │                       ▼
                  ▼                     任意 OpenChamber 实例
          OpenCode server (:4096)        （桌面 App 或 server，
                                         各自作为 host 接入）
```

## 第二步：选安装与运行方式

| 安装方式 | 形态 A | 形态 B | 运行方式 |
|---|---|---|---|
| [Docker Compose](#方式一docker-compose推荐) | `docker-compose.yml` | `docker-compose.relay.yml` | 容器 |
| [安装包 / 二进制](#方式二安装包--二进制裸机) | Relay 用包 + OpenChamber 用官方包/桌面 App | 只装 Relay | systemd 或命令行直接跑 |
| [源码构建](#方式三源码构建) | 同上 | `go build` | 同上 |

三种安装方式覆盖两种形态：Relay 本身用哪种装都行，区别只在 OpenChamber
那一半——Docker 里有现成镜像，裸机则直接用官方 npm 包或桌面 App。

### 方式一：Docker Compose（推荐）

前置：Docker + Compose v2。

```bash
git clone https://github.com/yangyaofei/openchamber-relay.git
cd openchamber-relay
cp .env.example .env
$EDITOR .env
```

- **形态 A**：宿主机需已运行 `opencode serve`（默认 `:4096`，容器经
  `host.docker.internal` 访问）。`.env` 里至少改两个密码，然后：

  ```bash
  docker compose up -d
  curl http://127.0.0.1:23001/health    # relay 健康检查
  ```

  浏览器打开 `http://127.0.0.1:23000`（密码见 `OPENCHAMBER_UI_PASSWORD`）。
- **形态 B**：`RELAY_BIND=0.0.0.0`（或在反向代理后面跑），然后：

  ```bash
  docker compose -f docker-compose.relay.yml up -d
  ```

镜像来自 GHCR，随本仓库 release 更新；部署机更新用
`docker compose pull && docker compose up -d`。

### 方式二：安装包 / 二进制（裸机）

Relay 是单个静态二进制，无任何运行时依赖。从
[releases](https://github.com/yangyaofei/openchamber-relay/releases)
获取（tar.gz 里同时带二进制和两个 systemd unit 文件），或
`go install github.com/yangyaofei/openchamber-relay/relay@latest`。

**deb / rpm**（装时一次 sudo，system / user scope 自选）：

```bash
sudo apt install ./openchamber-relay_<version>_linux_amd64.deb   # 或 dnf install .rpm
```

包内容：二进制 `/usr/bin/openchamber-relay`、两套 systemd unit、
`/etc/openchamber-relay/env`。安装后不自动启动，自选 scope：

```bash
sudo systemctl enable --now openchamber-relay          # system scope
# 或
systemctl --user enable --now openchamber-relay        # user scope（免 sudo）
loginctl enable-linger "$USER"                         # user scope 开机自启
```

**tar.gz**（完全无 sudo）：

```bash
tar -xzf openchamber-relay_<version>_linux_amd64.tar.gz
mkdir -p ~/.local/bin && mv openchamber-relay ~/.local/bin/
mkdir -p ~/.config/systemd/user
mv openchamber-relay-user.service ~/.config/systemd/user/openchamber-relay.service
systemctl --user daemon-reload && systemctl --user enable --now openchamber-relay
loginctl enable-linger "$USER"
```

**命令行直接跑**（不配 systemd，前台运行）：

```bash
PORT=8080 ./openchamber-relay
```

**改配置**：user unit 用 drop-in 覆盖（`~/.config/systemd/user/openchamber-relay.service.d/override.conf`
写 `[Service]\nEnvironment=PORT=9000`，然后 daemon-reload + restart）；
system scope 编辑 `/etc/openchamber-relay/env` 后 restart。

**形态 A 裸机**：Relay 用上面的方式跑；同机的 OpenChamber 直接用官方
npm 包（`npm install -g @openchamber/web` 后 `openchamber serve`）或
桌面 App 充当 host，Relay 地址按下一节填进它的设置。

### 方式三：源码构建

```bash
git clone https://github.com/yangyaofei/openchamber-relay.git
cd openchamber-relay/relay
go build -o openchamber-relay .
PORT=8080 ./openchamber-relay
```

构建出的二进制与 release 版等价，按方式二的 systemd / 命令行方式运行
即可（unit 文件在 [packaging/](packaging/)）。Docker 镜像同样可以从
本仓库源码构建：把 compose 里的 `image:` 换成注释掉的 `build:` 块，
OpenChamber 镜像支持 `OPENCHAMBER_VERSION` build arg 指定 npm 版本。

## 对外暴露（通用）

容器默认只绑回环地址。Relay 的 WebSocket 端点是 `/ws`（健康检查
`/health`），对外怎么暴露由你自己决定：

- **反向代理 + TLS（推荐）**：用任意支持 WebSocket 的代理（Nginx、Caddy、
  Traefik 等）把流量转到 Relay 端口，客户端即可用 `wss://` 接入；
- **Tailscale Funnel**：机器已在你的 tailnet 里的话，`tailscale funnel 8080`
  一条命令发布端口并自带 TLS——免域名、免证书、免公网 IP；
- **裸跑**：`RELAY_BIND=0.0.0.0`，客户端用 `ws://` 直连（明文，仅测试用）。

客户端填的 Relay 地址由你的暴露方式决定（域名/路径/端口自定）。

## 把 OpenChamber 实例指向你的 Relay

远程客户端只是接入方，OpenChamber 实例本身（server 部署或桌面 App——
它内嵌同一个 server 进程）也要作为 host 拨到你的 Relay。OpenChamber
默认使用官方 `wss://relay.openchamber.dev/ws`，切换到自建 Relay 有两种
方式：

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
覆盖设置文件里的值（UI 中显示为锁定）。形态 A 的 compose 传给
openchamber 容器的就是它。

Host 侧设置好后，手机客户端照常扫码配对即可：配对 offer 会自动带上
你的 Relay 地址，客户端自动继承，无需在每台设备上单独配 Relay。

## 配置参考

Relay 全部配置就三个环境变量（无配置文件、无命令行参数）：

| 变量 | 默认 | 作用 |
|---|---|---|
| `PORT` | `8080` | 监听端口 |
| `RELAY_VERIFY_AUTH` | `false` | 校验 host 连接的 ECDSA P-256 签名（详见已知取舍） |
| `RELAY_SERVICE_NAME` | `openchamber-relay` | `/health` JSON 里的 `service` 字段 |

其余行为参数为编译期常量（单帧上限 64MB、每 host 客户端上限 128、
ping 30s / pong 90s、host 掉线宽限 60s 等），见 `relay/relay.go` 顶部
注释。Docker 编排的全部可配置项（OpenChamber 密码、端口、挂载等）见
`.env.example`。

## 仓库结构

```
openchamber-relay/
├── docker-compose.yml          形态 A：OpenChamber + Relay 一体
├── docker-compose.relay.yml    形态 B：纯 Relay
├── .env.example                编排全部可配置项与注释
├── docker/                     两个镜像的 Dockerfile
├── packaging/                  systemd unit（system / user）与 env 文件
└── relay/                      Go WebSocket Relay 源码 + 测试
```

## 已知取舍

- `RELAY_VERIFY_AUTH=false`（默认）：开启后，host 连接（host-control /
  host-data）必须携带对 `{ts}.{serverId}.{role}.{connectionId}` 的
  ECDSA P-256 签名，且 serverId 与签名密钥密码学绑定（与官方 relay
  同一套方案），可阻止他人在你的 relay 上冒占 serverId。因流量本身
  端到端加密，默认关闭；官方 host 始终发送签名，可随时开启。
- 容器以 UID 1000 运行（复用 node 基础镜像用户），`./data` 目录权限需匹配。
