# Wonder Mesh Net - System Guide

## 1. 项目定位

**一句话**: 让散落在各处的机器 (家里、办公室、云上) 能被 PaaS 平台统一管理。

**解决的核心问题**:
- 机器在 NAT 后面，没有公网 IP
- 机器 IP 动态变化
- 防火墙阻挡入站连接

**我们做什么**:
- 身份认证 (OIDC)
- 安全组网 (WireGuard mesh via Tailscale/Headscale)
- Join Token 机制

**我们不做什么**:
- 应用部署 (交给 K8s, Coolify, Zeabur 等)
- 容器编排
- 服务发现

---

## 2. 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                         用户浏览器                               │
└─────────────────────────────────────────────────────────────────┘
                                │
                                │ 1. OIDC 登录
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                       单容器部署                                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                      Coordinator                          │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │  │
│  │  │ OIDC Auth   │  │ Session Mgmt│  │ Join Token  │        │  │
│  │  │ (多Provider) │  │ (SQLite)    │  │ (JWT)       │        │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘        │  │
│  │                          │                                 │  │
│  │          ┌───────────────┴───────────────┐                │  │
│  │          ▼                               ▼                │  │
│  │  ┌─────────────────────┐  ┌─────────────────────────────┐ │  │
│  │  │  Embedded Headscale │  │ Headscale Integration       │ │  │
│  │  │  (进程管理)          │  │ (gRPC 客户端)                │ │  │
│  │  │                     │  │                              │ │  │
│  │  │  - Start()          │  │  - CreateUser()             │ │  │
│  │  │  - Stop()           │  │  - CreatePreAuthKey()       │ │  │
│  │  │  - WaitReady()      │  │  - ListNodes()              │ │  │
│  │  │                     │  │  - SetPolicy()              │ │  │
│  │  └─────────────────────┘  └─────────────────────────────┘ │  │
│  │          │ 管理进程                │ gRPC 调用            │  │
│  │          └───────────────┬─────────┘                      │  │
│  │                          ▼                                 │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                 Headscale (子进程)                         │  │
│  │  - 独立的 SQLite 数据库                                    │  │
│  │  - HTTP: :8080  |  gRPC: :50443                           │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                                │
                    ┌───────────┴───────────┐
                    ▼                       ▼
            ┌─────────────┐         ┌─────────────┐
            │   Worker A  │ ◄─────► │   Worker B  │
            │ (tailscale) │  Mesh   │ (tailscale) │
            └─────────────┘         └─────────────┘
```

---

## 3. 核心概念

### 3.1 多租户隔离

每个 OIDC 用户对应一个 Headscale "user" (命名空间):

```
Tenant ID = SHA256(issuer + subject)[:12]
Headscale User = "tenant-{tenant_id}"

例如:
- GitHub 用户 12345 → tenant-a1b2c3d4e5f6
- Google 用户 xyz   → tenant-9f8e7d6c5b4a
```

**隔离效果**: 不同租户的机器在 mesh 网络中互不可见。

### 3.2 认证流程

```
用户                Coordinator              OIDC Provider        Headscale
 │                      │                         │                   │
 │ GET /auth/login      │                         │                   │
 │ ?provider=github     │                         │                   │
 │─────────────────────►│                         │                   │
 │                      │                         │                   │
 │◄─────────────────────│                         │                   │
 │  302 → GitHub OAuth  │                         │                   │
 │                      │                         │                   │
 │ (用户在 GitHub 登录)  │                         │                   │
 │─────────────────────────────────────────────►│                   │
 │                      │                         │                   │
 │◄─────────────────────────────────────────────│                   │
 │  302 → /auth/callback?code=xxx               │                   │
 │                      │                         │                   │
 │ GET /auth/callback   │                         │                   │
 │─────────────────────►│                         │                   │
 │                      │ Exchange code           │                   │
 │                      │────────────────────────►│                   │
 │                      │◄────────────────────────│                   │
 │                      │ ID Token + User Info    │                   │
 │                      │                         │                   │
 │                      │ GetOrCreateUser         │                   │
 │                      │─────────────────────────────────────────────►
 │                      │◄─────────────────────────────────────────────
 │                      │                         │                   │
 │                      │ Create Session (DB)     │                   │
 │                      │                         │                   │
 │◄─────────────────────│                         │                   │
 │  session=abc123      │                         │                   │
