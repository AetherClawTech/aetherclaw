# PRP: AetherCrew — Synthetic Teams Project (Separate Repository)

## Document Info

| Field | Value |
|-------|-------|
| **Feature** | AetherCrew — Synthetic Developer Teams on AetherClaw |
| **Author** | leeaandrob |
| **Created** | 2026-03-06 |
| **Status** | Blocked (waiting for AetherClaw core team-ready changes) |
| **Priority** | High |
| **Confidence** | 8/10 |
| **Scope** | Separate project — NOT part of AetherClaw core |
| **Depends On** | [Team-Ready Core PRP](./aethercrew-synthetic-teams.md) |

## 1. Goal

Build AetherCrew as an independent project that transforms AetherClaw into a platform for synthetic developer teams coordinated via Discord. AetherCrew provides team orchestration, Discord mirroring, agent presets, task board, and knowledge distillation — all consuming AetherClaw's team-ready core primitives.

### Relationship to AetherClaw

```
AetherClaw (core binary)          AetherCrew (separate project)
========================          ===========================
Autonomy Ladder (L0-L3)    <──── Configures per-agent levels
ACL Enforcement             <──── Defines permission matrices
Thread-Bound Spawn          <──── Routes responses to Discord threads
Team Config schema          <──── Generates team configs from presets
ToolHook system             <──── Registers Discord MirrorHook
Blackboard/Announcer        <──── Subscribes for coordination events
MCP Client                  <──── Optionally runs as MCP server
```

AetherCrew can connect to AetherClaw in 3 ways:
- **A) Skill pack** — installed to `~/.aetherclaw/skills/aethercrew/`
- **B) MCP server** — consumed via AetherClaw's MCP client (stdio or HTTP)
- **C) Config generator** — CLI that generates config.json + starts AetherClaw

### Success Criteria

1. `aethercrew init --preset dev-team --discord-guild G123` generates a working config
2. 3-agent team (CoS + CTO + Builder) operational within 5 minutes
3. All agent coordination mirrored to Discord with per-agent identity via webhooks
4. Circuit breaker prevents A2A loop storms (maxPingPongTurns=5)
5. Task board with QAPS classification and closeout summaries
6. Knowledge distillation: raw → closeout (~25x compression) → principles

## 2. Prerequisites (AetherClaw Core)

These MUST be merged into AetherClaw before AetherCrew can work:

- [ ] **A.1 Autonomy Ladder** — `pkg/tools/autonomy.go` with AutonomyHook
- [ ] **A.2 ACL Enforcement** — AllowlistChecker enforced in handoff.go/spawn.go
- [ ] **A.3 Thread-Bound Spawn** — ReplyToChannel/ReplyToThread in SpawnRequest
- [ ] **A.4 Team Config** — TeamConfig/TeamLayers/A2AConfig in config schema

## 3. Architecture

### Project Structure

```
aethercrew/
  cmd/
    aethercrew/           # CLI: init, team create/list, setup discord
  pkg/
    discord/              # Discord webhook relay, rate throttling
      mirror.go           # MirrorHook (ToolHook), PostAsAgent, PostToThread
      throttle.go         # RateThrottler (500ms per webhook)
      identity.go         # AgentIdentity, TeamDiscordConfig
      setup.go            # Channel creation, webhook provisioning
    a2a/                  # A2A coordinator
      coordinator.go      # A2A protocol, WAIT discipline
      circuit_breaker.go  # Per-pair + total limits
    presets/               # Team presets
      loader.go           # LoadPreset(), MergeWithConfig()
      templates/
        dev-team.json
        research-team.json
        devops-team.json
        full-team.json
    taskboard/             # Task management
      board.go            # TaskBoardTool (SQLite)
      schema.go           # DB schema, migrations
      qaps.go             # QAPS classification
    knowledge/             # Knowledge distillation
      distill.go          # Closeout generation
      patterns.go         # Pattern extraction → memory
    config/                # AetherCrew-specific config
      discord.go          # TeamDiscordConfig, TeamWebhook
  skills/                  # Packaged as AetherClaw skills
    aethercrew/
      SKILL.md
  go.mod                   # depends on github.com/AetherClawTech/aetherclaw
```

### Component Details

#### Discord Mirror (pkg/discord/)

- Single bot token for gateway (message receiving)
- One webhook per agent channel (identity: name + avatar via WebhookParams)
- `MirrorHook` implements `tools.ToolHook` → AfterExecute posts to Discord
- `PostAsAgent(webhook, content)` → `discordgo.WebhookExecute()`
- `PostToThread(webhook, threadID, content)` → `discordgo.WebhookThreadExecute()`
- `RateThrottler` — 500ms proactive per-webhook to avoid guild bucket collisions
- `SetupTeam(guildID)` — Creates category, channels, webhooks programmatically

