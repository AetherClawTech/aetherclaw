// Package mcpclient provides an MCP client that connects to external MCP servers
// and exposes their tools as native AetherClaw tools.
package mcpclient

import (
	"context"
	"fmt"
	"os"
	"strings"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/AetherClawTech/aetherclaw/pkg/logger"
	mcptypes "github.com/AetherClawTech/aetherclaw/pkg/mcp"
)

// MCPClient manages a connection to a single external MCP server.
type MCPClient struct {
	cfg    mcptypes.MCPClientConfig
	client *mcpclient.Client
	tools  []mcp.Tool
}

// New creates an MCPClient from config but does not connect yet.
func New(cfg mcptypes.MCPClientConfig) *MCPClient {
	return &MCPClient{cfg: cfg}
}

// Start connects to the MCP server and performs initialization.
func (c *MCPClient) Start(ctx context.Context) error {
	var err error

	switch c.cfg.Transport {
	case "stdio", "":
		env := os.Environ()
		for k, v := range c.cfg.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		c.client, err = mcpclient.NewStdioMCPClient(c.cfg.Command, env, c.cfg.Args...)
		if err != nil {
			return fmt.Errorf("mcp client stdio start: %w", err)
		}
	case "sse":
		c.client, err = mcpclient.NewSSEMCPClient(c.cfg.URL)
		if err != nil {
			return fmt.Errorf("mcp client sse connect: %w", err)
		}
	case "http":
		c.client, err = mcpclient.NewStreamableHttpClient(c.cfg.URL)
		if err != nil {
			return fmt.Errorf("mcp client http connect: %w", err)
		}
	default:
		return fmt.Errorf("unsupported MCP client transport: %s", c.cfg.Transport)
	}

	// Initialize the MCP session
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "aetherclaw",
		Version: "1.0.0",
	}

	_, err = c.client.Initialize(ctx, initReq)
	if err != nil {
		c.client.Close()
		return fmt.Errorf("mcp client initialize: %w", err)
	}

	// Discover tools
	if err := c.discoverTools(ctx); err != nil {
		logger.WarnCF("mcp-client", "Failed to discover tools", map[string]any{
			"server": c.cfg.Name,
			"error":  err.Error(),
		})
	}

	return nil
}

// discoverTools fetches the tool list from the MCP server.
func (c *MCPClient) discoverTools(ctx context.Context) error {
	result, err := c.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return err
	}
	c.tools = result.Tools
	return nil
}

// Tools returns the discovered MCP tools.
func (c *MCPClient) Tools() []mcp.Tool {
	return c.tools
}

// CallTool invokes a tool on the remote MCP server.
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return c.client.CallTool(ctx, req)
}

// Name returns the configured server name.
func (c *MCPClient) Name() string {
	return c.cfg.Name
}

// Stop closes the MCP client connection.
func (c *MCPClient) Stop() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// ToolPrefix returns the prefix used for tool names from this server.
func (c *MCPClient) ToolPrefix() string {
	return "mcp_" + sanitizeName(c.cfg.Name) + "_"
}

// sanitizeName converts a server name to a safe tool prefix component.
func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}
