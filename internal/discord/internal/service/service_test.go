package service_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	discordv1 "github.com/lasthearth/vsservice/gen/discord/v1"
	"github.com/lasthearth/vsservice/internal/discord/internal/discord"
	"github.com/lasthearth/vsservice/internal/discord/internal/service"
	"github.com/lasthearth/vsservice/internal/discord/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	// dummyWebhookToken stands in for the credential Discord puts in the last
	// path segment of a webhook URL.
	dummyWebhookToken = "DUMMYtokenAbcdef_ghi-jkl"
	dummyWebhookURL   = "https://discord.invalid/api/webhooks/123456789012345678/" + dummyWebhookToken
	// maskedWebhookPath is what httpclient.MaskSecrets leaves of that URL.
	maskedWebhookPath = "webhooks/123456789012345678/[redacted]"
	sendNewsErrorMsg  = "failed to send discord webhook"
)

// failingWebhookClient reproduces the error shape of a failed webhook POST: a
// *url.Error whose text prints the whole request URL, token included.
type failingWebhookClient struct {
	cause error
}

var _ service.DiscordClient = failingWebhookClient{}

func (failingWebhookClient) GetChannelMessages(context.Context, string, int, string) ([]discord.RawMessage, error) {
	return nil, errors.New("not used by this test")
}

func (c failingWebhookClient) SendWebhook(_ context.Context, webhookURL string, _ []byte) error {
	return fmt.Errorf("webhook request failed: %w", &url.Error{
		Op:  "Post",
		URL: webhookURL,
		Err: c.cause,
	})
}

// A Discord webhook token lives in the URL path, and a failed request prints the
// whole URL: the failure path must not carry it into a log record. The logger
// under test is built with logger.NewWithCore, so the production redacting core
// sits in front of the observer.
func TestSendNewsFailureDoesNotLogTheWebhookToken(t *testing.T) {
	tests := []struct {
		name      string
		cause     error
		wantCause string
	}{
		{
			name:      "context deadline exceeded",
			cause:     context.DeadlineExceeded,
			wantCause: "context deadline exceeded",
		},
		{
			name:      "dns failure",
			cause:     errors.New("dial tcp: lookup discord.invalid: no such host"),
			wantCause: "no such host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)

			svc := service.New(service.Opts{
				Client: failingWebhookClient{cause: tt.cause},
				Config: config.Config{DiscordNewsWebhook: dummyWebhookURL},
				Log:    logger.NewWithCore(core),
				Mapper: &sermapper.MapperImpl{},
			})

			_, err := svc.SendNews(context.Background(), &discordv1.SendNewsRequest{
				Title:   "t",
				Content: "c",
				NewsUrl: "https://lasthearth.ru/news/1",
			})
			if err == nil {
				t.Fatal("want the webhook failure to surface")
			}

			// The gate must see the record: a logger that emits nothing, or logs
			// under another message, would let a leak pass.
			records := logs.FilterMessage(sendNewsErrorMsg).FilterLevelExact(zapcore.ErrorLevel).All()
			if len(records) != 1 {
				t.Fatalf("want one %q record at error level, got %d", sendNewsErrorMsg, len(records))
			}

			// No record may carry the token, whatever the field key.
			for _, e := range logs.All() {
				for key, val := range e.ContextMap() {
					if s := fieldString(val); strings.Contains(s, dummyWebhookToken) {
						t.Errorf("the webhook token reached record %q field %q: %s", e.Message, key, s)
					}
				}
			}

			got, ok := records[0].ContextMap()["error"].(string)
			if !ok {
				t.Fatalf("want a string error field, got %#v", records[0].ContextMap()["error"])
			}
			if !strings.Contains(got, maskedWebhookPath) {
				t.Errorf("want the masked webhook path %q in the error field, got %q", maskedWebhookPath, got)
			}
			// Masking must keep the debugging signal.
			if !strings.Contains(got, tt.wantCause) {
				t.Errorf("want the cause %q kept in the error field, got %q", tt.wantCause, got)
			}
		})
	}
}

func fieldString(val any) string {
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprint(val)
}
