package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ChannelActionCallback is called when a channel action is requested.
type ChannelActionCallback func(channel, chatID, action string, params map[string]any) error

// ChannelActionsTool provides channel-specific actions (pin, delete, react, etc.).
type ChannelActionsTool struct {
	callback ChannelActionCallback
	channel  string
	chatID   string
}

func NewChannelActionsTool() *ChannelActionsTool {
	return &ChannelActionsTool{}
}

func (t *ChannelActionsTool) Name() string { return "channel_actions" }
func (t *ChannelActionsTool) Description() string {
	return "Perform channel-specific actions: pin/unpin messages, delete messages, add reactions, forward messages. Works with Telegram, Discord, and Slack."
}

func (t *ChannelActionsTool) SetContext(channel, chatID string) {
	t.channel = channel
	t.chatID = chatID
}

func (t *ChannelActionsTool) SetCallback(cb ChannelActionCallback) {
	t.callback = cb
}

func (t *ChannelActionsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Action to perform",
				"enum":        []string{"pin", "unpin", "delete", "react", "unreact", "forward", "edit"},
			},
			"message_id": map[string]any{
				"type":        "string",
				"description": "Message ID to act on",
			},
			"channel": map[string]any{
				"type":        "string",
				"description": "Target channel (defaults to current channel)",
			},
			"chat_id": map[string]any{
				"type":        "string",
				"description": "Target chat ID (defaults to current chat)",
			},
			"emoji": map[string]any{
				"type":        "string",
				"description": "Emoji for react/unreact actions",
			},
			"text": map[string]any{
				"type":        "string",
				"description": "New text for edit action",
			},
			"target_chat_id": map[string]any{
				"type":        "string",
				"description": "Target chat for forward action",
			},
		},
		"required": []string{"action", "message_id"},
	}
}

func (t *ChannelActionsTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	messageID, _ := args["message_id"].(string)
	if action == "" || messageID == "" {
		return ErrorResult("action and message_id are required")
	}

	channel, _ := args["channel"].(string)
	if channel == "" {
		channel = t.channel
	}
	chatID, _ := args["chat_id"].(string)
	if chatID == "" {
		chatID = t.chatID
	}

	if channel == "" {
		return ErrorResult("no channel specified")
	}

	if t.callback == nil {
		return ErrorResult("channel actions not configured (no callback)")
	}

	params := map[string]any{
		"message_id": messageID,
	}
	if emoji, ok := args["emoji"].(string); ok {
		params["emoji"] = emoji
	}
	if text, ok := args["text"].(string); ok {
		params["text"] = text
	}
	if targetChatID, ok := args["target_chat_id"].(string); ok {
		params["target_chat_id"] = targetChatID
	}

	err := t.callback(channel, chatID, action, params)
	if err != nil {
		return ErrorResult(fmt.Sprintf("channel action '%s' failed: %v", action, err))
	}

	result := map[string]any{
		"action":     action,
		"message_id": messageID,
		"channel":    channel,
		"status":     "success",
	}
	data, _ := json.Marshal(result)
	return SilentResult(string(data))
}
