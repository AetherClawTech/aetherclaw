package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// AgentsListDeps holds dependencies for the agents_list tool.
type AgentsListDeps struct {
	// ListAgentIDs returns all registered agent IDs.
	ListAgentIDs func() []string
	// GetAgentInfo returns info about a specific agent.
	GetAgentInfo func(agentID string) *AgentInfo
}

// AgentInfo contains information about a registered agent.
type AgentInfo struct {
	ID        string `json:"id"`
	Model     string `json:"model,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Status    string `json:"status"` // "running", "stopped", "unknown"
}

// AgentsListTool lists all available agents.
type AgentsListTool struct {
	deps *AgentsListDeps
}

func NewAgentsListTool(deps *AgentsListDeps) *AgentsListTool {
	return &AgentsListTool{deps: deps}
}

func (t *AgentsListTool) Name() string { return "agents_list" }
func (t *AgentsListTool) Description() string {
	return "List all available agents with their configuration (ID, model, provider, status)."
}

func (t *AgentsListTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *AgentsListTool) Execute(_ context.Context, _ map[string]any) *ToolResult {
	ids := t.deps.ListAgentIDs()
	if len(ids) == 0 {
		return SilentResult("[]")
	}

	agents := make([]*AgentInfo, 0, len(ids))
	for _, id := range ids {
		info := t.deps.GetAgentInfo(id)
		if info != nil {
			agents = append(agents, info)
		} else {
			agents = append(agents, &AgentInfo{ID: id, Status: "unknown"})
		}
	}

	data, err := json.Marshal(agents)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to marshal agents: %v", err))
	}

	return SilentResult(string(data))
}
