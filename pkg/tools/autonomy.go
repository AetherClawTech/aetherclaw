package tools

import (
	"context"
	"fmt"
	"sync"
)

// AutonomyLevel defines what a tool or agent is allowed to do autonomously.
type AutonomyLevel int

const (
	L0_Observe      AutonomyLevel = 0 // read-only, no side effects
	L1_Reversible   AutonomyLevel = 1 // reversible writes (files, memory)
	L2_Recoverable  AutonomyLevel = 2 // recoverable actions (network, API calls)
	L3_Irreversible AutonomyLevel = 3 // irreversible actions (exec, deploy, delete)
)

// ApprovalRequester is called when a tool requires approval beyond the agent's level.
type ApprovalRequester interface {
	RequestApproval(ctx context.Context, agentID, toolName string, agentLevel, toolLevel AutonomyLevel) (bool, error)
}

// DefaultToolClassification maps built-in tool names to their autonomy level.
var DefaultToolClassification = map[string]AutonomyLevel{
	// L0: Observe (read-only)
	"read_file":        L0_Observe,
	"list_dir":         L0_Observe,
	"agents_list":      L0_Observe,
	"list_agents":      L0_Observe,
	"sessions_list":    L0_Observe,
	"sessions_history": L0_Observe,
	"blackboard":       L0_Observe,
	"usage":            L0_Observe,

	// L1: Reversible (file writes, memory)
	"write_file":     L1_Reversible,
	"edit_file":      L1_Reversible,
	"append_file":    L1_Reversible,
	"memory":         L1_Reversible,
	"auto_reply":     L1_Reversible,
	"pairing":        L1_Reversible,
	"message":        L1_Reversible,
	"tts":            L1_Reversible,
	"channel_actions": L1_Reversible,

	// L2: Recoverable (network, API, agent coordination)
	"web_search":    L2_Recoverable,
	"web_fetch":     L2_Recoverable,
	"image_gen":     L2_Recoverable,
	"find_skills":   L2_Recoverable,
	"install_skill": L2_Recoverable,
	"handoff":       L2_Recoverable,
	"spawn_agent":   L2_Recoverable,
	"sessions_send": L2_Recoverable,

	// L3: Irreversible (shell exec, cron, hardware)
	"exec":          L3_Irreversible,
	"exec_approval": L3_Irreversible,
	"cron":          L3_Irreversible,
	"i2c":           L3_Irreversible,
	"spi":           L3_Irreversible,
}

// AutonomyHook enforces autonomy levels on tool execution.
// It implements the ToolHook interface.
type AutonomyHook struct {
	agentLevel      AutonomyLevel
	agentID         string
	approver        ApprovalRequester
	classifications map[string]AutonomyLevel
	mu              sync.RWMutex
}

// NewAutonomyHook creates an AutonomyHook for an agent at the given level.
func NewAutonomyHook(agentID string, level AutonomyLevel, approver ApprovalRequester) *AutonomyHook {
	cls := make(map[string]AutonomyLevel, len(DefaultToolClassification))
	for k, v := range DefaultToolClassification {
		cls[k] = v
	}
	return &AutonomyHook{
		agentLevel:      level,
		agentID:         agentID,
		approver:        approver,
		classifications: cls,
	}
}

// SetClassification overrides the autonomy level for a specific tool.
func (h *AutonomyHook) SetClassification(toolName string, level AutonomyLevel) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.classifications[toolName] = level
}

// getToolLevel returns the autonomy level for a tool, defaulting to L1 for unknown tools.
func (h *AutonomyHook) getToolLevel(toolName string) AutonomyLevel {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if level, ok := h.classifications[toolName]; ok {
		return level
	}
	return L1_Reversible // safe default for unknown tools
}

// BeforeExecute checks if the agent's autonomy level permits the tool.
func (h *AutonomyHook) BeforeExecute(ctx context.Context, toolName string, _ map[string]any) error {
	toolLevel := h.getToolLevel(toolName)

	if h.agentLevel >= toolLevel {
		return nil // agent has sufficient autonomy
	}

	// Agent level is below tool level — try approval
	if h.approver != nil {
		approved, err := h.approver.RequestApproval(ctx, h.agentID, toolName, h.agentLevel, toolLevel)
		if err != nil {
			return fmt.Errorf("autonomy: approval request failed for %q: %w", toolName, err)
		}
		if approved {
			return nil
		}
		return fmt.Errorf("autonomy: approval denied for agent %q (L%d) to use tool %q (L%d)",
			h.agentID, h.agentLevel, toolName, toolLevel)
	}

	return fmt.Errorf("autonomy: agent %q (L%d) cannot use tool %q (requires L%d)",
		h.agentID, h.agentLevel, toolName, toolLevel)
}

// AfterExecute is a no-op for autonomy enforcement.
func (h *AutonomyHook) AfterExecute(_ context.Context, _ string, _ map[string]any, _ *ToolResult) {}