```

### 3.3 Worker 加入流程

```
用户 (已登录)        Coordinator                Headscale
 │                      │                           │
 │ POST /api/v1/join-token                          │
 │ X-Session-Token: abc │                           │
 │─────────────────────►│                           │
 │                      │                           │
 │◄─────────────────────│                           │
 │  {token: "eyJ..."}   │  (JWT, 包含 tenant info)  │
 │                      │                           │

Worker 机器           Coordinator                Headscale
 │                      │                           │
 │ POST /api/v1/worker/join                         │
 │ {token: "eyJ..."}    │                           │
 │─────────────────────►│                           │
 │                      │ 验证 JWT                  │
 │                      │                           │
 │                      │ CreatePreAuthKey          │
 │                      │──────────────────────────►│
 │                      │◄──────────────────────────│
 │                      │                           │
 │◄─────────────────────│                           │
 │  {authkey: "xxx",    │                           │
 │   headscale_url}     │                           │
 │                      │                           │
 │ tailscale up --authkey=xxx                       │
 │─────────────────────────────────────────────────►│
 │                      │                           │
 │◄─────────────────────────────────────────────────│
 │  (机器加入 mesh 网络)  │                          │
```

---

## 4. 代码结构

```
wonder-mesh-net/
├── cmd/wonder/              # CLI 入口
│   ├── main.go              # Cobra root command
│   ├── coordinator.go       # coordinator 子命令
│   └── worker.go            # worker 子命令 (join/status/leave)
│
├── app/coordinator/         # Coordinator 应用层
│   ├── server.go            # Server 结构体, 初始化所有依赖, gRPC 连接
│   ├── bootstrap.go         # HTTP 路由注册
│   ├── config.go            # 配置结构
│   └── handlers/
│       ├── auth.go          # /auth/* 路由处理
│       ├── worker.go        # /api/v1/worker/* 路由处理
│       ├── nodes.go         # /api/v1/nodes 路由处理
│       └── health.go        # /health 路由处理
│
├── pkg/                     # 可复用的包
│   ├── database/            # 数据库层
│   │   ├── manager.go       # DB 连接 + 迁移
│   │   ├── migrations/      # SQL 迁移文件
│   │   └── queries/         # sqlc 查询定义
│   │
│   ├── headscale/           # Headscale 管理
│   │   ├── process.go       # Embedded Headscale 进程管理器
│   │   ├── tenant.go        # 租户管理 (使用 gRPC SDK)
│   │   └── acl.go           # ACL 策略管理 (使用 gRPC SDK)
│   │
│   ├── jointoken/           # Join Token (JWT)
│   │   └── generator.go     # 生成和验证
│   │
│   └── oidc/                # OIDC 认证
│       ├── provider.go      # Provider 接口 + 实现
│       ├── store.go         # AuthStateStore 接口
│       ├── db_store.go      # AuthStateStore DB 实现
│       ├── session.go       # SessionStore 接口 + 实现
│       └── user_store.go    # UserStore 接口 + 实现
│
├── configs/                 # 配置文件
│   └── headscale-embedded.yaml  # 嵌入模式 Headscale 默认配置
│
├── e2e/                     # 端到端测试
│   ├── docker-compose.yml   # Headscale + Keycloak + Coordinator
│   ├── test.sh              # 自动化测试脚本
│   └── keycloak-realm.json  # Keycloak 测试配置
│
└── Dockerfile               # 单容器部署 (嵌入模式)
```

---

## 5. 数据模型

### 5.1 数据库 Schema (SQLite)

```sql
-- OIDC 认证状态 (防 CSRF)
CREATE TABLE auth_states (
    state TEXT PRIMARY KEY,
    nonce TEXT NOT NULL,
    redirect_uri TEXT NOT NULL,
    provider_name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL
);

-- 用户会话
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,           -- 随机生成的 session ID
    user_id TEXT NOT NULL,         -- 关联 users.id
    issuer TEXT NOT NULL,          -- OIDC issuer
    subject TEXT NOT NULL,         -- OIDC subject
    created_at TIMESTAMP NOT NULL,
    last_used_at TIMESTAMP NOT NULL
);

