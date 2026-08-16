# Cursor助手 前端软件设计文档 (SDD)

> **文档类型**: Software Design Document (SDD)  
> **目标读者**: 前端开发者、架构师、技术负责人、AI 协作者  
> **版本**: v1.0  
> **日期**: 2026-03-14

---

## 1. 设计目标与约束

### 1.1 设计目标

| 目标 | 说明 | 优先级 |
|---|---|---|
| **跨平台桌面应用** | 基于 Wails v3 构建，支持 macOS/Windows/Linux | P0 |
| **企业级状态管理** | 支持复杂业务状态机、持久化、事件驱动 | P0 |
| **模块化架构** | 组件、服务、状态、工具函数职责分离 | P0 |
| **类型安全** | 关键业务逻辑使用 TypeScript 风格类型约束（JSDoc） | P1 |
| **性能优化** | 最小化渲染、懒加载、缓存策略 | P1 |
| **可维护性** | 清晰的代码结构、可测试、可扩展 | P1 |

### 1.2 技术约束

| 约束 | 说明 |
|---|---|
| **Wails v3 Runtime** | 前端通过 `@wailsio/runtime` 与 Go 后端通信，API 为运行时生成 |
| **Vue 3 Composition API** | 必须使用 `<script setup>` 语法 |
| **Tailwind CSS** | 原子化 CSS，禁止自定义 CSS 文件（除必要全局样式外） |
| **无测试** | 项目明确禁止编写测试代码（`.agents/skills/test-requirements/SKILL.md`） |
| **嵌入构建** | 前端产物通过 `//go:embed all:frontend/dist` 嵌入 Go 二进制 |

---

## 2. 架构设计

### 2.1 分层架构

```
┌─────────────────────────────────────────┐
│           Presentation Layer            │
│  ┌─────────┐ ┌─────────┐ ┌──────────┐ │
│  │  Views  │ │Layouts  │ │  Pages   │ │
│  │(Home.vue│ │(Main    │ │(Model   │ │
│  │ Config  │ │Layout)  │ │Editor)  │ │
│  └────┬────┘ └────┬────┘ └────┬────┘ │
│       │           │           │       │
│  ┌────┴───────────┴───────────┴────┐ │
│  │         Component Layer          │ │
│  │  ┌─────────┐ ┌───────────────┐  │ │
│  │  │ UI Base │ │  Business     │  │ │
│  │  │(Button,│ │  (HomeMetrics  │  │ │
│  │  │ Modal) │ │   Card, Model │  │ │
│  │  └─────────┘ │   AdapterTest│  │ │
│  │              │   Card)       │  │ │
│  │              └───────────────┘  │ │
│  └─────────────────────────────────┘ │
│                   │                   │
│  ┌────────────────┴────────────────┐ │
│  │         State Management         │ │
│  │    (Reactive + Event-Driven)     │ │
│  └─────────────────────────────────┘ │
│                   │                   │
│  ┌────────────────┴────────────────┐ │
│  │         Service Layer            │ │
│  │   (Wails Bridge + API Client)    │ │
│  └─────────────────────────────────┘ │
└─────────────────────────────────────────┘
```

### 2.2 目录结构规范

```
frontend/src/
├── main.js                 # 应用入口，初始化 Vue + Wails
├── App.vue                 # 根组件，全局模态框/消息管理
├── router/
│   └── index.js            # Vue Router 配置
├── layouts/
│   └── MainLayout.vue      # 主布局（标题栏、内容区、底部栏）
├── views/
│   ├── Home.vue            # 首页（服务状态、统计、快捷操作）
│   ├── Config.vue          # 配置页（运行模式、语言、模型列表）
│   ├── ModelConfig.vue     # 模型配置列表页
│   └── ModelEditor.vue     # 模型编辑/新增页
├── components/
│   ├── ui/                 # 基础 UI 组件（原子组件）
│   │   ├── Button.vue
│   │   ├── Card.vue
│   │   ├── Modal.vue
│   │   ├── InputModal.vue
│   │   ├── Switch.vue
│   │   ├── Select.vue
│   │   ├── Input.vue
│   │   ├── Tooltip.vue
│   │   └── MessageProvider.vue
│   ├── charts/
│   │   └── CacheHitRateChart.vue
│   ├── HomeMetricsCard.vue
│   ├── ModelAdapterTestCard.vue
│   ├── ModelAdapterModal.vue
│   ├── LocaleSelect.vue
│   └── AdModelProvider.vue
├── state/
│   └── appState.js         # 全局状态管理（单一事实源）
├── services/
│   └── clientApi.js        # Wails API 封装与日志
├── composables/
│   ├── useModal.js          # 模态框状态管理
│   ├── useInputModal.js    # 输入框状态管理
│   └── useMessage.js       # 消息提示状态管理
├── utils/
│   ├── isWindows.js        # 平台检测
│   └── numberFormat.js     # 数字格式化
├── i18n/
│   ├── config.js           # 国际化配置
│   └── runtime.js          # 运行时语言切换
└── assets/                 # 静态资源（图片、字体等）
```

