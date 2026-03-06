# PRP: Team-Ready Core — AetherClaw Foundation for Synthetic Teams

## Document Info

| Field | Value |
|-------|-------|
| **Feature** | Team-Ready Core (Autonomy, ACL, Thread-Bound Spawn, Team Config) |
| **Author** | leeaandrob |
| **Created** | 2026-03-06 |
| **Status** | Draft |
| **Priority** | High |
| **Confidence** | 9/10 |
| **Scope** | AetherClaw core only — no AetherCrew, no Discord, no skills |

## 1. Goal

Make AetherClaw's core "team-ready" with 4 surgical changes that enable any external project (AetherCrew or others) to build synthetic teams on top. These changes are platform primitives — they don't know about Discord, presets, or team orchestration.

### Success Criteria

1. Autonomy Ladder (L0-L3) enforced as ToolHook — agents blocked from executing tools above their level
2. ACL enforced in ExecuteHandoff() and AsyncSpawn() — not just tool layer
3. SpawnRequest supports ReplyToChannel/ReplyToThread for thread-bound responses
4. TeamConfig parsed from config.json with validation
5. All existing tests still pass (`make test`)
6. New code has >80% test coverage with table-driven tests
7. Zero breaking changes to existing config format (all new fields are optional/omitempty)

## 2. Background

AetherClaw already has a mature multi-agent infrastructure:
- AgentRegistry with 7-level priority routing
- Handoff (sync) with depth limit 3 and cycle detection
- Spawn (async) with semaphore, dedup, timeout, cascade cancellation
- Blackboard (shared context per-session)
- Capability-based routing (FindAgentsByCapability)
- AllowlistChecker interface (exists but NOT enforced in core functions)
- ApprovalManager (request/approve/reject workflow)
- ToolHook system (BeforeExecute/AfterExecute)

What's missing for synthetic teams:
- No concept of tool reversibility or agent autonomy levels
- AllowlistChecker wired in tool layer only, not enforced in handoff.go/spawn.go
- Spawned agents can't reply to specific channel threads
- No config schema for team definitions

These 4 gaps are all that blocks external projects from building team orchestration.

### Research Sources

- Anthropic "Measuring Agent Autonomy": Only 0.8% of tool calls truly irreversible. Risk scored 1-10. Source: https://www.anthropic.com/research/measuring-agent-autonomy
- Claude Code permission model: deny -> ask -> allow evaluation order. Source: https://code.claude.com/docs/en/permissions
- OpenCrew Autonomy Ladder (L0-L3): https://github.com/openclaw/openclaw/discussions/17246
- OpenCrew A2A anti-loop (maxPingPongTurns): https://github.com/AlexAnys/opencrew

## 3. Implementation

### 3.1 Autonomy Ladder (L0-L3)

**New file:** `pkg/tools/autonomy.go`

