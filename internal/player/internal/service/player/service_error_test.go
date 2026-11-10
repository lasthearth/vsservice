package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/player/internal/service/player/sermapper"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// driverDetail is the text a MongoDB driver failure carries: the server
// address, the topology description and the collection a decode failed on. None
// of it may reach a client, and all of it must reach the log.
const driverDetail = "server selection error: context deadline exceeded, " +
	"current topology: { Type: ReplicaSetNoPrimary, Servers: " +
	"[{ Addr: mongo-0.vsservice.svc.cluster.local:27017, Type: Unknown }] }; " +
	"collection user_players: (BsonSerializationError) read map[string]interface {}"

// driverFragments are the pieces of driverDetail a client message must not
// contain, checked one by one so a failure names the leaked piece.
var driverFragments = []string{
	"mongo-0.vsservice.svc.cluster.local",
	"27017",
	"ReplicaSetNoPrimary",
	"topology",
	"user_players",
	"BsonSerializationError",
	"server selection error",
}

// internalErrorEvent is the record the domain-error interceptor writes when it
// sanitizes a cause. Spelled out here: the constant is unexported in
// internal/server/interceptor.
const internalErrorEvent = "grpc.call.internal_error"

// leakRequestID is the correlation id these calls carry, so the sanitized
// message and the hidden record can be tied together.
const leakRequestID = "rid-leak"

// leakHarness holds a service, a repository that fails on demand, the real
// domain-error interceptor and the records that interceptor captured.
type leakHarness struct {
	svc  *Service
	repo *fakeRepo
	de   *interceptor.DomainErrors
	logs *observer.ObservedLogs
}

func newLeakHarness(t *testing.T) leakHarness {
	t.Helper()

	core, logs := observer.New(zap.DebugLevel)
	log := logger.NewWithCore(core)

	repo := &fakeRepo{players: map[string]*playerState{}}
	svc := New(Opts{
		DbRepo:  repo,
		SsoRepo: &fakeSso{},
		Mapper:  &sermapper.MapperImpl{},
		Logger:  log,
	})
	svc.cnuc = &fakeNotifier{}

	return leakHarness{
		svc:  svc,
		repo: repo,
		de:   interceptor.NewDomainErrors(log),
		logs: logs,
	}
}

// call runs one handler under the real interceptor with the correlation fields
// the access interceptor injects, and returns what the client reads.
func (h leakHarness) call(method string, handler func(ctx context.Context) error) error {
	ctx := logging.InjectFields(
		interceptor.ContextWithUserID(context.Background(), "u1"),
		logging.Fields{"request_id", leakRequestID, "user_id", "u1"},
	)

	_, err := h.de.Unary()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method},
		func(context.Context, any) (any, error) { return nil, handler(ctx) })

	return err
}

// internalRecords returns the sanitizer's records.
func (h leakHarness) internalRecords() []observer.LoggedEntry {
	return h.logs.FilterField(zap.String("event", internalErrorEvent)).All()
}

func fieldOf(e observer.LoggedEntry, key string) (zap.Field, bool) {
	for _, f := range e.Context {
		if f.Key == key {
			return f, true
		}
	}

	return zap.Field{}, false
}

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

