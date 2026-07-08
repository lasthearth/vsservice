package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/discord/internal/discord"
)

// DiscordClient fetches raw data from the Discord HTTP API.
type DiscordClient interface {
	GetChannelMessages(ctx context.Context, channelID string, limit int, before string) ([]discord.RawMessage, error)
	SendWebhook(ctx context.Context, webhookURL string, payload []byte) error
}