```go
package tools

import (
    "context"
    "fmt"
)

type AutonomyLevel int

const (
    L0_Observe      AutonomyLevel = 0 // Read-only, always allowed
    L1_Reversible   AutonomyLevel = 1 // Safe write ops, auto-execute
    L2_Recoverable  AutonomyLevel = 2 // Impactful but fixable
    L3_Irreversible AutonomyLevel = 3 // Requires human approval
)

// ReversibilityClassifier — optional interface for tools to self-classify
type ReversibilityClassifier interface {
    Reversibility() AutonomyLevel
}

// DefaultToolClassification for all 27+ built-in tools
var DefaultToolClassification = map[string]AutonomyLevel{
    // L0 - Observe (read-only)
    "read_file": L0_Observe, "list_dir": L0_Observe,
    "web_search": L0_Observe, "web_fetch": L0_Observe,
    "sessions_list": L0_Observe, "sessions_history": L0_Observe,
    "list_agents": L0_Observe, "memory": L0_Observe,
    "usage": L0_Observe, "find_skills": L0_Observe,

    // L1 - Reversible (safe writes)
    "write_file": L1_Reversible, "edit_file": L1_Reversible,
    "append_file": L1_Reversible, "blackboard": L1_Reversible,
    "sessions_send": L1_Reversible, "auto_reply": L1_Reversible,
    "message": L1_Reversible,

    // L2 - Recoverable (impactful but fixable)
    "handoff": L2_Recoverable, "spawn_agent": L2_Recoverable,
    "install_skill": L2_Recoverable, "cron": L2_Recoverable,

    // L3 - Irreversible (requires approval)
    "exec": L3_Irreversible, "request_approval": L3_Irreversible,
    "image_gen": L3_Irreversible, "tts": L3_Irreversible,
}

// ApprovalRequester is the interface AutonomyHook needs to request approval
type ApprovalRequester interface {
    RequestAndWait(ctx context.Context, toolName string, args map[string]any, reason string) error
}

// AutonomyHook implements ToolHook to enforce autonomy levels
type AutonomyHook struct {
    agentLevel     AutonomyLevel
    classification map[string]AutonomyLevel
    approver       ApprovalRequester // nil = block without approval path
}

func NewAutonomyHook(agentLevel AutonomyLevel, approver ApprovalRequester) *AutonomyHook {
    return &AutonomyHook{
        agentLevel:     agentLevel,
        classification: DefaultToolClassification,
        approver:       approver,
    }
}

// SetClassification allows overriding tool levels (e.g., from config or MCP tools)
func (h *AutonomyHook) SetClassification(toolName string, level AutonomyLevel) {
    h.classification[toolName] = level
}

func (h *AutonomyHook) BeforeExecute(ctx context.Context, toolName string, args map[string]any) error {
    toolLevel, ok := h.classification[toolName]
    if !ok {
        toolLevel = L1_Reversible // unknown tools default to L1
    }

    if toolLevel <= h.agentLevel {
        return nil // allowed
    }

    // Tool requires higher autonomy than agent has
    if h.approver == nil {
        return fmt.Errorf("tool %q requires L%d but agent has L%d (no approval path)",
            toolName, toolLevel, h.agentLevel)
    }
    return h.approver.RequestAndWait(ctx, toolName, args,
        fmt.Sprintf("Tool %q requires L%d approval (agent autonomy: L%d)",
            toolName, toolLevel, h.agentLevel))
}

func (h *AutonomyHook) AfterExecute(_ context.Context, _ string, _ map[string]any, _ *ToolResult) {
    // Future: audit logging for L2+ executions
}
```

**New file:** `pkg/tools/autonomy_test.go`

```go
package tools

import (
    "context"
    "fmt"
    "testing"

    "github.com/stretchr/testify/assert"
)

type mockApprover struct{ called bool }

func (m *mockApprover) RequestAndWait(_ context.Context, _ string, _ map[string]any, _ string) error {
    m.called = true
    return nil
}

type denyApprover struct{}

func (d *denyApprover) RequestAndWait(_ context.Context, toolName string, _ map[string]any, _ string) error {
    return fmt.Errorf("approval denied for %s", toolName)
}

func TestAutonomyHook_BeforeExecute(t *testing.T) {
    tests := []struct {
        name       string
        agentLevel AutonomyLevel
        toolName   string
        approver   ApprovalRequester
        wantErr    bool
        wantApproval bool
    }{
        {"L1 agent executes L0 tool", L1_Reversible, "read_file", nil, false, false},
        {"L1 agent executes L1 tool", L1_Reversible, "write_file", nil, false, false},
        {"L1 agent blocked on L2 tool (no approver)", L1_Reversible, "handoff", nil, true, false},
        {"L1 agent blocked on L3 tool (no approver)", L1_Reversible, "exec", nil, true, false},
        {"L1 agent requests approval for L3 (approver)", L1_Reversible, "exec", &mockApprover{}, false, true},
        {"L1 agent denied by approver", L1_Reversible, "exec", &denyApprover{}, true, false},
        {"L3 agent executes L3 tool", L3_Irreversible, "exec", nil, false, false},
        {"L0 agent blocked on L1 tool", L0_Observe, "write_file", nil, true, false},
        {"L2 agent executes L2 tool", L2_Recoverable, "handoff", nil, false, false},
        {"unknown tool defaults to L1", L1_Reversible, "some_mcp_tool", nil, false, false},
        {"unknown tool blocked for L0 agent", L0_Observe, "some_mcp_tool", nil, true, false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            hook := NewAutonomyHook(tt.agentLevel, tt.approver)
            err := hook.BeforeExecute(context.Background(), tt.toolName, nil)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
            if tt.wantApproval {
                if m, ok := tt.approver.(*mockApprover); ok {
                    assert.True(t, m.called)
                }
            }
        })
    }
}

func TestAutonomyHook_SetClassification(t *testing.T) {
    hook := NewAutonomyHook(L1_Reversible, nil)

    // Custom MCP tool defaults to L1, should pass for L1 agent
    err := hook.BeforeExecute(context.Background(), "mcp__github__create_pr", nil)
    assert.NoError(t, err)

    // Override to L3
    hook.SetClassification("mcp__github__create_pr", L3_Irreversible)
    err = hook.BeforeExecute(context.Background(), "mcp__github__create_pr", nil)
    assert.Error(t, err)
}

func TestAutonomyHook_AfterExecute(t *testing.T) {
    hook := NewAutonomyHook(L1_Reversible, nil)
    // Should not panic
    hook.AfterExecute(context.Background(), "read_file", nil, nil)
}
```

