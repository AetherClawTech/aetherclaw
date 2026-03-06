package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// mockApprover implements ApprovalRequester for testing.
type mockApprover struct {
	approve bool
	err     error
	calls   []string
}

func (m *mockApprover) RequestApproval(_ context.Context, agentID, toolName string, agentLevel, toolLevel AutonomyLevel) (bool, error) {
	m.calls = append(m.calls, fmt.Sprintf("%s:%s:L%d->L%d", agentID, toolName, agentLevel, toolLevel))
	return m.approve, m.err
}

func TestAutonomyHook_AgentAtOrAboveToolLevel(t *testing.T) {
	tests := []struct {
		name       string
		agentLevel AutonomyLevel
		tool       string
		toolLevel  AutonomyLevel
		wantErr    bool
	}{
		{"L0 agent, L0 tool", L0_Observe, "read_file", L0_Observe, false},
		{"L1 agent, L0 tool", L1_Reversible, "read_file", L0_Observe, false},
		{"L1 agent, L1 tool", L1_Reversible, "write_file", L1_Reversible, false},
		{"L2 agent, L0 tool", L2_Recoverable, "read_file", L0_Observe, false},
		{"L2 agent, L1 tool", L2_Recoverable, "write_file", L1_Reversible, false},
		{"L2 agent, L2 tool", L2_Recoverable, "web_search", L2_Recoverable, false},
		{"L3 agent, L3 tool", L3_Irreversible, "exec", L3_Irreversible, false},
		{"L0 agent, L1 tool blocked", L0_Observe, "write_file", L1_Reversible, true},
		{"L1 agent, L2 tool blocked", L1_Reversible, "web_search", L2_Recoverable, true},
		{"L1 agent, L3 tool blocked", L1_Reversible, "exec", L3_Irreversible, true},
		{"L2 agent, L3 tool blocked", L2_Recoverable, "exec", L3_Irreversible, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := NewAutonomyHook("test-agent", tt.agentLevel, nil)
			err := hook.BeforeExecute(context.Background(), tt.tool, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("BeforeExecute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAutonomyHook_UnknownToolDefaultsToL1(t *testing.T) {
	hook := NewAutonomyHook("test-agent", L0_Observe, nil)
	err := hook.BeforeExecute(context.Background(), "unknown_custom_tool", nil)
	if err == nil {
		t.Error("L0 agent should be blocked by unknown tool (defaults to L1)")
	}

	hook2 := NewAutonomyHook("test-agent", L1_Reversible, nil)
	err = hook2.BeforeExecute(context.Background(), "unknown_custom_tool", nil)
	if err != nil {
		t.Errorf("L1 agent should be allowed for unknown tool (defaults to L1): %v", err)
	}
}

func TestAutonomyHook_SetClassification(t *testing.T) {
	hook := NewAutonomyHook("test-agent", L1_Reversible, nil)

	// exec is L3 by default — should block L1 agent
	err := hook.BeforeExecute(context.Background(), "exec", nil)
	if err == nil {
		t.Error("exec should be blocked for L1 agent")
	}

	// Override exec to L1
	hook.SetClassification("exec", L1_Reversible)
	err = hook.BeforeExecute(context.Background(), "exec", nil)
	if err != nil {
		t.Errorf("exec should be allowed after override: %v", err)
	}
}

func TestAutonomyHook_ApproverCalled(t *testing.T) {
	approver := &mockApprover{approve: true}
	hook := NewAutonomyHook("builder", L1_Reversible, approver)

	err := hook.BeforeExecute(context.Background(), "exec", nil)
	if err != nil {
		t.Errorf("expected approval to succeed: %v", err)
	}
	if len(approver.calls) != 1 {
		t.Fatalf("expected 1 approval call, got %d", len(approver.calls))
	}
	if !strings.Contains(approver.calls[0], "builder:exec:L1->L3") {
		t.Errorf("approval call = %q, expected builder:exec:L1->L3", approver.calls[0])
	}
}

func TestAutonomyHook_ApproverDenies(t *testing.T) {
	approver := &mockApprover{approve: false}
	hook := NewAutonomyHook("builder", L1_Reversible, approver)

	err := hook.BeforeExecute(context.Background(), "exec", nil)
	if err == nil {
		t.Error("expected denial error")
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Errorf("error = %q, expected 'denied'", err.Error())
	}
}

func TestAutonomyHook_ApproverError(t *testing.T) {
	approver := &mockApprover{err: fmt.Errorf("timeout")}
	hook := NewAutonomyHook("builder", L1_Reversible, approver)

	err := hook.BeforeExecute(context.Background(), "exec", nil)
	if err == nil {
		t.Error("expected error from approver")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error = %q, expected 'timeout'", err.Error())
	}
}

func TestAutonomyHook_AfterExecuteNoop(t *testing.T) {
	hook := NewAutonomyHook("test", L1_Reversible, nil)
	// Should not panic
	hook.AfterExecute(context.Background(), "read_file", nil, nil)
}

func TestAutonomyHook_ApproverNotCalledWhenAllowed(t *testing.T) {
	approver := &mockApprover{approve: true}
	hook := NewAutonomyHook("admin", L3_Irreversible, approver)

	err := hook.BeforeExecute(context.Background(), "exec", nil)
	if err != nil {
		t.Errorf("L3 agent should be allowed for exec: %v", err)
	}
	if len(approver.calls) != 0 {
		t.Error("approver should not be called when agent has sufficient level")
	}
}

func TestAutonomyHook_IntegrationWithRegistry(t *testing.T) {
	reg := NewToolRegistry()
	reg.Register(&dummyTool{name: "read_file"})
	reg.Register(&dummyTool{name: "exec"})

	hook := NewAutonomyHook("builder", L1_Reversible, nil)
	reg.AddHook(hook)

	// read_file (L0) should work for L1 agent
	result := reg.Execute(context.Background(), "read_file", nil)
	if result.IsError {
		t.Errorf("read_file should be allowed: %s", result.ForLLM)
	}

	// exec (L3) should be blocked for L1 agent
	result = reg.Execute(context.Background(), "exec", nil)
	if !result.IsError {
		t.Error("exec should be blocked for L1 agent")
	}
	if !strings.Contains(result.ForLLM, "autonomy") {
		t.Errorf("error should mention autonomy: %s", result.ForLLM)
	}
}
