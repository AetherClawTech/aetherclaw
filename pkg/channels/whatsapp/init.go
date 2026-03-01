package whatsapp

import (
	"github.com/AetherClawTech/aetherclaw/pkg/bus"
	"github.com/AetherClawTech/aetherclaw/pkg/channels"
	"github.com/AetherClawTech/aetherclaw/pkg/config"
)

func init() {
	channels.RegisterFactory("whatsapp", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewWhatsAppChannel(cfg.Channels.WhatsApp, b)
	})
}
