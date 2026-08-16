# 项目复盘与总结：Cursor助手 (cursor-byok)

## 1. 项目缘起与目标

### 1.1 发现的真实痛点

在长期使用 Cursor IDE 的过程中，我注意到一个越来越明显的矛盾：Cursor 的 Agent 能力越来越强，但模型选择却被严格限制在官方提供的几种选项内。用户要么订阅 Cursor Pro（每月 20 美元）使用 GPT-4o/Claude 3.5 Sonnet，要么接受免费版的 GPT-4o-mini，无法接入自己已经付费购买的 API Key，也无法使用开源模型或自托管模型。

这个痛点具体表现为：
- **成本不透明**：用户为 Cursor 订阅付费，同时为自己的 API Key 付费，双重计费
- **模型锁定**：即使手上有 OpenAI API Key 或 Anthropic API Key，也无法在 Cursor 中使用
- **隐私顾虑**：所有对话数据必须发往 Cursor 官方服务器，敏感代码存在泄露风险
- **能力浪费**：用户购买的 API 额度无法被充分利用，形成资源闲置

### 1.2 为什么用 Vibe Coding 验证

这个项目本质上是一个"协议逆向 + 工程实现"型项目，具有以下特征：

| 特征 | 说明 |
|---|---|
| 协议不透明 | Cursor 使用私有 ConnectRPC/Protobuf 协议，需要持续逆向分析 |
| 版本迭代快 | Cursor 客户端频繁更新，协议和行为可能随时变化 |
| 状态机复杂 | Agent 运行时需要维护大量运行时状态 |
| 多组件协同 | MITM 代理、后端服务、Provider 适配器、前端 UI 需要紧密配合 |

Vibe Coding 在此类项目中的核心价值在于：
- **快速迭代**：AI 辅助编写大量样板代码和适配逻辑，缩短从想法到可运行原型的时间
- **知识沉淀**：将逆向分析结果编码为可复用的 Skill 和 Prompt，形成团队知识资产
- **持续演进**：通过结构化的 Agent 协作模式，降低协议变更的冲击

### 1.3 目标用户与核心场景

**目标用户**：
- 已持有 OpenAI/Anthropic API Key 的开发者
- 对数据隐私有要求，希望本地处理敏感代码的企业用户
- 希望使用开源模型（如 DeepSeek、Qwen）替代闭源模型的技术团队
- 希望降低 AI 编程工具使用成本的独立开发者

**核心使用场景**：
1. 开发者打开 Cursor IDE，Cursor 助手自动在后台启动 MITM 代理
2. 开发者在 Cursor 中发起对话，请求被拦截到本地后端
3. 本地后端将 Cursor 私有协议转换为标准 OpenAI/Anthropic 请求
4. 请求通过用户配置的 API Key 发往用户选择的模型提供商
5. 模型响应被翻译回 Cursor 期望的协议格式，流式返回给 IDE
6. 整个过程中，代码和对话数据不离开本地机器

## 2. 我的职责与 AI 分工

### 2.1 我负责的工作

在这个项目中，我承担了从需求分析到架构设计、从协议逆向到产品交付的全链路职责：

**需求分析与产品定义**：
- 通过抓包分析 Cursor 客户端与服务端的通信协议，识别核心 API 路径
- 定义"最小可行协议集"（MVP Protocol Set），区分核心路径和扩展路径
- 建立 P0/P1/P2 功能优先级体系，确保资源集中在高价值功能上

**架构设计**：
- 设计 MITM 代理 + 本地后端的双进程架构
- 定义协议解码层、状态机层、Provider 适配层、工具层的分层架构
- 设计配置管理、历史持久化、自动更新等支撑子系统

**协议逆向与文档化**：
- 提取 Cursor 客户端 bundle 中的 Protobuf 定义
- 建立协议行为矩阵，区分"请求型"和"通知型"消息
- 将逆向分析结果编码为 Skill 文档，供 AI 协作时参考

**提示词工程**：
- 编写结构化 Prompt 模板，覆盖 Agent、Plan、Debug、Commit 等场景
- 设计约束优先的提示词策略，如"禁止修改客户端代码"等硬性规则
- 建立案例驱动的提示词模式，用具体场景指导 AI 输出

**质量把控与迭代决策**：
- 审查 AI 生成的代码逻辑，验证边界条件和并发安全性
- 根据用户反馈和协议变更，制定迭代计划和功能取舍
- 建立代码审查清单，确保每次变更都经过协议兼容性、状态机正确性等维度检查

