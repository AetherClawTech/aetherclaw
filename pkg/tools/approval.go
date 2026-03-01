package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ApprovalRequest represents a pending tool execution approval.
type ApprovalRequest struct {
	ID         string                 `json:"id"`
	ToolName   string                 `json:"tool_name"`
	Args       map[string]interface{} `json:"args"`
	Reason     string                 `json:"reason"`
	Channel    string                 `json:"channel"`
	ChatID     string                 `json:"chat_id"`
	CreatedAt  time.Time              `json:"created_at"`
	Status     string                 `json:"status"` // "pending", "approved", "rejected"
	ResolvedAt *time.Time             `json:"resolved_at,omitempty"`
}

// ApprovalManager manages execution approval requests.
type ApprovalManager struct {
	mu       sync.RWMutex
	requests map[string]*ApprovalRequest
	counter  int
}

// NewApprovalManager creates a new approval manager.
func NewApprovalManager() *ApprovalManager {
	return &ApprovalManager{
		requests: make(map[string]*ApprovalRequest),
	}
}

// Request creates a new approval request.
func (m *ApprovalManager) Request(toolName string, args map[string]interface{}, reason, channel, chatID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counter++
	id := fmt.Sprintf("approval-%d", m.counter)

	m.requests[id] = &ApprovalRequest{
		ID:        id,
		ToolName:  toolName,
		Args:      args,
		Reason:    reason,
		Channel:   channel,
		ChatID:    chatID,
		CreatedAt: time.Now(),
		Status:    "pending",
	}

	return id
}

// Approve approves a pending request.
func (m *ApprovalManager) Approve(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.requests[id]
	if !ok {
		return fmt.Errorf("approval request not found: %s", id)
	}
	if req.Status != "pending" {
		return fmt.Errorf("request %s already %s", id, req.Status)
	}

	now := time.Now()
	req.Status = "approved"
	req.ResolvedAt = &now
	return nil
}

// Reject rejects a pending request.
func (m *ApprovalManager) Reject(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.requests[id]
	if !ok {
		return fmt.Errorf("approval request not found: %s", id)
	}
	if req.Status != "pending" {
		return fmt.Errorf("request %s already %s", id, req.Status)
	}

	now := time.Now()
	req.Status = "rejected"
	req.ResolvedAt = &now
	return nil
}

// GetStatus returns the status of a request.
func (m *ApprovalManager) GetStatus(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	req, ok := m.requests[id]
	if !ok {
		return "", fmt.Errorf("approval request not found: %s", id)
	}
	return req.Status, nil
}

// ListPending returns all pending requests.
func (m *ApprovalManager) ListPending() []*ApprovalRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ApprovalRequest, 0)
	for _, req := range m.requests {
		if req.Status == "pending" {
			result = append(result, req)
		}
	}
	return result
}

// ApprovalTool provides the approval management interface to agents.
type ApprovalTool struct {
	manager *ApprovalManager
}

func NewApprovalTool(manager *ApprovalManager) *ApprovalTool {
	if manager == nil {
		return nil
	}
	return &ApprovalTool{manager: manager}
}

func (t *ApprovalTool) Name() string { return "exec_approval" }
func (t *ApprovalTool) Description() string {
	return "Manage execution approvals: list pending requests, approve or reject tool executions that require user confirmation."
}

func (t *ApprovalTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: 'list', 'approve', 'reject'",
				"enum":        []string{"list", "approve", "reject"},
			},
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Approval request ID (for approve/reject)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *ApprovalTool) Execute(_ context.Context, args map[string]interface{}) *ToolResult {
	action, _ := args["action"].(string)

	switch action {
	case "list":
		pending := t.manager.ListPending()
		if len(pending) == 0 {
			return SilentResult("No pending approval requests.")
		}
		data, _ := json.MarshalIndent(pending, "", "  ")
		return SilentResult(string(data))

	case "approve":
		id, _ := args["id"].(string)
		if id == "" {
			return ErrorResult("id is required")
		}
		if err := t.manager.Approve(id); err != nil {
			return ErrorResult(err.Error())
		}
		return SilentResult(fmt.Sprintf("Request %s approved.", id))

	case "reject":
		id, _ := args["id"].(string)
		if id == "" {
			return ErrorResult("id is required")
		}
		if err := t.manager.Reject(id); err != nil {
			return ErrorResult(err.Error())
		}
		return SilentResult(fmt.Sprintf("Request %s rejected.", id))

	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}
