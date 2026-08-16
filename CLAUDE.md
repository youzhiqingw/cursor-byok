# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Cursor助手 (cursor-byok)** — A local proxy + Wails desktop app that lets users bring their own API keys (BYOK) to use any OpenAI/Anthropic-compatible model with Cursor IDE. It intercepts Cursor's API calls via MITM proxy, rewrites requests to user-configured model providers, and streams responses back in Cursor's expected protocol format.

## Tech Stack

- **Backend**: Go 1.25 (module name: `cursor`)
- **Frontend**: Vue 3 + Vite + Tailwind CSS + Wails v3 runtime
- **Desktop**: Wails v3 (`github.com/wailsapp/wails/v3`)
- **Proto**: ConnectRPC (`connectrpc.com/connect`) with generated `agentv1`/`aiserverv1` types
- **DB**: SQLite via `modernc.org/sqlite` (pure Go, no CGo)
- **Build**: Taskfile (`task`) + Wails CLI (`wails3`)
- **Config storage**: YAML on disk (`internal/backend/server/config/store.go`)

## Common Commands

```bash
# Development (starts Wails dev mode with hot-reload)
task dev

# Build distributable for current platform
task build

# Run the built binary
task run

# Build all platforms (macOS only)
task build:all

# Frontend only (inside frontend/)
cd frontend && pnpm dev          # Vite dev server
cd frontend && pnpm build        # Production build

# Release (macOS/Linux)
task release:prepare             # Build all release assets
task release:github              # Publish to GitHub Releases

# cursor-tab-server Docker image
task cursor-tab-server:docker:amd64

# Cache hit rate analysis
go run ./scripts/historymetrics [conversationId|path]
```

## Architecture

```
┌─────────────┐    HTTPS     ┌──────────────┐    HTTP     ┌─────────────────┐
│  Cursor IDE  │────────────▶│  MITM Proxy  │───────────▶│  Backend Server  │
│              │   :18080     │  (goproxy)   │   :18090    │  (chi router)    │
└─────────────┘              └──────────────┘             └────────┬────────┘
                                                                    │
                              ┌─────────────────────────────────────┘
                              │
                    ┌─────────▼──────────┐
                    │  Forwarder Module  │  Core request pipeline
                    │  (service.go)      │
                    └─────────┬──────────┘
                              │
              ┌───────────────┼───────────────┐
              │               │               │
    ┌─────────▼──────┐ ┌─────▼──────┐ ┌──────▼───────┐
    │ Prompt Engine  │ │  Provider   │ │ Agent Core   │
    │ (compile &     │ │  Gateway    │ │ (tool exec,  │
    │  tool filter)  │ │  (router)   │ │  MCP, plans) │
    └────────────────┘ └─────┬──────┘ └──────────────┘
                             │
                   ┌─────────┼─────────┐
                   │                   │
          ┌────────▼──────┐  ┌────────▼──────┐
          │ OpenAI Adapter│  │Anthropic      │
          │ (chat/respons)│  │Adapter        │
          └───────────────┘  └───────────────┘
```

### Key Packages (`internal/`)

