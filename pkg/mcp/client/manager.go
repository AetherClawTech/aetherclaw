package mcpclient

import (
	"context"
	"sync"

	"github.com/AetherClawTech/aetherclaw/pkg/logger"
	mcptypes "github.com/AetherClawTech/aetherclaw/pkg/mcp"
	"github.com/AetherClawTech/aetherclaw/pkg/tools"
)

// MCPClientManager manages connections to multiple external MCP servers.
type MCPClientManager struct {
	clients []*MCPClient
	mu      sync.Mutex
}

// NewMCPClientManager creates a new empty manager.
func NewMCPClientManager() *MCPClientManager {
	return &MCPClientManager{}
}

// StartFromConfig connects to all configured MCP servers.
// Individual failures are logged but don't prevent other servers from starting.
func (m *MCPClientManager) StartFromConfig(ctx context.Context, configs []mcptypes.MCPClientConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cfg := range configs {
		c := New(cfg)
		if err := c.Start(ctx); err != nil {
			logger.ErrorCF("mcp-client", "Failed to start MCP client", map[string]any{
				"server": cfg.Name,
				"error":  err.Error(),
			})
			continue
		}
		m.clients = append(m.clients, c)
		logger.InfoCF("mcp-client", "MCP client connected", map[string]any{
			"server":     cfg.Name,
			"tools":      len(c.Tools()),
			"transport":  cfg.Transport,
		})
	}
}

// RegisterToolsTo registers all discovered MCP tools to a ToolRegistry.
func (m *MCPClientManager) RegisterToolsTo(registry *tools.ToolRegistry) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, c := range m.clients {
		for _, mcpTool := range c.Tools() {
			adapter := NewMCPToolAdapter(c, mcpTool)
			registry.Register(adapter)
			count++
		}
	}
	return count
}

// StopAll gracefully disconnects from all MCP servers.
func (m *MCPClientManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, c := range m.clients {
		if err := c.Stop(); err != nil {
			logger.WarnCF("mcp-client", "Error stopping MCP client", map[string]any{
				"server": c.Name(),
				"error":  err.Error(),
			})
		}
	}
	m.clients = nil
}

// ClientCount returns the number of active MCP client connections.
func (m *MCPClientManager) ClientCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.clients)
}
