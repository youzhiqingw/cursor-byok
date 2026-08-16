# Cursor助手 (cursor-byok) 技术规格说明书

## 1. 文档信息

| 项 | 内容 |
|---|---|
| 项目名称 | Cursor助手 (cursor-byok) |
| 版本 | 依据 `buildinfo.CurrentVersion()` 动态生成 |
| 编写日期 | 2026-03-14 |
| 目标读者 | 开发者、安全审计人员、贡献者 |

---

## 2. 项目概述

Cursor助手是一款本地代理 + Wails 桌面应用，允许用户携带自有 API 密钥（BYOK），将任意 OpenAI/Anthropic 兼容模型接入 Cursor IDE。其核心机制为：

1. **MITM 代理拦截**：通过本地 HTTPS 代理（`goproxy`）拦截 Cursor 发往 `api2.cursor.sh` 的流量；
2. **请求重写**：将拦截到的 Cursor 私有协议（ConnectRPC / Protobuf）转换为标准 OpenAI/Anthropic 请求；
3. **响应回灌**：将模型返回的流式响应翻译回 Cursor 期望的 `AgentServerMessage` SSE 流；
4. **本地后端**：提供配置管理、历史记录、工具执行、Provider 路由等能力。

---

## 3. 技术栈

| 层级 | 技术选型 | 说明 |
|---|---|---|
| 后端语言 | Go 1.25.0 | 模块名 `cursor` |
| 桌面框架 | Wails v3 (`alpha.74`) | 跨平台窗口、系统托盘、Go↔JS 绑定 |
| 前端框架 | Vue 3 + Vite + Tailwind CSS | 嵌入 `frontend/dist` 通过 `embed.FS` 提供 |
| RPC 协议 | ConnectRPC (`connectrpc.com/connect`) | 兼容 gRPC 的 HTTP/1.1 流式传输 |
| Protobuf | `google.golang.org/protobuf` | `agentv1` / `aiserverv1` 生成类型 |
| 数据库 | SQLite (`modernc.org/sqlite`) | 纯 Go 实现，无 CGO |
| 代理引擎 | `github.com/elazarl/goproxy` | MITM HTTPS 代理，动态签发证书 |
| 路由框架 | `github.com/go-chi/chi/v5` | 本地后端 HTTP 路由 |
| 配置存储 | YAML 文件 (`gopkg.in/yaml.v3`) | 用户配置持久化 |
| 构建工具 | Task (`taskfile.yml`) + Wails CLI | 跨平台打包 |

---

## 4. 系统架构

### 4.1 总体架构图

```
┌─────────────┐    HTTPS     ┌──────────────┐    HTTP     ┌─────────────────┐
│  Cursor IDE  │────────────▶│  MITM Proxy  │───────────▶│  Backend Server  │
│              │   :18080     │  (goproxy)   │   :18090    │  (chi router)    │
└─────────────┘              └──────────────┘             └────────┬────────┘
                                                                    │
                              ┌─────────────────────────────────────┘
                              │
                    ┌─────────▼──────────┐
                    │  Forwarder Module  │  核心请求管道
                    │  (service.go)      │
                    └─────────┬──────────┘
                              │
              ┌───────────────┼───────────────┐
              │               │               │
    ┌─────────▼──────┐ ┌─────▼──────┐ ┌──────▼───────┐
    │ Prompt Engine    │ │ Provider   │ │ Agent Core   │
    │ (compile &       │ │ Gateway    │ │ (tool exec,  │
    │  tool filter)    │ │  (router)  │ │  MCP, plans) │
    └────────────────┘ └─────┬──────┘ └──────────────┘
                             │
                   ┌─────────┼─────────┐
                   │                   │
          ┌────────▼──────┐  ┌────────▼──────┐
          │ OpenAI Adapter│  │Anthropic      │
          │ (chat/respons)│  │Adapter        │
          └───────────────┘  └───────────────┘
```

### 4.2 进程与端口

| 服务 | 默认地址 | 职责 |
|---|---|---|
| MITM Proxy | `127.0.0.1:18080` | 拦截 Cursor HTTPS 流量，动态 TLS 证书 |
| Backend Server | `127.0.0.1:18090` | 本地业务后端，ConnectRPC + HTTP 路由 |
| cursor-tab-server | `:8041`（独立二进制） | 远程 Tab 功能代理，转发至 Cursor 官方 API |

---

## 5. 核心模块规格

