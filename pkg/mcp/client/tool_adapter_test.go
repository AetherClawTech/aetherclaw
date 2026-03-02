package mcpclient

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"

	mcptypes "github.com/AetherClawTech/aetherclaw/pkg/mcp"
)

func TestMCPToolAdapter_Name(t *testing.T) {
	client := New(mcptypes.MCPClientConfig{Name: "my-server"})
	tool := mcp.Tool{Name: "read_file", Description: "Read a file"}
	adapter := NewMCPToolAdapter(client, tool)

	assert.Equal(t, "mcp_my_server_read_file", adapter.Name())
}

func TestMCPToolAdapter_Description(t *testing.T) {
	client := New(mcptypes.MCPClientConfig{Name: "test"})
	tool := mcp.Tool{Name: "do_stuff", Description: "Does important stuff"}
	adapter := NewMCPToolAdapter(client, tool)

	assert.Equal(t, "Does important stuff", adapter.Description())
}

func TestMCPToolAdapter_Parameters_Fallback(t *testing.T) {
	client := New(mcptypes.MCPClientConfig{Name: "test"})
	tool := mcp.Tool{Name: "empty_tool"}
	adapter := NewMCPToolAdapter(client, tool)

	params := adapter.Parameters()
	assert.NotNil(t, params)
}

func TestMCPToolAdapter_Parameters_WithSchema(t *testing.T) {
	client := New(mcptypes.MCPClientConfig{Name: "test"})
	tool := mcp.NewTool("greet", mcp.WithDescription("Greet someone"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name to greet")),
	)
	adapter := NewMCPToolAdapter(client, tool)

	params := adapter.Parameters()
	assert.NotNil(t, params)
	assert.Equal(t, "object", params["type"])
}

func TestToolPrefix(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"my-server", "mcp_my_server_"},
		{"MY SERVER", "mcp_my_server_"},
		{"simple", "mcp_simple_"},
		{"a-b-c", "mcp_a_b_c_"},
	}

	for _, tt := range tests {
		client := New(mcptypes.MCPClientConfig{Name: tt.name})
		assert.Equal(t, tt.expected, client.ToolPrefix(), "prefix for %q", tt.name)
	}
}

func TestSanitizeName(t *testing.T) {
	assert.Equal(t, "hello_world", sanitizeName("Hello-World"))
	assert.Equal(t, "my_server", sanitizeName("My Server"))
	assert.Equal(t, "simple", sanitizeName("simple"))
}

func TestExtractTextContent(t *testing.T) {
	t.Run("text content", func(t *testing.T) {
		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Text: "hello"},
				mcp.TextContent{Text: "world"},
			},
		}
		assert.Equal(t, "hello\nworld", extractTextContent(result))
	})

	t.Run("empty content", func(t *testing.T) {
		result := &mcp.CallToolResult{
			Content: []mcp.Content{},
		}
		assert.Equal(t, "no content", extractTextContent(result))
	})

	t.Run("image content", func(t *testing.T) {
		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.ImageContent{MIMEType: "image/png", Data: "base64data"},
			},
		}
		assert.Contains(t, extractTextContent(result), "[Image: image/png]")
	})
}