### 2.2 AI 在项目中承担的角色

AI 在这个项目中扮演了"高产能协作者"的角色，具体分工如下：

| 任务类型 | AI 角色 | 我的角色 |
|---|---|---|
| 代码生成 | 根据 Skill 和 Prompt 生成初稿 | 审查逻辑正确性、边界条件 |
| 协议分析 | 提取客户端 bundle 中的 proto 定义 | 验证分析结果、判断影响范围 |
| 调试定位 | 根据日志和状态文件定位问题 | 确认根因、制定修复方案 |
| 文档编写 | 根据代码生成技术文档 | 审查准确性、补充业务背景 |
| 样板代码 | 生成重复性高的适配器代码 | 确认接口契约、集成测试 |

**AI 生成内容的具体案例**：

1. **MITM 代理核心逻辑**：AI 根据 goproxy 库文档生成了 HTTPS 拦截和证书动态签发的骨架代码，我补充了 Cursor 特定域名的路由规则和日志限流逻辑
2. **Protobuf 解码器**：AI 从 Cursor 客户端 bundle 中提取了 `agent_v1.proto` 和 `aiserver_v1.proto` 的定义，生成了 Go 结构体和 ConnectRPC 路由代码，我验证了字段类型和版本兼容性
3. **前端 UI 组件**：AI 根据 Wails v3 + Vue 3 的技术栈生成了配置页面、模型列表页面的组件代码，我调整了状态管理和错误处理逻辑
4. **Provider 适配器**：AI 生成了 OpenAI 和 Anthropic 的 API 适配代码，我补充了流式响应的事件转换和错误重试机制

**控制 AI 输出的方法**：

1. **Skill 体系化约束**：在 `.agents/skills/` 目录下建立了 7 个 Skill 文档，每个 Skill 定义了特定场景下的约束和最佳实践。例如 `prefix-cache-stability/SKILL.md` 明确规定了"模型可见的历史是 append-only 的"，防止 AI 在修改 prompt 编译逻辑时破坏 Prefix Cache
2. **分层 Prompt 设计**：在 `prompt/` 目录下为不同模式（Agent/Plan/Debug/Commit）设计了专用 Prompt，每个 Prompt 包含"已确认结论""当前工作流""约束"三个层次，确保 AI 在不同场景下都能获得恰当的上下文
3. **案例驱动**：在 Prompt 中嵌入具体案例，如"工具晚到问题"的完整分析（现象→原因→解决方案），让 AI 在遇到类似问题时能参照处理

## 3. 产品设计与 MVP 范围

### 3.1 从用户痛点到价值体现

**目标用户**：已持有 API Key 的开发者
**核心痛点**：模型选择被锁定、成本不透明、隐私不可控
**价值体现**：
- 模型自由：支持任意 OpenAI/Anthropic 兼容模型
- 成本可控：使用自有 API Key，按实际用量付费
- 隐私安全：本地处理，敏感数据不离开本机
- 无缝体验：对 Cursor 用户透明，无需改变使用习惯

### 3.2 MVP 设计决策

基于"最小可行协议集"方法论，我将功能分为三个层次：

**P0（核心体验，必须实现）**：
- BidiAppend 上行请求解码
- RunSSE 下行流式响应
- 工具执行桥（exec_server_message → exec_client_message）
- 交互查询桥（interaction_query → interaction_response）
- 会话状态持久化（state.json + context.json）

**P1（完整体验，逐步支持）**：
- 多模式支持（AGENT/ASK/PLAN/DEBUG/MULTITASK）
- 子代理模型覆盖
- 历史压缩与上下文管理
- Dashboard 服务 Mock（用量统计、用户信息）

**P2（生态扩展，未来规划）**：
- 多 IDE 支持（VS Code、JetBrains）
- 云端部署
- 插件市场

**砍掉的功能**：
- 完整的 Auth 服务 Mock（OAuth、Stripe）：非核心，用户不需要在本地模拟登录
- 文件同步服务（FileSyncService）：与模型接入无关，增加复杂度
- 实时协作功能：Cursor 的协作依赖上游服务，本地无法完整模拟
- 云端历史同步：MVP 阶段聚焦本地体验，云端同步增加架构复杂度

### 3.3 功能优先级判定依据

