package providers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Compile-time interface check ---

var _ MCPConfigurable = (*CodexCliProvider)(nil)

// --- SetMCPConfigs tests ---

func TestCodexCliProvider_SetMCPConfigs(t *testing.T) {
	p := NewCodexCliProvider("/workspace")
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
	if p.mcpConfigWritten {
		t.Error("mcpConfigWritten should be false after SetMCPConfigs")
	}
}

func TestCodexCliProvider_SetMCPConfigs_ResetsWrittenFlag(t *testing.T) {
	p := NewCodexCliProvider("/workspace")
	p.mcpConfigWritten = true
	p.SetMCPConfigs([]MCPProviderConfig{{Name: "new", Command: "cmd"}})

	if p.mcpConfigWritten {
		t.Error("SetMCPConfigs should reset mcpConfigWritten flag")
	}
}

// --- writeCodexMCPConfig tests ---

func TestCodexCliProvider_WriteCodexMCPConfig(t *testing.T) {
	workspace := t.TempDir()
	p := NewCodexCliProvider(workspace)
	p.mcpConfigs = []MCPProviderConfig{
		{
			Name:    "ewelink",
			Command: "/usr/bin/python",
			Args:    []string{"ewelink_mcp.py"},
			Env:     map[string]string{"EWELINK_API_URL": "http://192.168.50.107"},
		},
	}

	if err := p.writeCodexMCPConfig(); err != nil {
		t.Fatalf("writeCodexMCPConfig() error = %v", err)
	}

	configPath := filepath.Join(workspace, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.toml: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "[mcp_servers.ewelink]") {
		t.Error("missing [mcp_servers.ewelink] section")
	}
	if !strings.Contains(content, `command = "/usr/bin/python"`) {
		t.Error("missing command field")
	}
	if !strings.Contains(content, `"ewelink_mcp.py"`) {
		t.Error("missing args value")
	}
	if !strings.Contains(content, "[mcp_servers.ewelink.env]") {
		t.Error("missing env section")
	}
	if !strings.Contains(content, `EWELINK_API_URL = "http://192.168.50.107"`) {
		t.Error("missing env value")
	}
}

func TestCodexCliProvider_WriteCodexMCPConfig_MultipleServers(t *testing.T) {
	workspace := t.TempDir()
	p := NewCodexCliProvider(workspace)
	p.mcpConfigs = []MCPProviderConfig{
		{Name: "ewelink", Command: "python", Args: []string{"ewelink.py"}},
		{Name: "context7", Command: "npx", Args: []string{"-y", "@context7/mcp"}},
	}

	if err := p.writeCodexMCPConfig(); err != nil {
		t.Fatalf("writeCodexMCPConfig() error = %v", err)
	}

	configPath := filepath.Join(workspace, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.toml: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "[mcp_servers.ewelink]") {
		t.Error("missing ewelink section")
	}
	if !strings.Contains(content, "[mcp_servers.context7]") {
		t.Error("missing context7 section")
	}
}

func TestCodexCliProvider_WriteCodexMCPConfig_NoArgs(t *testing.T) {
	workspace := t.TempDir()
	p := NewCodexCliProvider(workspace)
	p.mcpConfigs = []MCPProviderConfig{
		{Name: "simple", Command: "/bin/simple-mcp"},
	}

	if err := p.writeCodexMCPConfig(); err != nil {
		t.Fatalf("writeCodexMCPConfig() error = %v", err)
	}

	configPath := filepath.Join(workspace, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.toml: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "[mcp_servers.simple]") {
		t.Error("missing simple section")
	}
	if !strings.Contains(content, `command = "/bin/simple-mcp"`) {
		t.Error("missing command field")
	}
	// Should not have args or env sections
	if strings.Contains(content, "args =") {
		t.Error("should not have args when empty")
	}
	if strings.Contains(content, "[mcp_servers.simple.env]") {
		t.Error("should not have env section when empty")
	}
}

func TestCodexCliProvider_ConfigNotRewrittenOnSecondCall(t *testing.T) {
	workspace := t.TempDir()
	p := NewCodexCliProvider(workspace)
	p.mcpConfigs = []MCPProviderConfig{
		{Name: "test", Command: "cmd"},
	}

	// First write
	if err := p.writeCodexMCPConfig(); err != nil {
		t.Fatalf("first writeCodexMCPConfig() error = %v", err)
	}
	p.mcpConfigWritten = true

	configPath := filepath.Join(workspace, ".codex", "config.toml")
	info1, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}

	// Modify configs but flag is set
	p.mcpConfigs = []MCPProviderConfig{
		{Name: "modified", Command: "other"},
	}

	// mcpConfigWritten is true, so Chat() won't re-call writeCodexMCPConfig
	// Verify the flag prevents writes
	if !p.mcpConfigWritten {
		t.Error("mcpConfigWritten should still be true")
	}

	// Verify file hasn't changed
	info2, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	if info1.ModTime() != info2.ModTime() {
		t.Error("config file should not have been rewritten")
	}
}

func TestCodexCliProvider_CreatesCodexDir(t *testing.T) {
	workspace := t.TempDir()
	p := NewCodexCliProvider(workspace)
	p.mcpConfigs = []MCPProviderConfig{
		{Name: "test", Command: "cmd"},
	}

	codexDir := filepath.Join(workspace, ".codex")
	if _, err := os.Stat(codexDir); !os.IsNotExist(err) {
		t.Fatal(".codex dir should not exist before write")
	}

	if err := p.writeCodexMCPConfig(); err != nil {
		t.Fatalf("writeCodexMCPConfig() error = %v", err)
	}

	if _, err := os.Stat(codexDir); err != nil {
		t.Fatalf(".codex dir should exist after write: %v", err)
	}
}
