# Cursor助手 Vibe Coding 经验文档

> **考察点**：是否能清晰定义项目方向、规划开发路径，是 Vibe Coding 的基础规划能力。在研发过程中踩过哪些坑？是如何和 AI 协作、后续避免踩坑的？

---

## 1. 项目背景与 Vibe Coding 定位

### 1.1 什么是 Cursor助手

Cursor助手 (`cursor-byok`) 是一个本地代理 + Wails 桌面应用，允许用户携带自有 API 密钥（BYOK），将任意 OpenAI/Anthropic 兼容模型接入 Cursor IDE。核心机制是通过 MITM 代理拦截 Cursor 的 API 流量，重写为标准 Provider 请求，再将响应翻译回 Cursor 期望的协议格式。

### 1.2 为什么选择 Vibe Coding

本项目是典型的 **"协议逆向 + 工程实现"** 型项目，具有以下特征：

| 特征 | 说明 |
|---|---|
| 协议不透明 | Cursor 使用私有 ConnectRPC/Protobuf 协议，需要持续逆向分析 |
| 版本迭代快 | Cursor 客户端频繁更新，协议和行为可能随时变化 |
| 状态机复杂 | Agent 运行时需要维护大量运行时状态（pending exec、interaction、checkpoint 等） |
| 多组件协同 | MITM 代理、后端服务、Provider 适配器、前端 UI 需要紧密配合 |
| 调试困难 | 问题可能出现在协议层、模型层、工具层或 UI 层，定位困难 |

**Vibe Coding 在此类项目中的价值**：
- 快速迭代：AI 辅助编写大量样板代码和适配逻辑
- 知识沉淀：将逆向分析结果编码为可复用的 Skill 和 Prompt
- 持续演进：通过结构化的 Agent 协作模式，降低协议变更的冲击

---

## 2. 项目方向定义：从混沌到清晰

### 2.1 初期方向模糊的问题

在项目早期，面临的核心困惑：

```
问题 1：到底要兼容多少 Cursor 功能？
  - 全部兼容？→ 工作量巨大，Cursor 更新即失效
  - 只兼容核心对话？→ 用户体验差，很多功能无法使用
  
问题 2：模型接入的边界在哪里？
  - 只支持 OpenAI 格式？→ 错过 Anthropic 生态
  - 支持所有 Provider？→ 维护成本爆炸
  
问题 3：本地模式 vs 上游模式的取舍？
  - 完全本地处理？→ 需要完整实现 Cursor 服务端逻辑
  - 部分透传上游？→ 需要处理认证和隐私问题
```

### 2.2 方向澄清的方法论

**方法 1：最小可行协议集（MVP Protocol Set）**

通过分析 Cursor 客户端的核心使用路径，定义了"最小可行协议集"：

```
核心路径（必须实现）：
├── BidiAppend（上行请求解码）
├── RunSSE（下行流式响应）
├── 工具执行桥（exec_server_message → exec_client_message）
├── 交互查询桥（interaction_query → interaction_response）
└── 会话状态持久化（state.json + context.json）

扩展路径（逐步支持）：
├── Dashboard 服务 Mock（用量统计、用户信息）
├── Auth 服务 Mock（OAuth、Stripe）
├── Tab 预测服务（cursor-tab-server）
└── 文件同步服务（FileSyncService）
```

**方法 2：能力分层架构**

将系统拆分为"核心稳定层"和"扩展适配层"：

| 层级 | 稳定性 | 变更频率 | 示例 |
|---|---|---|---|
| 协议解码层 | 高 | 低 | `proto/` 定义、ConnectRPC 路由 |
| 状态机层 | 中 | 中 | `forwarder/` 核心逻辑 |
| Provider 适配层 | 中 | 高 | `agent/model/` OpenAI/Anthropic 适配 |
| 工具层 | 低 | 高 | `agent/bridge/exec/` 工具实现 |
| Mock 层 | 低 | 高 | `backend/server/upstream/mocks.go` |

**方法 3：用户价值导向的优先级**

定义了清晰的目标优先级：

```
P0（核心体验）：
  - 模型对话能正常进行
  - 工具执行能正常返回
  - 会话历史能正确持久化

P1（完整体验）：
  - 多模式支持（AGENT/ASK/PLAN/DEBUG/MULTITASK）
  - 子代理模型覆盖
  - 历史压缩与上下文管理

P2（生态扩展）：
  - 多 IDE 支持
  - 云端部署
  - 插件市场
```

---

## 3. 研发过程中的关键坑点

### 3.1 协议逆向坑

#### 坑 1：协议版本漂移

