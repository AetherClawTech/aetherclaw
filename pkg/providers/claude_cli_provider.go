package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ClaudeCliProvider implements LLMProvider using the claude CLI as a subprocess.
type ClaudeCliProvider struct {
	command    string
	workspace  string
	mcpConfigs []MCPProviderConfig
}

// NewClaudeCliProvider creates a new Claude CLI provider.
func NewClaudeCliProvider(workspace string) *ClaudeCliProvider {
	return &ClaudeCliProvider{
		command:   "claude",
		workspace: workspace,
	}
}

// SetMCPConfigs sets MCP server configurations for CLI passthrough.
func (p *ClaudeCliProvider) SetMCPConfigs(configs []MCPProviderConfig) {
	p.mcpConfigs = configs
}

// Chat implements LLMProvider.Chat by executing the claude CLI.
func (p *ClaudeCliProvider) Chat(
	ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]any,
) (*LLMResponse, error) {
	systemPrompt := p.buildSystemPrompt(messages, tools)
	prompt := p.messagesToPrompt(messages)

	args := []string{"-p", "--output-format", "json", "--dangerously-skip-permissions", "--no-chrome"}
	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}
	if model != "" && model != "claude-code" {
		args = append(args, "--model", model)
	}

	args = append(args, "-") // read from stdin

	// Inject MCP config AFTER "-" because --mcp-config is variadic
	// and would consume "-" as another config path if placed before it.
	if len(p.mcpConfigs) > 0 {
		mcpConfigPath, err := p.writeMCPConfigFile()
		if err != nil {
			return nil, fmt.Errorf("failed to write MCP config: %w", err)
		}
		defer os.Remove(mcpConfigPath)
		args = append(args, "--mcp-config", mcpConfigPath)
	}

	cmd := exec.CommandContext(ctx, p.command, args...)
	if p.workspace != "" {
		cmd.Dir = p.workspace
	}
	cmd.Stdin = bytes.NewReader([]byte(prompt))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderrStr := stderr.String(); stderrStr != "" {
			return nil, fmt.Errorf("claude cli error: %s", stderrStr)
		}
		return nil, fmt.Errorf("claude cli error: %w", err)
	}

	return p.parseClaudeCliResponse(stdout.String())
}

// writeMCPConfigFile creates a temporary JSON file in Claude Code's MCP config format.
func (p *ClaudeCliProvider) writeMCPConfigFile() (string, error) {
	servers := make(map[string]any, len(p.mcpConfigs))
	for _, cfg := range p.mcpConfigs {
		entry := map[string]any{
			"command": cfg.Command,
		}
		if len(cfg.Args) > 0 {
			entry["args"] = cfg.Args
		}
		if len(cfg.Env) > 0 {
			entry["env"] = cfg.Env
		}
		servers[cfg.Name] = entry
	}

	data, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err != nil {
		return "", fmt.Errorf("marshal MCP config: %w", err)
	}

	f, err := os.CreateTemp("", "aetherclaw-mcp-*.json")
	if err != nil {
		return "", fmt.Errorf("create temp MCP config file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("write MCP config file: %w", err)
	}
	return f.Name(), nil
}

// GetDefaultModel returns the default model identifier.
func (p *ClaudeCliProvider) GetDefaultModel() string {
	return "claude-code"
}

// messagesToPrompt converts messages to a CLI-compatible prompt string.
func (p *ClaudeCliProvider) messagesToPrompt(messages []Message) string {
	var parts []string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			// handled via --system-prompt flag
		case "user":
			text := msg.Content
			// For multimodal messages, append image references as text
			for _, cp := range msg.ContentParts {
				if cp.Type == "image" && cp.Source != nil && cp.Source.FilePath != "" {
					text += fmt.Sprintf("\n[Image: %s]", cp.Source.FilePath)
				}
			}
			parts = append(parts, "User: "+text)
		case "assistant":
			parts = append(parts, "Assistant: "+msg.Content)
		case "tool":
			parts = append(parts, fmt.Sprintf("[Tool Result for %s]: %s", msg.ToolCallID, msg.Content))
		}
	}

	// Simplify single user message
	if len(parts) == 1 && strings.HasPrefix(parts[0], "User: ") {
		return strings.TrimPrefix(parts[0], "User: ")
	}

	return strings.Join(parts, "\n")
}

