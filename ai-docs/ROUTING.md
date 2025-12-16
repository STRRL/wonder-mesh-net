# Wonder Mesh Net 路由设计

## 当前路由结构

### Coordinator API（我们自己的）

| 路径 | 用途 |
|------|------|
| `/livez` | Kubernetes存活探针 |
| `/health` | 健康检查 |
| `/auth/providers` | 列出可用的OIDC providers |
| `/auth/login` | 开始OIDC登录流程 |
| `/auth/callback` | OIDC回调处理 |
| `/auth/complete` | 登录完成页面 |
| `/api/v1/authkey` | 创建Headscale AuthKey |
| `/api/v1/nodes` | 列出当前租户的节点 |
| `/api/v1/join-token` | 创建Worker加入令牌 |
| `/api/v1/worker/join` | Worker用令牌换取AuthKey |

### Headscale 路径

| 路径 | 协议 | 用途 |
|------|------|------|
| `/ts2021` | Noise over WebSocket | Tailscale控制面通信（主要） |
| `/machine/*` | Legacy HTTP | 旧版机器注册（已弃用） |
| `/key` | HTTP | 获取服务器公钥 |
| `/health` | HTTP | Headscale健康检查 |
| `/derp/*` | HTTP/WebSocket | DERP中继服务 |
| `/bootstrap-dns` | HTTP | DNS引导 |

---

## TS2021 协议说明

### 什么是 TS2021？

TS2021 是 Tailscale 在 2021 年引入的新版控制面协议，用于替代旧的 HTTP-based 协议。

### 协议特点

```
┌─────────────────────────────────────────────────────────────┐
│                    TS2021 协议栈                             │
├─────────────────────────────────────────────────────────────┤
│  Application Layer:  Tailscale Control Messages            │
├─────────────────────────────────────────────────────────────┤
│  Security Layer:     Noise Protocol (加密 + 认证)           │
├─────────────────────────────────────────────────────────────┤
│  Transport Layer:    WebSocket                              │
├─────────────────────────────────────────────────────────────┤
│  HTTP Layer:         POST /ts2021 → 101 Switching Protocols│
└─────────────────────────────────────────────────────────────┘
```

### 连接流程

```
Client (Tailscale)                    Server (Headscale)
       │                                     │
       │  POST /ts2021                       │
       │  Upgrade: websocket                 │
       │  Connection: Upgrade                │
       │────────────────────────────────────>│
       │                                     │
       │  HTTP 101 Switching Protocols       │
       │<────────────────────────────────────│
       │                                     │
       │  ══════ WebSocket 建立 ══════       │
       │                                     │
       │  Noise IK Handshake                 │
       │  (客户端用AuthKey或已知密钥认证)      │
       │<═══════════════════════════════════>│
       │                                     │
       │  ══════ Noise加密通道建立 ══════     │
       │                                     │
       │  MapRequest (请求网络配置)           │
       │────────────────────────────────────>│
       │                                     │
       │  MapResponse (返回peers、路由等)     │
       │<────────────────────────────────────│
       │                                     │
       │  (长连接，持续接收网络变更推送)        │
       │                                     │
```

### 为什么 Headscale 不支持路径前缀？

Headscale 直接监听根路径，内部硬编码了路由：

```go
// headscale 内部实现（简化）
mux.HandleFunc("/ts2021", handleTS2021)
mux.HandleFunc("/key", handleKey)
mux.HandleFunc("/health", handleHealth)
```

如果我们用 `/hs/ts2021` 代理，Headscale 会返回 404，因为它只认 `/ts2021`。

**官方文档明确说明**：
> Headscale does not support running behind a reverse proxy with a path prefix.

---

## 建议的新路由结构

将 Coordinator API 放到 `/coordinator/` 前缀下，根路径留给 Headscale：

```
单端口 :9080
│
├── /                              → Headscale (直接暴露)
│   ├── /ts2021                   → Noise over WebSocket
│   ├── /key                      → 公钥
│   ├── /health                   → Headscale健康检查
│   └── /derp/*                   → DERP中继
│
└── /coordinator/                  → Coordinator API (我们的)
    ├── /coordinator/health       → Coordinator健康检查
    ├── /coordinator/livez        → 存活探针
    ├── /coordinator/auth/*       → OIDC认证
    └── /coordinator/api/v1/*     → REST API
```

### 完整路径示例

| 完整路径 | 用途 |
|----------|------|
| `/coordinator/health` | Coordinator健康检查 |
| `/coordinator/livez` | Kubernetes存活探针 |
| `/coordinator/auth/providers` | 列出OIDC providers |
| `/coordinator/auth/login` | 开始OIDC登录 |
| `/coordinator/auth/callback` | OIDC回调 |
| `/coordinator/api/v1/join-token` | 创建Worker加入令牌 |
| `/coordinator/api/v1/worker/join` | Worker加入网络 |
| `/coordinator/api/v1/nodes` | 列出节点 |

### 实现方式

```go
// 1. 先匹配我们的前缀
if strings.HasPrefix(r.URL.Path, "/coordinator/") {
    // 去掉前缀，交给 coordinator handlers
    http.StripPrefix("/coordinator", coordinatorMux).ServeHTTP(w, r)
    return
}

// 2. 其他所有请求转发给 Headscale
headscaleProxy.ServeHTTP(w, r)
```

### 优势

1. **单端口** - 只需暴露 9080，简化部署和防火墙配置
2. **Headscale兼容** - 直接在根路径，无需任何修改
3. **清晰分离** - `/coordinator/` 前缀明确标识我们的API，一目了然
4. **易于扩展** - 未来可以加更多前缀如 `/admin/`

---

## 参考资料

- [Tailscale Control Protocol](https://tailscale.com/blog/how-tailscale-works)
- [Noise Protocol Framework](http://noiseprotocol.org/)
- [Headscale 反向代理说明](https://headscale.net/stable/ref/integration/reverse-proxy/)