### 5.1 入口与启动 (`main.go` / `internal/app/runner.go`)

- **资源嵌入**：
  - `//go:embed all:frontend/dist` —— 前端构建产物；
  - `//go:embed build/appicon.png` / `build/tray.png` —— 应用图标与托盘图标。
- **启动流程**：
  1. `logger.Init()` 初始化日志；
  2. 加载嵌入式 CA 证书（`certs.EmbeddedCACertPEM`）；
  3. 创建 `mitm.NewProxyServer`（默认 `:18080` → 后端 `:18090`）；
  4. 注册 Wails 服务：`ProxyService`、`MetricsService`、`WindowService`；
  5. 创建主窗口（700×520，macOS 液态玻璃背景，Windows 无边框）；
  6. 创建系统托盘菜单（启动/停止服务、检查更新、显示/隐藏窗口、退出）；
  7. 应用启动后自动后台启动代理服务。

### 5.2 MITM 代理 (`internal/mitm/service.go`)

| 属性 | 说明 |
|---|---|
| 类型 | `ProxyServer` |
| 核心依赖 | `goproxy.ProxyHttpServer` |
| 证书管理 | `certs.Manager` 动态签发域名证书 |
| 上游重写 | 通过 `X-Server-Upstream-URL` 头将原始目标传递给后端 |
| 日志限流 | 基于时间窗口的日志限流器（默认 30s 窗口，5min TTL，1024 个 key） |
| 并发安全 | `sync.RWMutex` 保护 `baseURL`、`httpServer` 状态 |

### 5.3 后端主机 (`internal/backend/host.go`)

| 属性 | 说明 |
|---|---|
| 类型 | `Host` |
| 配置管理 | `serverconfig.Manager`（YAML 文件 + 热重载） |
| 健康检查 | `/healthz` 返回 `ok`；支持进程内与 loopback 双路径 |
| 路由协议 | `ProtocolHTTP`、`ProtocolConnectUnary`、`ProtocolConnectStream` |
| 路由模式 | `Local`（本地处理）/ `Upstream`（透传上游） |

**主要路由注册表**：

| 路由 | 协议 | 本地行为 | 上游行为 |
|---|---|---|---|
| `/aiserver.v1.BidiService/BidiAppend` | ConnectUnary | `LocalBidiHandler` | Direct |
| `/agent.v1.AgentService/RunSSE` | ConnectStream | `LocalRunSSE` | Direct |
| `/aiserver.v1.AiService/ServerTime` | ConnectUnary | MockProto | Direct |
| `/aiserver.v1.AiService/GetServerConfig` | ConnectUnary | MockProto | Direct |
| `/aiserver.v1.AiService/AvailableModels` | ConnectUnary | MockProto | Direct |
| `/aiserver.v1.AiService/StreamCpp` | ConnectStream | — | Tab Server |
| `/aiserver.v1.AiService/StreamNextCursorPrediction` | ConnectStream | — | Tab Server |
| `/aiserver.v1.DashboardService/*` | HTTP/Connect | MockProto/HTTP | Direct |
| `/oauth/token` | HTTP | MockOAuth | Direct |
| `/auth/*` | HTTP | MockJSON/FixedStatus | Direct |
| `/aiserver.v1.AiService/*` | HTTP | `AiHandler` | Direct |

### 5.4 配置管理 (`internal/backend/server/config/`)

#### 5.4.1 配置类型 (`types.go`)

```go
type Config struct {
    Log                       bool                 `json:"log" yaml:"log"`
    ProviderStreamIdleTimeout int                  `json:"providerStreamIdleTimeout" yaml:"providerStreamIdleTimeout"`
    BackendListenAddr         string               `json:"backendListenAddr" yaml:"backendListenAddr"`
    ProxyListenAddr           string               `json:"proxyListenAddr" yaml:"proxyListenAddr"`
    ModelAdapters             []ModelAdapterConfig `json:"modelAdapters" yaml:"modelAdapters"`
    Routing                   RoutingConfig        `json:"routing" yaml:"routing"`
    HomeMetrics               HomeMetricsConfig    `json:"homeMetrics" yaml:"homeMetrics"`
    LastAgentModelHash        string               `json:"lastAgentModelHash" yaml:"lastAgentModelHash"`
}
```

#### 5.4.2 模型适配器 (`ModelAdapterConfig`)

