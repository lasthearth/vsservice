package interceptor

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// A public method never reaches the auth interceptor, so identity is the only
// place its request_id can come from. Without it an anonymous call has no
// correlation at all.
func TestWithRequestIDMintsForAnonymousCall(t *testing.T) {
	ctx, rid := withRequestID(context.Background(), "/serverinfo.v1.ServerInfoService/WorldTime")

	if rid == "" {
		t.Fatal("want a minted request id, got empty")
	}
	if _, err := uuid.Parse(rid); err != nil {
		t.Errorf("want a uuid v7 id, got %q: %v", rid, err)
	}

	got, err := GetRequestID(ctx)
	if err != nil {
		t.Fatalf("GetRequestID: %v", err)
	}
	if got != rid {
		t.Errorf("ctx id %q != returned id %q", got, rid)
	}

	fields := logging.ExtractFields(ctx)
	if !containsField(fields, "request_id", rid) {
		t.Errorf("request_id must be a middleware log field, got %v", fields)
	}
	if !containsField(fields, "method", "/serverinfo.v1.ServerInfoService/WorldTime") {
		t.Errorf("method must be a middleware log field, got %v", fields)
	}
}

func TestWithRequestIDAdoptsValidIncoming(t *testing.T) {
	incoming := uuid.MustParse("0193f0a2-1b3c-7d4e-8f50-6172839405ab").String()
	ctx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs(RequestIDMetadataKey, incoming),
	)

	_, rid := withRequestID(ctx, "/m.v1.S/M")

	if rid != incoming {
		t.Errorf("want the caller id %q to be adopted, got %q", incoming, rid)
	}
}

// A client controls this header, so a malformed or absurd value must never
// reach a log line or a response header.
func TestWithRequestIDRejectsUnusableIncoming(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"not a uuid", "not-a-uuid"},
		{"empty", ""},
		{"overlong", strings.Repeat("a", maxIncomingRequestID+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(
				context.Background(),
				metadata.Pairs(RequestIDMetadataKey, tt.value),
			)

			_, rid := withRequestID(ctx, "/m.v1.S/M")

			if rid == tt.value {
				t.Fatalf("want the incoming id to be replaced, got %q", rid)
			}
			if _, err := uuid.Parse(rid); err != nil {
				t.Errorf("want a minted uuid, got %q: %v", rid, err)
			}
		})
	}
}

// Identity must not overwrite an id another interceptor already provisioned,
// or one call would appear under two ids.
func TestWithRequestIDKeepsExistingID(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey{"rid"}, "existing-id")

	got, rid := withRequestID(ctx, "/m.v1.S/M")

	if rid != "existing-id" {
		t.Errorf("want the existing id kept, got %q", rid)
	}
	if got != ctx {
		t.Error("want the same ctx back when an id already exists")
	}
}

func TestIdentityUnaryPassesContextToHandler(t *testing.T) {
	var seenID string

	interceptor := NewIdentity()
	resp, err := interceptor.Unary()(
		context.Background(),
		"req",
		&grpc.UnaryServerInfo{FullMethod: "/donate.v1.DonateService/AddCoins"},
		func(ctx context.Context, _ any) (any, error) {
			rid, ridErr := GetRequestID(ctx)
			if ridErr != nil {
				return nil, ridErr
			}
			seenID = rid
			return "ok", nil
		},
	)

	if err != nil {
		t.Fatalf("unary: %v", err)
	}
	if resp != "ok" {
		t.Errorf("want the handler response, got %v", resp)
	}
	if seenID == "" {
		t.Error("handler ctx must carry a request id")
	}
}

func containsField(fields logging.Fields, key, want string) bool {
	for i := 0; i+1 < len(fields); i += 2 {
		k, ok := fields[i].(string)
		if !ok || k != key {
			continue
		}
		if v, ok := fields[i+1].(string); ok && v == want {
			return true
		}
	}
	return false
}
