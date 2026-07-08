package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	discordv1 "github.com/lasthearth/vsservice/gen/discord/v1"
	"github.com/lasthearth/vsservice/internal/discord/internal/discord"
	"github.com/lasthearth/vsservice/internal/discord/internal/lib"
	"github.com/lasthearth/vsservice/internal/discord/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	_ discordv1.DiscordServiceServer = (*Service)(nil)
	_ DiscordClient                  = (*discord.Client)(nil)
)

// Service implements discordv1.DiscordServiceServer.
type Service struct {
	client DiscordClient
	cfg    config.Config
	log    logger.Logger
	mapper Mapper
}

// Opts are fx-injected dependencies.
type Opts struct {
	fx.In

	Client DiscordClient
	Config config.Config
	Log    logger.Logger
	Mapper Mapper
}

// New creates a new Discord service.
func New(opts Opts) *Service {
	return &Service{
		client: opts.Client,
		cfg:    opts.Config,
		log:    opts.Log,
		mapper: opts.Mapper,
	}
}

const discordPageSize = 100

// ListMessages lists cleaned messages from a Discord channel.
func (s *Service) ListMessages(ctx context.Context, req *discordv1.ListMessagesRequest) (*discordv1.ListMessagesResponse, error) {
	l := s.log.With(zap.String("method", "ListMessages"), zap.String("channel_id", req.GetChannelId()))

	if req.GetChannelId() == "" {
		return nil, status.Error(codes.InvalidArgument, "channel_id is required")
	}

	if !s.isAllowedChannel(req.GetChannelId()) {
		return nil, status.Error(codes.InvalidArgument, "channel_id is not allowed")
	}

	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	rawMessages, err := s.client.GetChannelMessages(ctx, req.GetChannelId(), limit, req.GetBefore())
	if err != nil {
		l.Error("failed to fetch discord messages", zap.Error(err))
		return nil, status.Error(codes.Internal, "discord request failed")
	}

	messages := make([]model.Message, 0, len(rawMessages))
	for _, raw := range rawMessages {
		messages = append(messages, s.mapMessage(raw))
	}

	return &discordv1.ListMessagesResponse{
		Messages:   s.mapper.ToProtoMessages(messages),
		IsLastPage: len(rawMessages) < limit,
	}, nil
}

// ListImages lists image attachments from a Discord channel.
func (s *Service) ListImages(ctx context.Context, req *discordv1.ListImagesRequest) (*discordv1.ListImagesResponse, error) {
	l := s.log.With(zap.String("method", "ListImages"), zap.String("channel_id", req.GetChannelId()))

	if req.GetChannelId() == "" {
		return nil, status.Error(codes.InvalidArgument, "channel_id is required")
	}

	if !s.isAllowedChannel(req.GetChannelId()) {
		return nil, status.Error(codes.InvalidArgument, "channel_id is not allowed")
	}

	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	images, isLastPage, err := s.fetchImages(ctx, req.GetChannelId(), limit, req.GetBefore())
	if err != nil {
		l.Error("failed to fetch discord images", zap.Error(err))
		return nil, status.Error(codes.Internal, "discord request failed")
	}

	return &discordv1.ListImagesResponse{
		Images:     s.mapper.ToProtoImages(images),
		IsLastPage: isLastPage,
	}, nil
}

// SendNews sends a news embed to the configured Discord webhook.
func (s *Service) SendNews(ctx context.Context, req *discordv1.SendNewsRequest) (*emptypb.Empty, error) {
	l := s.log.With(zap.String("method", "SendNews"))

	if s.cfg.DiscordNewsWebhook == "" {
		return nil, status.Error(codes.Internal, "discord news webhook is not configured")
	}

	if req.GetTitle() == "" || req.GetContent() == "" || req.GetNewsUrl() == "" {
		return nil, status.Error(codes.InvalidArgument, "title, content and news_url are required")
	}

	payload, err := s.buildNewsPayload(req)
	if err != nil {
		l.Error("failed to build news payload", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to build news payload")
	}

	if err := s.client.SendWebhook(ctx, s.cfg.DiscordNewsWebhook, payload); err != nil {
		l.Error("failed to send discord webhook", zap.Error(err))
		return nil, status.Error(codes.Internal, "discord webhook request failed")
	}

	return &emptypb.Empty{}, nil
}

// Scope declares JWT scope requirements.
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/discord.v1.DiscordService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "SendNews"): interceptor.Scope("news:create"),
	}
}

