package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	verificationv1 "github.com/lasthearth/vsservice/gen/verification/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	httpdto "github.com/lasthearth/vsservice/internal/player/internal/dto/http"
	playerierror "github.com/lasthearth/vsservice/internal/player/internal/ierror"
	playermodel "github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/verification/repository/repoerr"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc/codes"
)

// telegramToken has the shape of a real bot token: <bot id>:<secret>, both in
// the charset the masking regex accepts. It is a dummy value.
const telegramToken = "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"

// telegramSecret is the part of the token that grants control of the bot. No
// log record may carry it.
const telegramSecret = "AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"

// callLog records the order of the repository and transport calls, so a test
// can assert which write happened first.
type callLog struct {
	calls []string
}

func (c *callLog) add(name string) { c.calls = append(c.calls, name) }

func (c *callLog) count(name string) int {
	n := 0
	for _, call := range c.calls {
		if call == name {
			n++
		}
	}
	return n
}

func (c *callLog) index(name string) int {
	for i, call := range c.calls {
		if call == name {
			return i
		}
	}
	return -1
}

// fakeDb is an in-memory DbRepository. GetVerification hands out a copy, so the
// caller's in-place transition cannot change the stored document behind the
// test's back.
type fakeDb struct {
	calls     *callLog
	doc       *verification.Verification
	getErr    error
	createErr error
	updateErr error
	created   *verification.Verification
	updated   *verification.Verification
}

func (f *fakeDb) GetVerification(_ context.Context, _ string) (*verification.Verification, error) {
	f.calls.add("db.GetVerification")
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.doc == nil {
		return nil, repoerr.ErrNotFound
	}
	c := *f.doc
	return &c, nil
}

func (f *fakeDb) GetVerificationRequests(context.Context) ([]verification.Verification, error) {
	return nil, errors.New("GetVerificationRequests is not used by these tests")
}

func (f *fakeDb) GetVerificationStatusByUserGameName(context.Context, string) (verification.Status, error) {
	return "", errors.New("GetVerificationStatusByUserGameName is not used by these tests")
}

func (f *fakeDb) Create(_ context.Context, _ string, v verification.Verification) error {
	f.calls.add("db.Create")
	if f.createErr != nil {
		return f.createErr
	}
	f.created = &v
	return nil
}

func (f *fakeDb) Update(_ context.Context, _ string, v verification.Verification) error {
	f.calls.add("db.Update")
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = &v
	return nil
}

// fakeSso is an in-memory SsoRepository that records every grant and every
// profile write it was asked for.
type fakeSso struct {
	calls       *callLog
	tenantRoles []httpdto.Role
	userRoles   []httpdto.Role
	getRolesErr error
	grantErr    error
	nickErr     error
	grants      [][]string
	nicks       []string
}

func (f *fakeSso) GetUserRoles(_ context.Context, _ string) ([]httpdto.Role, error) {
	f.calls.add("sso.GetUserRoles")
	return f.userRoles, nil
}

func (f *fakeSso) GetRoles(context.Context) ([]httpdto.Role, error) {
	f.calls.add("sso.GetRoles")
	return f.tenantRoles, f.getRolesErr
}

func (f *fakeSso) UpdateUserRoles(_ context.Context, _ string, roleIds []string) error {
	f.calls.add("sso.UpdateUserRoles")
	f.grants = append(f.grants, roleIds)
	return f.grantErr
}

func (f *fakeSso) UpdateUserProfileNick(_ context.Context, _, nickname string) error {
	f.calls.add("sso.UpdateUserProfileNick")
	f.nicks = append(f.nicks, nickname)
	return f.nickErr
}

// fakeTelegram stands in for the outbound client: it records the request and
// answers, or fails the way a transport failure does.
type fakeTelegram struct {
	urls   []string
	bodies []string
	err    error
}

func (f *fakeTelegram) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	f.urls = append(f.urls, req.URL.String())
	f.bodies = append(f.bodies, string(body))
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		Request:    req,
	}, nil
}

type testEnv struct {
	svc  *Service
	db   *fakeDb
	sso  *fakeSso
	tg   *fakeTelegram
	logs *observer.ObservedLogs
}