**问题**：Cursor 客户端更新后，protobuf 定义发生变化，导致原有解码逻辑失效。

**案例**：
- `agent_v1.proto` 中的 `AgentClientMessage` 新增了 `subagent_model_overrides` 字段
- 旧代码未处理该字段，导致新客户端的请求被错误解析
- 症状：模型选择逻辑异常，子代理使用了错误的模型

**解决方案**：
1. 建立协议版本监控机制，定期对比客户端 bundle 中的 proto 定义
2. 使用 `unknown fields` 保留策略，避免新增字段导致解析失败
3. 将 proto 定义纳入版本控制，每次更新都进行 diff 审查

#### 坑 2：隐式协议假设

**问题**：客户端和服务端之间存在大量隐式假设，文档中未明确说明。

**案例**：
- 假设：`exec_server_message` 和 `interaction_query` 都需要客户端回复
- 现实：`interaction_update` 和 `conversation_checkpoint_update` 是通知型，不需要回复
- 后果：服务端错误地等待 ack，导致请求超时

**解决方案**：
1. 通过大量抓包和日志分析，建立"协议行为矩阵"
2. 将分析结果编码为 Skill（如 `cursor-client-e2e-debugging/SKILL.md`）
3. 在代码中显式区分"请求型"和"通知型"消息

```
请求型下行（client 必须回复）：
  - exec_server_message → ExecClientMessage / ExecClientControlMessage
  - interaction_query → InteractionResponse

通知型下行（client 无需回复）：
  - interaction_update
  - conversation_checkpoint_update
  - kv_server_message
```

### 3.2 状态机坑

#### 坑 3：Provider Pass 隔离不清

**问题**：同一个 `request_id` 可能包含多次 provider pass，但状态机未正确隔离。

**案例**：
- 第一轮 provider 返回工具调用，客户端执行后回复
- 第二轮 provider 使用工具结果继续生成
- 问题：工具结果"晚到"（在第一轮 `[DONE]` 后到达），被错误地归属到第二轮

**症状**：
- 同一个 `request_id` 在 `[DONE]` 后又出现新的 `model_call_id`
- 工具结果污染了当前轮次
- 出现 "No tool output found for function call ..." 错误

**解决方案**：

```go
// 核心原则：resume 必须按 provider pass 隔离
type Command struct {
    Kind     CommandKind
    IsResume bool
    // 关键：必须携带来源 pass 信息
    SourcePass int
}

// 工具结果只能影响所属 pass
func selectPendingExec(stream *ActiveStream, execID string, pass int) (*PendingExec, error) {
    // 严格按 exec_id + provider_pass 匹配
    // 不允许"当前只有一个 pending 就直接返回"的兜底逻辑
}
```

#### 坑 4：Compaction 递归风险

**问题**：历史压缩逻辑在特定条件下触发无限递归。

**案例**：
- 上下文超过阈值，触发 compaction
- Compaction 失败（如模型返回错误）
- 错误处理逻辑又触发了新的 compaction
- 循环往复，直到栈溢出

**解决方案**：
1. 增加 compaction 递归深度限制（最大 3 次）
2. 增加 compaction 冷却时间（失败后 30 秒内不再触发）
3. 将 compaction 失败降级为"截断历史"而非"重试 compaction"

### 3.3 并发坑

#### 坑 5：StreamBroker 竞态条件

**问题**：`Subscribe` 和 `Unsubscribe` 方法对 `stream.Subscribers` 的访问存在竞态。

**案例**：
- Goroutine A 正在添加订阅者
- Goroutine B 正在移除订阅者（因客户端断开）
- 结果：订阅者列表损坏，信号丢失或重复发送

**症状**：
- SSE 流意外断开
- 客户端收不到后续消息
- 内存泄漏（订阅者未正确清理）

**解决方案**：
1. 统一使用 `sync.RWMutex` 保护所有字段访问
2. 复杂操作封装为原子方法
3. 使用 `defer` 确保清理逻辑执行

```go
func (broker *StreamBroker) Subscribe(requestID string) (string, <-chan struct{}, error) {
    broker.mu.Lock()
    defer broker.mu.Unlock()
    
    // 所有操作在锁保护下完成
    broker.stopTerminalCleanupTimerLocked(stream)
    stream.Subscribers[subscriberID] = subscriber
    stream.UpdatedAt = time.Now().UTC()
}
```

### 3.4 持久化坑

#### 坑 6：历史存储格式演进

**问题**：项目经历了多次历史存储格式变更，导致兼容性问题。