| 字段 | 类型 | 约束 |
|---|---|---|
| `ID` | string | 由 `baseURL`+`modelID`+`apiKey`+`displayName`+`openAIEndpoint` 哈希生成，不可重复 |
| `DisplayName` | string | 必填 |
| `Type` | string | 仅支持 `openai` / `anthropic` |
| `BaseURL` | string | 必填，经 `modelchannel.NormalizeBaseURL` 规范化 |
| `APIKey` | string | 必填 |
| `ModelID` | string | 必填 |
| `ReasoningEffort` | string | `low`/`medium`/`high`/`xhigh`/`max`（OpenAI） |
| `OpenAIEndpoint` | string | `/v1/responses`、`/v1/chat/completions`、`/custom` |
| `AnthropicThinkingEffort` | string | `low`/`medium`/`high`/`xhigh`/`max` |
| `ContextWindowTokens` | int | ≥0 |
| `MaxCompletionTokens` | int | ≥0 |

#### 5.4.3 配置存储 (`store.go`)

- 文件路径：由 `appdata` 解析的用户配置目录下的 `config.yaml`；
- 读写方式：`yaml.Marshal` / `yaml.Unmarshal`；
- 非原子写：直接覆盖文件，崩溃可能留下损坏配置。

### 5.5 请求转发器 (`internal/backend/forwarder/`)

#### 5.5.1 模块入口 (`module.go`)

```go
type Module struct {
    historyRoot string
    configs     *serverconfig.Manager
}
```

对外暴露四个主要 Handler：

| Handler | 职责 |
|---|---|
| `LocalBidiHandler` | 处理 BidiAppend 上行请求，归一化为内部事件 |
| `LocalRunSSE` | 处理 RunSSE 下行流，驱动 Provider 并回灌 SSE |
| `AiHandler` | 通用 AI 服务透传/处理 |
| `RepositoryServiceHandler` | 仓库同步服务 |
| `UploadServiceHandler` | 文档上传服务 |

#### 5.5.2 核心类型 (`types.go`)

**会话文件 (`ConversationFile`)**：
- 存储路径：`history/<conversationId>/state.json`
- 包含：会话元数据、历史条目 (`HistoryEntry`)、当前计划、待办事项、Provider 调用记录等。

**活跃流 (`ActiveStream`)**：
- 状态机：`created` → `streaming` → `completed`/`canceled`/`failed`
- 并发控制：`sync.Mutex` + `atomic.Uint64`
- 核心字段：
  - `ActorMailbox` —— Actor 模型命令邮箱；
  - `Subscribers` —— SSE 订阅者映射；
  - `PendingExecs` / `PendingInteractions` —— 未收口执行/交互桥；
  - `BackgroundShells` —— 后台 Shell 状态机。

#### 5.5.3 Provider 网关 (`provider.go`)

```go
type ProviderGateway interface {
    StartStream(ctx context.Context, req ProviderRequest, sink func(modeladapter.ModelEvent) error) error
}
```

- **路由逻辑**：根据 `ModelID` 匹配 `ModelAdapterConfig`，选择 `openai` 或 `anthropic` 适配器；
- **流式超时**：`ProviderStreamIdleTimeout`（默认 240s，最小 30s）；
- **重试机制**：Provider 错误时根据 `ProviderPassCount` 决定是否重试或切换渠道。

#### 5.5.4 模型适配器 (`internal/backend/agent/model/`)

**统一消息结构 (`Message`)**：

```go
type Message struct {
    Role                     string          `json:"role"`
    Content                  string          `json:"content"`
    ContentParts             []ContentPart   `json:"content_parts,omitempty"`
    ReasoningContent         string          `json:"reasoning_content,omitempty"`
    ReasoningSignature       string          `json:"reasoning_signature,omitempty"`
    ToolCalls                []ToolCallDescriptor `json:"tool_calls,omitempty"`
    ToolCallID               string          `json:"tool_call_id,omitempty"`
    Name                     string          `json:"name,omitempty"`
}
```

**统一模型事件 (`ModelEvent`)**：

| 事件类型 | 说明 |
|---|---|
| `text_delta` | 文本增量 |
| `thinking_delta` | 思考增量 |
| `thinking_completed` | 思考结束 |
| `partial_tool_call` | 工具调用开始，参数流式生成中 |
| `tool_call_delta` | 工具调用参数增量 |
| `tool_like_completed` | 工具意图完整收口 |
| `turn_finished` | 模型回合结束 |
| `provider_error` | Provider 错误 |

**适配器接口**：

