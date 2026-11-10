package interceptor

import (
	"context"
	"fmt"
	"strings"
	"testing"

	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"google.golang.org/grpc"
)

// truncateForLog is the only bound a caller-controlled string meets before it
// becomes a log field: maxSummaryFields caps how many fields a record carries,
// never how long one value is.
func TestTruncateForLogBoundsOneValue(t *testing.T) {
	over := strings.Repeat("x", maxLoggedString+1)
	overWant := strings.Repeat("x", maxLoggedString) +
		fmt.Sprintf("...(truncated, %d bytes total)", maxLoggedString+1)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"short stays whole", "ev-42", "ev-42"},
		{"the cap itself stays whole", strings.Repeat("x", maxLoggedString), strings.Repeat("x", maxLoggedString)},
		{"one byte over is cut", over, overWant},
		{"a megabyte is cut", strings.Repeat("y", 1<<20), strings.Repeat("y", maxLoggedString) +
			fmt.Sprintf("...(truncated, %d bytes total)", 1<<20)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateForLog(tt.in)
			if got != tt.want {
				t.Fatalf("want %d bytes, got %d bytes: %.60q", len(tt.want), len(got), got)
			}
		})
	}
}

// The invariant the amplification finding is about: one anonymous request whose
// id field is 1 MiB must not produce a record several times the size of that
// request. The record keeps a correlatable prefix and states the real size.
func TestAccessLogBoundsAOneMiBRequestID(t *testing.T) {
	const idLen = 1 << 20

	log, logs := newObsLogger()
	a := NewAccessLog(log, config.Config{}, nil)

	req := &eventv1.GetEventRequest{Id: strings.Repeat("a", idLen)}
	if _, err := a.Unary()(ctxWithIDs("rid-trunc", "uid-trunc"), req,
		&grpc.UnaryServerInfo{FullMethod: "/event.v1.EventService/GetEvent"},
		func(context.Context, any) (any, error) { return nil, nil }); err != nil {
		t.Fatalf("unary: %v", err)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}

	want := strings.Repeat("a", maxLoggedString) +
		fmt.Sprintf("...(truncated, %d bytes total)", idLen)
	got := stringFieldOf(t, entries[0], "id")
	if got != want {
		t.Fatalf("want a %d-byte truncated id, got %d bytes: %.80q...", len(want), len(got), got)
	}
	if !strings.HasPrefix(got, strings.Repeat("a", maxLoggedString)) {
		t.Error("want the first 256 bytes of the caller value kept for correlation")
	}
}

// The response id takes the same path, so a handler that echoes oversized text
// is bounded as well.
func TestResponseSummaryBoundsAnOversizedID(t *testing.T) {
	const idLen = 4096

	fields := responseSummary(&eventv1.Event{Id: strings.Repeat("b", idLen)})

	var id string
	var found bool
	for _, f := range fields {
		if f.Key == "id" {
			id, found = f.String, true
		}
	}
	if !found {
		t.Fatalf("want an id field in %+v", fields)
	}
	if want := fmt.Sprintf("...(truncated, %d bytes total)", idLen); !strings.HasSuffix(id, want) {
		t.Errorf("want the response id bounded with %q, got %d bytes", want, len(id))
	}
	if len(id) > maxLoggedString+len("...(truncated, 4096 bytes total)") {
		t.Errorf("want a bounded field, got %d bytes", len(id))
	}
}