---

## 3. 状态管理设计

### 3.1 核心原则

**单一事实源（Single Source of Truth）**：

```javascript
// state/appState.js
export const appState = reactive({
  // 服务状态
  serviceRunning: false,
  backendRunning: false,
  proxyRunning: false,
  serviceBusy: false,
  serviceLastError: "",
  
  // 配置状态
  modelAdapters: [],
  routingMode: "local",
  configSaving: false,
  
  // 统计状态
  homeMetrics: createEmptyHomeMetrics(),
  homeMetricsLoading: false,
  homeMetricsError: "",
  
  // 更新状态
  updateState: "idle",
  updateVersion: "",
  // ...
});
```

### 3.2 状态分层

| 层级 | 说明 | 示例 |
|---|---|---|
| **原始状态** | 从后端同步的原始数据 | `proxyRunning`、`modelAdapters` |
| **派生状态** | 通过 `computed` 从原始状态派生 | `serviceStatusText`、`serviceStatusClass` |
| **视图状态** | 仅 UI 关心的临时状态 | `homeMetricsLoading`、`configSaving` |

```javascript
// 派生状态示例（appViewState）
export const appViewState = reactive({
  serviceStatusText: computed(() => {
    if (appState.proxyRunning && appState.backendRunning) {
      return "服务运行中";
    }
    if (appState.backendRunning) {
      return "后端已启动，代理未启动";
    }
    return "服务未启动";
  }),
  serviceStatusClass: computed(() =>
    appState.serviceRunning ? "text-[#22c55e]" : "text-[#f59e0b]",
  ),
  // ...
});
```

### 3.3 状态持久化

**localStorage 缓存策略**：

```javascript
// 缓存键名版本化，避免旧数据冲突
const APP_STATE_STORAGE_KEY = "cursor-client:runtime-state:v2";

// 使用 watchSyncEffect 自动同步到 localStorage
watchSyncEffect(() => {
  if (!canUseLocalStorage()) return;
  try {
    window.localStorage.setItem(
      APP_STATE_STORAGE_KEY,
      JSON.stringify({
        // 只缓存必要字段
        serviceRunning: appState.serviceRunning,
        modelAdapters: appState.modelAdapters,
        // ...
      }),
    );
  } catch (_error) {
    // ignore local persistence failures
  }
});
```

**设计要点**：
- 缓存键名带版本号（`v2`），便于未来迁移
- 只缓存"恢复体验"必需的字段，不缓存敏感信息
- 失败时静默处理，不影响主流程

### 3.4 事件驱动架构

**Wails 事件订阅**：

```javascript
// 在 appState.js 中统一订阅后端事件
watchSyncEffect((onCleanup) => {
  if (typeof window === "undefined") return;
  const unsubscribe = Events.On(PROXY_STATE_EVENT, handleProxyStateEvent);
  onCleanup(() => unsubscribe());
});
```

**事件类型清单**：

