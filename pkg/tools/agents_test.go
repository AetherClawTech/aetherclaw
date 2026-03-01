package tools

import (
	"context"
	"testing"
)

func TestAgentsListTool_Execute(t *testing.T) {
	deps := &AgentsListDeps{
		ListAgentIDs: func() []string { return []string{"main", "research"} },
		GetAgentInfo: func(id string) *AgentInfo {
			switch id {
			case "main":
				return &AgentInfo{ID: "main", Model: "gpt-4o", Provider: "openai", Status: "running"}
			case "research":
				return &AgentInfo{ID: "research", Model: "claude-3-5-sonnet", Provider: "anthropic", Status: "running"}
			}
			return nil
		},
	}

	tool := NewAgentsListTool(deps)

	result := tool.Execute(context.Background(), nil)
	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.ForLLM)
	}

	if result.ForLLM == "" {
		t.Error("Expected non-empty result")
	}

	// Should contain both agents
	if !contains(result.ForLLM, "main") || !contains(result.ForLLM, "research") {
		t.Errorf("Expected both agents in result: %s", result.ForLLM)
	}
}

func TestAgentsListTool_EmptyList(t *testing.T) {
	deps := &AgentsListDeps{
		ListAgentIDs: func() []string { return []string{} },
		GetAgentInfo: func(id string) *AgentInfo { return nil },
	}

	tool := NewAgentsListTool(deps)
	result := tool.Execute(context.Background(), nil)
	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.ForLLM)
	}
	if result.ForLLM != "[]" {
		t.Errorf("Expected empty array, got: %s", result.ForLLM)
	}
}

func TestAgentsListTool_UnknownAgent(t *testing.T) {
	deps := &AgentsListDeps{
		ListAgentIDs: func() []string { return []string{"ghost"} },
		GetAgentInfo: func(id string) *AgentInfo { return nil },
	}

	tool := NewAgentsListTool(deps)
	result := tool.Execute(context.Background(), nil)
	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "unknown") {
		t.Errorf("Expected unknown status for missing agent info: %s", result.ForLLM)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