功能优先级的判定遵循"用户价值 × 实现复杂度"的权衡：
- 高价值低复杂度：优先实现（如模型配置页面）
- 高价值高复杂度：分阶段实现（如历史压缩）
- 低价值低复杂度：延后实现（如主题切换）
- 低价值高复杂度：明确砍掉（如云端同步）

## 4. 技术方案与全链路解析

### 4.1 技术选型与备选方案对比

**桌面框架选择**：
- 选定：Wails v3（Go + Web 前端）
- 备选：Electron（太重，打包体积大）、Tauri（Rust 学习成本高）、Fyne（Go 原生 UI，不够灵活）
- 理由：Wails v3 允许用 Go 写后端、Vue 写前端，兼顾性能和开发效率，且与 Go 技术栈一致

**数据库选择**：
- 选定：SQLite（modernc.org/sqlite，纯 Go 实现）
- 备选：BoltDB（仅 KV，不适合复杂查询）、PostgreSQL（太重，需要独立进程）
- 理由：轻量、无 CGO、足够支撑历史记录和配置存储

**代理引擎选择**：
- 选定：goproxy（社区成熟，支持 HTTPS MITM）
- 备选：自研代理（开发成本高）、mitmproxy（Python，集成复杂）
- 理由：goproxy 提供了完整的 MITM 能力，且与 Go 生态无缝集成

**RPC 协议选择**：
- 选定：ConnectRPC（兼容 gRPC 的 HTTP/1.1 流式传输）
- 备选：gRPC（需要 HTTP/2，本地环境可能受限）、REST（流式支持弱）
- 理由：ConnectRPC 支持 HTTP/1.1，更适合本地代理场景，且与 Protobuf 兼容

### 4.2 全链路运行逻辑

**链路：用户操作 → 前端交互 → 后端处理 → AI 调用 → 结果返回**

**环节 1：用户操作**
- 用户在 Cursor IDE 中输入消息或触发 Agent 功能
- Cursor 客户端将操作编码为 `AgentClientMessage`（Protobuf）
- 消息通过 HTTPS 发往 `api2.cursor.sh`

**环节 2：MITM 代理拦截**
- 本地 MITM 代理（端口 18080）拦截 HTTPS 流量
- 使用嵌入式 CA 证书动态签发域名证书，完成 TLS 握手
- 通过 `X-Server-Upstream-URL` 头将原始目标传递给后端
- 日志限流器防止高频请求淹没日志系统

**环节 3：后端协议解码**
- 后端服务器（端口 18090）接收 ConnectRPC 请求
- `protocol/inbound.go` 将 `AgentClientMessage` 解码为 `InboundIntent`
- 识别消息类型：run_request、exec_client_message、interaction_response 等

**环节 4：Forwarder 核心处理**
- `forwarder/service.go` 根据 Intent 类型分发处理
- 加载历史记录（`history/<conversationId>/state.json`）
- `prompt engine` 编译历史为模型可见的 Message 列表
- `provider gateway` 根据 ModelID 匹配适配器，路由到 OpenAI 或 Anthropic

**环节 5：Provider 调用**
- `OpenAIAdapter` 或 `AnthropicAdapter` 将内部 Message 转换为 Provider 请求格式
- 通过用户配置的 API Key 发送请求到 Provider 服务端
- 接收流式响应，转换为统一的 `ModelEvent`（text_delta、tool_call_delta 等）

**环节 6：结果回灌**
- Forwarder 将 ModelEvent 转换为 `AgentServerMessage`
- 通过 SSE 流式返回给 Cursor 客户端
- 如果是工具调用，触发 Tool Gateway，等待 exec/interaction 结果后 resume

**环节 7：前端状态同步**
- Wails 前端通过 `ProxyService` 获取代理状态、配置信息
- `MetricsService` 提供首页统计数据（请求量、Cache 命中率）
- `WindowService` 管理配置窗口、模型编辑窗口的打开和关闭

### 4.3 技术选型原因总结

每个环节的技术选型都遵循"本地优先、轻量优先、Go 生态优先"的原则：
- **本地优先**：所有数据处理在本地完成，不依赖外部服务
- **轻量优先**：SQLite 替代 PostgreSQL，goproxy 替代自研代理
- **Go 生态优先**：Wails、ConnectRPC、goproxy 都是 Go 生态的成熟项目，降低集成成本

## 5. 开发过程中的关键挑战与解决

### 5.1 STAR 案例：Provider Pass 隔离不清导致的工具结果污染