// A handler must not build a client status out of a driver error. Each case
// drives one site that used to answer status.Error(codes.Internal, err.Error())
// through the real interceptor and checks both halves of the contract: the
// client message is generic, and the cause survives in the log.
func TestDriverDetailStaysInTheLogOnThePublicUserMethods(t *testing.T) {
	driverErr := errors.New(driverDetail)

	cases := []struct {
		name   string
		method string
		setup  func(repo *fakeRepo)
		call   func(svc *Service, ctx context.Context) error
	}{
		{
			name:   "GetUser",
			method: "/user.v1.UserService/GetUser",
			setup:  func(repo *fakeRepo) { repo.getByIDErr = driverErr },
			call: func(svc *Service, ctx context.Context) error {
				_, err := svc.GetUser(ctx, &userv1.GetUserRequest{UserId: "u1"})
				return err
			},
		},
		{
			name:   "BatchGetUsers",
			method: "/user.v1.UserService/BatchGetUsers",
			setup:  func(repo *fakeRepo) { repo.batchErr = driverErr },
			call: func(svc *Service, ctx context.Context) error {
				_, err := svc.BatchGetUsers(ctx, &userv1.BatchGetUsersRequest{UserIds: []string{"u1"}})
				return err
			},
		},
		{
			name:   "ChangeNickname reads the player",
			method: "/user.v1.UserService/ChangeNickname",
			setup:  func(repo *fakeRepo) { repo.getByIDErr = driverErr },
			call: func(svc *Service, ctx context.Context) error {
				_, err := svc.ChangeNickname(ctx, &userv1.ChangeNicknameRequest{
					UserId: "u1", NewNickname: "Dozhd",
				})
				return err
			},
		},
		{
			name:   "ChangeNickname writes the nickname",
			method: "/user.v1.UserService/ChangeNickname",
			setup: func(repo *fakeRepo) {
				// No recorded change, so the cooldown lets the call reach the
				// write that fails here.
				repo.players["u1"] = &playerState{id: "u1", gameName: "Veter"}
				repo.nickUpdateErr = driverErr
			},
			call: func(svc *Service, ctx context.Context) error {
				_, err := svc.ChangeNickname(ctx, &userv1.ChangeNicknameRequest{
					UserId: "u1", NewNickname: "Dozhd",
				})
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newLeakHarness(t)
			tc.setup(h.repo)

			err := h.call(tc.method, func(ctx context.Context) error { return tc.call(h.svc, ctx) })

			// The client reads a code the service chose and a message that
			// carries nothing but the ref id.
			if code := status.Code(err); code != codes.Internal {
				t.Fatalf("code = %v, want Internal", code)
			}
			msg := status.Convert(err).Message()
			if want := fmt.Sprintf("internal server error (ref %s)", leakRequestID); msg != want {
				t.Errorf("client message = %q, want %q", msg, want)
			}
			for _, frag := range driverFragments {
				if strings.Contains(msg, frag) {
					t.Errorf("client message %q leaked the driver fragment %q", msg, frag)
				}
			}

			// The cause is not lost: one internal_error record carries the
			// driver text, the method and the request id.
			entries := h.internalRecords()
			if len(entries) != 1 {
				t.Fatalf("%s records = %d, want 1", internalErrorEvent, len(entries))
			}
			e := entries[0]
			if e.Level != zap.ErrorLevel {
				t.Errorf("record level = %v, want Error", e.Level)
			}
			if got := stringFieldOf(t, e, "method"); got != tc.method {
				t.Errorf("recorded method = %q, want %q", got, tc.method)
			}
			if got := stringFieldOf(t, e, "request_id"); got != leakRequestID {
				t.Errorf("recorded request_id = %q, want %q", got, leakRequestID)
			}
			if _, ok := fieldOf(e, "error_chain"); !ok {
				t.Error("the record has no error_chain field")
			}
			errField, ok := fieldOf(e, "error")
			if !ok {
				t.Fatal("the record has no error field")
			}
			// zap.Error stores the error value, not its text.
			if got := fmt.Sprint(errField.Interface); !strings.Contains(got, driverDetail) {
				t.Errorf("logged cause = %q, want it to carry %q", got, driverDetail)
			}
		})
	}
}

// The NotFound branch answers static text, so a missing user still tells the
// client nothing about the store behind it.
func TestGetUserAnswersNotFoundWithStaticText(t *testing.T) {
	h := newLeakHarness(t)

	err := h.call("/user.v1.UserService/GetUser", func(ctx context.Context) error {
		_, err := h.svc.GetUser(ctx, &userv1.GetUserRequest{UserId: "nobody"})
		return err
	})

	if code := status.Code(err); code != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", code)
	}
	if got := status.Convert(err).Message(); got != "user not found" {
		t.Errorf("message = %q, want the static %q", got, "user not found")
	}
	if n := len(h.logs.All()); n != 0 {
		t.Errorf("a handled answer wrote %d records, want none", n)
	}
}

// The static Internal message the audit cleared stays static: GetUserID fails
// with interceptor.ErrGetUserID, whose text the service may show.
func TestChangeNicknameKeepsTheStaticIdentityFailure(t *testing.T) {
	h := newLeakHarness(t)

	_, err := h.svc.ChangeNickname(
		context.Background(),
		&userv1.ChangeNicknameRequest{UserId: "u1", NewNickname: "Dozhd"},
	)

	if code := status.Code(err); code != codes.Internal {
		t.Fatalf("code = %v, want Internal", code)
	}
	if got := status.Convert(err).Message(); got != interceptor.ErrGetUserID.Error() {
		t.Errorf("message = %q, want the static %q", got, interceptor.ErrGetUserID.Error())
	}
}
