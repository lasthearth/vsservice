package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// obsLogger adapts a zap observer core to the project Logger interface, so a
// test asserts on the records the handler emits instead of parsing output.
type obsLogger struct {
	*zap.Logger
}

// newObsLogger deliberately builds the observer without logger.New: the
// redacting core would mask a sensitive key and hide the leak these tests look
// for. What is under test is the call site, not the core.
func newObsLogger() (logger.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.DebugLevel)
	return obsLogger{Logger: zap.New(core)}, logs
}

func (l obsLogger) With(fields ...zap.Field) logger.Logger {
	return obsLogger{Logger: l.Logger.With(fields...)}
}

func (l obsLogger) Named(name string) logger.Logger {
	return obsLogger{Logger: l.Logger.Named(name)}
}

func (l obsLogger) WithScope(scope string) logger.Logger {
	return l.With(zap.String("scope", scope))
}

func (l obsLogger) WithMethod(method string) logger.Logger {
	return l.With(zap.String("method", method))
}

func (l obsLogger) WithComponent(component string) logger.Logger {
	return l.With(zap.String("component", component))
}

func (l obsLogger) WithOptions(opts ...zap.Option) logger.Logger {
	return obsLogger{Logger: l.Logger.WithOptions(opts...)}
}

// ToLoggingLogger is unused by the handler under test.
func (l obsLogger) ToLoggingLogger() logging.Logger { return nil }

func (l obsLogger) Sync() error { return l.Logger.Sync() }

// webhookSecret is a fixture, not a credential: the tests sign and verify with
// it in the same process.
const webhookSecret = "test-secret"

// post runs one webhook request against a handler logging into a fresh observer.
func post(t *testing.T, cfg config.Config, body string, header http.Header) (*httptest.ResponseRecorder, *observer.ObservedLogs) {
	t.Helper()

	log, logs := newObsLogger()
	s := NewLogtoWebhookService(log, cfg)

	req := httptest.NewRequest(http.MethodPost, "/logtohook", strings.NewReader(body))
	req.Header = header
	rec := httptest.NewRecorder()
	s.HandleWebhook(rec, req)
	return rec, logs
}

// sign builds the header Logto sends with a body.
func sign(body string) http.Header {
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write([]byte(body))
	h := http.Header{}
	h.Set("logto-signature-sha-256", hex.EncodeToString(mac.Sum(nil)))
	return h
}

// entriesOf returns the captured records carrying this stable event key.
func entriesOf(logs *observer.ObservedLogs, event string) []observer.LoggedEntry {
	return logs.FilterField(zap.String("event", event)).All()
}

// fieldOf returns one field of a captured record.
func fieldOf(e observer.LoggedEntry, key string) (zap.Field, bool) {
	for _, f := range e.Context {
		if f.Key == key {
			return f, true
		}
	}
	return zap.Field{}, false
}

// stringFieldOf returns one string field, failing the test when it is absent or
// of another type.
func stringFieldOf(t *testing.T, e observer.LoggedEntry, key string) string {
	t.Helper()
	f, ok := fieldOf(e, key)
	if !ok {
		t.Fatalf("record %q has no field %q: %+v", e.Message, key, e.Context)
	}
	if f.Type != zapcore.StringType {
		t.Fatalf("field %q is not a string: %+v", key, f)
	}
	return f.String
}

// assertFieldSet fails unless the record carries exactly these keys. It is what
// keeps a payload field from creeping back into a record.
func assertFieldSet(t *testing.T, e observer.LoggedEntry, keys ...string) {
	t.Helper()
	if len(e.Context) != len(keys) {
		got := make([]string, 0, len(e.Context))
		for _, f := range e.Context {
			got = append(got, f.Key)
		}
		t.Fatalf("record %q carries %v, want %v", e.Message, got, keys)
	}
	for _, key := range keys {
		if _, ok := fieldOf(e, key); !ok {
			t.Fatalf("record %q has no field %q: %+v", e.Message, key, e.Context)
		}
	}
}

// A body that does not decode is a 400. Without the return the handler carried
// on with a zero payload and answered "unsupported event", which tells the
// caller its event type is wrong when the real problem is the body.
func TestHandleWebhookMalformedBodyIsBadRequest(t *testing.T) {
	rec, logs := post(t, config.Config{}, "{not json", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want status %d, got %d", http.StatusBadRequest, rec.Code)
	}
	if got := len(entriesOf(logs, webhookEventParseFailed)); got != 1 {
		t.Errorf("want 1 parse-failed record, got %d", got)
	}
	if got := len(entriesOf(logs, webhookEventUnsupported)); got != 0 {
		t.Errorf("want no unsupported-event record for a malformed body, got %d", got)
	}
}

