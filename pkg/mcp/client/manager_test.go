package mcpclient

import (
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcptypes "github.com/AetherClawTech/aetherclaw/pkg/mcp"
	"github.com/AetherClawTech/aetherclaw/pkg/tools"
)

func TestNewMCPClientManager(t *testing.T) {
	m := NewMCPClientManager()
	assert.NotNil(t, m)
	assert.Equal(t, 0, m.ClientCount())
}

func TestMCPClientManager_RegisterToolsTo_Empty(t *testing.T) {
	m := NewMCPClientManager()
	registry := tools.NewToolRegistry()
	count := m.RegisterToolsTo(registry)
	assert.Equal(t, 0, count)
	assert.Equal(t, 0, registry.Count())
}

func TestMCPClientManager_RegisterToolsToAgent_Empty(t *testing.T) {
	m := NewMCPClientManager()
	registry := tools.NewToolRegistry()
	count := m.RegisterToolsToAgent("main", registry)
	assert.Equal(t, 0, count)
	assert.Equal(t, 0, registry.Count())
}

func TestMCPClientManager_StopAll_Empty(t *testing.T) {
	m := NewMCPClientManager()
	// Should not panic on empty manager
	m.StopAll()
	assert.Equal(t, 0, m.ClientCount())
}

// --- AllowsAgent ---

func TestAllowsAgent_EmptyList(t *testing.T) {
	c := New(mcptypes.MCPClientConfig{Name: "test"})
	assert.True(t, c.AllowsAgent("main"))
	assert.True(t, c.AllowsAgent("cto"))
	assert.True(t, c.AllowsAgent("anything"))
}

func TestAllowsAgent_WithList(t *testing.T) {
	c := New(mcptypes.MCPClientConfig{
		Name:   "ewelink",
		Agents: []string{"main", "casa"},
	})
	assert.True(t, c.AllowsAgent("main"))
	assert.True(t, c.AllowsAgent("casa"))
	assert.False(t, c.AllowsAgent("cto"))
	assert.False(t, c.AllowsAgent("dev"))
	assert.False(t, c.AllowsAgent("researcher"))
}

func TestAllowsAgent_SingleAgent(t *testing.T) {
	c := New(mcptypes.MCPClientConfig{
		Name:   "test",
		Agents: []string{"dev"},
	})
	assert.True(t, c.AllowsAgent("dev"))
	assert.False(t, c.AllowsAgent("main"))
}

// --- RegisterToolsToAgent with filtering ---

// fakeClient creates an MCPClient with pre-populated tools for testing.
func fakeClient(name string, agents []string, toolNames ...string) *MCPClient {
	c := New(mcptypes.MCPClientConfig{
		Name:   name,
		Agents: agents,
	})
	for _, tn := range toolNames {
		c.tools = append(c.tools, mcp.Tool{Name: tn, Description: "test tool " + tn})
	}
	return c
}

func TestRegisterToolsToAgent_Filtering(t *testing.T) {
	m := NewMCPClientManager()
	m.mu.Lock()
	m.clients = append(m.clients,
		fakeClient("context7", nil, "query_docs"),             // no filter => all agents
		fakeClient("ewelink", []string{"main", "casa"}, "toggle_device", "get_status"), // filtered
	)
	m.failures = make([]int, 2)
	m.backoffs = make([]time.Duration, 2)
	m.mu.Unlock()

	// main agent should get all 3 tools
	mainRegistry := tools.NewToolRegistry()
	count := m.RegisterToolsToAgent("main", mainRegistry)
	assert.Equal(t, 3, count)

	// casa agent should get all 3 tools
	casaRegistry := tools.NewToolRegistry()
	count = m.RegisterToolsToAgent("casa", casaRegistry)
	assert.Equal(t, 3, count)

	// cto agent should only get context7 tools (1)
	ctoRegistry := tools.NewToolRegistry()
	count = m.RegisterToolsToAgent("cto", ctoRegistry)
	assert.Equal(t, 1, count)

	// dev agent should only get context7 tools (1)
	devRegistry := tools.NewToolRegistry()
	count = m.RegisterToolsToAgent("dev", devRegistry)
	assert.Equal(t, 1, count)
}

func TestRegisterToolsToAgent_AllUnfiltered(t *testing.T) {
	m := NewMCPClientManager()
	m.mu.Lock()
	m.clients = append(m.clients,
		fakeClient("server1", nil, "tool_a"),
		fakeClient("server2", nil, "tool_b"),
	)
	m.failures = make([]int, 2)
	m.backoffs = make([]time.Duration, 2)
	m.mu.Unlock()

	registry := tools.NewToolRegistry()
	count := m.RegisterToolsToAgent("any_agent", registry)
	assert.Equal(t, 2, count)
}