**演进路径**：
```
v0: data.sqlite（SQLite 数据库存储）
v1: conversation.json + turns/<n>/request.json|sse.jsonl|summary.json
v2: state.json + context.json（当前）
```

**问题**：
- 旧版本数据无法直接迁移
- 用户升级后历史记录丢失
- 调试时需要同时理解多种格式

**解决方案**：
1. 定义清晰的"事实源"（source of truth）
2. `state.json`：会话元数据和当前状态
3. `context.json`：append-only 的语义历史
4. 提供迁移脚本（`internal/appdata/migrate.go`）
5. 旧格式标记为 deprecated，逐步清理

#### 坑 7：非原子文件写入

**问题**：配置和历史文件直接覆盖写入，崩溃时可能留下损坏文件。

**案例**：
- `config.yaml` 写入过程中程序崩溃
- 文件内容部分写入，成为无效 YAML
- 下次启动时解析失败，无法恢复

**解决方案**：
1. 使用"写入临时文件 + 原子重命名"策略
2. 定期备份（如 `config.yaml.bak`）
3. 解析失败时回退到默认配置

### 3.5 安全坑

#### 坑 8：Shell 工具命令注入

**问题**：`Shell` 工具直接执行 LLM 生成的命令，未做充分校验。

**案例**：
- LLM 生成命令：`ls -la; rm -rf /`
- 直接通过 `exec.Command` 执行
- 后果：数据丢失、系统损坏

**解决方案**：
1. 命令白名单：只允许预定义的安全命令
2. 参数化执行：禁止字符串拼接，使用参数列表
3. 沙箱环境：在隔离环境中执行（如 Docker、受限用户）

#### 坑 9：调试日志敏感信息泄露

**问题**：调试日志记录了完整的请求/响应数据，包括 API Key、代码内容等敏感信息。

**案例**：
- 用户开启 `log: true` 排查问题
- `provider.jsonl` 中记录了完整的 `Authorization` 头
- 日志文件被意外分享，导致 API Key 泄露

**解决方案**：
1. 对敏感字段进行脱敏处理
2. 提供配置选项控制日志粒度
3. 定期自动清理调试日志

---

## 4. AI 协作模式：从试错到体系化

### 4.1 协作模式演进

#### 阶段 1：直接提问（早期）

```
用户：帮我写个 MITM 代理
AI：好的，这是代码...

问题：
- AI 不了解项目上下文，生成的代码与现有架构不匹配
- 需要大量手动修改
- 重复劳动多
```

#### 阶段 2：上下文增强（中期）

```
用户：参考 internal/backend/host.go 的路由注册方式，
     帮我添加一个新的 Dashboard 服务路由
AI：好的，参考现有模式，这是代码...

改进：
- 提供了代码参考，AI 生成的代码更贴合项目风格
- 但仍需手动处理细节
```

#### 阶段 3：Skill 体系化（当前）

建立了结构化的 Agent Skill 体系，将领域知识编码为可复用的指导文件：

```
.agents/skills/
├── coding-guidance/SKILL.md          # 本地模式实现指南
├── cursor-client-e2e-debugging/SKILL.md  # 客户端 E2E 调试
├── cursor-debug-log/SKILL.md         # Debug 日志调查
├── prefix-cache-stability/SKILL.md   # Prefix Cache 稳定性
├── cursor-app-formatted/SKILL.md     # 客户端 Bundle 格式化
└── test-requirements/SKILL.md        # 测试约束（禁止写测试）
```

**Skill 的核心作用**：
1. **知识沉淀**：将逆向分析结果、协议理解、调试经验编码为结构化文档
2. **上下文传递**：AI 在处理相关任务时自动加载对应 Skill，无需重复说明
3. **约束定义**：明确告知 AI "能做什么、不能做什么"

### 4.2 Prompt 工程实践

项目使用了一套结构化的 Prompt 模板，针对不同场景提供指导：

```
prompt/
├── agent/prompt.md      # Agent 模式通用指导
├── plan/prompt.md       # 规划模式指导
├── ask/prompt.md        # 问答模式指导
├── subagent/prompt.md   # 子代理模式指导
├── debug/prompt.md      # 调试模式指导
├── compaction/prompt.md # 历史压缩指导
├── commit/prompt.md     # Commit 消息生成
└── common_prefix.md     # 通用前缀约束
```

**Prompt 设计原则**：

1. **明确约束优先**
   ```
   先遵守这个约束：
   - 不要修改已安装的 Cursor 客户端代码、bundle 或 app 副本。
   - 允许且推荐读取、搜索、比对和分析客户端 bundle、日志、协议与仓库代码。
   ```

