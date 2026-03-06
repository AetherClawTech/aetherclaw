# AetherClaw Roadmap

> A lightweight, single-binary AI agent that runs anywhere — from cloud servers to Raspberry Pi Zero.

This document outlines the development roadmap for AetherClaw as an independent project. Our goal is to build the most capable, deployable, and extensible personal AI agent in the Go ecosystem.

---

## Current State

AetherClaw ships as a single Go binary with zero runtime dependencies. Today it includes:

- **30+ tools** — file ops, shell, web search/fetch, messaging, image generation (DALL-E 3), TTS (OpenAI + ElevenLabs + Edge TTS), per-agent memory (BM25 + vector search), cron scheduling, cross-agent sessions, device pairing, usage tracking, approval workflows, and external MCP server tools
- **14 messaging channels** — Telegram, Discord, Slack, WhatsApp (native + bridge), Feishu, DingTalk, LINE, QQ, OneBot, WeCom, MaixCam, Pico WebSocket
- **20+ LLM providers** — OpenAI, Anthropic, Gemini, Groq, DeepSeek, Ollama, OpenRouter, Mistral, Qwen, and others via OpenAI-compatible HTTP + dedicated Anthropic adapter + CLI providers (Claude Code, Codex) + auth rotation with multi-key cooldown
- **Multimodal messaging** — images from channels converted to native content parts (Anthropic image blocks, OpenAI multimodal format, CLI fallback)
- **MCP client** — consume any external MCP server (stdio/SSE/HTTP), tools auto-registered to all agents at startup
- **Multi-agent architecture** — agent registry, 7-level priority routing, spawn/subagent delegation, cross-agent session communication, model fallback chains with cooldown tracking
- **Infrastructure** — loop detection hooks, cron service, heartbeat, device event monitoring, skills marketplace integration, context caching (Anthropic + OpenAI), link enrichment

---

## Design Principles

1. **Single binary, zero dependencies** — `scp AetherClaw server:` and you're running
2. **Runs anywhere** — x86, ARM, RISC-V, from cloud VMs to embedded hardware
3. **MCP-native** — first-class Model Context Protocol support (not an afterthought)
4. **Extend in any language** — MCP servers replace language-locked plugin SDKs
5. **Minimal resource footprint** — ~20MB idle RAM, <100ms startup
6. **Embeddable** — use AetherClaw as a Go library in your own projects

---

## Phase 1 — Foundation

Core capabilities that unlock real-world usage.

### Inbound Vision / Multimodal

Complete the media pipeline so images sent via messaging channels reach the LLM as multimodal content.

- [x] Wire `InboundMessage.Media` files through to `ContentParts` on user messages
- [x] Base64 encoding with MIME detection for HTTP providers (OpenAI/Gemini format)
- [x] Anthropic-native image blocks (`NewImageBlockBase64`)
- [x] Telegram photo/document lifecycle (stop premature cleanup)
- [x] CLI provider fallback (save image, reference path in prompt text)

### MCP Client

Consume external MCP servers to extend AetherClaw's capabilities without writing Go code.

- [x] stdio transport (launch local MCP server processes)
- [x] HTTP+SSE transport (connect to remote MCP servers)
- [x] Dynamic tool discovery and registration
- [x] Automatic tool registration to all agents at startup
- [x] Graceful lifecycle management (start/stop with agent loop)
- [x] MCP server health monitoring and auto-reconnect
- [x] Per-agent MCP server filtering

### Quick Wins

- [x] Wire `EnrichMessageWithLinks` into the message processing pipeline
- [ ] Register `BlackboardTool` and `HandoffTool` from the multiagent package
- [x] Wire auth rotation into the provider factory (round-robin multi-key with cooldown)
- [x] Add Edge TTS provider (free, no API key required)

---

## Phase 2 — Differentiation

Features that set AetherClaw apart through Go's strengths.

### Multi-Node Mesh

Distributed agent network across multiple machines.

- [ ] mDNS auto-discovery on local network
- [ ] WireGuard tunnel for remote nodes
- [ ] Node capabilities: camera, screen, location, notifications
- [ ] Companion protocol for iOS/Android apps
- [ ] Task delegation across nodes based on capabilities

### ACP (Agent Control Protocol)

Interoperability with Claude Code, Codex, and other ACP-compatible agents.

- [ ] ACP server implementation
- [ ] Spawn ACP sessions as subagents
- [ ] Persistent ACP session management

### Memory Upgrade

Migrate from JSON-file persistence to SQLite for production-grade memory.

- [ ] SQLite storage via `modernc.org/sqlite` (pure Go, no CGO)
- [ ] sqlite-vec for vector similarity search
- [ ] FTS5 full-text search (replaces custom BM25)
- [ ] MMR (Maximal Marginal Relevance) for result diversity
- [ ] Temporal decay scoring
- [ ] Session transcript indexing with incremental delta sync

### Subagent Management

Full lifecycle control for spawned subagents.

- [ ] `subagents` tool: list, kill, steer running subagents
- [ ] Depth limits to prevent infinite recursion
- [ ] Thread-binding (subagent replies go to specific channel thread)
- [ ] Timeout and cleanup policies

### Security Hardening

- [ ] SSRF protection on `web_fetch` (block internal IP ranges)
- [ ] Docker sandbox for `exec` tool (optional, opt-in)
- [ ] Audit logging for tool executions
- [ ] Workspace-only file access guards (configurable per agent)

---

## Phase 3 — Surpass

Capabilities that go beyond what existing AI agent platforms offer.

