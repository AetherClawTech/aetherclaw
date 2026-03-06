package mcpclient

import (
	"context"
	"sync"
	"time"

	"github.com/AetherClawTech/aetherclaw/pkg/logger"
	mcptypes "github.com/AetherClawTech/aetherclaw/pkg/mcp"
	"github.com/AetherClawTech/aetherclaw/pkg/tools"
)

const (
	defaultHealthInterval  = 30 * time.Second
	defaultPingTimeout     = 5 * time.Second
	maxConsecutiveFailures = 3
	maxBackoff             = 5 * time.Minute
)

// AgentToolsProvider gives the health monitor access to agent tool registries
// so it can re-register tools after reconnection.
type AgentToolsProvider interface {
	ListAgentIDs() []string
	GetAgentTools(agentID string) *tools.ToolRegistry
}

// MCPClientManager manages connections to multiple external MCP servers.
type MCPClientManager struct {
	clients  []*MCPClient
	mu       sync.Mutex
	stopChan chan struct{}

	// health tracking per client index
	failures []int
	backoffs []time.Duration
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
		m.failures = append(m.failures, 0)
		m.backoffs = append(m.backoffs, defaultHealthInterval)
		logger.InfoCF("mcp-client", "MCP client connected", map[string]any{
			"server":    cfg.Name,
			"tools":     len(c.Tools()),
			"transport": cfg.Transport,
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

// RegisterToolsToAgent registers tools from servers allowed for the given agent ID.
func (m *MCPClientManager) RegisterToolsToAgent(agentID string, registry *tools.ToolRegistry) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, c := range m.clients {
		if !c.AllowsAgent(agentID) {
			continue
		}
		for _, mcpTool := range c.Tools() {
			adapter := NewMCPToolAdapter(c, mcpTool)
			registry.Register(adapter)
			count++
		}
	}
	return count
}

// StartHealthMonitor starts a background goroutine that periodically pings
// each connected MCP server and attempts reconnection on consecutive failures.
func (m *MCPClientManager) StartHealthMonitor(provider AgentToolsProvider) {
	m.mu.Lock()
	if m.stopChan != nil {
		m.mu.Unlock()
		return // already running
	}
	m.stopChan = make(chan struct{})
	m.mu.Unlock()

	go m.healthLoop(provider)
}

func (m *MCPClientManager) healthLoop(provider AgentToolsProvider) {
	ticker := time.NewTicker(defaultHealthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.checkAllClients(provider)
		}
	}
}

func (m *MCPClientManager) checkAllClients(provider AgentToolsProvider) {
	m.mu.Lock()
	clients := make([]*MCPClient, len(m.clients))
	copy(clients, m.clients)
	m.mu.Unlock()

	for i, c := range clients {
		m.checkClient(i, c, provider)
	}
}

func (m *MCPClientManager) checkClient(idx int, c *MCPClient, provider AgentToolsProvider) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPingTimeout)
	defer cancel()

	err := c.Ping(ctx)

	m.mu.Lock()
	defer m.mu.Unlock()

	if err == nil {
		if m.failures[idx] > 0 {
			logger.InfoCF("mcp-health", "Ping OK (recovered)", map[string]any{
				"server": c.Name(),
			})
		}
		m.failures[idx] = 0
		m.backoffs[idx] = defaultHealthInterval
		return
	}

	m.failures[idx]++
	logger.WarnCF("mcp-health", "Ping failed", map[string]any{
		"server":              c.Name(),
		"error":               err.Error(),
		"consecutive_failures": m.failures[idx],
	})

	if m.failures[idx] < maxConsecutiveFailures {
		return
	}

	// Attempt reconnect
	logger.InfoCF("mcp-health", "Reconnecting", map[string]any{
		"server":   c.Name(),
		"failures": m.failures[idx],
	})

	reconnCtx, reconnCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer reconnCancel()

	if err := c.Reconnect(reconnCtx); err != nil {
		// Increase backoff
		m.backoffs[idx] = min(m.backoffs[idx]*2, maxBackoff)
		logger.ErrorCF("mcp-health", "Reconnect failed", map[string]any{
			"server":       c.Name(),
			"error":        err.Error(),
			"next_backoff": m.backoffs[idx].String(),
		})
		return
	}

	// Reconnect succeeded — re-register tools to allowed agents
	m.failures[idx] = 0
	m.backoffs[idx] = defaultHealthInterval

	toolCount := 0
	if provider != nil {
		for _, agentID := range provider.ListAgentIDs() {
			if !c.AllowsAgent(agentID) {
				continue
			}
			registry := provider.GetAgentTools(agentID)
			if registry == nil {
				continue
			}
			for _, mcpTool := range c.Tools() {
				adapter := NewMCPToolAdapter(c, mcpTool)
				registry.Register(adapter)
				toolCount++
			}
		}
	}

	logger.InfoCF("mcp-health", "Reconnected", map[string]any{
		"server":          c.Name(),
		"tools_registered": toolCount,
	})
}

// StopAll gracefully disconnects from all MCP servers and stops health monitoring.
func (m *MCPClientManager) StopAll() {
	m.stopHealthMonitor()

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
	m.failures = nil
	m.backoffs = nil
}

func (m *MCPClientManager) stopHealthMonitor() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopChan != nil {
		close(m.stopChan)
		m.stopChan = nil
	}
}

// ClientCount returns the number of active MCP client connections.
func (m *MCPClientManager) ClientCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.clients)
}
