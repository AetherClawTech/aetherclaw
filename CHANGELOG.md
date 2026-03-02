# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-03-02

### Added

- **Multimodal messaging**: Images from channels are now sent to LLMs as native content parts
  - `ContentPart` and `ImageSource` types in protocol types
  - Base64 encoding with MIME detection (JPEG, PNG, GIF, WebP) via `pkg/media/encode.go`
  - Anthropic native image blocks (`NewImageBlockBase64`)
  - OpenAI-compatible multimodal format with data URLs
  - CLI provider fallback (appends `[Image: /path]` text)
  - Media files converted to ContentParts in `BuildMessages`
  - Media cleanup re-enabled after base64 encoding completes
- **MCP Client**: Consume external MCP servers to extend capabilities
  - stdio, SSE, and HTTP transport support via mcp-go v0.44.1
  - Dynamic tool discovery and registration as `mcp_<server>_<tool>`
  - Multi-server manager with graceful start/stop lifecycle
  - Tools registered to all agents at startup (works with both `agent` and `gateway` commands)
  - Config: `mcp.servers[]` with name, transport, command, args, url, env
- **Edge TTS provider**: Free text-to-speech via Microsoft Edge WebSocket API (no API key required)
- **Auth rotation**: Round-robin API key rotation with per-key cooldown tracking
  - `APIKeys []string` field on `ModelConfig` for multi-key configurations
  - Automatic `AuthRotatingProvider` wrapping when multiple keys configured
- **Link enrichment**: `EnrichMessageWithLinks` wired into message processing pipeline
  - Configurable via `tools.web.link_enrichment` config section

### Changed

- `openaiMessage.Content` changed from `string` to `any` to support multimodal content parts
- `processOptions` now includes `Media []string` for media file paths
- `BuildMessages` accepts media parameter and converts files to ContentParts

### Dependencies

- Added `github.com/mark3labs/mcp-go` v0.44.1

## [0.1.0] - 2026-03-01

First release of AetherClaw as an independent project.

### Core

- Single Go binary with zero runtime dependencies
- Cross-compilation for Linux, macOS, Windows, ARM, ARM64, RISC-V
- ~20MB idle RAM footprint, <100ms startup time
- Interactive CLI agent mode and multi-channel gateway mode
- Interactive onboard wizard for first-time setup

### Tools (27+)

- **File operations**: read_file, write_file, list_dir, edit_file, append_file
- **Shell**: exec with configurable deny patterns and timeout
- **Web**: web_search (Brave, Tavily, DuckDuckGo, Perplexity), web_fetch with proxy support
- **Messaging**: message tool with send callback, channel_actions (pin, delete, react, forward)
- **Media**: image_gen (DALL-E 3), tts (OpenAI + ElevenLabs)
- **Memory**: per-agent hybrid BM25 + vector search with OpenAI/Gemini embeddings
- **Sessions**: sessions_list, sessions_history, sessions_send for cross-agent communication
- **Agents**: agents_list for agent discovery
- **Scheduling**: cron with one-shot, recurring, and cron expression support
- **Skills**: find_skills and install_skill from registry marketplace
- **Multi-agent**: spawn with allowlist-based subagent delegation
- **Workflow**: approval management, auto_reply rules
- **Device**: pairing with code-based device approval
- **Usage**: per-model LLM cost tracking
- **Hardware**: I2C and SPI tools (Linux)

### LLM Providers (20+)

- OpenAI, Anthropic (dedicated adapter with prompt caching), Gemini, Groq, DeepSeek
- Ollama, OpenRouter, Mistral, Qwen, Moonshot, Cerebras
- VLLM, NVIDIA, Volcengine, ShengSuanYun, Zhipu
- GitHub Copilot (gRPC), Antigravity (Google Cloud Code Assist)
- CLI providers: Claude Code, Codex
- Model fallback chains with per-provider cooldown tracking and error classification

### Messaging Channels (14)

- Telegram (polling + webhook, typing, placeholder, group triggers)
- Discord (bot with typing indicator)
- Slack (Socket Mode)
- WhatsApp (native go-whatsmeow + mautrix bridge)
- Feishu/Lark, DingTalk, LINE, QQ, OneBot v11
- WeCom (webhook + app mode)
- MaixCam (TCP), Pico WebSocket

### Multi-Agent Architecture

- Agent registry with independent workspace, sessions, tools, and model per agent
- 7-level priority routing: peer > parent_peer > guild > team > account > channel_wildcard > default
- Subagent spawn with allowlist enforcement
- Cross-agent session communication
- Blackboard and handoff protocols (pkg/multiagent)
- Loop detector hook for infinite loop prevention

### Infrastructure

- Cron service with persistent store and job execution through agent loop
- Heartbeat with configurable interval for periodic agent check-ins
- USB device event monitoring (Linux)
- Context caching (Anthropic ephemeral blocks + OpenAI prompt_cache_key)
- Session summarization with automatic compression at 75% context window
- Emergency context compression (drop oldest 50%) on token limit errors
- Skills marketplace integration with ClawHub registry
- Structured logging with configurable levels
- Health check endpoints (/health, /ready)
- State manager for last-active channel tracking

### Configuration

- JSON config with environment variable overrides (AETHERCLAW_ prefix)
- Per-agent model, workspace, skills filter, and subagent allowlist
- Route bindings for channel/account/peer-based agent assignment
- Model list with round-robin load balancing
- Auto-migration from legacy provider config format

[0.3.0]: https://github.com/AetherClawTech/aetherclaw/releases/tag/v0.3.0
[0.1.0]: https://github.com/AetherClawTech/aetherclaw/releases/tag/v0.1.0