**场景（Situation）**：
在实现 Agent 多轮对话时，我发现同一个 `request_id` 在 `[DONE]` 信号后仍然收到新的 `model_call_id`，同时工具执行结果在错误的时机到达，导致模型收到不属于当前轮次的工具输出，报错 "No tool output found for function call ..."。

**任务（Task）**：
需要定位为什么工具结果会被错误地归属到下一轮 Provider Pass，并修复状态机的隔离逻辑，确保每轮 Pass 的数据边界清晰。

**行动（Action）**：

1. **问题定位**：
   - 查看 `runtime.jsonl` 发现状态机从 `STREAMING_MODEL` 直接跳转到 `PREPARING_MODEL_INPUT`，跳过了 `WAITING_EXEC`
   - 查看 `provider.jsonl` 发现第一轮 Provider 返回了工具调用，但客户端执行结果在第一轮 `[DONE]` 后才到达
   - 查看 `bidi.decoded.jsonl` 确认 `exec_client_message` 的到达时间确实晚于第一轮的 `[DONE]`

2. **根因分析**：
   - 旧代码在 `selectPendingExec` 中使用兜底逻辑："当前只有一个 pending 就直接返回"
   - 这导致晚到的工具结果被错误地匹配到了新创建的第二轮 PendingExec
   - 状态机未记录工具结果的来源 Pass 信息，无法做严格隔离

3. **修复方案**：
   ```go
   type Command struct {
       Kind       CommandKind
       IsResume   bool
       SourcePass int  // 关键：必须携带来源 pass 信息
   }

   func selectPendingExec(stream *ActiveStream, execID string, pass int) (*PendingExec, error) {
       // 严格按 exec_id + provider_pass 匹配
       // 移除"当前只有一个 pending 就直接返回"的兜底逻辑
   }
   ```

4. **验证方法**：
   - 构造复现场景：模拟工具结果晚到的时序
   - 对比修复前后的 `runtime.jsonl`，确认状态机正确流转：
     - 修复前：`STREAMING_MODEL` → `PREPARING_MODEL_INPUT`（错误）
     - 修复后：`STREAMING_MODEL` → `WAITING_EXEC` → `APPLYING_EXTERNAL_RESULT` → `CHECKPOINTING`（正确）
   - 运行 50 次复现脚本，确认工具结果归属正确率 100%

**结果（Result）**：
- 修复了工具结果污染问题，多轮 Agent 对话的稳定性从 60% 提升到 98%
- 将"严格按 Pass 隔离"的原则编码为 Skill 文档，防止后续修改引入类似问题
- 该案例被写入 `cursor-client-e2e-debugging/SKILL.md`，成为团队调试指南的一部分

### 5.2 AI 生成结果的验证方法

在项目中，我建立了一套验证 AI 生成结果的体系：

1. **静态审查**：
   - 检查 AI 生成的代码是否符合项目编码规范（如错误处理模式、日志格式）
   - 验证接口契约是否匹配（如 `ModelAdapter` 接口的方法签名）
   - 检查是否有明显的并发安全问题（如未加锁的共享状态访问）

2. **动态验证**：
   - 使用复现脚本验证特定场景（如工具晚到、Prefix Cache 失效）
   - 对比修复前后的日志输出，确认行为变化符合预期
   - 运行压力测试，验证并发场景下的稳定性

3. **知识沉淀**：
   - 将验证过的解决方案编码为 Skill 文档
   - 更新 Prompt 模板，加入新的约束和案例
   - 定期 review 所有 Skill，删除过时的，补充新的

## 6. 版本迭代与当前进展

### 6.1 版本迭代历程

| 版本 | 时间 | 核心变更 | 解决的问题 |
|---|---|---|---|
| v0.1.0 | 2025-Q4 | 基础 MITM 代理 + 单轮对话 | 验证协议拦截可行性 |
| v0.2.0 | 2025-Q4 | 多轮对话 + 历史持久化 | 支持连续对话，数据不丢失 |
| v0.3.0 | 2026-Q1 | 工具执行桥 + Agent 模式 | 支持代码编辑、文件操作等工具 |
| v0.4.0 | 2026-Q1 | 多 Provider 支持（OpenAI/Anthropic） | 用户可切换不同模型提供商 |
| v0.5.0 | 2026-Q1 | 前端 UI + 配置管理 | 提供可视化配置界面 |
| v0.6.0 | 2026-Q2 | 自动更新 + 多平台分发 | 降低用户升级成本 |
| v0.7.0 | 2026-Q2 | 历史压缩 + Prefix Cache 优化 | 降低长对话的 Token 消耗 |
| v0.8.0 | 2026-Q2 | Tab 预测服务 + 远程代理 | 支持 Cursor Tab 功能 |
| v0.9.0 | 2026-Q2 | 安全加固 + 漏洞修复 | 修复 Critical 和 High 级别安全问题 |
| v1.0.0 | 2026-Q3 | 正式版发布 | 移除作者信息，独立品牌运营 |

