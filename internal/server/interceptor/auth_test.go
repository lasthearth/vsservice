package interceptor

import (
	"context"
	"errors"
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/jwt"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// --- fakes -----------------------------------------------------------------

type fakeVerifier struct {
	claims *jwt.Claims
	err    error
}

func (f fakeVerifier) Verify(string) (*jwt.Claims, error) { return f.claims, f.err }

type fakeScoper map[Method]Scope

func (f fakeScoper) Scope() map[Method]Scope { return f }

// nopLogger counts Warn calls. The embedded nil interface is never touched:
// the interceptor only logs warnings.
type nopLogger struct {
	logger.Logger
	warns int
}

func (l *nopLogger) Warn(string, ...zap.Field) { l.warns++ }

const (
	testMethod   = "/donate.v1.DonateService/AddCoins"
	testScope    = "donate:coins:add"
	unclaimedMtd = "/media.v1.MediaService/CreateUploadUrls"
)

func newTestAuth(v tokenVerifier, log logger.Logger, scopers ...Scoper) *Auth {
	return &Auth{
		jwtManager: v,
		log:        log,
		policy:     buildPolicy(scopers, log),
	}
}

func bearerCtx(token string) context.Context {
	return metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("authorization", token),
	)
}

// --- authorize -------------------------------------------------------------

func TestAuthorize(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		method  string
		verify  fakeVerifier
		wantErr codes.Code // codes.OK means "expect nil error"
	}{
		{
			name:    "no metadata",
			ctx:     context.Background(),
			method:  testMethod,
			wantErr: codes.Unauthenticated,
		},
		{
			name:    "missing authorization header",
			ctx:     metadata.NewIncomingContext(context.Background(), metadata.MD{}),
			method:  testMethod,
			wantErr: codes.Unauthenticated,
		},
		{
			name:    "missing Bearer prefix",
			ctx:     bearerCtx("token-without-prefix"),
			method:  testMethod,
			wantErr: codes.Unauthenticated,
		},
		{
			name:    "jwt verify failure",
			ctx:     bearerCtx("Bearer bad"),
			method:  testMethod,
			verify:  fakeVerifier{err: errors.New("expired")},
			wantErr: codes.Unauthenticated,
		},
		{
			name:   "valid token with required scope",
			ctx:    bearerCtx("Bearer good"),
			method: testMethod,
			verify: fakeVerifier{claims: &jwt.Claims{Scope: "other " + testScope}},
		},
		{
			name:    "valid token missing required scope",
			ctx:     bearerCtx("Bearer good"),
			method:  testMethod,
			verify:  fakeVerifier{claims: &jwt.Claims{Scope: "unrelated:scope"}},
			wantErr: codes.PermissionDenied,
		},
		{
			// Fail-open by omission: a method no scoper claims is allowed for
			// any authenticated caller. Deliberately unchanged — pinned here
			// so a future change to this default shows up as a test failure.
			name:   "method claimed by no scoper is permitted",
			ctx:    bearerCtx("Bearer good"),
			method: unclaimedMtd,
			verify: fakeVerifier{claims: &jwt.Claims{Scope: ""}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAuth(tt.verify, &nopLogger{}, fakeScoper{testMethod: testScope})

			_, err := a.authorize(tt.ctx, tt.method)

			if tt.wantErr == codes.OK {
				if err != nil {
					t.Fatalf("want nil error, got %v", err)
				}
				return
			}
			if status.Code(err) != tt.wantErr {
				t.Fatalf("want code %v, got %v (err=%v)", tt.wantErr, status.Code(err), err)
			}
		})
	}
}

func TestAuthorizePopulatesContext(t *testing.T) {
	a := newTestAuth(
		fakeVerifier{claims: &jwt.Claims{Scope: testScope}},
		&nopLogger{},
		fakeScoper{testMethod: testScope},
	)

	ctx, err := a.authorize(bearerCtx("Bearer good"), testMethod)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}

	if _, err := GetRequestID(ctx); err != nil {
		t.Errorf("GetRequestID: %v", err)
	}
	if _, err := GetClaims(ctx); err != nil {
		t.Errorf("GetClaims: %v", err)
	}
}

