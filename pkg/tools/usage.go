package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AetherClawTech/aetherclaw/pkg/usage"
)

// UsageTool provides usage statistics to the agent.
type UsageTool struct {
	tracker *usage.Tracker
}

func NewUsageTool(tracker *usage.Tracker) *UsageTool {
	if tracker == nil {
		return nil
	}
	return &UsageTool{tracker: tracker}
}

func (t *UsageTool) Name() string { return "usage" }
func (t *UsageTool) Description() string {
	return "View LLM usage statistics: token counts, costs, per-model breakdown. Use action 'today' for today's usage, 'range' for multiple days, or 'summary' for a quick overview."
}

func (t *UsageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Action: 'today', 'range', or 'summary'",
				"enum":        []string{"today", "range", "summary"},
			},
			"days": map[string]any{
				"type":        "integer",
				"description": "Number of days for 'range' action (default: 7)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *UsageTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	if action == "" {
		action = "today"
	}

	switch action {
	case "today":
		today := time.Now().Format("2006-01-02")
		summary, err := t.tracker.GetDailySummary(today)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to get usage: %v", err))
		}
		if summary == nil || summary.TotalCalls == 0 {
			return SilentResult("No usage recorded today.")
		}
		data, _ := json.MarshalIndent(summary, "", "  ")
		return SilentResult(string(data))

	case "range":
		days := 7
		if d, ok := args["days"].(float64); ok && int(d) > 0 {
			days = int(d)
			if days > 90 {
				days = 90
			}
		}
		summaries, err := t.tracker.GetRangeSummary(days)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to get usage: %v", err))
		}
		if len(summaries) == 0 {
			return SilentResult(fmt.Sprintf("No usage recorded in the last %d days.", days))
		}
		data, _ := json.MarshalIndent(summaries, "", "  ")
		return SilentResult(string(data))

	case "summary":
		summaries, err := t.tracker.GetRangeSummary(30)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to get usage: %v", err))
		}

		var totalTokens, totalCalls int
		var totalCost float64
		models := make(map[string]int)

		for _, s := range summaries {
			totalTokens += s.TotalTokens
			totalCalls += s.TotalCalls
			totalCost += s.TotalCostUSD
			for model, mu := range s.ByModel {
				models[model] += mu.Calls
			}
		}

		result := map[string]any{
			"period":       "last 30 days",
			"total_calls":  totalCalls,
			"total_tokens": totalTokens,
			"total_cost":   usage.FormatCost(totalCost),
			"models_used":  models,
			"days_active":  len(summaries),
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return SilentResult(string(data))

	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s (use 'today', 'range', or 'summary')", action))
	}
}
