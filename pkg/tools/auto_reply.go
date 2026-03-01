package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// AutoReplyRule defines an auto-reply rule for a session/channel.
type AutoReplyRule struct {
	ID         string `json:"id"`
	Channel    string `json:"channel"`
	ChatID     string `json:"chat_id,omitempty"`
	Pattern    string `json:"pattern,omitempty"`    // regex pattern to match (empty = all messages)
	Response   string `json:"response,omitempty"`   // static response (empty = use agent)
	Activation string `json:"activation"`           // "always", "mention", "scheduled"
	Enabled    bool   `json:"enabled"`
}

// AutoReplyManager manages auto-reply rules.
type AutoReplyManager struct {
	mu    sync.RWMutex
	rules map[string]*AutoReplyRule
}

// NewAutoReplyManager creates a new auto-reply manager.
func NewAutoReplyManager() *AutoReplyManager {
	return &AutoReplyManager{
		rules: make(map[string]*AutoReplyRule),
	}
}

// AddRule adds or updates an auto-reply rule.
func (m *AutoReplyManager) AddRule(rule *AutoReplyRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules[rule.ID] = rule
}

// RemoveRule removes an auto-reply rule.
func (m *AutoReplyManager) RemoveRule(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rules[id]; !ok {
		return fmt.Errorf("rule not found: %s", id)
	}
	delete(m.rules, id)
	return nil
}

// GetRules returns all rules.
func (m *AutoReplyManager) GetRules() []*AutoReplyRule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*AutoReplyRule, 0, len(m.rules))
	for _, rule := range m.rules {
		result = append(result, rule)
	}
	return result
}

// ShouldReply checks if a message matches any active auto-reply rule.
func (m *AutoReplyManager) ShouldReply(channel, chatID, content string) *AutoReplyRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rule := range m.rules {
		if !rule.Enabled {
			continue
		}
		if rule.Channel != "" && rule.Channel != channel {
			continue
		}
		if rule.ChatID != "" && rule.ChatID != chatID {
			continue
		}
		if rule.Activation == "always" {
			return rule
		}
		// For "mention" activation, the caller needs to check if the bot was mentioned
	}
	return nil
}

// AutoReplyTool provides auto-reply management to the agent.
type AutoReplyTool struct {
	manager *AutoReplyManager
	counter int
}

func NewAutoReplyTool(manager *AutoReplyManager) *AutoReplyTool {
	if manager == nil {
		return nil
	}
	return &AutoReplyTool{manager: manager}
}

func (t *AutoReplyTool) Name() string { return "auto_reply" }
func (t *AutoReplyTool) Description() string {
	return "Manage auto-reply rules: add rules to automatically respond to messages on specific channels, list active rules, or remove rules."
}

func (t *AutoReplyTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: 'add', 'remove', 'list', 'enable', 'disable'",
				"enum":        []string{"add", "remove", "list", "enable", "disable"},
			},
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Rule ID (for remove/enable/disable)",
			},
			"channel": map[string]interface{}{
				"type":        "string",
				"description": "Channel for the rule (for add)",
			},
			"chat_id": map[string]interface{}{
				"type":        "string",
				"description": "Chat ID for the rule (for add, optional)",
			},
			"response": map[string]interface{}{
				"type":        "string",
				"description": "Static response text (for add, optional — empty means use agent)",
			},
			"activation": map[string]interface{}{
				"type":        "string",
				"description": "Activation mode: 'always' or 'mention' (for add)",
				"enum":        []string{"always", "mention"},
			},
		},
		"required": []string{"action"},
	}
}

func (t *AutoReplyTool) Execute(_ context.Context, args map[string]interface{}) *ToolResult {
	action, _ := args["action"].(string)

	switch action {
	case "add":
		channel, _ := args["channel"].(string)
		if channel == "" {
			return ErrorResult("channel is required for add action")
		}
		activation, _ := args["activation"].(string)
		if activation == "" {
			activation = "always"
		}

		t.counter++
		rule := &AutoReplyRule{
			ID:         fmt.Sprintf("rule-%d", t.counter),
			Channel:    channel,
			Activation: activation,
			Enabled:    true,
		}
		if chatID, ok := args["chat_id"].(string); ok {
			rule.ChatID = chatID
		}
		if response, ok := args["response"].(string); ok {
			rule.Response = response
		}

		t.manager.AddRule(rule)
		data, _ := json.Marshal(rule)
		return SilentResult(fmt.Sprintf("Rule added: %s", string(data)))

	case "remove":
		id, _ := args["id"].(string)
		if id == "" {
			return ErrorResult("id is required")
		}
		if err := t.manager.RemoveRule(id); err != nil {
			return ErrorResult(err.Error())
		}
		return SilentResult(fmt.Sprintf("Rule %s removed.", id))

	case "list":
		rules := t.manager.GetRules()
		if len(rules) == 0 {
			return SilentResult("No auto-reply rules configured.")
		}
		data, _ := json.MarshalIndent(rules, "", "  ")
		return SilentResult(string(data))

	case "enable":
		id, _ := args["id"].(string)
		if id == "" {
			return ErrorResult("id is required")
		}
		rules := t.manager.GetRules()
		for _, r := range rules {
			if r.ID == id {
				r.Enabled = true
				t.manager.AddRule(r)
				return SilentResult(fmt.Sprintf("Rule %s enabled.", id))
			}
		}
		return ErrorResult(fmt.Sprintf("rule not found: %s", id))

	case "disable":
		id, _ := args["id"].(string)
		if id == "" {
			return ErrorResult("id is required")
		}
		rules := t.manager.GetRules()
		for _, r := range rules {
			if r.ID == id {
				r.Enabled = false
				t.manager.AddRule(r)
				return SilentResult(fmt.Sprintf("Rule %s disabled.", id))
			}
		}
		return ErrorResult(fmt.Sprintf("rule not found: %s", id))

	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}