| 事件名 | 来源 | 处理逻辑 |
|---|---|---|
| `proxy:state` | Go 后端 | 更新服务运行状态 |
| `user-config:changed` | Go 后端 | 重新加载用户配置 |
| `model-adapter-test:updated` | Go 后端 | 更新模型测试结果 |
| `update:state` | Go 后端 | 更新应用更新状态 |
| `update:progress` | Go 后端 | 更新下载进度 |
| `update:ready` | Go 后端 | 更新就绪，提示用户 |
| `update:error` | Go 后端 | 更新失败，显示错误 |

---

## 4. 服务层设计

### 4.1 API 封装模式

**统一日志与错误处理**：

```javascript
// services/clientApi.js
function withApiLogging(name, payload, runner) {
  return Promise.resolve()
    .then(() => runner())
    .then((result) => {
      logSuccess(name, payload, result);
      return result;
    })
    .catch((error) => {
      logError(name, payload, error);
      throw error;
    });
}

export function loadUserConfig() {
  return withApiLogging("LoadUserConfig", undefined, () => LoadUserConfig());
}
```

**设计要点**：
- 所有 API 调用统一包装，自动记录请求/响应日志
- 错误在日志层统一处理，业务层只关心结果
- 支持调试时快速定位问题

### 4.2 服务分层

| 层级 | 职责 | 示例 |
|---|---|---|
| **Wails 绑定层** | 由 Wails 自动生成，直接调用 Go 方法 | `@bindings/cursor/internal/bridge/proxyservice.js` |
| **API 封装层** | 统一日志、错误处理、参数校验 | `services/clientApi.js` |
| **业务逻辑层** | 状态更新、流程控制 | `state/appState.js` 中的 action 函数 |

---

## 5. 组件设计规范

### 5.1 组件分类

| 类型 | 说明 | 示例 |
|---|---|---|
| **原子组件** | 无业务逻辑，纯 UI 展示 | `Button.vue`, `Card.vue`, `Modal.vue` |
| **复合组件** | 组合原子组件，含简单业务逻辑 | `HomeMetricsCard.vue`, `ModelAdapterTestCard.vue` |
| **页面组件** | 完整页面，含复杂业务逻辑 | `Home.vue`, `ModelConfig.vue` |
| **布局组件** | 页面结构框架 | `MainLayout.vue` |

### 5.2 原子组件设计示例

**Button 组件** (`components/ui/Button.vue`)：

```vue
<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    :class="buttonClasses"
    @click="handleClick"
  >
    <slot />
  </button>
</template>

<script setup>
const props = defineProps({
  variant: { type: String, default: "default" }, // default, primary, text
  size: { type: String, default: "md" },
  disabled: { type: Boolean, default: false },
  loading: { type: Boolean, default: false },
  type: { type: String, default: "button" },
});

const emit = defineEmits(["click"]);

const buttonClasses = computed(() => {
  const base = "inline-flex items-center justify-center rounded-md font-medium transition-colors";
  const variants = {
    default: "bg-[#2a2a2a] text-[#e5e5e5] hover:bg-[#333]",
    primary: "bg-[#10AD5D] text-white hover:bg-[#0d8f4d]",
    text: "bg-transparent text-[#a3a3a3] hover:text-[#e5e5e5]",
  };
  return `${base} ${variants[props.variant]}`;
});

function handleClick(event) {
  if (!props.disabled && !props.loading) {
    emit("click", event);
  }
}
</script>
```

**设计要点**：
- 通过 `variant` 属性控制样式变体，避免样式类名硬编码
- 支持 `disabled` 和 `loading` 状态，统一交互体验
- 使用 Tailwind 的原子类，保持样式一致性

### 5.3 复合组件设计示例

**ModelAdapterTestCard** (`components/ModelAdapterTestCard.vue`)：

```vue
<script setup>
const props = defineProps({
  result: { type: Object, default: null },
  compact: { type: Boolean, default: false },
  title: { type: String, default: "测试" },
  emptyText: { type: String, default: "未测试" },
});

const statusConfig = computed(() => {
  const status = props.result?.status ?? "idle";
  const configs = {
    idle: { icon: "", text: props.emptyText, class: "text-[#737373]" },
    running: { icon: "icon-[mdi--loading]", text: "测试中...", class: "text-[#f59e0b]" },
    success: { icon: "icon-[mdi--check-circle]", text: props.result.summaryText, class: "text-[#22c55e]" },
    error: { icon: "icon-[mdi--alert-circle]", text: props.result.summaryText, class: "text-[#ef4444]" },
  };
  return configs[status] ?? configs.idle;
});
</script>
```

