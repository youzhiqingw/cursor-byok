# Windows 编译指南

## 本机环境

| 工具 | 版本 | 路径 |
|------|------|------|
| Go | 1.26.3 windows/amd64 | `C:\Program Files\Go\bin\go.exe` |
| Node.js | v24.15.0 | `C:\nvm4w\nodejs\node.exe` |
| Yarn | 1.22.22 | — |
| pnpm | 10.33.2 | — |
| Wails v3 | v3.0.0-alpha2.112 | `C:\Users\21186\go\bin\wails3.exe` |
| Taskfile (task) | ❌ 未安装 | 需要安装 |
| GCC/MinGW | ❌ 未安装 | 不需要（CGO_ENABLED=0） |
| protoc | ❌ 未检测 | 需要（生成 proto Go 代码） |

## 前置条件

### 必须安装

1. **Taskfile runner** — 构建系统的任务编排工具
   ```bash
   # Windows (PowerShell)
   go install github.com/go-task/task/v3/cmd/task@latest
   ```
   安装后确认：`task --version`

2. **protoc + Go 插件** — 生成 proto Go 代码
   ```bash
   # 安装 protoc (Windows)
   # 方式1: scoop install protobuf
   # 方式2: choco install protobuf
   # 方式3: 从 https://github.com/protocolbuffers/protobuf/releases 下载

   # 安装 Go 插件
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
   ```

### 不需要

- **GCC/MinGW** — 项目使用 `CGO_ENABLED=0`，纯 Go 编译，无需 C 编译器
- **Docker** — 仅 Linux 交叉编译需要

## 编译步骤

### 完整编译（推荐）

```bash
# 1. 安装前端依赖
cd frontend
yarn install
cd ..

# 2. 生成 proto Go 代码（需要 protoc）
task common:generate:proto

# 3. 生成 Wails 前端绑定
task common:generate:bindings

# 4. 构建 Windows amd64 可执行文件
task build:windows:amd64
```

### 使用 Wails 开发模式

```bash
# 启动开发模式（热重载）
task dev
```

### 手动分步编译（不依赖 Taskfile）

如果 Taskfile 不可用，可以手动执行：

```bash
# 1. 生成 proto
protoc -I ./proto --go_out=. --go_opt=module=cursor --connect-go_out=. --connect-go_opt=module=cursor ./proto/agent_v1.proto ./proto/aiserver_v1.proto
gofmt -w gen/

# 2. 安装前端依赖
cd frontend && yarn install && cd ..

# 3. 生成 Wails 绑定
wails3 generate bindings -clean=true

# 4. 构建前端
cd frontend && yarn build && cd ..

# 5. 生成图标
cd build && wails3 generate icons -input appicon.png -macfilename darwin/icons.icns -windowsfilename windows/icon.ico && cd ..

# 6. 更新构建资产
cd build && wails3 update build-assets -name "Cursor助手" -binaryname "Cursor助手" -config config.yml -dir . && cd ..

# 7. 生成 Windows syso 资源文件
cd build && wails3 generate syso -arch amd64 -icon windows/icon.ico -manifest windows/wails.exe.manifest -info windows/info.json -out ../wails_windows_amd64.syso && cd ..

# 8. 编译 Go 二进制
go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui -X cursor/internal/buildinfo.Version=$(go run ./scripts/release version -config ./build/config.yml)" -o "bin/windows-64.exe"

# 9. 清理 syso
rm -f *.syso
```

## 产物

| 文件 | 说明 |
|------|------|
| `bin/windows-64.exe` | Windows amd64 可执行文件 |
| `bin/windows-64.zip` | Windows amd64 ZIP 包（含 exe） |

## 常见问题

### `gen/` 目录不存在

需要先执行 `task common:generate:proto` 生成 proto Go 代码。前置条件是安装 `protoc`、`protoc-gen-go`、`protoc-gen-connect-go`。

### `frontend/dist` 不存在

需要先构建前端：`task common:build:frontend`，或手动 `cd frontend && yarn install && yarn build`。

### `frontend/bindings` 不存在

需要先生成 Wails 绑定：`task common:generate:bindings`。

### `task: command not found`

安装 Taskfile：`go install github.com/go-task/task/v3/cmd/task@latest`

### 编译报 `package cursor/gen/agentv1 is not in std`

proto 代码未生成。执行步骤 2 生成 proto。