// newTestEnv builds the service on fakes, with the production redaction core in
// the logging path, so a test sees exactly what a log record would carry.
func newTestEnv(t *testing.T, token string) *testEnv {
	t.Helper()

	calls := &callLog{}
	core, logs := observer.New(zap.DebugLevel)

	env := &testEnv{
		db:   &fakeDb{calls: calls},
		sso:  &fakeSso{calls: calls},
		tg:   &fakeTelegram{},
		logs: logs,
	}
	env.svc = New(Opts{
		Client:        &http.Client{Transport: env.tg},
		Log:           logger.NewWithCore(core),
		Cfg:           config.Config{TelegramToken: token, GroupId: "-100123"},
		DbRepo:        env.db,
		SsoRepository: env.sso,
	})
	return env
}

func playerRole() []httpdto.Role {
	return []httpdto.Role{{Id: "role-player", Name: "player"}, {Id: "role-mod", Name: "moderator"}}
}

func docWithStatus(status verification.Status) *verification.Verification {
	return &verification.Verification{
		Id:           "64f000000000000000000001",
		UserId:       "user-1",
		UserName:     "John",
		UserGameName: "Player1",
		Status:       status,
	}
}

func userCtx(userId string) context.Context {
	return interceptor.ContextWithUserID(context.Background(), userId)
}

// recordText flattens one captured record into a single string: the message and
// every field, whatever its type. A credential that survives in any of them is
// a leak, so the check must not look at string fields only.
func recordText(e observer.LoggedEntry) string {
	var b strings.Builder
	b.WriteString(e.Message)
	for _, f := range e.Context {
		b.WriteString(" | ")
		b.WriteString(f.Key)
		b.WriteString("=")
		switch f.Type {
		case zapcore.StringType:
			b.WriteString(f.String)
		case zapcore.ErrorType:
			if err, ok := f.Interface.(error); ok {
				b.WriteString(err.Error())
			}
		default:
			fmt.Fprintf(&b, "%v", f.Interface)
		}
	}
	return b.String()
}

// assertNoSecret fails when any captured record carries one of the secrets.
func assertNoSecret(t *testing.T, logs *observer.ObservedLogs, secrets ...string) {
	t.Helper()

	entries := logs.All()
	if len(entries) == 0 {
		t.Fatal("expected at least one log record, got none")
	}
	for _, e := range entries {
		text := recordText(e)
		for _, secret := range secrets {
			if strings.Contains(text, secret) {
				t.Errorf("log record carries the secret %q:\n%s", secret, text)
			}
		}
	}
}

// assertMasked fails when no record carries the masked form: a field that was
// dropped instead of masked must not pass as a fix.
func assertMasked(t *testing.T, logs *observer.ObservedLogs, want string) {
	t.Helper()

	for _, e := range logs.All() {
		if strings.Contains(recordText(e), want) {
			return
		}
	}
	t.Errorf("no log record carries %q; records: %v", want, logs.All())
}

func assertDomainCode(t *testing.T, err error, code codes.Code) {
	t.Helper()

	var de *ierror.DomainError
	if !errors.As(err, &de) {
		t.Fatalf("expected a *ierror.DomainError with code %v, got %v (%T)", code, err, err)
	}
	if de.Code != code {
		t.Fatalf("expected code %v, got %v (%q)", code, de.Code, de.Message)
	}
}

// A transport failure is the common case: net/http wraps the RoundTripper error
// into a *url.Error whose text repeats the whole request URL, bot token
// included. The record must carry the masked URL only.
func TestSendToTelegramMasksTokenOnTransportFailure(t *testing.T) {
	env := newTestEnv(t, telegramToken)
	env.tg.err = errors.New("dial tcp 149.154.167.220:443: i/o timeout")

	err := env.svc.SendToTelegram(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected the transport failure to surface")
	}

	// Prove the error really carries the token, otherwise the test proves nothing.
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Fatalf("expected a *url.Error, got %T: %v", err, err)
	}
	if !strings.Contains(urlErr.Error(), telegramToken) {
		t.Fatalf("the transport error does not carry the token, the test is vacuous: %v", urlErr)
	}

	assertNoSecret(t, env.logs, telegramToken, telegramSecret)
	assertMasked(t, env.logs, "bot[redacted]")
}

