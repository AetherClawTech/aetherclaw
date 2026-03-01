package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AetherClawTech/aetherclaw/pkg/memory"
)

// MemoryTool provides memory search and storage to the agent.
type MemoryTool struct {
	index *memory.Index
}

func NewMemoryTool(index *memory.Index) *MemoryTool {
	if index == nil {
		return nil
	}
	return &MemoryTool{index: index}
}

func (t *MemoryTool) Name() string { return "memory" }
func (t *MemoryTool) Description() string {
	return "Search, store, list, or delete memories. Memories are persistent and searchable with keyword and semantic (vector) search. Use 'search' to find relevant memories, 'store' to save new information, 'list' to browse, 'delete' to remove."
}

func (t *MemoryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: 'search', 'store', 'list', 'delete', 'count'",
				"enum":        []string{"search", "store", "list", "delete", "count"},
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query (for search action)",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to store (for store action)",
			},
			"source": map[string]interface{}{
				"type":        "string",
				"description": "Source/origin of the memory (for store action)",
			},
			"tags": map[string]interface{}{
				"type":        "string",
				"description": "Comma-separated tags (for store/list actions)",
			},
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Memory chunk ID (for delete action)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum results (default: 10)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *MemoryTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	action, _ := args["action"].(string)

	switch action {
	case "search":
		query, _ := args["query"].(string)
		if query == "" {
			return ErrorResult("query is required for search action")
		}
		limit := 10
		if l, ok := args["limit"].(float64); ok && int(l) > 0 {
			limit = int(l)
		}

		results := t.index.Search(ctx, query, limit)
		if len(results) == 0 {
			return SilentResult("No memories found matching your query.")
		}

		data, _ := json.MarshalIndent(results, "", "  ")
		return SilentResult(string(data))

	case "store":
		content, _ := args["content"].(string)
		if content == "" {
			return ErrorResult("content is required for store action")
		}
		source, _ := args["source"].(string)
		var tags []string
		if tagsStr, ok := args["tags"].(string); ok && tagsStr != "" {
			for _, tag := range strings.Split(tagsStr, ",") {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					tags = append(tags, tag)
				}
			}
		}

		chunk, err := t.index.Store(ctx, content, source, tags)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to store memory: %v", err))
		}

		return SilentResult(fmt.Sprintf("Memory stored (id: %s, %d chunks total)", chunk.ID, t.index.Count()))

	case "list":
		limit := 20
		if l, ok := args["limit"].(float64); ok && int(l) > 0 {
			limit = int(l)
		}

		var tags []string
		if tagsStr, ok := args["tags"].(string); ok && tagsStr != "" {
			for _, tag := range strings.Split(tagsStr, ",") {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					tags = append(tags, tag)
				}
			}
		}

		chunks := t.index.List(tags, limit)
		if len(chunks) == 0 {
			return SilentResult("No memories stored.")
		}

		// Return simplified list (without embeddings)
		simplified := make([]map[string]interface{}, len(chunks))
		for i, c := range chunks {
			simplified[i] = map[string]interface{}{
				"id":         c.ID,
				"content":    truncateStr(c.Content, 200),
				"source":     c.Source,
				"tags":       c.Tags,
				"created_at": c.CreatedAt.Format("2006-01-02 15:04"),
			}
		}

		data, _ := json.MarshalIndent(simplified, "", "  ")
		return SilentResult(string(data))

	case "delete":
		id, _ := args["id"].(string)
		if id == "" {
			return ErrorResult("id is required for delete action")
		}
		if err := t.index.Delete(id); err != nil {
			return ErrorResult(err.Error())
		}
		return SilentResult(fmt.Sprintf("Memory %s deleted.", id))

	case "count":
		count := t.index.Count()
		return SilentResult(fmt.Sprintf("%d memories stored.", count))

	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