func (s *Service) mapMessage(raw discord.RawMessage) model.Message {
	author := raw.Author.GlobalName
	if author == "" {
		author = raw.Author.Username
	}

	cleaned := lib.CleanContent(raw.Content)
	cleaned = lib.ReplaceEmojis(cleaned)
	content, msgType := lib.ParseMessageType(cleaned)

	return model.Message{
		Id:        raw.ID,
		Content:   content,
		Author:    author,
		Timestamp: raw.Timestamp,
		Type:      msgType,
	}
}

func (s *Service) fetchImages(ctx context.Context, channelID string, limit int, before string) ([]model.Image, bool, error) {
	var result []model.Image
	beforeCursor := before
	isLastPage := true

	for len(result) < limit {
		remaining := limit - len(result)
		fetchLimit := max(remaining, discordPageSize)

		messages, err := s.client.GetChannelMessages(ctx, channelID, fetchLimit, beforeCursor)
		if err != nil {
			return nil, false, err
		}

		if len(messages) == 0 {
			isLastPage = true
			break
		}

		for _, msg := range messages {
			for _, att := range msg.Attachments {
				if att.ContentType == "" || !isImageContentType(att.ContentType) {
					continue
				}

				author := msg.Author.GlobalName
				if author == "" {
					author = msg.Author.Username
				}

				result = append(result, model.Image{
					Id:        fmt.Sprintf("%s-%s", msg.ID, att.ID),
					Url:       att.URL,
					ProxyUrl:  att.ProxyURL,
					Alt:       chooseAlt(msg.Content, att.Filename),
					Author:    author,
					Timestamp: msg.Timestamp,
					Width:     int32(att.Width),
					Height:    int32(att.Height),
				})

				if len(result) >= limit {
					isLastPage = len(messages) < fetchLimit
					return result, isLastPage, nil
				}
			}
		}

		if len(messages) < fetchLimit {
			isLastPage = true
			break
		}

		beforeCursor = messages[len(messages)-1].ID
		isLastPage = false
	}

	return result, isLastPage, nil
}

func isImageContentType(ct string) bool {
	return len(ct) > 6 && ct[:6] == "image/"
}

func chooseAlt(content, filename string) string {
	if content != "" {
		return content
	}
	return filename
}

// isAllowedChannel checks whether the channel is in the configured allowlist.
func (s *Service) isAllowedChannel(channelID string) bool {
	return slices.Contains(s.cfg.DiscordAllowedChannels, channelID)
}

func (s *Service) buildNewsPayload(req *discordv1.SendNewsRequest) ([]byte, error) {
	plainContent := lib.StripHTML(req.GetContent())
	plainContent = lib.Truncate(plainContent, 4096)
	title := lib.Truncate(req.GetTitle(), 256)

	embed := map[string]any{
		"title":       title,
		"description": plainContent,
		"url":         req.GetNewsUrl(),
		"color":       0xc35a17,
		"timestamp":   "",
	}

	if req.GetPreviewUrl() != "" {
		embed["image"] = map[string]string{"url": req.GetPreviewUrl()}
	}

	if req.GetAuthor() != "" {
		embed["footer"] = map[string]string{"text": "Автор: " + req.GetAuthor()}
	}

	payload := map[string]any{
		"username":   "Last Hearth News",
		"avatar_url": "https://lasthearth.ru/images/logo.png",
		"embeds":     []any{embed},
	}

	return json.Marshal(payload)
}
