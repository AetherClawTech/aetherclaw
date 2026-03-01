package onebot

import (
	"github.com/AetherClawTech/aetherclaw/pkg/bus"
	"github.com/AetherClawTech/aetherclaw/pkg/channels"
	"github.com/AetherClawTech/aetherclaw/pkg/config"
)

func init() {
	channels.RegisterFactory("onebot", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewOneBotChannel(cfg.Channels.OneBot, b)
	})
}
