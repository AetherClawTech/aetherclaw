package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer("aetherclaw-test", "1.0.0")

	// Register echo tool
	echoTool := mcp.NewTool("echo",
		mcp.WithDescription("Echo back the input message"),
		mcp.WithString("message", mcp.Required(), mcp.Description("Message to echo")),
	)
	s.AddTool(echoTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		msg, _ := args["message"].(string)
		return mcp.NewToolResultText(fmt.Sprintf("echo: %s", msg)), nil
	})

	// Register mock_device_control tool (simulates eWeLink)
	deviceTool := mcp.NewTool("mock_device_control",
		mcp.WithDescription("Control a mock IoT device"),
		mcp.WithString("device_id", mcp.Required(), mcp.Description("Device identifier")),
		mcp.WithString("action", mcp.Required(), mcp.Description("Action: on, off, toggle")),
	)
	s.AddTool(deviceTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		deviceID, _ := args["device_id"].(string)
		action, _ := args["action"].(string)
		return mcp.NewToolResultText(fmt.Sprintf("device %s: %s", deviceID, action)), nil
	})

	// Run as stdio server
	stdio := server.NewStdioServer(s)
	if err := stdio.Listen(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "mock MCP server error: %v\n", err)
		os.Exit(1)
	}
}
