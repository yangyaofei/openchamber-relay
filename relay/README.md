# OpenChamber Relay

一个极简、无状态的 WebSocket 中继，让公网上的 OpenChamber 客户端（手机 App、
其他机器上的浏览器）连接到 NAT/防火墙后的自托管 OpenChamber 服务器。

## 设计要点

- **不看业务数据**：客户端与服务器之间的帧是端到端加密的（ECDH + AES-GCM，
  密钥在配对时协商），relay 只做 socket 配对与透传。
- **三角色连接模型**（都走 `/ws`，靠 query 参数区分）：

  | role          | 数量             | 用途                                       |
  |---------------|------------------|--------------------------------------------|
  | host-control  | 每服务器 1 个     | 控制通道：client 上线/下线/同步通知          |
  | host-data     | 每 client 1 个    | 与对应 client 的数据通道                    |
  | client        | 任意（上限 128）  | 远程客户端入口，等待 host 打开配对 socket   |

- **自愈**：host 断线后有 60 秒宽限期（重连不清空 client）；后台 sweeper
  每 30 秒清理未配对超时和空闲超时的连接。
- **可选 host 签名校验**：`RELAY_VERIFY_AUTH=true` 时校验 ECDSA P-256 签名。

## 运行

```bash
go run .                       # 默认 :8080
PORT=9000 go run .             # 自定义端口
RELAY_VERIFY_AUTH=true go run .
```

健康检查：`curl http://127.0.0.1:8080/health`

## 冒烟测试

```bash
cd test && npm install && cd ..
go run &                        # 或任意方式启动 relay
node test/smoke.mjs             # 默认 ws://127.0.0.1:8080/ws
```

## Docker

```bash
docker build -f ../docker/relay.Dockerfile -t openchamber-relay .
```

## 环境变量

| 变量               | 默认    | 说明                          |
|--------------------|---------|-------------------------------|
| `PORT`             | `8080`  | 监听端口                       |
| `RELAY_VERIFY_AUTH`| `false` | 校验 host 连接的 ECDSA 签名    |
