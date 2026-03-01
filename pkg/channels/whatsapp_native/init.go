package whatsapp

import (
	"path/filepath"

	"github.com/AetherClawTech/aetherclaw/pkg/bus"
	"github.com/AetherClawTech/aetherclaw/pkg/channels"
	"github.com/AetherClawTech/aetherclaw/pkg/config"
)

func init() {
	channels.RegisterFactory("whatsapp_native", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		waCfg := cfg.Channels.WhatsApp
		storePath := waCfg.SessionStorePath
		if storePath == "" {
			storePath = filepath.Join(cfg.WorkspacePath(), "whatsapp")
		}
		return NewWhatsAppNativeChannel(waCfg, b, storePath)
	})
}
