package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AetherClawTech/aetherclaw/pkg/bus"
	"github.com/AetherClawTech/aetherclaw/pkg/session"
)

// SessionsToolDeps holds the shared dependencies that session tools need.
type SessionsToolDeps struct {
	// GetSessions returns the SessionManager for a given agent ID.
	GetSessions func(agentID string) *session.SessionManager
	// CanAccess checks if currentAgentID is allowed to interact with targetAgentID.
	CanAccess func(currentAgentID, targetAgentID string) bool
	// ListAgentIDs returns all registered agent IDs.
	ListAgentIDs func() []string
	// Bus is the message bus for sessions_send.
	Bus *bus.MessageBus
}

// --- sessions_list ---

// SessionsListTool lists sessions for a target agent.
type SessionsListTool struct {
	deps           *SessionsToolDeps
	currentAgentID string
}

func NewSessionsListTool(deps *SessionsToolDeps, currentAgentID string) *SessionsListTool {
	return &SessionsListTool{deps: deps, currentAgentID: currentAgentID}
}

func (t *SessionsListTool) Name() string { return "sessions_list" }
func (t *SessionsListTool) Description() string {
	return "List sessions for an agent. Returns session keys, timestamps, and message counts."
}
func (t *SessionsListTool) SetContext(_, _ string) {}

func (t *SessionsListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"agent_id": map[string]interface{}{
				"type":        "string",
				"description": "Target agent ID to list sessions for",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum sessions to return (default 20)",
			},
		},
		"required": []string{"agent_id"},
	}
}

func (t *SessionsListTool) Execute(_ context.Context, args map[string]interface{}) *ToolResult {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return ErrorResult("agent_id is required")
	}

	if !t.deps.CanAccess(t.currentAgentID, agentID) {
		return ErrorResult(fmt.Sprintf("not allowed to access sessions for agent '%s'", agentID))
	}

	sm := t.deps.GetSessions(agentID)
	if sm == nil {
		return ErrorResult(fmt.Sprintf("agent '%s' not found", agentID))
	}

	limit := 20
	if l, ok := args["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
	}

	sessions := sm.ListSessions()
	if len(sessions) > limit {
		sessions = sessions[:limit]
	}

	data, err := json.Marshal(sessions)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to marshal sessions: %v", err))
	}

	return SilentResult(string(data))
}

// --- sessions_history ---

// SessionsHistoryTool reads message history from another agent's session.
type SessionsHistoryTool struct {
	deps           *SessionsToolDeps
	currentAgentID string
}

func NewSessionsHistoryTool(deps *SessionsToolDeps, currentAgentID string) *SessionsHistoryTool {
	return &SessionsHistoryTool{deps: deps, currentAgentID: currentAgentID}
}

func (t *SessionsHistoryTool) Name() string { return "sessions_history" }
func (t *SessionsHistoryTool) Description() string {
	return "Read message history from another agent's session. Returns recent messages (tool messages filtered out, capped at 80KB)."
}
func (t *SessionsHistoryTool) SetContext(_, _ string) {}

func (t *SessionsHistoryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"agent_id": map[string]interface{}{
				"type":        "string",
				"description": "Target agent ID",
			},
			"session_key": map[string]interface{}{
				"type":        "string",
				"description": "Session key to read history from",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum messages to return (default 50)",
			},
		},
		"required": []string{"agent_id", "session_key"},
	}
}

const maxHistoryBytes = 80 * 1024 // 80KB cap like OpenClaw

func (t *SessionsHistoryTool) Execute(_ context.Context, args map[string]interface{}) *ToolResult {
	agentID, _ := args["agent_id"].(string)
	sessionKey, _ := args["session_key"].(string)
	if agentID == "" || sessionKey == "" {
		return ErrorResult("agent_id and session_key are required")
	}

	if !t.deps.CanAccess(t.currentAgentID, agentID) {
		return ErrorResult(fmt.Sprintf("not allowed to access sessions for agent '%s'", agentID))
	}

	sm := t.deps.GetSessions(agentID)
	if sm == nil {
		return ErrorResult(fmt.Sprintf("agent '%s' not found", agentID))
	}

	limit := 50
	if l, ok := args["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
	}

	history := sm.GetHistory(sessionKey)

	// Filter out tool-role messages to reduce noise
	filtered := make([]map[string]string, 0, len(history))
	for _, msg := range history {
		if msg.Role == "tool" {
			continue
		}
		content := msg.Content
		// Truncate very long messages
		if len(content) > 4000 {
			content = content[:4000] + "... (truncated)"
		}
		filtered = append(filtered, map[string]string{
			"role":    msg.Role,
			"content": content,
		})
	}

	// Apply limit (take the most recent)
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	data, err := json.Marshal(filtered)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to marshal history: %v", err))
	}

	// Cap at maxHistoryBytes
	if len(data) > maxHistoryBytes {
		data = data[:maxHistoryBytes]
	}

	return SilentResult(string(data))
}

// --- sessions_send ---

// SessionsSendTool sends a message to another agent's session via the message bus.
type SessionsSendTool struct {
	deps           *SessionsToolDeps
	currentAgentID string
	channel        string
	chatID         string
}

func NewSessionsSendTool(deps *SessionsToolDeps, currentAgentID string) *SessionsSendTool {
	return &SessionsSendTool{deps: deps, currentAgentID: currentAgentID}
}

func (t *SessionsSendTool) Name() string { return "sessions_send" }
func (t *SessionsSendTool) Description() string {
	return "Send a message to another agent. The message is published to the target agent's inbound queue as a fire-and-forget operation."
}

func (t *SessionsSendTool) SetContext(channel, chatID string) {
	t.channel = channel
	t.chatID = chatID
}

func (t *SessionsSendTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"agent_id": map[string]interface{}{
				"type":        "string",
				"description": "Target agent ID to send the message to",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Message content to send",
			},
			"channel": map[string]interface{}{
				"type":        "string",
				"description": "Optional origin channel for routing the reply (defaults to current channel)",
			},
			"chat_id": map[string]interface{}{
				"type":        "string",
				"description": "Optional chat ID for routing the reply",
			},
		},
		"required": []string{"agent_id", "message"},
	}
}

func (t *SessionsSendTool) Execute(_ context.Context, args map[string]interface{}) *ToolResult {
	agentID, _ := args["agent_id"].(string)
	message, _ := args["message"].(string)
	if agentID == "" || message == "" {
		return ErrorResult("agent_id and message are required")
	}

	if !t.deps.CanAccess(t.currentAgentID, agentID) {
		return ErrorResult(fmt.Sprintf("not allowed to send messages to agent '%s'", agentID))
	}

	if t.deps.Bus == nil {
		return ErrorResult("message bus not available")
	}

	channel, _ := args["channel"].(string)
	chatID, _ := args["chat_id"].(string)
	if channel == "" {
		channel = t.channel
	}
	if chatID == "" {
		chatID = t.chatID
	}

	// Prefix the message content so the routing system knows it's for a specific agent
	content := fmt.Sprintf("[From agent %s]: %s", t.currentAgentID, message)

	t.deps.Bus.PublishInbound(context.Background(), bus.InboundMessage{
		Channel:  "internal",
		SenderID: t.currentAgentID,
		ChatID:   chatID,
		Content:  content,
		Metadata: map[string]string{
			"target_agent": agentID,
			"source_agent": t.currentAgentID,
			"peer_kind":    "agent",
			"peer_id":      agentID,
		},
	})

	sessionKey := fmt.Sprintf("agent:%s", agentID)
	return SilentResult(fmt.Sprintf("Message sent to agent '%s' (session: %s)", agentID, sessionKey))
}