-- 用户信息
CREATE TABLE users (
    id TEXT PRIMARY KEY,              -- = tenant ID
    headscale_user TEXT NOT NULL,     -- Headscale 中的 user name
    headscale_user_id INTEGER NOT NULL, -- Headscale 中的 user ID (用于 gRPC API)
    issuer TEXT NOT NULL,
    subject TEXT NOT NULL,
    email TEXT,
    name TEXT,
    picture TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(issuer, subject)
);
```

### 5.2 Join Token (JWT) 结构

```json
{
  "iss": "http://localhost:9080",      // Coordinator URL
  "sub": "tenant-a1b2c3d4e5f6",        // Headscale user name
  "exp": 1234567890,                   // 过期时间
  "iat": 1234567800,                   // 签发时间
  "headscale_url": "http://...",       // Worker 用于连接 Headscale
  "tenant_id": "a1b2c3d4e5f6"          // 租户 ID
}
```

---

## 6. API 端点

### 认证相关

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/auth/providers` | 列出可用的 OIDC providers |
| GET | `/auth/login?provider=xxx` | 开始 OIDC 登录流程 |
| GET | `/auth/callback` | OIDC 回调处理 |
| GET | `/auth/complete` | 登录完成页面 |

### API (需要 X-Session-Token)

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/v1/join-token` | 创建 Join Token |
| POST | `/api/v1/authkey` | 直接创建 Headscale AuthKey |
| GET | `/api/v1/nodes` | 列出当前用户的节点 |

### Worker API

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/v1/worker/join` | 用 Join Token 换取 AuthKey |

### 健康检查

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/health` | 健康检查 |

---

## 7. 配置项

### 环境变量

```bash
# OIDC Providers (至少配一个)
GITHUB_CLIENT_ID=xxx
GITHUB_CLIENT_SECRET=xxx

GOOGLE_CLIENT_ID=xxx
GOOGLE_CLIENT_SECRET=xxx

OIDC_ISSUER=https://...     # 通用 OIDC
OIDC_CLIENT_ID=xxx
OIDC_CLIENT_SECRET=xxx

# 可选
JWT_SECRET=xxx              # 不设则随机生成
```

### CLI 参数

```bash
wonder coordinator \
  --listen :9080 \
  --public-url http://localhost:9080
```

Headscale 作为子进程自动管理，使用内置默认配置。

---

## 8. 当前状态 & 待办

### ✅ 已完成

- [x] Coordinator 核心功能
- [x] OIDC 多 Provider 支持
- [x] 数据库持久化 (SQLite + goose + sqlc)
- [x] Session/User 管理
- [x] Join Token 机制
- [x] Worker CLI (`wonder worker join/status/leave`)
- [x] 嵌入式 Headscale (单容器部署)
- [x] Headscale 进程管理器 (Embedded Headscale)
- [x] Headscale gRPC 集成 (Headscale Integration)
- [x] Dockerfile 单容器部署

### ❌ 未完成
- [ ] 单元测试
- [ ] HTTPS 支持
- [ ] 日志结构化
- [ ] Metrics (Prometheus)
- [ ] Session 过期清理 (后台任务)

---

## 9. 运行方式

### Docker

```bash
docker build -t wonder-mesh-net .

docker run -d \
  -p 8080:8080 \
  -p 9080:9080 \
  -e GITHUB_CLIENT_ID=xxx \
  -e GITHUB_CLIENT_SECRET=xxx \
  -v wonder-data:/data \
  wonder-mesh-net
```

### 本地开发

```bash
# 需要 headscale 二进制在 PATH 中
export GITHUB_CLIENT_ID=xxx
export GITHUB_CLIENT_SECRET=xxx

go run ./cmd/wonder coordinator \
  --listen :9080 \
  --public-url http://localhost:9080
```

### E2E 测试

```bash
cd e2e
./test.sh
```

---

## 10. 设计决策记录

### 为什么用 SQLite?

- 单二进制部署，无需额外数据库
- WAL 模式支持并发读
- 对于 coordinator 的数据量足够

### 为什么用 JWT 作为 Join Token?

- 无状态，不需要存储
- 自包含租户信息
- 可设置过期时间
- Worker 可离线验证基本结构

### 为什么每个 OIDC 用户对应一个 Headscale user?

- 利用 Headscale 原生的 user 隔离机制
- ACL 规则可以按 user 配置
- 简化实现，不需要自己管理设备到租户的映射

### 为什么用嵌入式 Headscale?

- 单容器部署，适合 Railway/Zeabur 等 PaaS 平台
- 无需单独管理 Headscale 实例
- 通过 gRPC API 控制，不共享数据库，保持独立性
- Headscale 只需要 HTTP 端口，DERP 使用公共服务器

### 为什么用 gRPC 而不是 CLI?

- 使用 Headscale 官方 gRPC SDK (`github.com/juanfont/headscale/gen/go/headscale/v1`)
- 类型安全，编译时检查
- 比 exec 子进程调用 CLI 更高效
- 避免解析命令行输出的脆弱性