```go
type ModelAdapter interface {
    Stream(ctx context.Context, req StreamRequest, sink func(ModelEvent) error) error
}
```

当前实现：
- `OpenAIAdapter` —— 支持 `/v1/chat/completions` 与 `/v1/responses`；
- `AnthropicAdapter` —— 支持 Messages API 与 thinking 扩展。

### 5.6 Agent 运行时 (`internal/backend/agent/core/`)

#### 5.6.1 运行状态机 (`types.go`)

```
IDLE → RESTORING → PREPARING_MODEL_INPUT → STREAMING_MODEL → WAITING_EXEC / WAITING_INTERACTION
  → APPLYING_EXTERNAL_RESULT → CHECKPOINTING → COMPLETED / CANCELED / FAILED
```

#### 5.6.2 命令类型 (`CommandKind`)

| 命令 | 说明 |
|---|---|
| `run_requested` | 收到 `AgentRunRequest` |
| `prewarm_requested` | 收到 `PrewarmRequest` |
| `cancel_requested` | 收到取消动作 |
| `exec_client_message` | 收到执行桥结果 |
| `interaction_response` | 收到交互桥结果 |
| `client_heartbeat` | 客户端心跳 |

#### 5.6.3 支持的工具清单

```
Read, Write, PatchEdit, Delete, Shell, AwaitShell, WriteShellStdin, ForceBackgroundShell,
Glob, Grep, ReadLints,
AskQuestion, CreatePlan, SwitchMode, WebSearch, WebFetch,
TodoWrite, Task,
CallMcpTool, FetchMcpResource
```

### 5.7 协议层 (`internal/backend/agent/protocol/`)

- **入站解码**：`inbound.go` 负责将 `AgentClientMessage` 解码为 `InboundIntent`；
- **消息类型**：
  - `run_request` / `prewarm_request` / `conversation_action` / `exec_client_message` / `exec_client_control_message` / `interaction_response` / `kv_client_message` / `client_heartbeat`
- **模式支持**：`AGENT` / `ASK` / `PLAN` / `DEBUG` / `MULTITASK`；
- **子代理模型覆盖**：`SubagentModelOverride` 支持 `model` / `inherit` / `disabled` 三种选择策略。

### 5.8 桥接服务 (`internal/bridge/`)

Wails v3 服务绑定，供前端 JavaScript 调用：

| 服务 | 方法 | 说明 |
|---|---|---|
| `ProxyService` | `StartProxy` / `StopProxy` / `GetState` | 代理生命周期 |
| `ProxyService` | `LoadUserConfig` / `SaveUserConfig` | 配置读写 |
| `ProxyService` | `TestModelAdapter` / `GetModelAdapterTestResults` | 模型测速 |
| `ProxyService` | `ActivateLicense` / `BindLicenseDevice` / `SwitchLicenseDevice` | 许可证 |
| `ProxyService` | `QueryUsageRecords` | 用量查询 |
| `MetricsService` | `GetHomeMetricsSummary` | 首页统计 |
| `WindowService` | `OpenModelConfigWindow` / `OpenModelEditorWindow` | 窗口管理 |
| `WindowService` | `GetAppVersion` / `CheckForUpdates` / `InstallReadyUpdate` | 更新管理 |

### 5.9 前端 (`frontend/`)

| 目录/文件 | 说明 |
|---|---|
| `src/App.vue` | 根组件，挂载 `MainLayout`、`MessageProvider`、`AdModelProvider` |
| `src/layouts/MainLayout.vue` | 主布局 |
| `src/views/Home.vue` | 首页（统计、状态） |
| `src/views/Config.vue` | 配置页 |
| `src/views/ModelConfig.vue` | 模型列表 |
| `src/views/ModelEditor.vue` | 模型编辑/新增 |
| `src/components/charts/CacheHitRateChart.vue` | 缓存命中率图表 |
| `src/components/ModelAdapterTestCard.vue` | 测速卡片 |
| `src/composables/useModal.ts` | 模态框状态管理 |
| `src/composables/useInputModal.ts` | 输入框状态管理 |
| `src/state/appState.ts` | 应用全局状态 |

---

## 6. 数据流规格

### 6.1 标准对话流（RunSSE）