Rate limits: 50 req/s global, 5 msg/5s per channel, guild-shared webhook buckets.
Required permissions: SEND_MESSAGES_IN_THREADS, MANAGE_WEBHOOKS, MANAGE_CHANNELS.

#### A2A Coordinator (pkg/a2a/)

- `CircuitBreaker` — per agent-pair maxPingPongTurns (default 5) + total maxIterations (default 10)
- `A2ACall` struct: Source, Target, Title, TaskID, Content, Round
- WAIT discipline: each round handles 1-2 change points, agent outputs "Done:.../WAIT"
- Anti-loop: subagents cannot spawn additional sessions
- Uses existing handoff/spawn tools under the hood

#### Agent Presets (pkg/presets/)

Templates that generate valid AetherClaw config.json:

**dev-team.json** (Minimum Viable Team):
- CoS (L2): planning, coordination → allow [cto, builder]
- CTO (L2): architecture, review → allow [builder]
- Builder (L1): coding, testing → allow []

**research-team.json**:
- CoS (L2) → Lead Researcher (L2) → Analyst (L1) + Writer (L1)

**devops-team.json**:
- CoS (L2) → SRE (L2) → Builder (L1) + Security (L1)

**full-team.json** (All 7 OpenCrew roles):
- Layer 1: CoS (L2)
- Layer 2: CTO (L2), Builder (L1), CIO (L2), Research (L1)
- Layer 3: Knowledge Officer (L1), Operations (L1)

#### Task Board (pkg/taskboard/)

SQLite-backed task management (modernc.org/sqlite, pure Go).

QAPS classification: Q (quick question), A (action/deliverable), P (project), S (system change).
States: open → assigned → in_progress → review → done | blocked.
Closeout field: 10-15 line summary on completion.

#### Knowledge Distillation (pkg/knowledge/)

3-layer compression:
- Layer 0: Raw session history (audit trail)
- Layer 1: Closeout summary (~25x compression)
- Layer 2: KO-extracted principles (reusable patterns → agent memory)

## 4. Key Research (from PRP generation)

### Discord Webhook Relay Pattern

```go
// PostAsAgent sends with specific agent identity
func PostAsAgent(s *discordgo.Session, agent AgentIdentity, content string) error {
    _, err := s.WebhookExecute(agent.WebhookID, agent.WebhookToken, true,
        &discordgo.WebhookParams{
            Content:   content,
            Username:  agent.Name,
            AvatarURL: agent.AvatarURL,
        })
    return err
}
```

### Circuit Breaker Pattern

```go
type CircuitBreaker struct {
    maxPingPong   int
    maxIterations int
    pingPongCount map[string]int // "source->target" -> count
    totalCount    int
    mu            sync.Mutex
}

func (cb *CircuitBreaker) CanProceed(source, target string) bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    pair := source + "->" + target
    return cb.pingPongCount[pair] < cb.maxPingPong &&
        cb.totalCount < cb.maxIterations
}
```

### Gotchas

- Guild-shared webhook rate limit bucket → proactive 500ms throttling
- SEND_MESSAGES permission has NO effect in threads → need SEND_MESSAGES_IN_THREADS
- Bot self-message filtering drops inter-agent messages → explicit allow
- Thread auto-archive → WebhookThreadExecute auto-unarchives
- A2A loop storms → CircuitBreaker + WAIT discipline + no sub-session spawning

### Sources

- OpenCrew: https://github.com/AlexAnys/opencrew
- discordgo: https://pkg.go.dev/github.com/bwmarrin/discordgo
- Discord Threads: https://docs.discord.com/developers/topics/threads
- Discord Rate Limits: https://docs.discord.com/developers/topics/rate-limits
- Anthropic Autonomy Research: https://www.anthropic.com/research/measuring-agent-autonomy

## 5. Estimated Effort

| Component | LOC (prod) | LOC (test) |
|-----------|-----------|-----------|
| Discord Mirror | ~250 | ~150 |
| A2A Coordinator | ~150 | ~80 |
| Agent Presets | ~100 | ~50 |
| Task Board | ~200 | ~100 |
| Knowledge Distillation | ~120 | ~60 |
| CLI (aethercrew) | ~150 | ~50 |
| **Total** | **~970** | **~490** |
| **Grand Total** | | **~1460 LOC** |

## 6. Out of Scope (v1)

- Multi-instance deployment (one AetherClaw per agent)
- Dynamic agent provisioning at runtime
- Multi-guild team support
- Token budget enforcement
- Voice channel integration
- Automated Discord channel/webhook creation in v1 (CLI wizard in v2)
- Web dashboard

## 7. Next Steps

1. **First:** Implement AetherClaw core changes (Team-Ready Core PRP)
2. **Then:** Create `aethercrew` repository
3. **Then:** Implement components in order: Discord Mirror → A2A → Presets → Task Board → Knowledge