2. **分层信息组织**
   ```
   ## 已确认结论
   （稳定知识，不会频繁变更）

   ## 当前工作流
   （动态更新的操作流程）

   ## 约束
   （必须遵守的硬性规则）
   ```

3. **具体案例驱动**
   ```
   ## 案例 1：工具晚到问题
   现象：...
   原因：...
   解决方案：...

   ## 案例 2：Prefix Cache 失效
   现象：...
   原因：...
   解决方案：...
   ```

### 4.3 人机协作的最佳实践

#### 实践 1：AI 负责"写"，人负责"审"

| 任务 | AI 角色 | 人角色 |
|---|---|---|
| 代码生成 | 根据 Skill 和 Prompt 生成初稿 | 审查逻辑正确性、边界条件 |
| 协议分析 | 提取客户端 bundle 中的 proto 定义 | 验证分析结果、判断影响范围 |
| 调试定位 | 根据日志和状态文件定位问题 | 确认根因、制定修复方案 |
| 文档编写 | 根据代码生成技术文档 | 审查准确性、补充业务背景 |

#### 实践 2：持续迭代 Skill

```
发现问题 → 分析根因 → 编码为 Skill → 验证有效性 → 推广使用
    ↑___________________________________________________|
```

**案例**：`prefix-cache-stability/SKILL.md` 的诞生

1. **问题**：多次修改 prompt 编译逻辑后，Prefix Cache 命中率下降，导致模型调用成本上升
2. **分析**：发现是因为历史消息被重新排序或删除，导致模型可见的 prefix 发生变化
3. **编码**：将"append-only 历史"、"largest stable prefix first"等原则写入 Skill
4. **验证**：后续修改 prompt 相关代码时，AI 自动遵循该 Skill，Cache 命中率保持稳定
5. **推广**：该 Skill 成为所有 prompt 相关修改的必查项

#### 实践 3：分层调试策略

面对复杂问题，采用分层调试策略：

```
Layer 1: 日志层
  - 查看 bidi.raw.jsonl / bidi.decoded.jsonl
  - 确认客户端原始上行数据

Layer 2: 运行时层
  - 查看 runtime.jsonl
  - 确认后端状态机流转

Layer 3: Provider 层
  - 查看 provider.jsonl
  - 确认最终出站请求

Layer 4: 输出层
  - 查看 runsse.jsonl
  - 确认后端返回给客户端的内容

Layer 5: 状态层
  - 查看 state.json + context.json
  - 确认持久化状态一致性
```

---

## 5. 后续避免踩坑的策略

### 5.1 架构层面

#### 策略 1：协议变更监控

```go
// 定期对比客户端 bundle 中的 proto 定义
func MonitorProtoChanges() {
    // 1. 提取客户端最新 proto 定义
    // 2. 与仓库中的 proto 定义做 diff
    // 3. 生成变更报告
    // 4. 触发告警或自动创建适配任务
}
```

#### 策略 2：状态机显式化

```go
// 使用显式状态机替代隐式逻辑
type RunState string

const (
    RunStateIdle                   RunState = "IDLE"
    RunStateRestoring            RunState = "RESTORING"
    RunStatePreparingModelInput  RunState = "PREPARING_MODEL_INPUT"
    RunStateStreamingModel       RunState = "STREAMING_MODEL"
    RunStateWaitingExec          RunState = "WAITING_EXEC"
    RunStateWaitingInteraction   RunState = "WAITING_INTERACTION"
    RunStateApplyingExternalResult RunState = "APPLYING_EXTERNAL_RESULT"
    RunStateCheckpointing        RunState = "CHECKPOINTING"
    RunStateCompleted            RunState = "COMPLETED"
    RunStateCanceled             RunState = "CANCELED"
    RunStateFailed               RunState = "FAILED"
)

// 显式定义状态转换规则
var validTransitions = map[RunState][]RunState{
    RunStateIdle:      {RunStateRestoring},
    RunStateRestoring: {RunStatePreparingModelInput},
    // ...
}
```

#### 策略 3：防御式编程

```go
// 参数校验
func (service *Service) maybeCompactBeforeProvider(stream *ActiveStream, conversation *ConversationFile, compiled CompiledConversation) (bool, error) {
    if service == nil || stream == nil || conversation == nil {
        return false, fmt.Errorf("nil parameter")
    }
    
    // 深度限制
    if stream.CompactionDepth > maxCompactionDepth {
        return false, fmt.Errorf("compaction depth exceeded")
    }
    
    // ...
}
```

### 5.2 协作层面

#### 策略 4：Skill 持续更新

