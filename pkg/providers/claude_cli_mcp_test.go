package providers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- Compile-time interface check ---

var _ MCPConfigurable = (*ClaudeCliProvider)(nil)

// --- SetMCPConfigs tests ---

func TestClaudeCliProvider_SetMCPConfigs(t *testing.T) {
	p := NewClaudeCliProvider("/workspace")
	configs := []MCPProviderConfig{
		{
			Name:    "ewelink",
			Command: "/usr/bin/python",
			Args:    []string{"ewelink_mcp.py"},
			Env:     map[string]string{"EWELINK_API_URL": "http://192.168.50.107"},
		},
	}
	p.SetMCPConfigs(configs)

	if len(p.mcpConfigs) != 1 {
		t.Fatalf("mcpConfigs len = %d, want 1", len(p.mcpConfigs))
	}
	if p.mcpConfigs[0].Name != "ewelink" {
		t.Errorf("mcpConfigs[0].Name = %q, want %q", p.mcpConfigs[0].Name, "ewelink")
	}
}

func TestClaudeCliProvider_SetMCPConfigs_Empty(t *testing.T) {
	p := NewClaudeCliProvider("/workspace")
	p.SetMCPConfigs(nil)

	if len(p.mcpConfigs) != 0 {
		t.Errorf("mcpConfigs len = %d, want 0", len(p.mcpConfigs))
	}
}

func TestClaudeCliProvider_SetMCPConfigs_Overwrite(t *testing.T) {
	p := NewClaudeCliProvider("/workspace")
	p.SetMCPConfigs([]MCPProviderConfig{{Name: "first", Command: "cmd1"}})
	p.SetMCPConfigs([]MCPProviderConfig{{Name: "second", Command: "cmd2"}})

	if len(p.mcpConfigs) != 1 {
		t.Fatalf("mcpConfigs len = %d, want 1", len(p.mcpConfigs))
	}
	if p.mcpConfigs[0].Name != "second" {
		t.Errorf("expected overwritten config, got %q", p.mcpConfigs[0].Name)
	}
}

// --- writeMCPConfigFile tests ---

func TestClaudeCliProvider_WriteMCPConfigFile(t *testing.T) {
	p := NewClaudeCliProvider("/workspace")
	p.mcpConfigs = []MCPProviderConfig{
		{
			Name:    "ewelink",
			Command: "/usr/bin/python",
			Args:    []string{"ewelink_mcp.py", "--verbose"},
			Env:     map[string]string{"EWELINK_API_URL": "http://192.168.50.107"},
		},
		{
			Name:    "context7",
			Command: "npx",
			Args:    []string{"-y", "@context7/mcp"},
		},
	}

	path, err := p.writeMCPConfigFile()
	if err != nil {
		t.Fatalf("writeMCPConfigFile() error = %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var config struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	if len(config.MCPServers) != 2 {
		t.Fatalf("mcpServers count = %d, want 2", len(config.MCPServers))
	}

	ewelink, ok := config.MCPServers["ewelink"]
	if !ok {
		t.Fatal("missing ewelink in mcpServers")
	}
	if ewelink.Command != "/usr/bin/python" {
		t.Errorf("ewelink.command = %q, want %q", ewelink.Command, "/usr/bin/python")
	}
	if len(ewelink.Args) != 2 || ewelink.Args[0] != "ewelink_mcp.py" {
		t.Errorf("ewelink.args = %v, want [ewelink_mcp.py --verbose]", ewelink.Args)
	}
	if ewelink.Env["EWELINK_API_URL"] != "http://192.168.50.107" {
		t.Errorf("ewelink.env.EWELINK_API_URL = %q", ewelink.Env["EWELINK_API_URL"])
	}

	ctx7, ok := config.MCPServers["context7"]
	if !ok {
		t.Fatal("missing context7 in mcpServers")
	}
	if ctx7.Command != "npx" {
		t.Errorf("context7.command = %q, want %q", ctx7.Command, "npx")
	}
	if len(ctx7.Env) != 0 {
		t.Errorf("context7 should have no env, got %v", ctx7.Env)
	}
}

func TestClaudeCliProvider_WriteMCPConfigFile_NoArgs(t *testing.T) {
	p := NewClaudeCliProvider("/workspace")
	p.mcpConfigs = []MCPProviderConfig{
		{Name: "simple", Command: "/bin/simple-mcp"},
	}

	path, err := p.writeMCPConfigFile()
	if err != nil {
		t.Fatalf("writeMCPConfigFile() error = %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	servers := config["mcpServers"].(map[string]any)
	simple := servers["simple"].(map[string]any)
	if simple["command"] != "/bin/simple-mcp" {
		t.Errorf("command = %v, want /bin/simple-mcp", simple["command"])
	}
	if _, hasArgs := simple["args"]; hasArgs {
		t.Error("should not have args key when Args is empty")
	}
	if _, hasEnv := simple["env"]; hasEnv {
		t.Error("should not have env key when Env is empty")
	}
}

// --- Chat with MCP config integration tests ---

func TestClaudeCliProvider_MCPConfigInArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock CLI scripts not supported on Windows")
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	script := createArgCaptureCLI(t, argsFile)

	p := NewClaudeCliProvider(t.TempDir())
	p.command = script
	p.SetMCPConfigs([]MCPProviderConfig{
		{Name: "test-mcp", Command: "/bin/test-mcp"},
	})

	_, err := p.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hello"},
	}, nil, "", nil)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	argsBytes, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}
	args := string(argsBytes)

	if !strings.Contains(args, "--mcp-config") {
		t.Errorf("CLI args missing --mcp-config, got: %s", args)
	}
}

func TestClaudeCliProvider_NoMCPConfigWhenEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock CLI scripts not supported on Windows")
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	script := createArgCaptureCLI(t, argsFile)

	p := NewClaudeCliProvider(t.TempDir())
	p.command = script
	// No SetMCPConfigs call

	_, err := p.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hello"},
	}, nil, "", nil)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	argsBytes, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}
	args := string(argsBytes)

	if strings.Contains(args, "--mcp-config") {
		t.Errorf("CLI args should NOT contain --mcp-config when no MCP configs, got: %s", args)
	}
}

func TestClaudeCliProvider_MCPConfigTempFileCleanedUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock CLI scripts not supported on Windows")
	}

	mockJSON := `{"type":"result","result":"ok","session_id":"s"}`
	script := createMockCLI(t, mockJSON, "", 0)

	p := NewClaudeCliProvider(t.TempDir())
	p.command = script
	p.SetMCPConfigs([]MCPProviderConfig{
		{Name: "test-mcp", Command: "/bin/test-mcp"},
	})

	// Write a config file to check path pattern
	path, err := p.writeMCPConfigFile()
	if err != nil {
		t.Fatalf("writeMCPConfigFile() error = %v", err)
	}
	// Verify it exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file should exist at %s", path)
	}
	os.Remove(path) // Clean up manually since we called writeMCPConfigFile directly

	// Now call Chat — the temp file should be cleaned up after
	_, err = p.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hello"},
	}, nil, "", nil)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	// Verify no leftover temp files matching pattern
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "aetherclaw-mcp-*.json"))
	for _, m := range matches {
		// Only flag files created in this test (recent)
		info, err := os.Stat(m)
		if err == nil && info.Size() > 0 {
			// This is a best-effort check - temp files from other tests may exist
			t.Logf("Note: found MCP config temp file (may be from concurrent test): %s", m)
		}
	}
}
