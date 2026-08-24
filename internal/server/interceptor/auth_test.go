package interceptor

import (
	"context"
	"errors"
	"strings"
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
			// Regression: "Bearer" without the trailing space used to satisfy
			// a HasPrefix check and then panic slicing [7:] on a 6-byte string,
			// killing the process from an unauthenticated request.
			name:    "bare Bearer without space",
			ctx:     bearerCtx("Bearer"),
			method:  testMethod,
			wantErr: codes.Unauthenticated,
		},
		{
			name:    "Bearer with empty token",
			ctx:     bearerCtx("Bearer "),
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

func TestAuthorizeScopeMatching(t *testing.T) {
	tests := []struct {
		name     string
		required Scope
		claim    string
		wantErr  codes.Code
	}{
		{
			// Regression: strings.Split(" ") preserves empty tokens, so an
			// empty required scope matched a scope-less token.
			name: "empty required scope denies a scope-less token", required: "", claim: "",
			wantErr: codes.PermissionDenied,
		},
		{
			// Regression: the same bug denied every caller that actually held
			// scopes, because "a b" splits without an empty token.
			name: "ScopeAuthenticated admits a caller holding scopes", required: ScopeAuthenticated, claim: "openid profile",
		},
		{
			name: "ScopeAuthenticated admits a scope-less token", required: ScopeAuthenticated, claim: "",
		},
		{
			// Regression: leading, trailing and doubled spaces produced empty
			// tokens that satisfied an empty required scope.
			name: "padded claim does not satisfy an empty scope", required: "", claim: " a  b ",
			wantErr: codes.PermissionDenied,
		},
		{
			name: "exact scope match", required: testScope, claim: "other " + testScope,
		},
		{
			name: "no substring match", required: "donate:coins", claim: "donate:coins:add",
			wantErr: codes.PermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAuth(
				fakeVerifier{claims: &jwt.Claims{Scope: tt.claim}},
				&nopLogger{},
				fakeScoper{testMethod: tt.required},
			)

			_, err := a.authorize(bearerCtx("Bearer good"), testMethod)

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

// gRPC's own reflection and health services are not domain RPCs, so they are not
// subject to the classification requirement.
func TestUncoveredMethodsSkipsInfraServices(t *testing.T) {
	a := newTestAuth(fakeVerifier{}, &nopLogger{}, fakeScoper{})

	got := a.uncoveredMethods(map[string]grpc.ServiceInfo{
		"grpc.reflection.v1.ServerReflection": {Methods: []grpc.MethodInfo{{Name: "ServerReflectionInfo"}}},
		"grpc.health.v1.Health":               {Methods: []grpc.MethodInfo{{Name: "Check"}}},
	})

	if len(got) != 0 {
		t.Errorf("want infra services skipped, got %v", got)
	}
}

// --- coverage verification -------------------------------------------------

func TestVerifyCoverageRejectsAnUnclassifiedMethod(t *testing.T) {
	// The RemoveMember case: a method nobody declared, reachable by any
	// authenticated caller, indistinguishable from a deliberate omission.
	a := newTestAuth(fakeVerifier{}, &nopLogger{}, fakeScoper{testMethod: testScope})

	err := a.verifyCoverage(map[string]grpc.ServiceInfo{
		"donate.v1.DonateService": {Methods: []grpc.MethodInfo{
			{Name: "AddCoins"},
			{Name: "ListShopItems"}, // public, listed so it is not read as stale
			{Name: "GetBalance"},
		}},
	})
	if err == nil {
		t.Fatal("want an error for the unclassified method, got nil")
	}
	if !strings.Contains(err.Error(), "/donate.v1.DonateService/GetBalance") {
		t.Errorf("error must name the offending method, got: %v", err)
	}
}

func TestVerifyCoverageAcceptsScopeAuthenticated(t *testing.T) {
	// Declaring a method authenticated-only is a decision, not an omission.
	a := newTestAuth(fakeVerifier{}, &nopLogger{}, fakeScoper{
		testMethod:                                testScope,
		"/donate.v1.DonateService/GetMyBalance":   ScopeAuthenticated,
		"/media.v1.MediaService/CreateUploadUrls": ScopeAuthenticated,
	})

	err := a.verifyCoverage(map[string]grpc.ServiceInfo{
		"donate.v1.DonateService": {Methods: []grpc.MethodInfo{
			{Name: "AddCoins"},
			{Name: "GetMyBalance"},
			{Name: "ListShopItems"}, // public
		}},
		"media.v1.MediaService": {Methods: []grpc.MethodInfo{{Name: "CreateUploadUrls"}}},
	})
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
}

func TestVerifyCoverageRejectsStaleDeclarations(t *testing.T) {
	// A declaration naming a method that does not exist is a typo or a leftover,
	// and it silently goes live the day a method with that name is added.
	// publicMethods carries exactly this: VerifyCode, an RPC in neither proto/
	// nor gen/.
	a := newTestAuth(fakeVerifier{}, &nopLogger{}, fakeScoper{
		"/donate.v1.DonateService/RenamedAway": testScope,
	})

	err := a.verifyCoverage(map[string]grpc.ServiceInfo{
		"donate.v1.DonateService": {Methods: []grpc.MethodInfo{
			{Name: "AddCoins"},
			{Name: "ListShopItems"}, // public
		}},
	})
	if err == nil {
		t.Fatal("want an error for the stale declaration, got nil")
	}
	if !strings.Contains(err.Error(), "RenamedAway") {
		t.Errorf("error must name the stale entry, got: %v", err)
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
