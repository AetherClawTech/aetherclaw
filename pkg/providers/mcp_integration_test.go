package providers

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMCPIntegration_MockServerBuilds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	// Build the mock MCP server binary
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "mock-mcp-server")
	cmd := exec.Command("go", "build", "-o", binary, "./internal/testutil/mcpserver")
	cmd.Dir = findProjectRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build mock MCP server: %v\n%s", err, out)
	}

	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("binary not found at %s: %v", binary, err)
	}
}

func TestMCPIntegration_ClaudeCliConfigFormat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	// Build mock MCP server
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "mock-mcp-server")
	cmd := exec.Command("go", "build", "-o", binary, "./internal/testutil/mcpserver")
	cmd.Dir = findProjectRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build mock MCP server: %v\n%s", err, out)
	}

	// Create Claude CLI provider with MCP config pointing to mock server
	p := NewClaudeCliProvider(tmpDir)
	p.SetMCPConfigs([]MCPProviderConfig{
		{
			Name:    "aetherclaw-test",
			Command: binary,
		},
	})

	// Verify the generated config file format
	configPath, err := p.writeMCPConfigFile()
	if err != nil {
		t.Fatalf("writeMCPConfigFile() error = %v", err)
	}
	defer os.Remove(configPath)

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	var config struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("invalid JSON config: %v", err)
	}

	srv, ok := config.MCPServers["aetherclaw-test"]
	if !ok {
		t.Fatal("missing aetherclaw-test in mcpServers")
	}
	if srv.Command != binary {
		t.Errorf("command = %q, want %q", srv.Command, binary)
	}
}

func TestMCPIntegration_CodexConfigFormat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	// Build mock MCP server
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "mock-mcp-server")
	cmd := exec.Command("go", "build", "-o", binary, "./internal/testutil/mcpserver")
	cmd.Dir = findProjectRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build mock MCP server: %v\n%s", err, out)
	}

	workspace := t.TempDir()
	p := NewCodexCliProvider(workspace)
	p.SetMCPConfigs([]MCPProviderConfig{
		{
			Name:    "aetherclaw-test",
			Command: binary,
			Env:     map[string]string{"TEST_VAR": "hello"},
		},
	})

	if err := p.writeCodexMCPConfig(); err != nil {
		t.Fatalf("writeCodexMCPConfig() error = %v", err)
	}

	configPath := filepath.Join(workspace, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.toml: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "[mcp_servers.aetherclaw-test]") {
		t.Error("missing server section")
	}
	if !strings.Contains(content, binary) {
		t.Errorf("missing binary path in config, got:\n%s", content)
	}
	if !strings.Contains(content, `TEST_VAR = "hello"`) {
		t.Error("missing env var in config")
	}
}

func TestMCPIntegration_ClaudeCliArgsPassthrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	// Build mock MCP server
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "mock-mcp-server")
	cmd := exec.Command("go", "build", "-o", binary, "./internal/testutil/mcpserver")
	cmd.Dir = findProjectRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build mock MCP server: %v\n%s", err, out)
	}

	// Create arg-capturing mock Claude CLI
	argsFile := filepath.Join(t.TempDir(), "captured-args.txt")
	script := createArgCaptureCLI(t, argsFile)

	p := NewClaudeCliProvider(t.TempDir())
	p.command = script
	p.SetMCPConfigs([]MCPProviderConfig{
		{
			Name:    "aetherclaw-test",
			Command: binary,
		},
	})

	_, err = p.Chat(context.Background(), []Message{
		{Role: "user", Content: "test"},
	}, nil, "", nil)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	argsBytes, readErr := os.ReadFile(argsFile)
	if readErr != nil {
		t.Fatalf("failed to read args: %v", readErr)
	}
	args := string(argsBytes)

	if !strings.Contains(args, "--mcp-config") {
		t.Errorf("CLI args missing --mcp-config, got: %s", args)
	}

	// Verify --mcp-config appears AFTER the stdin dash arg.
	// --mcp-config is variadic and would consume "-" as a config path if placed before it.
	mcpIdx := strings.Index(args, "--mcp-config")
	// Find the standalone " - " (stdin marker) — not inside a path
	dashIdx := strings.Index(args, " - ")
	if dashIdx >= 0 && mcpIdx < dashIdx {
		t.Errorf("--mcp-config should appear after stdin dash (variadic flag issue), got: %s", args)
	}
}

// findProjectRoot walks up from the test file to find the project root (containing go.mod).
func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (go.mod)")
		}
		dir = parent
	}
}