// A body without an event takes the second unmarshal branch and must stop the
// handler the same way.
func TestHandleWebhookMissingEventIsBadRequest(t *testing.T) {
	rec, logs := post(t, config.Config{}, `{"userId":"u1"}`, nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want status %d, got %d", http.StatusBadRequest, rec.Code)
	}
	if got := len(entriesOf(logs, webhookEventUnsupported)); got != 0 {
		t.Errorf("want no unsupported-event record, got %d", got)
	}
}

// The signature check still runs before any parse, and a mismatch is a 401.
func TestHandleWebhookBadSignatureIsUnauthorized(t *testing.T) {
	h := http.Header{}
	h.Set("logto-signature-sha-256", "deadbeef")
	rec, logs := post(t, config.Config{LogtoWebhookSecret: webhookSecret}, `{"event":"PostSignIn"}`, h)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
	if got := len(entriesOf(logs, webhookEventSignatureFailed)); got != 1 {
		t.Errorf("want 1 signature-failed record, got %d", got)
	}
	if got := len(entriesOf(logs, webhookEventProcessed)); got != 0 {
		t.Errorf("want the handler to stop before PostSignIn, got %d processed records", got)
	}
}

// A valid signature reaches the event switch, so the 400 returns above did not
// move the accepted path.
func TestHandleWebhookValidSignatureReachesTheEvent(t *testing.T) {
	body := `{"event":"PostRegister"}`
	rec, logs := post(t, config.Config{LogtoWebhookSecret: webhookSecret}, body, sign(body))

	if rec.Code != http.StatusOK {
		t.Fatalf("want status %d, got %d", http.StatusOK, rec.Code)
	}
	if got := len(entriesOf(logs, webhookEventSignatureFailed)); got != 0 {
		t.Errorf("want no signature-failed record, got %d", got)
	}
	if got := len(entriesOf(logs, webhookEventUnsupported)); got != 1 {
		t.Errorf("want 1 unsupported-event record, got %d", got)
	}
}

// The two records that used to carry the payload keep the method, the path, the
// Logto event name and the Logto user id, and nothing else.
func TestHandleWebhookRecordFieldSets(t *testing.T) {
	rec, logs := post(t, config.Config{}, `{"event":"PostRegister"}`, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want status %d, got %d", http.StatusOK, rec.Code)
	}

	received := entriesOf(logs, webhookEventReceived)
	if len(received) != 1 {
		t.Fatalf("want 1 received record, got %d", len(received))
	}
	assertFieldSet(t, received[0], "event", "component", "method", "path")
	if got := stringFieldOf(t, received[0], "method"); got != http.MethodPost {
		t.Errorf("want method %q, got %q", http.MethodPost, got)
	}
	if got := stringFieldOf(t, received[0], "path"); got != "/logtohook" {
		t.Errorf("want path /logtohook, got %q", got)
	}

	processing := entriesOf(logs, webhookEventProcessing)
	if len(processing) != 1 {
		t.Fatalf("want 1 processing record, got %d", len(processing))
	}
	assertFieldSet(t, processing[0], "event", "component", "logto_event", "user_id")
	if got := stringFieldOf(t, processing[0], "logto_event"); got != "PostRegister" {
		t.Errorf("want logto_event PostRegister, got %q", got)
	}

	unsupported := entriesOf(logs, webhookEventUnsupported)
	if len(unsupported) != 1 {
		t.Fatalf("want 1 unsupported-event record, got %d", len(unsupported))
	}
	if unsupported[0].Level != zapcore.WarnLevel {
		t.Errorf("want Warn, got %s", unsupported[0].Level)
	}
	assertFieldSet(t, unsupported[0], "event", "component", "logto_event")
}

// The sign-in record keeps the Logto user id. No record of the request may
// carry the profile the body holds: the avatar URL, the display name or the
// username.
func TestHandleWebhookPostSignInKeepsProfileOutOfTheLog(t *testing.T) {
	const avatar = "https://cdn.example.com/a.png"
	body := `{"event":"PostSignIn","user":{"id":"u1","name":"Ann Example","avatar":"` +
		avatar + `","username":"annexample"}}`

	rec, logs := post(t, config.Config{}, body, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want status %d, got %d", http.StatusOK, rec.Code)
	}

	processed := entriesOf(logs, webhookEventProcessed)
	if len(processed) != 1 {
		t.Fatalf("want 1 processed record, got %d", len(processed))
	}
	assertFieldSet(t, processed[0], "event", "component", "user_id")
	if got := stringFieldOf(t, processed[0], "user_id"); got != "u1" {
		t.Errorf("want user_id u1, got %q", got)
	}

	// ContextMap renders every field, the error field included, so a value
	// smuggled in under any type is caught here and not only in a string field.
	banned := []string{avatar, "Ann Example", "annexample", body}
	for _, e := range logs.All() {
		blob := e.Message + fmt.Sprint(e.ContextMap())
		for _, b := range banned {
			if strings.Contains(blob, b) {
				t.Errorf("record %q leaks %q: %s", e.Message, b, blob)
			}
		}
	}
}