**Modify:** `pkg/config/config.go` — Add to AgentConfig:
```go
Autonomy *int `json:"autonomy,omitempty"` // 0-3, nil defaults to 1 (L1_Reversible)
```

**Modify:** `pkg/agent/loop.go` — In registerSharedTools(), after creating tool registry:
```go
autonomyLevel := tools.L1_Reversible
if agentCfg.Autonomy != nil {
    autonomyLevel = tools.AutonomyLevel(*agentCfg.Autonomy)
}
registry.AddHook(tools.NewAutonomyHook(autonomyLevel, approvalMgr))
```

### 3.2 ACL Enforcement

**Modify:** `pkg/multiagent/handoff.go` — In ExecuteHandoff(), after resolving target:
```go
if req.AllowlistChecker != nil && !req.AllowlistChecker.IsAllowed(req.ToAgentID) {
    return nil, fmt.Errorf("agent %q is not allowed to handoff to %q",
        req.FromAgentID, req.ToAgentID)
}
```

**Modify:** `pkg/multiagent/spawn.go` — In AsyncSpawn(), after validating request:
```go
if req.AllowlistChecker != nil && !req.AllowlistChecker.IsAllowed(req.TargetAgentID) {
    return SpawnResult{Status: "denied",
        Error: fmt.Errorf("agent %q not allowed to spawn %q",
            req.FromAgentID, req.TargetAgentID)}, nil
}
```

**Tests to add** in existing test files:
```go
func TestHandoff_ACLBlocked(t *testing.T) {
    checker := AllowlistCheckerFunc(func(id string) bool { return id == "builder" })
    // ... test handoff to "researcher" returns error
}

func TestHandoff_ACLAllowed(t *testing.T) {
    checker := AllowlistCheckerFunc(func(id string) bool { return id == "builder" })
    // ... test handoff to "builder" succeeds
}

func TestHandoff_NilChecker(t *testing.T) {
    // ... nil checker = all allowed (backwards compatible)
}

func TestSpawn_ACLBlocked(t *testing.T) {
    // ... test spawn denied returns SpawnResult{Status: "denied"}
}
```

### 3.3 Thread-Bound Subagents

**Modify:** `pkg/multiagent/spawn.go` — Add fields to SpawnRequest:
```go
type SpawnRequest struct {
    // ... existing fields ...
    ReplyToChannel string // optional: target channel for outbound
    ReplyToThread  string // optional: target thread for outbound
}
```

**Modify:** `pkg/multiagent/announce.go` — Add fields to Announcement:
```go
type Announcement struct {
    // ... existing fields ...
    ReplyToChannel string
    ReplyToThread  string
}
```

Pass through in AsyncSpawn() when building Announcement from SpawnRequest.

**Modify:** `pkg/multiagent/spawn_tool.go` — Add parameters:
```go
"reply_to_channel": map[string]any{
    "type": "string",
    "description": "Channel ID to reply in (for cross-channel responses)",
},
"reply_to_thread": map[string]any{
    "type": "string",
    "description": "Thread ID to reply in (for thread-bound responses)",
},
```

Parse in Execute() and set on SpawnRequest.

**Tests:**
```go
func TestSpawn_ThreadBound(t *testing.T) {
    // spawn with ReplyToThread="1234.5678"
    // verify Announcement.ReplyToThread == "1234.5678"
}

func TestSpawn_NoThread(t *testing.T) {
    // spawn without ReplyToThread (backwards compatible)
    // verify Announcement.ReplyToThread == ""
}
```

### 3.4 Team Config Model