**设计要点**：
- 通过 `status` 映射到不同的展示配置，避免大量 `if/else`
- 支持 `compact` 模式，适应不同场景
- 结果数据通过 `result` 属性传入，组件内部只负责展示

---

## 6. 业务逻辑设计

### 6.1 模型适配器管理

**数据流**：

```
用户操作 → View (ModelConfig.vue)
    ↓
Action (state/appState.js)
    ↓
Service (services/clientApi.js)
    ↓
Go Backend (SaveUserConfig)
    ↓
持久化 (config.yaml)
```

**关键函数**：

```javascript
// 保存模型适配器
export async function saveModelAdapterAt(index, adapter) {
  const currentConfig = await loadPersistedUserConfig();
  const nextAdapters = normalizeModelAdapters(currentConfig.modelAdapters);
  const nextAdapter = normalizeModelAdapter(adapter);
  
  // 更新或插入
  if (index >= 0 && index < nextAdapters.length) {
    nextAdapters.splice(index, 1, nextAdapter);
  } else {
    nextAdapters.push(nextAdapter);
  }
  
  // 持久化
  return persistConfigPayload({
    ...currentConfig,
    modelAdapters: nextAdapters,
  }, { modelAdaptersOnly: true });
}

// 删除模型适配器
export async function deleteModelAdapterAt(index) {
  const currentConfig = await loadPersistedUserConfig();
  const nextAdapters = normalizeModelAdapters(currentConfig.modelAdapters);
  
  if (index < 0 || index >= nextAdapters.length) {
    return { ok: false, error: "模型配置不存在，无法删除" };
  }
  
  nextAdapters.splice(index, 1);
  
  return persistConfigPayload({
    ...currentConfig,
    modelAdapters: nextAdapters,
  }, { modelAdaptersOnly: true });
}
```

### 6.2 批量测试并发控制

**并发限制**：

```javascript
const BATCH_TEST_CONCURRENCY = 10;

async function handleTestAllModelAdapters() {
  const adapters = filteredAdapters.value.slice();
  batchTotal.value = adapters.length;
  batchCompleted.value = 0;
  
  // 使用 Worker Pool 模式
  const workers = Array.from(
    { length: Math.min(BATCH_TEST_CONCURRENCY, adapters.length) },
    async () => {
      while (!batchStopRequested) {
        const currentIndex = nextIndex++;
        if (currentIndex >= adapters.length) return;
        
        const adapter = adapters[currentIndex];
        const call = startModelAdapterTest(adapter);
        batchActiveCalls.add(call);
        
        try {
          await call;
        } catch (error) {
          // 单个失败不影响其他测试
        } finally {
          batchActiveCalls.delete(call);
          batchCompleted.value++;
        }
      }
    }
  );
  
  await Promise.allSettled(workers);
}
```

**设计要点**：
- 限制并发数为 10，避免后端压力过大
- 使用 `Promise.allSettled` 确保所有测试完成
- 支持中途停止，通过 `batchStopRequested` 标志控制

---

## 7. 错误处理设计

### 7.1 错误分类

| 类型 | 说明 | 处理方式 |
|---|---|---|
| **API 错误** | Wails 调用失败 | 统一在 `clientApi.js` 中记录日志，业务层捕获 |
| **校验错误** | 用户输入不合法 | 在 `state/appState.js` 中校验，返回 `{ ok: false, error }` |
| **运行时错误** | 未预期的异常 | 使用 `try/catch` 包裹，显示友好提示 |

### 7.2 错误处理模式

```javascript
// 统一错误处理函数
export function toUserError(error) {
  const message = extractErrorMessage(error);
  return message || "服务错误";
}

// View 层使用
async function handleToggleService() {
  const result = await toggleService();
  if (!result.ok) {
    await showActionError("服务操作失败", result.error);
  }
}
```

---

## 8. 国际化设计

### 8.1 实现方式