func TestRegisterToolsToAgent_AllFiltered(t *testing.T) {
	m := NewMCPClientManager()
	m.mu.Lock()
	m.clients = append(m.clients,
		fakeClient("private", []string{"main"}, "secret_tool"),
	)
	m.failures = make([]int, 1)
	m.backoffs = make([]time.Duration, 1)
	m.mu.Unlock()

	registry := tools.NewToolRegistry()
	count := m.RegisterToolsToAgent("dev", registry)
	assert.Equal(t, 0, count)
}

// --- Health monitor start/stop ---

func TestStartHealthMonitor_DoesNotPanic(t *testing.T) {
	m := NewMCPClientManager()
	m.StartHealthMonitor(nil)
	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)
	m.StopAll()
}

func TestStartHealthMonitor_Idempotent(t *testing.T) {
	m := NewMCPClientManager()
	m.StartHealthMonitor(nil)
	m.StartHealthMonitor(nil) // should not panic or create second goroutine
	time.Sleep(10 * time.Millisecond)
	m.StopAll()
}

func TestStopAll_StopsHealthMonitor(t *testing.T) {
	m := NewMCPClientManager()
	m.StartHealthMonitor(nil)
	time.Sleep(10 * time.Millisecond)
	m.StopAll()

	m.mu.Lock()
	assert.Nil(t, m.stopChan)
	m.mu.Unlock()
}

// --- Config method ---

func TestMCPClient_Config(t *testing.T) {
	cfg := mcptypes.MCPClientConfig{
		Name:      "test-server",
		Transport: "stdio",
		Command:   "/usr/bin/test",
		Args:      []string{"--flag"},
		Agents:    []string{"main", "dev"},
	}
	c := New(cfg)
	got := c.Config()
	assert.Equal(t, cfg.Name, got.Name)
	assert.Equal(t, cfg.Transport, got.Transport)
	assert.Equal(t, cfg.Command, got.Command)
	assert.Equal(t, cfg.Args, got.Args)
	assert.Equal(t, cfg.Agents, got.Agents)
}

// --- Ping without connection ---

func TestMCPClient_Ping_NotConnected(t *testing.T) {
	c := New(mcptypes.MCPClientConfig{Name: "test"})
	err := c.Ping(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// --- mockAgentToolsProvider ---

type mockAgentToolsProvider struct {
	agents map[string]*tools.ToolRegistry
}

func (m *mockAgentToolsProvider) ListAgentIDs() []string {
	ids := make([]string, 0, len(m.agents))
	for id := range m.agents {
		ids = append(ids, id)
	}
	return ids
}

func (m *mockAgentToolsProvider) GetAgentTools(agentID string) *tools.ToolRegistry {
	return m.agents[agentID]
}

func TestCheckClient_FailureTracking(t *testing.T) {
	m := NewMCPClientManager()

	// Create an unconnected client — Ping will fail
	c := New(mcptypes.MCPClientConfig{Name: "dead-server"})
	m.mu.Lock()
	m.clients = append(m.clients, c)
	m.failures = append(m.failures, 0)
	m.backoffs = append(m.backoffs, defaultHealthInterval)
	m.mu.Unlock()

	provider := &mockAgentToolsProvider{
		agents: map[string]*tools.ToolRegistry{
			"main": tools.NewToolRegistry(),
		},
	}

	// First check — should increment failures to 1
	m.checkClient(0, c, provider)
	m.mu.Lock()
	assert.Equal(t, 1, m.failures[0])
	m.mu.Unlock()

	// Second check — failures = 2, still below threshold
	m.checkClient(0, c, provider)
	m.mu.Lock()
	assert.Equal(t, 2, m.failures[0])
	// Backoff should still be at default (no reconnect attempted yet)
	assert.Equal(t, defaultHealthInterval, m.backoffs[0])
	m.mu.Unlock()

	// Note: we don't test the 3rd failure here because Reconnect triggers
	// mcp-go's stdio transport which panics with no real server process.
	// Reconnection is tested via integration tests with real MCP servers.
}

func TestCheckClient_FailureIncrement(t *testing.T) {
	m := NewMCPClientManager()

	// Simulate a client that had 1 prior failure
	c := New(mcptypes.MCPClientConfig{Name: "test"})
	m.mu.Lock()
	m.clients = append(m.clients, c)
	m.failures = append(m.failures, 1) // had 1 prior failure
	m.backoffs = append(m.backoffs, defaultHealthInterval)
	m.mu.Unlock()

	// Ping will fail (not connected), incrementing to 2 — still below threshold
	m.checkClient(0, c, nil)
	m.mu.Lock()
	assert.Equal(t, 2, m.failures[0])
	// Backoff stays default since no reconnect was attempted
	assert.Equal(t, defaultHealthInterval, m.backoffs[0])
	m.mu.Unlock()
}
