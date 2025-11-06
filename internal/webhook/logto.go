package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LogtoPayload represents the structure of a Logto webhook payload
type LogtoPayload struct {
	Event     string          `json:"event"`
	UserID    string          `json:"userId"`
	Payload   json.RawMessage `json:"payload"`
	TenantID  string          `json:"tenantId"`
	Timestamp time.Time       `json:"timestamp"`
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.log.Info("Received Logto webhook request",
		zap.String("method", r.Method),
		zap.String("url", r.URL.Path),
		zap.String("remote_addr", r.RemoteAddr))

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("Failed to read request body", zap.Error(err))
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if s.config.LogtoWebhookSecret != "" {
		if err := s.ValidateSignature(r, body, s.config.LogtoWebhookSecret); err != nil {
			s.log.Error("Webhook signature validation failed", zap.Error(err))
			http.Error(w, "Unauthorized: Invalid signature", http.StatusUnauthorized)
			return
		}
	} else {
		s.log.Warn("Logto webhook secret is not configured, signature validation skipped")
	}

	var payload LogtoPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		s.log.Error("Failed to parse webhook payload", zap.Error(err))
		http.Error(w, "Invalid payload format", http.StatusBadRequest)
		return
	}

	s.log.Info("Processing Logto event",
		zap.String("event", payload.Event),
		zap.String("user_id", payload.UserID),
		zap.String("tenant_id", payload.TenantID))

	switch payload.Event {
	case "User.Created":
		s.handleUserCreated(w, r, payload)
	case "User.Updated":
		s.handleUserUpdated(w, r, payload)
	case "User.Deleted":
		s.handleUserDeleted(w, r, payload)
	case "PostSignIn":
		s.handleUserSignedIn(w, r, payload)
	case "User.SignedOut":
		s.handleUserSignedOut(w, r, payload)
	default:
		s.log.Info("Received unsupported event type", zap.String("event", payload.Event))
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Event %s is not handled but acknowledged", payload.Event)
	}
}

// handleUserCreated processes user creation events from Logto
func (s *LogtoWebhookService) handleUserCreated(w http.ResponseWriter, r *http.Request, payload LogtoPayload) {
	s.log.Info("Processing User.Created event", zap.String("user_id", payload.UserID))

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User creation event processed successfully")
}

// handleUserUpdated processes user update events from Logto
func (s *LogtoWebhookService) handleUserUpdated(w http.ResponseWriter, r *http.Request, payload LogtoPayload) {
	s.log.Info("Processing User.Updated event", zap.String("user_id", payload.UserID))

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User update event processed successfully")
}

// handleUserDeleted processes user deletion events from Logto
func (s *LogtoWebhookService) handleUserDeleted(w http.ResponseWriter, r *http.Request, payload LogtoPayload) {
	s.log.Info("Processing User.Deleted event", zap.String("user_id", payload.UserID))

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User deletion event processed successfully")
}

// handleUserSignedIn processes user sign-in events from Logto
func (s *LogtoWebhookService) handleUserSignedIn(w http.ResponseWriter, r *http.Request, payload LogtoPayload) {
	s.log.Info("Processing User.SignedIn event", zap.String("user_id", payload.UserID))
	fmt.Println(string(payload.Payload))
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User sign-in event processed successfully")
}

// handleUserSignedOut processes user sign-out events from Logto
func (s *LogtoWebhookService) handleUserSignedOut(w http.ResponseWriter, r *http.Request, payload LogtoPayload) {
	s.log.Info("Processing User.SignedOut event", zap.String("user_id", payload.UserID))

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User sign-out event processed successfully")
}

// ValidateSignature validates the webhook request signature using HMAC-SHA256
func (s *LogtoWebhookService) ValidateSignature(r *http.Request, payload []byte, secret string) error {
	signature := r.Header.Get("logto-signature-sha-256")
	if signature == "" {
		return status.Error(codes.Unauthenticated, "Missing logto-signature-sha-256 header")
	}

	timestampStr := r.Header.Get("logto-timestamp")
	if timestampStr == "" {
		return status.Error(codes.Unauthenticated, "Missing logto-timestamp header")
	}

	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		return status.Error(codes.InvalidArgument, "Invalid timestamp format")
	}

	if time.Since(timestamp) > 5*time.Minute {
		return status.Error(codes.DeadlineExceeded, "Webhook timestamp is too old")
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(timestampStr))
	h.Write([]byte("."))
	h.Write(payload)
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return status.Error(codes.Unauthenticated, "Invalid signature")
	}

	return nil
}
