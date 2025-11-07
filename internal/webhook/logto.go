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
func (s *LogtoWebhookService) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.log.Info("received Logto webhook request",
		zap.String("method", r.Method),
		zap.String("url", r.URL.Path),
	)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("failed to read request body", zap.Error(err))
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	if s.config.LogtoWebhookSecret != "" {
		if err := s.ValidateSignature(r, body, s.config.LogtoWebhookSecret); err != nil {
			s.log.Error("webhook signature validation failed", zap.Error(err))
			http.Error(w, "unauthorized: Invalid signature", http.StatusUnauthorized)
			return
		}
	} else {
		s.log.Warn("Logto webhook secret is not configured, signature validation skipped")
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &rawMap); err != nil {
		s.log.Error("unmarshal to rawMap", zap.Error(err))
	}
	var payload LogtoPayload
	if err := json.Unmarshal(rawMap["event"], &payload.Event); err != nil {
		s.log.Error("failed to parse event", zap.Error(err))
	}

	s.log.Info("Processing Logto event",
		zap.String("event", payload.Event),
		zap.String("user_id", payload.UserID),
	)

	switch payload.Event {
	case "PostSignIn":
		user, err := s.parseUser(rawMap)
		if err != nil {
			s.log.Error("failed to parse user", zap.Error(err))
			http.Error(w, "Failed to parse user data", http.StatusBadRequest)
			return
		}
		s.handleUserSignedIn(w, r, *user)
	default:
		s.log.Warn(
			"received unsupported event type",
			zap.String("event", payload.Event),
		)
		w.WriteHeader(http.StatusOK)
	}
}

func (s *LogtoWebhookService) parseUser(rawMap map[string]json.RawMessage) (*User, error) {
	var user User
	userJson, ok := rawMap["user"]
	if !ok {
		s.log.Error("user data not found in payload")
		return nil, errors.New("user data not found")
	}
	if err := json.Unmarshal(userJson, &user); err != nil {
		s.log.Error("failed to parse user data", zap.Error(err))
		return nil, errors.New("invalid user data format")
	}
	return &user, nil
}

// handleUserSignedIn processes user sign-in events from Logto
func (s *LogtoWebhookService) handleUserSignedIn(
	w http.ResponseWriter,
	_ *http.Request,
	user User,
) {
	s.log.Info("Processing PostSignIn event", zap.String("user_id", user.Id))
	s.log.Debug("avatar", zap.String("avatar", user.Avatar))
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User sign-in event processed successfully")
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