| Package | Purpose |
|---------|---------|
| `app/` | Wails app bootstrap — window, system tray, service registry, lifecycle |
| `bridge/` | Wails service bindings — ProxyService, MetricsService, WindowService, AdService (Go↔JS bridge) |
| `client/` | Client-side proxy lifecycle (start/stop/config), license operations |
| `mitm/` | MITM HTTPS proxy via `goproxy`, intercepts Cursor→api2.cursor.sh traffic, relays to local backend |
| `backend/host.go` | HTTP server construction — mounts all routes (ConnectRPC + HTTP), wires middleware and upstream |
| `backend/server/` | Lightweight HTTP framework — routing, middleware, policy, error encoding |
| `backend/server/config/` | Config management — `Store` (YAML I/O), `Manager` (normalize+watch), `Resolver` (model→channel) |
| `backend/forwarder/` | Main request pipeline — BidiAppend/RunSSE handlers, history, compaction, tool catalog |
| `backend/agent/model/` | Provider adapters — `Router` dispatches to `OpenAIAdapter` or `AnthropicAdapter` based on channel config |
| `backend/agent/prompt/` | Prompt compiler — assembles system prompt, injects tools, manages cache prefixes |
| `backend/agent/core/` | Agent logic — tool execution, MCP, create_plan |
| `backend/agent/bridge/` | Agent↔tool bridges — exec (shell) and interaction |
| `modelchannel/` | Channel resolution — maps Cursor model IDs to user-configured adapters |
| `runtime/` | Legacy runtime types — `ResolvedChannel`, `ModelAdapterConfig` |
| `certs/` | TLS cert generation/management for MITM |
| `netproxy/` | HTTP client factory with proxy support |
| `cursor/` | Cursor IDE integration — device ID, binary discovery, settings.json injection |
| `appdata/` | Platform-specific data paths (config dir, history, ads cache) |
| `ads/` | Ad system — fetch, cache, serve ad content |
| `updater/` | Auto-update via GitHub Releases (checks every 20min, SHA-256 verified) |
| `historymetrics/` | Usage metrics (turns, tokens) stored in SQLite |

### Frontend (`frontend/src/`)

Vue 3 SPA with Wails runtime bindings. Views: `Home.vue` (dashboard), `Config.vue` (proxy settings), `ModelConfig.vue` (model adapter list), `ModelEditor.vue` (adapter form). State management in `state/appState.js`. Communicates with Go via `@wailsio/runtime` bindings defined in `bridge/`.

### Request Flow (RunSSE — the critical path)

1. Cursor IDE sends `POST /agent.v1.AgentService/RunSSE` (ConnectRPC streaming)
2. MITM proxy on `:18080` intercepts, rewrites to local backend at `:18090`
3. Backend `forwarder.Module.LocalRunSSE` receives the request
4. Prompt engine compiles system prompt + tools from `prompt/` templates
5. `ProviderGateway` → `Router.Stream` resolves the model to a user-configured channel
6. `OpenAIAdapter` or `AnthropicAdapter` streams to the provider API
7. Provider events are normalized to `ModelEvent` and projected back to Cursor's proto format

### Model Adapter Config

Users configure "model adapters" (channels) via the UI. Each adapter has: `type` (openai|anthropic), `baseURL`, `apiKey`, `modelID`, `openAIEndpoint`, `reasoningEffort`, `anthropicThinkingEffort`, plus optional extra params and custom headers. Resolution logic in `modelchannel/resolve.go` matches Cursor's requested model ID against adapter `modelID` fields.

**Channel ID**: Deterministic hash of `baseURL + modelID + apiKey + displayName + openAIEndpoint` (short SHA-256, first 16 hex chars). Legacy IDs (`baseURL + modelID + apiKey + displayName`) are still supported for backward compatibility.

### Routing Modes

- `local` (default): All agent/chat requests go through the local backend with user's adapters
- `upstream`: Requests pass through to real `api2.cursor.sh` (requires valid Cursor subscription)

### Proto Definitions

- `proto/agent_v1.proto` — Agent protocol (RunSSE, tool calls, thinking)
- `proto/aiserver_v1.proto` — Cursor server protocol (auth, models, dashboard, etc.)
- Generated Go code lives under `gen/`

## Configuration

- **Runtime config**: `~/.cursor-local-assistant-v2/config.yaml` — hot-reloaded (500ms minimum interval). Key fields: `log`, `routing.mode`, `modelAdapters[]`, `backendListenAddr`, `proxyListenAddr`, `providerStreamIdleIdleTimeout`
- **Default addresses**: Backend `127.0.0.1:18090`, Proxy `127.0.0.1:18080`
- **Build config**: `build/config.yml` — version, product info, dev mode settings
- **Version**: Defined in `build/config.yml` under `info.version`

## Persistent Data Layout

All runtime data stored under `~/.cursor-local-assistant-v2/`:

| Path | Purpose |
|------|---------|
| `config.yaml` | User configuration |
| `data/ca.crt` | CA certificate for Cursor IDE |
| `data/ads/` | Ad content cache |
| `history/usage.json` | Aggregate usage metrics |
| `history/<conversationId>/state.json` | Conversation metadata & current state (next_turn_seq, current_todos, current_plans, latest_request_prefix, last_provider_call) |
| `history/<conversationId>/context.json` | Append-only semantic history entries; prompt replay is projected from `context.json.items` |
| `history/<conversationId>/debug/` | Optional debug JSONL logs (bidi.raw, bidi.decoded, runtime, provider, runsse) |
| `logs/app.log` | Application log |

**History is the source of truth.** New requests load `state.json + context.json`, then project prompt via `ProjectPromptReplay()`. Old artifacts (`summary.json`, `replay.json`, `runtime.json`, `request.json`, `conversation.json`, `entries.jsonl`, `turns/`) are legacy and cleaned by history maintenance.

## Testing

**This repository explicitly forbids writing tests** (per `.agents/skills/test-requirements/SKILL.md`). Existing test files are legacy:

- `internal/backend/agent/model/anthropic_test.go`
- `internal/backend/forwarder/path_resolution_test.go`
- `internal/backend/server/upstream/mocks_test.go`

Run with `go test ./...` if needed.

## Code Style Notes

- Chinese comments are used throughout (项目注释为中文)
- Package-level `doc.go` files describe each package's purpose
- Wails bridge services expose Go methods to JS via struct methods on service types
- Config normalization is strict — validates and defaults all fields before saving

## Critical Implementation Rules

These rules come from `.agents/skills/` and must be followed when modifying the local mode forwarder, prompt, or history code.

### No Modifying Cursor Client

- **Never** modify installed Cursor client code, bundles, or app copies
- Read-only analysis of client bundles is allowed and encouraged
- If `.cursor-app-formatted/` exists, prefer reading formatted snapshots over raw bundles

### Forwarder State Machine Rules

1. **Resume must be isolated per provider pass** — `request_id` is not a provider call generation; same request can have multiple provider passes. Resume must carry source pass context.
2. **Late-arriving tool results are normal** — They must only affect their owning pass, never bleed into subsequent passes.
3. **Pending exec must match by ID** — Only match on `exec_id` or `message_id`. Never use "there's only one pending, return it" fallback logic.
4. **Stale resume symptoms** — If you see: same `request_id` getting new `model_call_id` after `[DONE]`, or tool results arriving after `[DONE]`, check `ProviderPassCount` and resume source pass first, don't blame the client.

### Prefix Cache Stability

When changing prompt compilation, history replay, or provider request construction:

- **Model-visible history is append-only** — Never move previously sent messages to new positions
- **Largest stable prefix first**: system prompt → imported replay → persisted history → current-turn suffix
- **Dynamic attention is latest-only** — Current state blocks, edit guards, volatile reminders go at the end and must not become long-lived prefix
- **Never drop correctness-critical context** to optimize cache
- **Never remove/reorder `reasoning_content` replay** — Some providers need prior reasoning for valid continuation
- Verify changes by comparing adjacent provider request artifacts and computing longest common prefix

### Protocol Understanding

- `exec_server_message` / `interaction_query` are **request-type** downlink — client must reply, or server pending won't resolve
- `interaction_update` / `conversation_checkpoint_update` are **notification-type** — no client reply needed
- There is **no universal ack** — each `AgentServerMessage` type has its own reply semantics
- `ExecServerMessage` → `ExecClientMessage` (result) + `ExecClientControlMessage` (control: `stream_close`, `throw`, `heartbeat`)

### Debug Logging

Enable via `log: true` in `config.yaml`. Produces JSONL files under `history/<conversationId>/debug/`:

| File | Content |
|------|---------|
| `bidi.raw.jsonl` | Raw hex of client→backend BidiAppend data |
| `bidi.decoded.jsonl` | Decoded `AgentClientMessage` + extracted intent |
| `runtime.jsonl` | Backend state transitions |
| `provider.jsonl` | Provider pass metadata, request/summary artifacts |
| `runsse.jsonl` | Backend→client `AgentServerMessage` output |

Early events before `conversationId` is known go to `_debug/orphan/<requestId>/`.