// A malformed URL fails earlier, inside http.NewRequestWithContext, and that
// error text carries the URL as well.
func TestSendToTelegramMasksTokenOnRequestFailure(t *testing.T) {
	// A control character makes url.Parse reject the URL; the secret part keeps
	// the token charset, so the masker matches it.
	token := "123456789:" + telegramSecret + "\x7f"
	env := newTestEnv(t, token)

	err := env.svc.SendToTelegram(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected the malformed URL to fail the request build")
	}
	if !strings.Contains(err.Error(), "invalid control character") {
		t.Fatalf("the request build failed for another reason, the test is vacuous: %v", err)
	}
	if len(env.tg.urls) != 0 {
		t.Fatalf("no request may reach the transport, got %v", env.tg.urls)
	}

	assertNoSecret(t, env.logs, token, telegramSecret)
	assertMasked(t, env.logs, "bot[redacted]")
}

// A tenant without a "player" role cannot be granted anything. Approve must
// refuse before it writes, so the application stays pending and retryable.
func TestApproveWithoutPlayerRoleFailsBeforeAnyWrite(t *testing.T) {
	env := newTestEnv(t, telegramToken)
	env.sso.tenantRoles = []httpdto.Role{{Id: "role-mod", Name: "moderator"}}
	env.db.doc = docWithStatus(verification.VerificationStatusPending)

	_, err := env.svc.Approve(context.Background(), &verificationv1.ApproveRequest{UserId: "user-1"})

	assertDomainCode(t, err, codes.FailedPrecondition)
	if env.db.updated != nil {
		t.Errorf("the status was committed without a role to grant: %+v", env.db.updated)
	}
	if len(env.sso.grants) != 0 {
		t.Errorf("a role grant was attempted: %v", env.sso.grants)
	}
	for _, name := range []string{"db.Update", "db.Create", "sso.UpdateUserRoles"} {
		if n := env.db.calls.count(name); n != 0 {
			t.Errorf("%s ran %d times, want 0", name, n)
		}
	}
}

// The normal path keeps the documented order: the transition is committed, then
// the role is granted.
func TestApprovePendingCommitsThenGrants(t *testing.T) {
	env := newTestEnv(t, telegramToken)
	env.sso.tenantRoles = playerRole()
	env.db.doc = docWithStatus(verification.VerificationStatusPending)

	resp, err := env.svc.Approve(context.Background(), &verificationv1.ApproveRequest{UserId: "user-1"})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if resp == nil {
		t.Fatal("Approve returned a nil response")
	}
	if env.db.updated == nil {
		t.Fatal("the approved document was not written")
	}
	if env.db.updated.Status != verification.VerificationStatusApproved {
		t.Errorf("stored status = %q, want %q", env.db.updated.Status, verification.VerificationStatusApproved)
	}
	if len(env.sso.grants) != 1 || len(env.sso.grants[0]) != 1 || env.sso.grants[0][0] != "role-player" {
		t.Errorf("grants = %v, want [[role-player]]", env.sso.grants)
	}
	if dbWrite, grant := env.db.calls.index("db.Update"), env.db.calls.index("sso.UpdateUserRoles"); dbWrite < 0 || grant < dbWrite {
		t.Errorf("the grant must follow the database write, calls = %v", env.db.calls.calls)
	}
}

// A lost grant after a committed transition answers Unavailable, so the caller
// knows a retry is valid, and leaves the document approved and recoverable.
func TestApproveReportsFailedGrantAsUnavailable(t *testing.T) {
	env := newTestEnv(t, telegramToken)
	env.sso.tenantRoles = playerRole()
	env.sso.grantErr = errors.New("logto answered 503")
	env.db.doc = docWithStatus(verification.VerificationStatusPending)

	_, err := env.svc.Approve(context.Background(), &verificationv1.ApproveRequest{UserId: "user-1"})

	assertDomainCode(t, err, codes.Unavailable)
	if env.db.updated == nil || env.db.updated.Status != verification.VerificationStatusApproved {
		t.Fatalf("the approved transition was lost, updated = %+v", env.db.updated)
	}
}

