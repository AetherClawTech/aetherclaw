<p align="center">
  <img src="assets/brand/aetherclaw.png" width="128" alt="AetherClaw">
</p>

# AetherClaw

[![build](https://github.com/AetherClawTech/aetherclaw/actions/workflows/build.yml/badge.svg)](https://github.com/AetherClawTech/aetherclaw/actions/workflows/build.yml)

A lightweight, single-binary AI agent that runs anywhere — from cloud servers to Raspberry Pi Zero.

```
      .-----------.
     /             \
    |      ///      |
     \             /
      '-----------'
       AetherClaw
```

## Features

- **27+ tools** — file ops, shell, web search, image generation, TTS, per-agent memory, cron scheduling, cross-agent sessions, device pairing, and more
- **14 messaging channels** — Telegram, Discord, Slack, WhatsApp, Feishu, DingTalk, LINE, QQ, OneBot, WeCom, MaixCam, Pico WebSocket
- **20+ LLM providers** — OpenAI, Anthropic, Gemini, Groq, DeepSeek, Ollama, OpenRouter, Mistral, Qwen, and more
- **Multi-agent architecture** — agent registry, 7-level priority routing, spawn/subagent delegation, model fallback chains
- **Single binary** — zero runtime dependencies, cross-compiles to ARM/RISC-V, ~20MB RAM idle

## Quick Start

### From Release

```bash
# Download the latest release for your platform
curl -fsSL https://github.com/AetherClawTech/aetherclaw/releases/latest/download/aetherclaw_Linux_x86_64.tar.gz | tar xz
./aetherclaw onboard
```

### From Source

```bash
git clone https://github.com/AetherClawTech/aetherclaw.git
cd aetherclaw
go build ./cmd/aetherclaw
./aetherclaw onboard
```

## Usage

### Interactive Agent (CLI)

```bash
aetherclaw agent
```

### Gateway (Multi-Channel Server)

```bash
aetherclaw gateway
```

### Configuration

AetherClaw uses a `config.json` file (auto-created by `onboard`):

```bash
# Interactive setup wizard
aetherclaw onboard

# Or configure manually
cp config/config.example.json ~/.aetherclaw/config.json
```

Environment variables override config values using the `AETHERCLAW_` prefix:

```bash
export AETHERCLAW_PROVIDERS_OPENAI_API_KEY="sk-..."
aetherclaw gateway
```

## Architecture

```
cmd/
  aetherclaw/          # Main CLI binary
  aetherclaw-launcher/ # Gateway launcher with web UI
pkg/
  agent/               # Agent loop, registry, routing
  bus/                 # Message bus (inbound/outbound)
  channels/            # Messaging platform integrations
  config/              # Configuration system
  cron/                # Scheduled task service
  memory/              # Hybrid BM25 + vector search
  multiagent/          # Blackboard, handoff, cascade
  pairing/             # Device approval workflow
  providers/           # LLM provider adapters
  routing/             # Multi-agent message routing
  session/             # Conversation history management
  skills/              # Skill discovery and installation
  tools/               # 27+ tool implementations
  tts/                 # Text-to-speech providers
  usage/               # LLM cost tracking
```

## Comparison

| | AetherClaw | Typical AI Agent (Node.js) |
|---|---|---|
| **Binary** | Single file, ~30MB | node_modules, 500MB+ |
| **RAM (idle)** | ~20MB | ~150MB+ |
| **Startup** | <100ms | 3-5s |
| **Deploy** | `scp` + run | npm install, configure, PM2 |
| **Platforms** | Linux, macOS, Windows, ARM, RISC-V | Linux, macOS |
| **Edge/Embedded** | Raspberry Pi Zero, routers, NAS | Not practical |
| **Extensions** | MCP (any language) | npm packages (JS/TS only) |

## Roadmap

See [ROADMAP.md](ROADMAP.md) for the full development plan.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Submit a pull request

See existing patterns in `pkg/tools/` for tool examples and `pkg/channels/` for channel integrations.

## License

MIT License — see [LICENSE](LICENSE) for details.

Built upon work from [picoclaw](https://github.com/sipeed/picoclaw) and [nanobot](https://github.com/HKUDS/nanobot).
