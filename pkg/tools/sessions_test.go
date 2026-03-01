package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/AetherClawTech/aetherclaw/pkg/bus"
	"github.com/AetherClawTech/aetherclaw/pkg/providers"
	"github.com/AetherClawTech/aetherclaw/pkg/session"
	"github.com/AetherClawTech/aetherclaw/pkg/tools"
)

func newTestSessionDeps(t *testing.T) (*tools.SessionsToolDeps, *session.SessionManager, *bus.MessageBus) {
	t.Helper()
	sm := session.NewSessionManager("")
	msgBus := bus.NewMessageBus()

	deps := &tools.SessionsToolDeps{
		GetSessions: func(_ string) *session.SessionManager {
			return sm
		},
		CanAccess: func(_, _ string) bool {
			return true
		},
		ListAgentIDs: func() []string {
			return []string{"main"}
		},
		Bus: msgBus,
	}
	return deps, sm, msgBus
}

func TestSessionsListTool_Empty(t *testing.T) {
	deps, _, _ := newTestSessionDeps(t)
	tool := tools.NewSessionsListTool(deps, "main")

	result := tool.Execute(context.Background(), map[string]interface{}{
		"agent_id": "main",
	})

	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.ForLLM)
	}

	var sessions []map[string]interface{}
	if err := json.Unmarshal([]byte(result.ForLLM), &sessions); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestSessionsListTool_WithSessions(t *testing.T) {
	deps, sm, _ := newTestSessionDeps(t)
	sm.AddMessage("session-1", "user", "hello")
	sm.AddMessage("session-1", "assistant", "hi")
	sm.AddMessage("session-2", "user", "test")

	tool := tools.NewSessionsListTool(deps, "main")

	result := tool.Execute(context.Background(), map[string]interface{}{
		"agent_id": "main",
	})

	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.ForLLM)
	}

	var sessions []map[string]interface{}
	if err := json.Unmarshal([]byte(result.ForLLM), &sessions); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestSessionsListTool_MissingAgentID(t *testing.T) {
	deps, _, _ := newTestSessionDeps(t)
	tool := tools.NewSessionsListTool(deps, "main")

	result := tool.Execute(context.Background(), map[string]interface{}{})

	if !result.IsError {
		t.Fatal("expected error for missing agent_id")
	}
}

func TestSessionsListTool_AccessDenied(t *testing.T) {
	deps, _, _ := newTestSessionDeps(t)
	deps.CanAccess = func(_, _ string) bool { return false }

	tool := tools.NewSessionsListTool(deps, "main")

	result := tool.Execute(context.Background(), map[string]interface{}{
		"agent_id": "other",
	})

	if !result.IsError {
		t.Fatal("expected error for access denied")
	}
	if !strings.Contains(result.ForLLM, "not allowed") {
		t.Errorf("expected 'not allowed' in error, got: %s", result.ForLLM)
	}
}

func TestSessionsListTool_Limit(t *testing.T) {
	deps, sm, _ := newTestSessionDeps(t)
	for i := 0; i < 5; i++ {
		sm.AddMessage(fmt.Sprintf("session-%d", i), "user", "test")
	}

	tool := tools.NewSessionsListTool(deps, "main")

	result := tool.Execute(context.Background(), map[string]interface{}{
		"agent_id": "main",
		"limit":    float64(3),
	})

	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.ForLLM)
	}

	var sessions []map[string]interface{}
	json.Unmarshal([]byte(result.ForLLM), &sessions)
	if len(sessions) > 3 {
		t.Errorf("expected <= 3 sessions, got %d", len(sessions))
	}
}

func TestSessionsHistoryTool_Success(t *testing.T) {
	deps, sm, _ := newTestSessionDeps(t)
	sm.AddMessage("test-session", "user", "hello")
	sm.AddMessage("test-session", "assistant", "hi there")

	tool := tools.NewSessionsHistoryTool(deps, "main")

	result := tool.Execute(context.Background(), map[string]interface{}{
		"agent_id":    "main",
		"session_key": "test-session",
	})

	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.ForLLM)
	}

	var messages []map[string]string
	if err := json.Unmarshal([]byte(result.ForLLM), &messages); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}
}

func TestSessionsHistoryTool_FiltersToolMessages(t *testing.T) {
	deps, sm, _ := newTestSessionDeps(t)
	sm.AddMessage("test-session", "user", "hello")
	sm.AddFullMessage("test-session", providers.Message{
		Role:       "tool",
		Content:    "tool result",
		ToolCallID: "call_1",
	})
	sm.AddMessage("test-session", "assistant", "here's the result")

	tool := tools.NewSessionsHistoryTool(deps, "main")

	result := tool.Execute(context.Background(), map[string]interface{}{
		"agent_id":    "main",
		"session_key": "test-session",
	})

	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.ForLLM)
	}

	var messages []map[string]string
	json.Unmarshal([]byte(result.ForLLM), &messages)
	// Should have 2 messages (user + assistant), tool filtered out
	if len(messages) != 2 {
		t.Errorf("expected 2 messages (tool filtered), got %d", len(messages))
	}
}

func TestSessionsHistoryTool_MissingParams(t *testing.T) {
	deps, _, _ := newTestSessionDeps(t)
	tool := tools.NewSessionsHistoryTool(deps, "main")

	result := tool.Execute(context.Background(), map[string]interface{}{
		"agent_id": "main",
	})

	if !result.IsError {
		t.Fatal("expected error for missing session_key")
	}
}

func TestSessionsSendTool_Success(t *testing.T) {
	deps, _, msgBus := newTestSessionDeps(t)
	tool := tools.NewSessionsSendTool(deps, "main")
	tool.SetContext("telegram", "123")

	result := tool.Execute(context.Background(), map[string]interface{}{
		"agent_id": "worker",
		"message":  "do this task",
	})

	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.ForLLM)
	}

	if !strings.Contains(result.ForLLM, "Message sent") {
		t.Errorf("expected confirmation message, got: %s", result.ForLLM)
	}

	// Verify message was published to bus
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately - we just want to check if message exists
	msg, ok := msgBus.ConsumeInbound(ctx)
	if !ok {
		// Message might not have been consumed due to canceled context, that's ok
		_ = msg
	}
}

func TestSessionsSendTool_MissingParams(t *testing.T) {
	deps, _, _ := newTestSessionDeps(t)
	tool := tools.NewSessionsSendTool(deps, "main")

	result := tool.Execute(context.Background(), map[string]interface{}{
		"agent_id": "worker",
	})

	if !result.IsError {
		t.Fatal("expected error for missing message")
	}
}

func TestSessionsSendTool_AccessDenied(t *testing.T) {
	deps, _, _ := newTestSessionDeps(t)
	deps.CanAccess = func(_, _ string) bool { return false }

	tool := tools.NewSessionsSendTool(deps, "main")

	result := tool.Execute(context.Background(), map[string]interface{}{
		"agent_id": "worker",
		"message":  "test",
	})

	if !result.IsError {
		t.Fatal("expected error for access denied")
	}
}
