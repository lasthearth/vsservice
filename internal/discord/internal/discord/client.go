package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
)

// RawMessage is the subset of the Discord message object we need.
type RawMessage struct {
	ID          string `json:"id"`
	Content     string `json:"content"`
	Timestamp   string `json:"timestamp"`
	Author      Author `json:"author"`
	Attachments []struct {
		ID          string `json:"id"`
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		URL         string `json:"url"`
		ProxyURL    string `json:"proxy_url"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
	} `json:"attachments"`
}

// Author is the Discord message author.
type Author struct {
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
}

// Client calls the Discord HTTP API using a bot token.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient creates a Discord API client.
func NewClient(cfg config.Config, httpClient *http.Client) *Client {
	return &Client{
		baseURL: cfg.DiscordBaseURL,
		token:   cfg.DiscordBotToken,
		http:    httpClient,
	}
}

// GetChannelMessages fetches messages from a Discord channel.
// limit is clamped to [1, 100] by Discord.
func (c *Client) GetChannelMessages(ctx context.Context, channelID string, limit int, before string) ([]RawMessage, error) {
	u, err := url.Parse(fmt.Sprintf("%s/channels/%s/messages", c.baseURL, channelID))
	if err != nil {
		return nil, fmt.Errorf("build discord url: %w", err)
	}

	q := u.Query()
	q.Set("limit", strconv.Itoa(limit))
	if before != "" {
		q.Set("before", before)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create discord request: %w", err)
	}

	req.Header.Set("Authorization", "Bot "+c.token)
	req.Header.Set("User-Agent", "LastHearthBot (https://lasthearth.ru, 1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read discord response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord returned %d: %s", resp.StatusCode, string(body))
	}

	var messages []RawMessage
	if err := json.Unmarshal(body, &messages); err != nil {
		return nil, fmt.Errorf("decode discord messages: %w", err)
	}

	return messages, nil
}

// SendWebhook posts a JSON payload to a Discord webhook URL.
func (c *Client) SendWebhook(ctx context.Context, webhookURL string, payload []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LastHearthBot (https://lasthearth.ru, 1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord webhook returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
