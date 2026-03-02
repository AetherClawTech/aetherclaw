package mcpclient

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

func TestMCPClientManager_StopAll_Empty(t *testing.T) {
	m := NewMCPClientManager()
	// Should not panic on empty manager
	m.StopAll()
	assert.Equal(t, 0, m.ClientCount())
}