### MCP Hub

Orchestrate multiple external MCP servers through AetherClaw's MCP client — unified tool namespace, health management, and dynamic discovery.

- [ ] Tool namespace management (avoid collisions between MCP sources)
- [x] MCP server health monitoring and auto-reconnect
- [x] Per-agent MCP server filtering (allow/deny lists per agent)
- [ ] Dynamic MCP server discovery (config reload without restart)

### Plugin Architecture via MCP

Extend AetherClaw by consuming MCP servers written in any language — no Go code required.

- [ ] Curated MCP server recipes for common use cases (filesystem, databases, APIs)
- [ ] One-command MCP server installation (`aetherclaw install mcp <name>`)
- [ ] Plugin marketplace discovery via ClawHub registry
- [ ] Sandboxed stdio subprocess management with resource limits

### Observability

Production-grade monitoring and metrics.

- [ ] Prometheus metrics endpoint (`/metrics`)
- [ ] OpenTelemetry traces for tool executions and LLM calls
- [ ] Structured JSON logging (already partially implemented)
- [ ] Health check endpoints with dependency status

---

## Phase 4 — Ecosystem

Building the community and platform around AetherClaw.

### Browser Automation (CDP Native)

Direct Chrome DevTools Protocol integration — no Playwright, no Node.js, no external dependencies.

- [ ] CDP WebSocket client in pure Go
- [ ] Core actions: navigate, click, type, screenshot, evaluate JS
- [ ] AI-optimized page snapshots (accessibility tree extraction)
- [ ] Tab management and multi-profile support
- [ ] Optional Chrome Extension relay for user's live tabs

### Speech-to-Text

Multi-provider STT so voice messages from Telegram/WhatsApp/Discord are transcribed and understood.

- [ ] OpenAI Whisper API
- [ ] Groq Whisper (fast, free tier)
- [ ] Deepgram
- [ ] Provider fallback chain (reuse existing `FallbackChain` pattern)

### WebAssembly Plugin Runtime

Sandboxed, polyglot tool execution via Wasm.

- [ ] [wazero](https://github.com/tetragonix/wazero) integration (pure Go Wasm runtime, zero CGO)
- [ ] WASI support for filesystem and network access
- [ ] Tool hot-loading without agent restart
- [ ] Resource limits (memory, CPU time) per plugin

### Streaming Voice Pipeline

Real-time STT to Agent to TTS pipeline leveraging Go's concurrency model.

- [ ] Goroutine-based audio pipeline (parallel STT + LLM + TTS)
- [ ] WebSocket audio streaming for web clients
- [ ] VAD (Voice Activity Detection) for natural turn-taking
- [ ] Telephony integration (SIP/WebRTC)

### Web Dashboard

Server-rendered admin panel using Go-native templating — no JavaScript build step.

- [ ] [templ](https://github.com/a-h/templ) + htmx for reactive UI without JS framework
- [ ] Agent configuration and status monitoring
- [ ] Session browser with message history
- [ ] Tool execution logs and metrics
- [ ] Channel connection status and management

### Terminal UI

Premium CLI experience with real-time tool streaming.

- [ ] [bubbletea](https://github.com/charmbracelet/bubbletea) based TUI
- [ ] Live tool call visualization
- [ ] Multi-agent session switching
- [ ] Inline image rendering (iTerm2/Kitty protocols)

### Additional Channels

Expand messaging platform coverage.

- [ ] Matrix (E2E encrypted)
- [ ] Microsoft Teams
- [ ] Google Chat
- [ ] Nostr (decentralized)
- [ ] IRC
- [ ] Mattermost
- [ ] Twitch
- [ ] Generic webhook channel (receive any HTTP POST as inbound message)

### Embedded Mode

Use AetherClaw as a Go library — import it into your own applications.

- [ ] Clean public API surface (`AetherClaw.New()`, `agent.Chat()`, `tools.Register()`)
- [ ] No global state, fully injectable dependencies
- [ ] Example: embed AetherClaw in a Go web server
- [ ] Example: embed AetherClaw in a CLI tool

---

## Why AetherClaw?

|                   | AetherClaw                         | Typical AI Agent (Node.js)  |
| ----------------- | ---------------------------------- | --------------------------- |
| **Binary**        | Single file, ~30MB                 | node_modules, 500MB+        |
| **RAM (idle)**    | ~20MB                              | ~150MB+                     |
| **Startup**       | <100ms                             | 3-5s                        |
| **Deploy**        | `scp` + run                        | npm install, configure, PM2 |
| **Platforms**     | Linux, macOS, Windows, ARM, RISC-V | Linux, macOS (x86/ARM)      |
| **Edge/Embedded** | Raspberry Pi Zero, routers, NAS    | Not practical               |
| **Extensions**    | MCP (any language)                 | npm packages (JS/TS only)   |
| **Embedding**     | `import "AetherClaw"`              | Not embeddable              |

---

## Contributing

We welcome contributions at any phase of the roadmap. If you're interested in working on a specific item:

1. Open an issue to discuss the approach
2. Reference this roadmap in your PR description
3. Follow existing patterns in the codebase (see `pkg/tools/` for tool examples, `pkg/channels/` for channel examples)

Priority areas where help is most needed:

- **Browser CDP** (Phase 4) — pure Go CDP client
- **New channels** (Phase 4) — each channel is relatively self-contained
- **Testing** — improving coverage across all packages

---

_This roadmap is a living document. Priorities may shift based on community feedback and real-world usage._
