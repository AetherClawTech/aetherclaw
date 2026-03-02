package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/AetherClawTech/aetherclaw/pkg/tools"
)

// MCPToolAdapter adapts an MCP tool to the AetherClaw Tool interface.
type MCPToolAdapter struct {
	client   *MCPClient
	mcpTool  mcp.Tool
	fullName string
}

// NewMCPToolAdapter creates a tool adapter that delegates execution to an MCP server.
func NewMCPToolAdapter(client *MCPClient, mcpTool mcp.Tool) *MCPToolAdapter {
	return &MCPToolAdapter{
		client:   client,
		mcpTool:  mcpTool,
		fullName: client.ToolPrefix() + mcpTool.Name,
	}
}

// Name returns the prefixed tool name: mcp_<server>_<tool>.
func (a *MCPToolAdapter) Name() string {
	return a.fullName
}

// Description returns the MCP tool description.
func (a *MCPToolAdapter) Description() string {
	return a.mcpTool.Description
}

// Parameters returns the MCP tool input schema as a map.
func (a *MCPToolAdapter) Parameters() map[string]any {
	if a.mcpTool.RawInputSchema != nil {
		var params map[string]any
		if err := json.Unmarshal(a.mcpTool.RawInputSchema, &params); err == nil {
			return params
		}
	}
	// Fallback: if InputSchema has populated fields, serialize it
	data, err := json.Marshal(a.mcpTool.InputSchema)
	if err != nil {
		return map[string]any{"type": "object"}
	}
	var params map[string]any
	if err := json.Unmarshal(data, &params); err != nil {
		return map[string]any{"type": "object"}
	}
	return params
}

// Execute delegates the tool call to the remote MCP server.
func (a *MCPToolAdapter) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	result, err := a.client.CallTool(ctx, a.mcpTool.Name, args)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("MCP tool %q error: %v", a.fullName, err))
	}

	if result == nil {
		return tools.ErrorResult(fmt.Sprintf("MCP tool %q returned nil", a.fullName))
	}

	// Extract text content from the MCP result
	content := extractTextContent(result)

	if result.IsError {
		return tools.ErrorResult(content)
	}
	return &tools.ToolResult{ForLLM: content}
}

// extractTextContent extracts text from MCP CallToolResult content blocks.
func extractTextContent(result *mcp.CallToolResult) string {
	var parts []string
	for _, c := range result.Content {
		switch tc := c.(type) {
		case mcp.TextContent:
			parts = append(parts, tc.Text)
		case mcp.ImageContent:
			parts = append(parts, fmt.Sprintf("[Image: %s]", tc.MIMEType))
		case mcp.AudioContent:
			parts = append(parts, "[Audio content]")
		}
	}
	if len(parts) == 0 {
		return "no content"
	}
	return strings.Join(parts, "\n")
}