**New file:** `pkg/config/team.go`
```go
package config

// TeamConfig defines a synthetic team of agents
type TeamConfig struct {
    Name   string            `json:"name"`
    Layers TeamLayers        `json:"layers"`
    A2A    A2AConfig         `json:"a2a,omitempty"`
}

type TeamLayers struct {
    Intent      []string `json:"intent"`
    Execution   []string `json:"execution"`
    Maintenance []string `json:"maintenance,omitempty"`
}

type A2AConfig struct {
    MaxPingPongTurns int  `json:"max_ping_pong_turns,omitempty"` // default 5
    MaxIterations    int  `json:"max_iterations,omitempty"`      // default 10
    TimeoutSeconds   int  `json:"timeout_seconds,omitempty"`     // default 300
    WaitDiscipline   bool `json:"wait_discipline,omitempty"`     // default true
}
```

Note: `TeamDiscordConfig` is NOT in core — that belongs to AetherCrew project.

**Modify:** `pkg/config/config.go` — Add to Config struct:
```go
Teams map[string]TeamConfig `json:"teams,omitempty"`
```

**Modify:** `pkg/config/defaults.go` — Add A2A defaults:
```go
func DefaultA2AConfig() A2AConfig {
    return A2AConfig{
        MaxPingPongTurns: 5,
        MaxIterations:    10,
        TimeoutSeconds:   300,
        WaitDiscipline:   true,
    }
}
```

**New file:** `pkg/config/team_test.go`
```go
func TestTeamConfig_Parse(t *testing.T) {
    // JSON with full team config -> verify layers parsed correctly
}

func TestTeamConfig_Defaults(t *testing.T) {
    // JSON without a2a section -> verify defaults applied
}

func TestTeamConfig_Empty(t *testing.T) {
    // JSON without teams section -> verify nil/empty map
}
```

## 4. Files Summary

### New Files (4)

| File | LOC (est.) |
|------|-----------|
| `pkg/tools/autonomy.go` | ~90 |
| `pkg/tools/autonomy_test.go` | ~120 |
| `pkg/config/team.go` | ~40 |
| `pkg/config/team_test.go` | ~60 |

### Modified Files (6)

| File | Change | LOC (est.) |
|------|--------|-----------|
| `pkg/config/config.go` | Add `Teams`, `Autonomy` fields | ~5 |
| `pkg/config/defaults.go` | Add DefaultA2AConfig | ~10 |
| `pkg/agent/loop.go` | Wire AutonomyHook | ~10 |
| `pkg/multiagent/handoff.go` | Enforce AllowlistChecker | ~5 |
| `pkg/multiagent/spawn.go` | ACL + ReplyToChannel/Thread | ~15 |
| `pkg/multiagent/spawn_tool.go` | Add thread params | ~15 |
| `pkg/multiagent/announce.go` | Add thread fields | ~5 |

### Tests Added to Existing Files (3)

| File | Tests | LOC (est.) |
|------|-------|-----------|
| `pkg/multiagent/handoff_test.go` | ACL blocked/allowed/nil | ~40 |
| `pkg/multiagent/spawn_test.go` | ACL + thread-bound | ~50 |
| `pkg/multiagent/announce_test.go` | Thread fields propagation | ~20 |

**Total: ~485 LOC production + ~290 LOC tests = ~775 LOC**

## 5. Validation

```bash
# All existing tests still pass
make test

# New tests pass
CGO_ENABLED=0 go test ./pkg/tools/... -v -run TestAutonomy
CGO_ENABLED=0 go test ./pkg/config/... -v -run TestTeam
CGO_ENABLED=0 go test ./pkg/multiagent/... -v -run TestACL
CGO_ENABLED=0 go test ./pkg/multiagent/... -v -run TestSpawn_Thread

# Lint clean
make lint

# Build still produces single binary
CGO_ENABLED=0 go build -v ./cmd/aetherclaw
```

## 6. Risks

| Risk | Mitigation |
|------|------------|
| AutonomyHook adds latency to every tool call | Hook is a simple map lookup + int comparison — O(1), no I/O |
| Breaking existing configs | All new fields are `omitempty` with nil/zero defaults |
| ACL enforcement breaks existing setups | Nil AllowlistChecker = all allowed (backwards compatible) |
| Thread-bound fields unused without AetherCrew | Empty strings ignored — zero overhead when not used |

## 7. Out of Scope

- Discord integration (AetherCrew project)
- Agent presets/templates (AetherCrew project)
- Task board (AetherCrew project)
- A2A coordinator / circuit breaker (AetherCrew project)
- Knowledge distillation (AetherCrew project)
- CLI `aetherclaw team` commands (AetherCrew project)
- Token budgets per agent (v2)

## 8. Task Breakdown

See [docs/tasks/aethercrew-synthetic-teams.md](../tasks/aethercrew-synthetic-teams.md) for Phase A tasks only.