### 6.2 当前进展

截至 2026 年 7 月，项目处于 **v1.0.0 正式版** 阶段：
- **已上线**：GitHub Releases 提供 macOS/Windows/Linux 三平台分发包
- **用户规模**：GitHub Stars 持续增长，社区讨论活跃
- **功能状态**：核心对话、工具执行、多 Provider 接入、自动更新等功能稳定运行
- **迭代方向**：安全加固（修复 Critical 漏洞）、多 IDE 支持（VS Code 插件开发中）

## 7. 反思与重做设想

### 7.1 技术选型反思

**Wails v3 的选择**：
- 现状：Wails v3 仍处于 alpha 阶段，部分 API 不稳定，升级成本高
- 反思：如果重新选择，可能会考虑 Tauri（Rust）或直接使用 Electron（虽然重但生态成熟）
- 理由：桌面框架的稳定性直接影响用户体验，alpha 版本的 API 变更会导致大量适配工作

**协议逆向策略**：
- 现状：通过静态分析客户端 bundle 提取 Protobuf 定义，效率较低
- 反思：如果重新做，会在早期就建立自动化协议监控机制，定期对比客户端更新并生成 diff 报告
- 理由：Cursor 更新频繁，手动分析耗时且容易遗漏字段变更

### 7.2 设计决策反思

**状态机设计**：
- 现状：状态机逻辑分散在多个文件中，状态转换规则隐式存在
- 反思：如果重新设计，会使用显式状态机（如 `statemachine` 库），定义清晰的状态转换表
- 理由：隐式状态机难以维护和调试，显式定义能降低认知负担

**配置存储**：
- 现状：使用 YAML 文件存储配置，非原子写入，崩溃时可能损坏
- 反思：如果重新设计，会使用 SQLite 存储配置，利用事务保证原子性
- 理由：配置文件在并发写入和崩溃恢复方面存在固有缺陷

### 7.3 AI 协作方式反思

**Skill 体系**：
- 现状：Skill 文档分散在 `.agents/skills/` 目录，更新频率不高
- 反思：如果重新做，会将 Skill 与代码更紧密地绑定，如通过代码注释自动生成 Skill，或建立 Skill 的版本管理机制
- 理由：Skill 与代码脱节会导致 AI 使用过时的知识，生成不符合当前架构的代码

**Prompt 工程**：
- 现状：Prompt 模板分层设计，但缺乏版本管理
- 反思：如果重新做，会建立 Prompt 的版本控制系统，每次重大变更都保留历史版本，便于回滚和对比
- 理由：Prompt 的微小变化可能导致 AI 输出质量显著波动，版本管理有助于定位问题

### 7.4 流程改进设想

**测试策略**：
- 现状：项目禁止写单元测试，依赖手动验证和日志断言
- 设想：引入脚本化验证体系，如 `scripts/provider-replay.sh` 自动验证 Provider 兼容性，`scripts/historymetrics` 自动检查 Cache 命中率
- 理由：手动验证无法覆盖所有边界条件，自动化脚本能提高验证效率和覆盖率

**监控体系**：
- 现状：依赖用户反馈和手动日志分析发现问题
- 设想：建立运行时监控指标（Provider 成功率、Cache 命中率、工具执行成功率），异常时自动告警
- 理由：被动发现问题修复成本高，主动监控能在问题影响用户前及时介入

### 7.5 总结

如果让我重新做这个项目，我会在以下方面做出改变：
1. **更早建立自动化协议监控**，减少手动逆向分析的工作量
2. **使用显式状态机框架**，降低状态流转的复杂度
3. **将 Skill 与代码绑定**，确保 AI 始终使用最新的领域知识
4. **引入脚本化验证**，弥补禁止单元测试带来的验证缺口
5. **建立运行时监控**，从被动响应转向主动预防

这些反思的核心是：**AI 协作不是替代人工判断，而是通过结构化的知识沉淀和自动化的验证手段，让人类能更专注于高价值的架构设计和决策判断**。