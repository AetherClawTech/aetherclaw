package feishu

import (
	"github.com/AetherClawTech/aetherclaw/pkg/bus"
	"github.com/AetherClawTech/aetherclaw/pkg/channels"
	"github.com/AetherClawTech/aetherclaw/pkg/config"
)

func init() {
	channels.RegisterFactory("feishu", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewFeishuChannel(cfg.Channels.Feishu, b)
	})
}