// buildSystemPrompt combines system messages and tool definitions.
func (p *ClaudeCliProvider) buildSystemPrompt(messages []Message, tools []ToolDefinition) string {
	var parts []string

	for _, msg := range messages {
		if msg.Role == "system" {
			parts = append(parts, msg.Content)
		}
	}

	if len(tools) > 0 {
		parts = append(parts, p.buildToolsPrompt(tools))
	}

	return strings.Join(parts, "\n\n")
}

// buildToolsPrompt creates the tool definitions section for the system prompt.
func (p *ClaudeCliProvider) buildToolsPrompt(tools []ToolDefinition) string {
	var sb strings.Builder

	sb.WriteString("## Available Tools\n\n")
	sb.WriteString("When you need to use a tool, respond with ONLY a JSON object:\n\n")
	sb.WriteString("```json\n")
	sb.WriteString(
		`{"tool_calls":[{"id":"call_xxx","type":"function","function":{"name":"tool_name","arguments":"{...}"}}]}`,
	)
	sb.WriteString("\n```\n\n")
	sb.WriteString("CRITICAL: The 'arguments' field MUST be a JSON-encoded STRING.\n\n")
	sb.WriteString("### Tool Definitions:\n\n")

	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		sb.WriteString(fmt.Sprintf("#### %s\n", tool.Function.Name))
		if tool.Function.Description != "" {
			sb.WriteString(fmt.Sprintf("Description: %s\n", tool.Function.Description))
		}
		if len(tool.Function.Parameters) > 0 {
			paramsJSON, _ := json.Marshal(tool.Function.Parameters)
			sb.WriteString(fmt.Sprintf("Parameters:\n```json\n%s\n```\n", string(paramsJSON)))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// parseClaudeCliResponse parses the JSON output from the claude CLI.
func (p *ClaudeCliProvider) parseClaudeCliResponse(output string) (*LLMResponse, error) {
	var resp claudeCliJSONResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse claude cli response: %w", err)
	}

	if resp.IsError {
		return nil, fmt.Errorf("claude cli returned error: %s", resp.Result)
	}

	toolCalls := p.extractToolCalls(resp.Result)

	finishReason := "stop"
	content := resp.Result
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
		content = p.stripToolCallsJSON(resp.Result)
	}

	var usage *UsageInfo
	if resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0 {
		usage = &UsageInfo{
			PromptTokens:     resp.Usage.InputTokens + resp.Usage.CacheCreationInputTokens + resp.Usage.CacheReadInputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.CacheCreationInputTokens + resp.Usage.CacheReadInputTokens + resp.Usage.OutputTokens,
		}
	}

	return &LLMResponse{
		Content:      strings.TrimSpace(content),
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
		Usage:        usage,
	}, nil
}

// extractToolCalls delegates to the shared extractToolCallsFromText function.
func (p *ClaudeCliProvider) extractToolCalls(text string) []ToolCall {
	return extractToolCallsFromText(text)
}

// stripToolCallsJSON delegates to the shared stripToolCallsFromText function.
func (p *ClaudeCliProvider) stripToolCallsJSON(text string) string {
	return stripToolCallsFromText(text)
}

// findMatchingBrace finds the index after the closing brace matching the opening brace at pos.
func findMatchingBrace(text string, pos int) int {
	depth := 0
	for i := pos; i < len(text); i++ {
		if text[i] == '{' {
			depth++
		} else if text[i] == '}' {
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return pos
}

// claudeCliJSONResponse represents the JSON output from the claude CLI.
// Matches the real claude CLI v2.x output format.
type claudeCliJSONResponse struct {
	Type         string             `json:"type"`
	Subtype      string             `json:"subtype"`
	IsError      bool               `json:"is_error"`
	Result       string             `json:"result"`
	SessionID    string             `json:"session_id"`
	TotalCostUSD float64            `json:"total_cost_usd"`
	DurationMS   int                `json:"duration_ms"`
	DurationAPI  int                `json:"duration_api_ms"`
	NumTurns     int                `json:"num_turns"`
	Usage        claudeCliUsageInfo `json:"usage"`
}

// claudeCliUsageInfo represents token usage from the claude CLI response.
type claudeCliUsageInfo struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}
