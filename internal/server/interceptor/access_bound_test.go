package interceptor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// A caller composes both the user agent and the forwarded chain, so both are
// attacker-sized text on the anonymous path. Neither may grow the record.
func TestClientFieldsBoundsCallerMetadata(t *testing.T) {
	const uaLen, xffLen = 4096, 8192

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"user-agent", strings.Repeat("u", uaLen),
		"x-forwarded-for", strings.Repeat("f", xffLen),
	))

	got := map[string]string{}
	for _, f := range clientFields(ctx) {
		if f.Type != zapcore.StringType {
			t.Fatalf("want a string field for %q, got %+v", f.Key, f)
		}
		got[f.Key] = f.String
	}

	if len(got) != 2 {
		t.Fatalf("want user_agent and client_ip, got %v", keysOf(got))
	}

	wants := map[string]struct {
		fill  string
		total int
	}{
		"user_agent": {strings.Repeat("u", maxLoggedString), uaLen},
		"client_ip":  {strings.Repeat("f", maxLoggedString), xffLen},
	}
	for key, want := range wants {
		if !strings.HasPrefix(got[key], want.fill) {
			t.Errorf("want the first %d bytes of %q kept, got %.40q", maxLoggedString, key, got[key])
		}
		suffix := fmt.Sprintf("...(truncated, %d bytes total)", want.total)
		if !strings.HasSuffix(got[key], suffix) {
			t.Errorf("want %q bounded with %q, got %d bytes", key, suffix, len(got[key]))
		}
		if len(got[key]) != maxLoggedString+len(suffix) {
			t.Errorf("want %q at exactly %d bytes, got %d", key, maxLoggedString+len(suffix), len(got[key]))
		}
	}
}

// A normal client is not punished: metadata within the cap is recorded whole.
func TestClientFieldsKeepsShortMetadata(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"user-agent", "grpc-go/1.82.1",
		"grpcgateway-x-forwarded-for", "203.0.113.7, 10.0.0.1",
	))

	got := map[string]string{}
	for _, f := range clientFields(ctx) {
		got[f.Key] = f.String
	}

	if got["user_agent"] != "grpc-go/1.82.1" {
		t.Errorf("want the user agent whole, got %q", got["user_agent"])
	}
	if got["client_ip"] != "203.0.113.7, 10.0.0.1" {
		t.Errorf("want the forwarded chain whole, got %q", got["client_ip"])
	}
}

// No metadata at all stays no fields: the record must not invent a caller.
func TestClientFieldsWithoutMetadata(t *testing.T) {
	if got := clientFields(context.Background()); got != nil {
		t.Errorf("want no fields without incoming metadata, got %+v", got)
	}
}

// End to end: no string field of one access record exceeds the cap plus the
// marker, whatever the caller sends in the metadata.
func TestAccessLogBoundsEveryCallerStringField(t *testing.T) {
	log, logs := newObsLogger()
	a := NewAccessLog(log, config.Config{}, nil)

	ctx := metadata.NewIncomingContext(ctxWithIDs("rid-md", "uid-md"), metadata.Pairs(
		"user-agent", strings.Repeat("u", 1<<16),
		"x-forwarded-for", strings.Repeat("1.2.3.4, ", 1<<13),
	))

	if _, err := a.Unary()(ctx, nil,
		&grpc.UnaryServerInfo{FullMethod: writeMethod},
		func(context.Context, any) (any, error) { return nil, errors.New("boom") }); err == nil {
		t.Fatal("want the handler error returned")
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}

	// The marker is at most len("...(truncated, 1048576 bytes total)"), so a
	// generous ceiling still fails on the unbounded behaviour this replaces.
	const ceiling = maxLoggedString + 64
	for _, f := range entries[0].Context {
		if f.Type != zapcore.StringType || len(f.String) <= ceiling {
			continue
		}
		t.Errorf("field %q is %d bytes, want at most %d", f.Key, len(f.String), ceiling)
	}
	if got := stringFieldOf(t, entries[0], "user_agent"); !strings.HasPrefix(got, strings.Repeat("u", maxLoggedString)) {
		t.Errorf("want the user agent prefix kept, got %.40q", got)
	}
	if got := stringFieldOf(t, entries[0], "client_ip"); !strings.HasSuffix(got, "...(truncated, 73728 bytes total)") {
		t.Errorf("want the forwarded chain bounded, got %d bytes: %.40q", len(got), got)
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