// The recovery path: an already-approved document means an earlier call
// committed and lost the grant. Approve re-sends the grant only, and it must
// not run checkUserRoles, because the role may be exactly what is missing.
func TestApproveReentryOnApprovedDocumentRegrantsOnly(t *testing.T) {
	env := newTestEnv(t, telegramToken)
	env.sso.tenantRoles = playerRole()
	// The user has no "player" role: the earlier grant was lost.
	env.sso.userRoles = nil
	env.db.doc = docWithStatus(verification.VerificationStatusApproved)

	resp, err := env.svc.Approve(context.Background(), &verificationv1.ApproveRequest{UserId: "user-1"})
	if err != nil {
		t.Fatalf("the retry of a lost grant must succeed, got %v", err)
	}
	if resp == nil {
		t.Fatal("Approve returned a nil response")
	}
	if len(env.sso.grants) != 1 || env.sso.grants[0][0] != "role-player" {
		t.Errorf("grants = %v, want [[role-player]]", env.sso.grants)
	}
	if n := env.db.calls.count("sso.GetUserRoles"); n != 0 {
		t.Errorf("checkUserRoles ran %d times on the re-entry path, want 0", n)
	}
	if n := env.db.calls.count("db.Update"); n != 0 {
		t.Errorf("db.Update ran %d times on the re-entry path, want 0", n)
	}
	if env.db.updated != nil {
		t.Errorf("the re-entry path rewrote the document: %+v", env.db.updated)
	}
	if env.db.doc.Status != verification.VerificationStatusApproved {
		t.Errorf("the stored document changed status: %q", env.db.doc.Status)
	}
}

// A failed re-grant stays retryable: Unavailable, and still no write.
func TestApproveReentryReportsFailedRegrantAsUnavailable(t *testing.T) {
	env := newTestEnv(t, telegramToken)
	env.sso.tenantRoles = playerRole()
	env.sso.grantErr = errors.New("logto answered 503")
	env.db.doc = docWithStatus(verification.VerificationStatusApproved)

	_, err := env.svc.Approve(context.Background(), &verificationv1.ApproveRequest{UserId: "user-1"})

	assertDomainCode(t, err, codes.Unavailable)
	if n := env.db.calls.count("db.Update"); n != 0 {
		t.Errorf("db.Update ran %d times on the re-entry path, want 0", n)
	}
}

// The existing guard stays: a user who already holds the role is refused, and
// nothing is written.
func TestApproveRefusesAUserWhoAlreadyHasTheRole(t *testing.T) {
	env := newTestEnv(t, telegramToken)
	env.sso.tenantRoles = playerRole()
	env.sso.userRoles = []httpdto.Role{{Id: "role-player", Name: "player"}}
	env.db.doc = docWithStatus(verification.VerificationStatusPending)

	_, err := env.svc.Approve(context.Background(), &verificationv1.ApproveRequest{UserId: "user-1"})

	if !errors.Is(err, verification.ErrAlreadyVerified) {
		t.Fatalf("expected ErrAlreadyVerified, got %v", err)
	}
	if env.db.updated != nil || len(env.sso.grants) != 0 {
		t.Errorf("a refused approval wrote something: updated=%+v grants=%v", env.db.updated, env.sso.grants)
	}
}

// The in-game name is a public attribution key, so Submit enforces the shape
// rule the nickname change enforces, before it writes anything anywhere.
func TestSubmitRefusesAnInvalidGameNameBeforeAnyWrite(t *testing.T) {
	names := []string{
		"",
		"Victim Name",
		"имяигрока",
		"name<script>",
		strings.Repeat("a", 16),
		"tab\tname",
	}

	for _, name := range names {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			env := newTestEnv(t, telegramToken)
			env.sso.tenantRoles = playerRole()

			_, err := env.svc.Submit(userCtx("user-1"), &verificationv1.SubmitRequest{
				UserName:     "John",
				UserGameName: name,
				Contacts:     "@john",
			})

			if !errors.Is(err, playermodel.ErrNicknameInvalid) {
				t.Fatalf("expected ErrNicknameInvalid, got %v", err)
			}
			assertDomainCode(t, err, codes.InvalidArgument)
			for _, call := range []string{
				"sso.UpdateUserProfileNick", "db.Create", "db.Update", "db.GetVerification",
			} {
				if n := env.db.calls.count(call); n != 0 {
					t.Errorf("%s ran %d times after a refused name, want 0", call, n)
				}
			}
			if len(env.tg.urls) != 0 {
				t.Errorf("a telegram notification was sent for a refused name: %v", env.tg.urls)
			}
		})
	}
}

