package mcp

// MCPClientConfig configures a connection to an external MCP server.
type MCPClientConfig struct {
	Name      string            `json:"name"`              // server name (used as tool prefix)
	Transport string            `json:"transport"`         // "stdio", "sse", or "http"
	Command   string            `json:"command,omitempty"` // for stdio: command to launch
	Args      []string          `json:"args,omitempty"`    // for stdio: command arguments
	URL       string            `json:"url,omitempty"`     // for sse/http: server URL
	Env       map[string]string `json:"env,omitempty"`     // environment variables for stdio subprocess
}