```
Skill 更新流程：
1. 发现问题并分析根因
2. 将解决思路编码为 Skill 文档
3. 在相关任务中验证 Skill 有效性
4. 根据反馈迭代优化
5. 定期 review 所有 Skill，删除过时的，补充新的
```

#### 策略 5：Prompt 版本管理

```
prompt/
├── v1/              # 历史版本（归档）
├── v2/              # 当前版本
│   ├── agent/
│   ├── plan/
│   └── ...
└── README.md        # 版本说明和迁移指南
```

#### 策略 6：代码审查清单

```
代码审查 Checklist：

□ 协议兼容性
  - 是否处理了新增的 proto 字段？
  - 是否兼容旧版本客户端？

□ 状态机正确性
  - 状态转换是否完整？
  - 是否有未处理的边界条件？

□ 并发安全
  - 共享状态是否有锁保护？
  - 是否存在死锁风险？

□ 持久化一致性
  - 文件写入是否原子？
  - 失败时是否能恢复？

□ 安全性
  - 是否处理了用户输入？
  - 是否有敏感信息泄露？

□ 性能
  - 是否有不必要的内存分配？
  - 是否有阻塞操作？
```

### 5.3 工具层面

#### 策略 7：自动化测试（尽管项目禁止写测试）

```
替代方案：
1. 脚本化验证
   - scripts/provider-replay.sh：验证 provider 请求
   - scripts/historymetrics：验证 cache 命中率

2. 日志断言
   - 在关键路径插入结构化日志
   - 通过日志分析验证行为正确性

3. 手动测试矩阵
   - 定义测试场景清单
   - 每次发布前执行
```

#### 策略 8：监控与告警

```
监控指标：
- Provider 请求成功率
- Cache 命中率
- 工具执行成功率
- 会话状态异常数
- 平均响应延迟

告警规则：
- Provider 成功率 < 95%
- Cache 命中率 < 80%
- 工具执行失败率 > 5%
- 异常状态数 > 10/小时
```

---

## 6. 总结：Vibe Coding 的核心经验

### 6.1 关键认知

| 认知 | 说明 |
|---|---|
| **AI 是加速器，不是替代者** | AI 能快速生成代码，但架构设计、边界判断、风险评估仍需人类 |
| **知识沉淀 > 单次解决** | 将解决问题的思路编码为 Skill/Prompt，比单次修复更有价值 |
| **约束即自由** | 明确的约束（如"禁止修改客户端"）让 AI 更聚焦，减少无效尝试 |
| **分层抽象是关键** | 将复杂系统拆分为稳定层和适配层，降低变更影响范围 |

### 6.2 成功要素

```
成功 = 清晰的方向 × 结构化的知识 × 持续的迭代 × 有效的人机协作

清晰的方向：
  - 最小可行协议集
  - 能力分层架构
  - 用户价值导向的优先级

结构化的知识：
  - Skill 体系
  - Prompt 模板
  - 协议行为矩阵

持续的迭代：
  - 问题 → 分析 → 编码 → 验证 → 推广
  - 定期 review 和更新

有效的人机协作：
  - AI 负责"写"，人负责"审"
  - 分层调试策略
  - 代码审查清单
```

### 6.3 未来展望

1. **更智能的 Skill 生成**：AI 自动从代码和日志中提取模式，生成 Skill
2. **自动化协议监控**：自动检测客户端协议变更，生成适配建议
3. **交互式调试助手**：AI 辅助分析复杂问题，提供根因假设和验证路径
4. **知识图谱化**：将 Skill、Prompt、代码、文档关联为知识图谱，支持更精准的检索和推理

---

## 附录

### A. 参考文档

- `spec.md` —— 技术规格说明书
- `plan.md` —— 项目计划书
- `SECURITY_AUDIT.md` —— 安全审计报告
- `.agents/skills/` —— Agent Skill 目录
- `prompt/` —— Prompt 模板目录

### B. 术语表

| 术语 | 说明 |
|---|---|
| Vibe Coding | 以 AI 协作为核心、强调快速迭代和直觉驱动的编程方式 |
| Skill | 结构化的领域知识文档，指导 AI 在特定场景下的行为 |
| Prompt | 指导 AI 生成特定输出的文本模板 |
| BYOK | Bring Your Own Key，用户自备 API 密钥 |
| Prefix Cache | 模型 Provider 的上下文缓存机制 |
| Provider Pass | 一次完整的模型调用周期 |

### C. 修订记录

| 日期 | 版本 | 修订内容 | 修订人 |
|---|---|---|---|
| 2026-03-14 | v1.0 | 初始版本 | 项目团队 |