// The first submission stores the document, and only then mirrors the name to
// the identity provider: a refused submission must leave nothing behind.
func TestSubmitWritesTheIdentityProviderAfterTheDatabase(t *testing.T) {
	env := newTestEnv(t, telegramToken)
	env.sso.tenantRoles = playerRole()

	resp, err := env.svc.Submit(userCtx("user-1"), &verificationv1.SubmitRequest{
		UserName:     "John",
		UserGameName: "Player1",
		Contacts:     "@john",
		Answers:      []*verificationv1.Answer{{Question: "q1", Answer: "a1"}},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if resp == nil {
		t.Fatal("Submit returned a nil response")
	}
	if env.db.created == nil {
		t.Fatal("the application was not stored")
	}
	if env.db.created.UserGameName != "Player1" {
		t.Errorf("stored name = %q, want %q", env.db.created.UserGameName, "Player1")
	}
	if len(env.sso.nicks) != 1 || env.sso.nicks[0] != "Player1" {
		t.Errorf("profile writes = %v, want [Player1]", env.sso.nicks)
	}
	create, nick := env.db.calls.index("db.Create"), env.db.calls.index("sso.UpdateUserProfileNick")
	if create < 0 || nick < create {
		t.Errorf("the identity-provider write must follow the database write, calls = %v", env.db.calls.calls)
	}
	if len(env.tg.urls) != 1 {
		t.Fatalf("telegram requests = %d, want 1", len(env.tg.urls))
	}
	if !strings.Contains(env.tg.bodies[0], "Player1") {
		t.Errorf("the notification does not carry the name: %s", env.tg.bodies[0])
	}
}

// A pending application is refused by the state check, and the refusal must not
// reach the identity provider.
func TestSubmitRefusesAPendingApplicationWithoutTouchingSSO(t *testing.T) {
	env := newTestEnv(t, telegramToken)
	env.db.doc = docWithStatus(verification.VerificationStatusPending)

	_, err := env.svc.Submit(userCtx("user-1"), &verificationv1.SubmitRequest{
		UserName:     "John",
		UserGameName: "Player2",
	})

	assertDomainCode(t, err, codes.FailedPrecondition)
	if len(env.sso.nicks) != 0 {
		t.Errorf("a refused submission wrote the profile: %v", env.sso.nicks)
	}
	if env.db.updated != nil {
		t.Errorf("a refused submission rewrote the document: %+v", env.db.updated)
	}
	if len(env.tg.urls) != 0 {
		t.Errorf("a refused submission sent a notification: %v", env.tg.urls)
	}
}

// A rejected application may be resubmitted, and the resubmission follows the
// same order.
func TestSubmitResubmitsAfterRejection(t *testing.T) {
	env := newTestEnv(t, telegramToken)
	env.db.doc = docWithStatus(verification.VerificationStatusRejected)

	if _, err := env.svc.Submit(userCtx("user-1"), &verificationv1.SubmitRequest{
		UserName:     "John",
		UserGameName: "Player3",
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if env.db.updated == nil || env.db.updated.UserGameName != "Player3" {
		t.Fatalf("the resubmission was not stored: %+v", env.db.updated)
	}
	update, nick := env.db.calls.index("db.Update"), env.db.calls.index("sso.UpdateUserProfileNick")
	if update < 0 || nick < update {
		t.Errorf("the identity-provider write must follow the database write, calls = %v", env.db.calls.calls)
	}
}

// A name collision is answered from the repository, and the identity provider
// is not written.
func TestSubmitReportsATakenNameWithoutTouchingSSO(t *testing.T) {
	env := newTestEnv(t, telegramToken)
	env.db.createErr = repoerr.ErrNickAlreadyExists

	_, err := env.svc.Submit(userCtx("user-1"), &verificationv1.SubmitRequest{
		UserName:     "John",
		UserGameName: "Taken1",
	})

	if !errors.Is(err, playerierror.ErrNickAlreadyExists) {
		t.Fatalf("expected ErrNickAlreadyExists, got %v", err)
	}
	assertDomainCode(t, err, codes.AlreadyExists)
	if len(env.sso.nicks) != 0 {
		t.Errorf("a refused submission wrote the profile: %v", env.sso.nicks)
	}
}

// The stored name is the authoritative one, so a failed mirror to Logto is
// reported in the log and does not fail the submission.
func TestSubmitKeepsTheStoredNameWhenTheProfileWriteFails(t *testing.T) {
	env := newTestEnv(t, telegramToken)
	env.sso.nickErr = errors.New("logto answered 500")

	if _, err := env.svc.Submit(userCtx("user-1"), &verificationv1.SubmitRequest{
		UserName:     "John",
		UserGameName: "Player4",
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if env.db.created == nil || env.db.created.UserGameName != "Player4" {
		t.Fatalf("the application was not stored: %+v", env.db.created)
	}
	assertMasked(t, env.logs, "logto nickname is out of sync with the stored one")
}
