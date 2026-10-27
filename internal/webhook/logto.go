package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LogtoPayload represents the structure of a Logto webhook payload
type LogtoPayload struct {
	Event  string `json:"event"`
	UserID string `json:"userId"`
}

type User struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	UserName string `json:"username"`
}

// Stable machine keys for the records this handler emits. Dashboards and alerts
// filter on them, never on the message text, which is free to change.
const (
	webhookEventReceived        = "webhook.logto.received"
	webhookEventProcessing      = "webhook.logto.event"
	webhookEventProcessed       = "webhook.logto.processed"
	webhookEventUnsupported     = "webhook.logto.unsupported_event"
	webhookEventSignatureFailed = "webhook.logto.signature_failed"
	webhookEventParseFailed     = "webhook.logto.parse_failed"
	webhookEventSecretMissing   = "webhook.logto.secret_missing"
	webhookEventResponseFailed  = "webhook.logto.response_write_failed"
)

type LogtoWebhookService struct {
	log    logger.Logger
	config config.Config
}

func NewLogtoWebhookService(log logger.Logger, config config.Config) *LogtoWebhookService {
	return &LogtoWebhookService{
		log:    log,
		config: config,
	}
}

// HandleWebhook processes incoming Logto webhook requests
//
// A record here carries the request's method and path, the Logto event name and
// the Logto user id, and nothing else. The body is the user's profile — avatar
// URL, name and username included — and the redacting core masks by key name
// only, so a payload logged under an innocent key defeats it.
func (s *LogtoWebhookService) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// The HTTP middleware already assigned this request its request_id, so the
	// webhook records join the rest of the call chain under the same id.
	log := logger.From(r.Context(), s.log)

	log.Info("received Logto webhook request",
		zap.String("event", webhookEventReceived),
		zap.String("component", "webhook"),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
	)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error("failed to read request body",
			zap.String("event", webhookEventParseFailed),
			zap.String("component", "webhook"),
			zap.Error(err),
		)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	if s.config.LogtoWebhookSecret != "" {
		if err := s.ValidateSignature(r, body, s.config.LogtoWebhookSecret); err != nil {
			log.Error("webhook signature validation failed",
				zap.String("event", webhookEventSignatureFailed),
				zap.String("component", "webhook"),
				zap.Error(err),
			)
			http.Error(w, "unauthorized: Invalid signature", http.StatusUnauthorized)
			return
		}
	} else {
		log.Warn("Logto webhook secret is not configured, signature validation skipped",
			zap.String("event", webhookEventSecretMissing),
			zap.String("component", "webhook"),
		)
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawMap); err != nil {
		// Returning here is what keeps a malformed body from being answered as an
		// unsupported event: without it the switch below sees a zero payload and
		// the caller is told the event type is wrong instead of the body.
		log.Error("unmarshal to rawMap",
			zap.String("event", webhookEventParseFailed),
			zap.String("component", "webhook"),
			zap.Error(err),
		)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var payload LogtoPayload
	if err := json.Unmarshal(rawMap["event"], &payload.Event); err != nil {
		log.Error("failed to parse event",
			zap.String("event", webhookEventParseFailed),
			zap.String("component", "webhook"),
			zap.Error(err),
		)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// The user id lives beside the event in some payload shapes and inside the
	// user object in others; both are filled here, absence included, because a
	// missing key is not a malformed body.
	if raw, ok := rawMap["userId"]; ok && payload.UserID == "" {
		// A userId of the wrong shape does not invalidate the event: the name
		// already parsed and the record is useful without the id.
		if err := json.Unmarshal(raw, &payload.UserID); err != nil {
			log.Debug("userId is not a string",
				zap.String("event", webhookEventParseFailed),
				zap.Error(err),
			)
		}
	}

	// The Logto event name goes under logto_event: event is the stable machine
	// key of the envelope, and one record cannot carry two meanings for it.
	log.Info("Processing Logto event",
		zap.String("event", webhookEventProcessing),
		zap.String("component", "webhook"),
		zap.String("logto_event", payload.Event),
		zap.String("user_id", payload.UserID),
	)

	switch payload.Event {
	case "PostSignIn":
		user, err := s.parseUser(log, rawMap)
		if err != nil {
			log.Error("failed to parse user",
				zap.String("event", webhookEventParseFailed),
				zap.String("component", "webhook"),
				zap.Error(err),
			)
			http.Error(w, "Failed to parse user data", http.StatusBadRequest)
			return
		}
		s.handleUserSignedIn(log, w, *user)
	default:
		log.Warn(
			"received unsupported event type",
			zap.String("event", webhookEventUnsupported),
			zap.String("component", "webhook"),
			zap.String("logto_event", payload.Event),
		)
		w.WriteHeader(http.StatusOK)
	}
}

func (s *LogtoWebhookService) parseUser(log logger.Logger, rawMap map[string]json.RawMessage) (*User, error) {
	var user User
	userJson, ok := rawMap["user"]
	if !ok {
		log.Error("user data not found in payload",
			zap.String("event", webhookEventParseFailed),
			zap.String("component", "webhook"),
		)
		return nil, errors.New("user data not found")
	}
	if err := json.Unmarshal(userJson, &user); err != nil {
		log.Error("failed to parse user data",
			zap.String("event", webhookEventParseFailed),
			zap.String("component", "webhook"),
			zap.Error(err),
		)
		return nil, errors.New("invalid user data format")
	}
	return &user, nil
}

// handleUserSignedIn processes user sign-in events from Logto
//
// The user id is the only part of the sign-in worth keeping: name, username and
// avatar URL are profile data, and none of them answers a replay question.
func (s *LogtoWebhookService) handleUserSignedIn(
	log logger.Logger,
	w http.ResponseWriter,
	user User,
) {
	log.Info("Processing PostSignIn event",
		zap.String("event", webhookEventProcessed),
		zap.String("component", "webhook"),
		zap.String("user_id", user.Id),
	)
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, "User sign-in event processed successfully"); err != nil {
		log.Error("failed to write response",
			zap.String("event", webhookEventResponseFailed),
			zap.String("component", "webhook"),
			zap.Error(err),
		)
	}
}

// ValidateSignature validates the webhook request signature using HMAC-SHA256
func (s *LogtoWebhookService) ValidateSignature(r *http.Request, payload []byte, secret string) error {
	signature := r.Header.Get("logto-signature-sha-256")
	if signature == "" {
		return status.Error(codes.Unauthenticated, "missing logto-signature-sha-256 header")
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return status.Error(codes.Unauthenticated, "invalid signature")
	}

	return nil
}