```
Cursor IDE ──► MITM Proxy (:18080) ──► Backend Server (:18090)
                                              │
                                              ▼
                                    ┌─────────────────────┐
                                    │   Inbound Decoder   │
                                    │ (protocol/inbound)  │
                                    └──────────┬──────────┘
                                               │
                                    ┌──────────▼──────────┐
                                    │   Forwarder Module  │
                                    │   (service.go)      │
                                    │                     │
                                    │  1. Parse Intent    │
                                    │  2. Load History    │
                                    │  3. Compile Prompt  │
                                    │  4. Route Provider  │
                                    │  5. Stream Events   │
                                    │  6. Handle Tools    │
                                    │  7. Checkpoint      │
                                    └──────────┬──────────┘
                                               │
                                    ┌──────────▼──────────┐
                                    │   Provider Gateway  │
                                    │  (openai/anthropic) │
                                    └──────────┬──────────┘
                                               │
                                    ┌──────────▼──────────┐
                                    │   Model Provider    │
                                    │  (User's API Key)   │
                                    └─────────────────────┘
```

### 6.2 历史记录流

- **写入**：每轮 turn 结束后，`forwarder` 将 `HistoryEntry` 追加到 `history/<conversationId>/state.json`；
- **读取**：新请求到达时，根据 `conversation_id` 加载历史，重放为 `modeladapter.Message` 列表；
- **压缩**：当上下文超过阈值时，触发 `compaction.go` 进行历史摘要压缩；
- **索引**：`history_maintenance.go` 维护 SQLite 索引，支持按会话查询统计。

### 6.3 工具执行流

```
Model Event (tool_like_completed)
        │
        ▼
┌───────────────┐
│  Tool Gateway  │
│ (exec/interact)│
└───────┬───────┘
        │
   ┌────┴────┐
   ▼         ▼
Exec      Interaction
Bridge    Bridge
   │         │
   ▼         ▼
Cursor    用户/前端
IDE       交互
   │         │
   └────┬────┘
        ▼
  ExecClientMessage /
  InteractionResponse
        │
        ▼
  Forwarder (resume run)
```

---

## 7. 接口规格

### 7.1 ConnectRPC 路由接口

所有 ConnectRPC 路由遵循以下模式：

```
POST /<package>.<Service>/<Method>
Content-Type: application/json 或 application/proto
```

**关键过程**：

| 过程 | 请求类型 | 响应类型 | 流式 |
|---|---|---|---|
| `BidiAppend` | `AgentClientMessage` | `AgentServerMessage` | 否（Unary） |
| `RunSSE` | `AgentRunRequest` | `AgentServerMessage` | 是（SSE） |

### 7.2 HTTP Mock 接口

本地 Mock 接口用于替代 Cursor 官方服务端功能：

| 接口 | 方法 | 响应 |
|---|---|---|
| `GET /healthz` | — | `ok` |
| `POST /aiserver.v1.AiService/ServerTime` | — | `ServerTimeResponse`（Mock） |
| `POST /aiserver.v1.AiService/GetServerConfig` | — | `GetServerConfigResponse`（Mock） |
| `POST /aiserver.v1.AiService/AvailableModels` | — | `AvailableModelsResponse`（Mock） |
| `POST /oauth/token` | — | OAuth Token（Mock） |
| `GET /auth/full_stripe_profile` | — | Stripe Profile（Mock） |

### 7.3 Wails 服务接口（前端调用）

```typescript
// ProxyService
interface ProxyService {
  StartProxy(): Promise<ProxyState>;
  StopProxy(): Promise<ProxyState>;
  GetState(): ProxyState;
  LoadUserConfig(): Promise<UserConfig>;
  SaveUserConfig(cfg: UserConfig): Promise<void>;
  TestModelAdapter(adapter: ModelAdapterConfig): Promise<ModelAdapterTestResult>;
  GetModelAdapterTestResults(): ModelAdapterTestResult[];
  ActivateLicense(req: LicenseActionRequest): Promise<LicenseAPIResult>;
  QueryUsageRecords(req: UsageRecordsRequest): Promise<UsageRecordsResult>;
  ApplyCursorSettings(): Promise<void>;
  ClearCursorSettings(): Promise<void>;
}

// MetricsService
interface MetricsService {
  GetHomeMetricsSummary(): Promise<HomeMetricsSummary>;
}

// WindowService
interface WindowService {
  GetAppVersion(): string;
  CheckForUpdates(): void;
  InstallReadyUpdate(): Promise<void>;
  OpenConfigWindow(): void;
  OpenModelConfigWindow(): void;
  OpenModelEditorWindow(index: number, adapterJSON: string): void;
  GetModelEditorContext(): { index: number; adapterJSON: string };
}
```

---

## 8. 存储规格

