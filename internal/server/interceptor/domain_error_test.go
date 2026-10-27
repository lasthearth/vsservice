package interceptor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The client message stays generic; the log must keep the real cause, or an
// Internal answer is undebuggable.
func TestDomainErrorsLogsTheCauseItHides(t *testing.T) {
	log, logs := newObsLogger()
	d := NewDomainErrors(log)

	cause := errors.New("mongo: no reachable server")
	wrapped := fmt.Errorf("read the settlement: %w", cause)

	resp, err := d.Unary()(ctxWithIDs("rid-1", "uid-1"), nil,
		&grpc.UnaryServerInfo{FullMethod: "/settlement.v1.SettlementService/Get"},
		func(context.Context, any) (any, error) { return nil, wrapped })
	if resp != nil {
		t.Errorf("want a nil response, got %v", resp)
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("want Internal, got %v", status.Code(err))
	}
	if got := status.Convert(err).Message(); got != "internal server error (ref rid-1)" {
		t.Errorf("want the generic client message with the ref id, got %q", got)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want exactly 1 record, got %d", len(entries))
	}
	e := entries[0]
	if e.Level != zap.ErrorLevel {
		t.Errorf("want Error, got %v", e.Level)
	}
	if got := stringFieldOf(t, e, "event"); got != internalErrorEvent {
		t.Errorf("want event %q, got %q", internalErrorEvent, got)
	}
	if got := stringFieldOf(t, e, "request_id"); got != "rid-1" {
		t.Errorf("want request_id rid-1, got %q", got)
	}
	if got := stringFieldOf(t, e, "method"); got != "/settlement.v1.SettlementService/Get" {
		t.Errorf("want the method recorded, got %q", got)
	}

	// The record must carry the chain and the cause text, not only the
	// sanitized message the client saw.
	if _, ok := fieldOf(e, "error_chain"); !ok {
		t.Error("want an error_chain field")
	}
	errField, ok := fieldOf(e, "error")
	if !ok {
		t.Fatal("want an error field")
	}
	// zap.Error stores the error itself, not its text.
	if got := fmt.Sprint(errField.Interface); !strings.Contains(got, cause.Error()) {
		t.Errorf("want the root cause %q in the error field, got %q", cause.Error(), got)
	}
}

// errorChain is what makes a wrapped cause visible when Error() drops it.
func TestErrorChainUnwraps(t *testing.T) {
	cause := errors.New("mongo: no reachable server")
	wrapped := fmt.Errorf("read the settlement: %w", cause)

	got := errorChain(wrapped)

	want := []string{"read the settlement: " + cause.Error(), cause.Error()}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chain[%d]: want %q, got %q", i, want[i], got[i])
		}
	}

	if chain := errorChain(nil); len(chain) != 0 {
		t.Errorf("want an empty chain for a nil error, got %v", chain)
	}
}

// A domain error is an expected outcome, not a defect: it maps to its code and
// produces no error record.
func TestDomainErrorsPassesDomainErrorThrough(t *testing.T) {
	log, logs := newObsLogger()
	d := NewDomainErrors(log)

	_, err := d.Unary()(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/settlement.v1.SettlementService/Get"},
		func(context.Context, any) (any, error) { return nil, ierror.NotFound("no such settlement") })

	if status.Code(err) != codes.NotFound {
		t.Errorf("want NotFound, got %v", status.Code(err))
	}
	if got := status.Convert(err).Message(); got != "no such settlement" {
		t.Errorf("want the domain message kept, got %q", got)
	}
	if n := logs.Len(); n != 0 {
		t.Errorf("want no record for an expected domain error, got %d", n)
	}
}

// An error that is already a gRPC status carries the caller's own decision.
func TestDomainErrorsPassesStatusThrough(t *testing.T) {
	log, logs := newObsLogger()
	d := NewDomainErrors(log)

	original := status.Error(codes.PermissionDenied, "no permission")
	_, err := d.Unary()(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/m.v1.S/M"},
		func(context.Context, any) (any, error) { return nil, original })

	if !errors.Is(err, original) && status.Convert(err).Message() != original.Error() {
		t.Errorf("want the status unchanged, got %v", err)
	}
	if n := logs.Len(); n != 0 {
		t.Errorf("want no record, got %d", n)
	}
}

func TestDomainErrorsSuccessIsSilent(t *testing.T) {
	log, logs := newObsLogger()
	d := NewDomainErrors(log)

	resp, err := d.Unary()(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/m.v1.S/M"},
		func(context.Context, any) (any, error) { return "ok", nil })

	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if resp != "ok" {
		t.Errorf("want the handler response, got %v", resp)
	}
	if n := logs.Len(); n != 0 {
		t.Errorf("want no record, got %d", n)
	}
}
