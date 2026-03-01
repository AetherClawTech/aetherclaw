package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AetherClawTech/aetherclaw/pkg/pairing"
)

// PairingTool manages device pairing approval and revocation.
type PairingTool struct {
	store *pairing.Store
}

func NewPairingTool(store *pairing.Store) *PairingTool {
	if store == nil {
		return nil
	}
	return &PairingTool{store: store}
}

func (t *PairingTool) Name() string { return "pairing" }
func (t *PairingTool) Description() string {
	return "Manage device pairing: approve or reject unknown senders, list pending/approved devices, revoke access."
}

func (t *PairingTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: 'list_pending', 'list_approved', 'approve', 'reject', 'revoke'",
				"enum":        []string{"list_pending", "list_approved", "approve", "reject", "revoke"},
			},
			"code": map[string]interface{}{
				"type":        "string",
				"description": "Pairing code (for approve/reject actions)",
			},
			"channel": map[string]interface{}{
				"type":        "string",
				"description": "Channel name (for revoke action)",
			},
			"sender_id": map[string]interface{}{
				"type":        "string",
				"description": "Sender ID (for revoke action)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *PairingTool) Execute(_ context.Context, args map[string]interface{}) *ToolResult {
	action, _ := args["action"].(string)

	switch action {
	case "list_pending":
		pending := t.store.ListPending()
		if len(pending) == 0 {
			return SilentResult("No pending pairing requests.")
		}
		data, _ := json.MarshalIndent(pending, "", "  ")
		return SilentResult(string(data))

	case "list_approved":
		approved := t.store.ListApproved()
		if len(approved) == 0 {
			return SilentResult("No approved devices.")
		}
		data, _ := json.MarshalIndent(approved, "", "  ")
		return SilentResult(string(data))

	case "approve":
		code, _ := args["code"].(string)
		if code == "" {
			return ErrorResult("code is required for approve action")
		}
		device, err := t.store.Approve(code, "agent")
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to approve: %v", err))
		}
		data, _ := json.Marshal(device)
		return &ToolResult{
			ForLLM:  fmt.Sprintf("Device approved: %s", string(data)),
			ForUser: fmt.Sprintf("Device approved: %s on %s", device.SenderID, device.Channel),
		}

	case "reject":
		code, _ := args["code"].(string)
		if code == "" {
			return ErrorResult("code is required for reject action")
		}
		if err := t.store.Reject(code); err != nil {
			return ErrorResult(fmt.Sprintf("failed to reject: %v", err))
		}
		return SilentResult("Pairing request rejected.")

	case "revoke":
		channel, _ := args["channel"].(string)
		senderID, _ := args["sender_id"].(string)
		if channel == "" || senderID == "" {
			return ErrorResult("channel and sender_id are required for revoke action")
		}
		if err := t.store.Revoke(channel, senderID); err != nil {
			return ErrorResult(fmt.Sprintf("failed to revoke: %v", err))
		}
		return SilentResult(fmt.Sprintf("Access revoked for %s:%s", channel, senderID))

	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}