### 8.1 文件系统布局

```
<AssistantHome>/
├── config.yaml              # 用户配置
├── history/
│   ├── <conversationId>/
│   │   ├── state.json       # 会话状态
│   │   ├── context.json     # 上下文快照
│   │   ├── debug/           # 调试日志（当 log: true）
│   │   │   ├── bidi.raw.jsonl
│   │   │   ├── bidi.decoded.jsonl
│   │   │   ├── runtime.jsonl
│   │   │   ├── provider.jsonl
│   │   │   └── runsse.jsonl
│   │   └── artifacts/       # LLM 工件
│   └── _debug/orphan/       # 未知 conversationId 的调试日志
├── usage.json               # 用量统计
└── logs/                    # 应用日志
```

### 8.2 SQLite 数据库 (`internal/cursor/state_db.go`)

- 驱动：`modernc.org/sqlite`（纯 Go）；
- 用途：Cursor 官方状态数据库的本地替代/代理；
- 查询方式：参数化查询（`database/sql`），无动态表名拼接风险。

---

## 9. 安全规格

### 9.1 TLS / 证书

- 嵌入式 CA 证书编译进二进制；
- 运行时通过 `certs.Manager` 动态签发域名证书；
- Windows 平台自动安装 CA 到系统信任 store（`internal/cursor/windows_cert.go`）；
- macOS 平台通过 `security add-trusted-cert` 安装（`internal/cursor/darwin_cert.go`）。

### 9.2 请求鉴权

- 本地后端 **不强制鉴权**（绑定在 `127.0.0.1`）；
- 上游请求携带用户配置的 `APIKey`（`Authorization: Bearer <apiKey>`）；
- `cursor-tab-server` 使用独立 Token 鉴权（YAML 配置）。

### 9.3 已知风险（摘要）

详见 `SECURITY_AUDIT.md`，核心风险包括：

| 风险 | 位置 | 等级 |
|---|---|---|
| 命令注入（PowerShell） | `internal/ads/service.go` | Critical |
| 路径遍历（ZIP 解压） | `internal/ads/service.go` | Critical |
| 任意文件写入 | `internal/backend/forwarder/file_store.go` | Critical |
| 竞态条件（订阅映射） | `internal/backend/forwarder/broker.go` | High |
| 资源泄漏（Response.Body） | `internal/backend/forwarder/service.go` | High |
| 敏感信息写入磁盘 | `internal/backend/forwarder/debug_recorder.go` | High |
| 动态 SQL 拼接 | `internal/backend/forwarder/history_maintenance.go` | Medium |

---

## 10. 构建与发布

### 10.1 构建命令

```bash
# 开发模式（热重载）
task dev

# 构建当前平台
task build

# 运行构建产物
task run

# 构建全平台（仅 macOS）
task build:all

# 前端开发
cd frontend && pnpm dev

# 前端构建
cd frontend && pnpm build
```

### 10.2 发布流程

```bash
# 准备发布资产
task release:prepare

# 验证资产完整性
task release:verify:assets

# 发布到 GitHub Releases
task release:github
```

### 10.3 支持平台

| 平台 | 架构 | 产物格式 |
|---|---|---|
| macOS | arm64 | `.dmg` / `.tar.gz` |
| macOS | amd64 | `.dmg` / `.tar.gz` |
| Windows | amd64 | `.zip` |
| Windows | 386 | `.zip` |
| Linux | amd64 | `.tar.gz` |

---

## 11. 附录

### 11.1 术语表

| 术语 | 说明 |
|---|---|
| BYOK | Bring Your Own Key，用户自备 API 密钥 |
| MITM | Man-In-The-Middle，中间人代理 |
| SSE | Server-Sent Events，服务端推送事件 |
| ConnectRPC | 基于 HTTP 的 gRPC 兼容协议 |
| Provider | 模型服务提供商（OpenAI/Anthropic 兼容） |
| Adapter | Provider 协议适配器 |
| Turn | 一次用户输入到模型回复完成的对话回合 |
| Run | 一次完整的 Agent 执行会话 |
| Bidi | Bidirectional，指 Cursor 的双向通信协议 |
| Tab Server | Cursor Tab 预测功能的远程代理服务 |

### 11.2 相关文档

- `README.md` —— 项目介绍与使用教程
- `SECURITY_AUDIT.md` —— 安全审计报告
- `CHANGELOG.md` —— 变更日志
- `release-notes.md` —— 发布说明源文件