// --- policy table ----------------------------------------------------------

func TestBuildPolicyFirstScoperWins(t *testing.T) {
	log := &nopLogger{}
	policy := buildPolicy([]Scoper{
		fakeScoper{testMethod: "first"},
		fakeScoper{testMethod: "second"},
	}, log)

	if got := policy[testMethod]; got != "first" {
		t.Errorf("want first scoper to win, got %q", got)
	}
	if log.warns != 1 {
		t.Errorf("want 1 duplicate WARN, got %d", log.warns)
	}
}

func TestBuildPolicyMerges(t *testing.T) {
	log := &nopLogger{}
	policy := buildPolicy([]Scoper{
		fakeScoper{"/a/A": "sa"},
		fakeScoper{"/b/B": "sb"},
		fakeScoper{},
	}, log)

	if len(policy) != 2 || policy["/a/A"] != "sa" || policy["/b/B"] != "sb" {
		t.Errorf("unexpected policy: %v", policy)
	}
	if log.warns != 0 {
		t.Errorf("want no warnings, got %d", log.warns)
	}
}

// --- uncovered methods -----------------------------------------------------

func TestUncoveredMethods(t *testing.T) {
	a := newTestAuth(fakeVerifier{}, &nopLogger{}, fakeScoper{testMethod: testScope})

	got := a.uncoveredMethods(map[string]grpc.ServiceInfo{
		"donate.v1.DonateService": {Methods: []grpc.MethodInfo{
			{Name: "AddCoins"},      // scoped
			{Name: "ListShopItems"}, // public
			{Name: "GetBalance"},    // uncovered
		}},
	})

	if len(got) != 1 || got[0] != "/donate.v1.DonateService/GetBalance" {
		t.Errorf("unexpected uncovered list: %v", got)
	}
}

// --- context constructors --------------------------------------------------

func TestContextWithUserIDRoundTrip(t *testing.T) {
	ctx := ContextWithUserID(context.Background(), "uid-1")

	uid, err := GetUserID(ctx)
	if err != nil {
		t.Fatalf("GetUserID: %v", err)
	}
	if uid != "uid-1" {
		t.Errorf("want uid-1, got %q", uid)
	}

	if _, err := GetUserID(context.Background()); !errors.Is(err, ErrGetUserID) {
		t.Errorf("want ErrGetUserID on empty ctx, got %v", err)
	}
}

func TestContextWithClaimsRoundTrip(t *testing.T) {
	ctx := ContextWithClaims(context.Background(), &jwt.Claims{Scope: "a b"})

	claims, err := GetClaims(ctx)
	if err != nil {
		t.Fatalf("GetClaims: %v", err)
	}
	if claims.Scope != "a b" {
		t.Errorf("want scope %q, got %q", "a b", claims.Scope)
	}

	if _, err := GetClaims(context.Background()); !errors.Is(err, ErrGetClaims) {
		t.Errorf("want ErrGetClaims on empty ctx, got %v", err)
	}
}

func TestRequestIDRoundTrip(t *testing.T) {
	ctx, err := provideReqID(context.Background())
	if err != nil {
		t.Fatalf("provideReqID: %v", err)
	}

	// Regression: rid used to be stored as uuid.UUID while GetRequestID
	// asserted string, so this assertion could never succeed.
	rid, err := GetRequestID(ctx)
	if err != nil {
		t.Fatalf("GetRequestID: %v", err)
	}
	if rid == "" {
		t.Error("want non-empty request id")
	}

	if _, err := GetRequestID(context.Background()); !errors.Is(err, ErrGetRequestID) {
		t.Errorf("want ErrGetRequestID on empty ctx, got %v", err)
	}
}