- 使用 Vue I18n 或自定义实现
- 语言配置保存在 `config.yaml` 中
- 支持运行时切换

### 8.2 关键组件

```javascript
// i18n/config.js
export const SUPPORTED_LOCALES = [
  { code: "zh-CN", name: "简体中文" },
  { code: "en-US", name: "English" },
];

// components/LocaleSelect.vue
// 切换语言时更新 config.yaml 并刷新界面
```

---

## 9. 性能优化策略

### 9.1 渲染优化

| 策略 | 实现 | 效果 |
|---|---|---|
| **Computed 缓存** | 使用 `computed` 缓存派生状态 | 减少重复计算 |
| **懒加载** | 路由级懒加载（`() => import('./views/ModelEditor.vue')`） | 减少首屏加载时间 |
| **列表虚拟化** | 模型列表使用 `grid` + `auto-fill` | 自适应布局，减少 DOM 节点 |

### 9.2 网络优化

| 策略 | 实现 | 效果 |
|---|---|---|
| **批量请求** | 批量测试使用并发控制 | 减少总请求时间 |
| **缓存策略** | 模型测试结果缓存到 `appState` | 避免重复测试 |
| **防抖/节流** | 配置保存防抖 | 减少频繁保存 |

---

## 10. 安全设计

### 10.1 敏感信息处理

```javascript
// 掩码显示 API Key
function maskSecret(value) {
  const text = String(value || "").trim();
  if (!text) return "-";
  if (text.length <= 8) {
    return `${"*".repeat(Math.max(text.length - 2, 0))}${text.slice(-2)}`;
  }
  return `${text.slice(0, 4)}****${text.slice(-4)}`;
}
```

### 10.2 输入校验

```javascript
// 模型适配器校验
export function validateModelAdapters(source) {
  const adapters = normalizeModelAdapters(source);
  const seenIdentityKeys = new Set();
  
  for (const [index, adapter] of adapters.entries()) {
    const prefix = `模型 ${index + 1}`;
    
    if (!adapter.displayName) {
      return `${prefix} 的显示名称不能为空`;
    }
    if (!SUPPORTED_MODEL_ADAPTER_TYPES.has(adapter.type)) {
      return `${prefix} 的类型仅支持 OpenAI 或 Anthropic`;
    }
    // ...
    
    // 去重校验
    const dedupeKey = buildModelAdapterIdentityKey(adapter);
    if (seenIdentityKeys.has(dedupeKey)) {
      return `模型渠道重复，请检查 url、modelID、apiKey、displayName、endpoint 组合`;
    }
    seenIdentityKeys.add(dedupeKey);
  }
  return "";
}
```

---

## 11. 与 AI 协作的规范

### 11.1 代码生成规范

**Prompt 模板**：

```
请为 Cursor助手 项目生成 Vue 3 组件，遵循以下规范：

1. 使用 <script setup> 语法
2. 使用 Tailwind CSS 进行样式设计
3. 状态管理使用 reactive + computed
4. API 调用使用 services/clientApi.js
5. 错误处理使用 showModal 或 message 提示
6. 组件命名使用 PascalCase
7. 文件路径遵循目录结构规范
```

### 11.2 代码审查清单

```
□ 组件是否遵循单一职责原则？
□ 状态管理是否清晰（原始状态 vs 派生状态）？
□ 是否使用了正确的错误处理模式？
□ 敏感信息是否有掩码处理？
□ 是否遵循了 Tailwind CSS 的使用规范？
□ 是否有不必要的重渲染？
□ 是否考虑了边界条件？
```

---

## 12. 附录

### 12.1 术语表

| 术语 | 说明 |
|---|---|
| Wails | Go + Web 技术栈的跨平台桌面应用框架 |
| Composition API | Vue 3 的组合式 API |
| Reactive | Vue 3 的响应式系统 |
| JSDoc | JavaScript 文档注释标准 |
| Tailwind CSS | 原子化 CSS 框架 |

### 12.2 参考文档

- `spec.md` —— 项目技术规格
- `plan.md` —— 项目计划
- `VIBE_CODING.md` —— Vibe Coding 经验文档
- `frontend/package.json` —— 前端依赖配置