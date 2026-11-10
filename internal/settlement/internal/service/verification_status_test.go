package service_test

import (
	"context"
	"errors"
	"testing"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/jwt"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// recordingRequestRepo wraps the shared fake and records every id VerificationStatus asks the
// repository about, so a refusal is proven to answer before the moderation state is read.
type recordingRequestRepo struct {
	*notifierRepo
	reads []string
}

func (r *recordingRequestRepo) GetSettlementRequestByLeader(
	ctx context.Context,
	leaderID string,
	kind model.SettlementKind,
) (*model.SettlementVerification, error) {
	r.reads = append(r.reads, leaderID)
	return r.notifierRepo.GetSettlementRequestByLeader(ctx, leaderID, kind)
}

func newVerificationService(t *testing.T, repo service.SettlementRepository) *service.Service {
	t.Helper()

	zc := zap.NewProductionConfig()
	log, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	return service.New(service.Opts{
		DbRepo:   repo,
		Mapper:   &sermapper.MapperImpl{},
		Log:      log,
		MediaURL: mediaurl.New(config.Config{CdnUrl: "https://cdn.test"}),
	})
}

// asSettlementModerator signs a caller in with the scope ListPending already answers to.
func asSettlementModerator(uid string) context.Context {
	return interceptor.ContextWithClaims(asUser(uid), &jwt.Claims{Scope: "settlements:manage"})
}

// asPlayer signs a caller in with an ordinary token: a subject and claims that carry no settlement
// moderation scope. Every authenticated call in production has claims, so this is what another
// player's request meets.
func asPlayer(uid string) context.Context {
	return interceptor.ContextWithClaims(asUser(uid), &jwt.Claims{Scope: "profile:read"})
}

// rejectedRequest is a settlement request a moderator turned down. The reason is free-form text
// about one player, which is what makes the read subject-bound.
func rejectedRequest() *model.SettlementVerification {
	return &model.SettlementVerification{
		Id:              "s1",
		Status:          model.SettlementStatusRejected,
		Type:            model.SettlementTypeCamp,
		RejectionReason: "coordinates fall inside another holding",
	}
}

// The applicant reads their own moderation state, reason included.
func TestVerificationStatusServesTheApplicant(t *testing.T) {
	repo := &recordingRequestRepo{notifierRepo: &notifierRepo{request: rejectedRequest()}}
	svc := newVerificationService(t, repo)

	res, err := svc.VerificationStatus(
		asUser("owner1"),
		&settlementv1.VerificationStatusRequest{UserId: "owner1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if res.GetStatus() != string(model.SettlementStatusRejected) {
		t.Errorf("status = %q, want rejected", res.GetStatus())
	}
	if res.GetRejectionReason() != "coordinates fall inside another holding" {
		t.Errorf("rejection_reason = %q, want the moderator's text", res.GetRejectionReason())
	}
	if len(repo.reads) != 1 || repo.reads[0] != "owner1" {
		t.Errorf("reads = %v, want one read of owner1", repo.reads)
	}
}

// Another signed-in player is refused, and the refusal answers before the request is read: the
// moderation state of a third party never leaves the repository for this caller.
func TestVerificationStatusRefusesAnotherPlayer(t *testing.T) {
	repo := &recordingRequestRepo{notifierRepo: &notifierRepo{request: rejectedRequest()}}
	svc := newVerificationService(t, repo)

	_, err := svc.VerificationStatus(
		asPlayer("attacker"),
		&settlementv1.VerificationStatusRequest{UserId: "owner1"},
	)
	if got := status.Code(err); got != codes.PermissionDenied {
		t.Fatalf("code = %v (%v), want PermissionDenied", got, err)
	}
	if len(repo.reads) != 0 {
		t.Errorf("reads = %v, want none: the refusal must answer before the lookup", repo.reads)
	}
}

// A moderator holding the manage scope reads any applicant's state, the way ListPending already
// lets them read the whole queue.
func TestVerificationStatusAdmitsAModerator(t *testing.T) {
	repo := &recordingRequestRepo{notifierRepo: &notifierRepo{request: rejectedRequest()}}
	svc := newVerificationService(t, repo)

	res, err := svc.VerificationStatus(
		asSettlementModerator("mod1"),
		&settlementv1.VerificationStatusRequest{UserId: "owner1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if res.GetStatus() != string(model.SettlementStatusRejected) {
		t.Errorf("status = %q, want rejected", res.GetStatus())
	}
	if len(repo.reads) != 1 || repo.reads[0] != "owner1" {
		t.Errorf("reads = %v, want one read of owner1", repo.reads)
	}
}

// A subject with no claims behind it is refused too: requireScope answers Unauthenticated, the same
// way it does for RemoveMember, and still reads nothing.
func TestVerificationStatusRefusesACallerWithoutClaims(t *testing.T) {
	repo := &recordingRequestRepo{notifierRepo: &notifierRepo{request: rejectedRequest()}}
	svc := newVerificationService(t, repo)

	_, err := svc.VerificationStatus(asUser("attacker"), &settlementv1.VerificationStatusRequest{UserId: "owner1"})
	if got := status.Code(err); got != codes.Unauthenticated {
		t.Fatalf("code = %v (%v), want Unauthenticated", got, err)
	}
	if len(repo.reads) != 0 {
		t.Errorf("reads = %v, want none", repo.reads)
	}
}

// A missing subject is a refusal, not an answer about somebody else.
func TestVerificationStatusNeedsASubject(t *testing.T) {
	repo := &recordingRequestRepo{notifierRepo: &notifierRepo{request: rejectedRequest()}}
	svc := newVerificationService(t, repo)

	if _, err := svc.VerificationStatus(
		context.Background(),
		&settlementv1.VerificationStatusRequest{UserId: "owner1"},
	); err == nil {
		t.Fatal("an anonymous call answered")
	}
	if len(repo.reads) != 0 {
		t.Errorf("reads = %v, want none", repo.reads)
	}
}

// Nothing found stays nothing found: the subject check must not turn an empty answer into a
// permission error for the player who owns the request.
func TestVerificationStatusKeepsNotFoundForTheApplicant(t *testing.T) {
	repo := &recordingRequestRepo{notifierRepo: &notifierRepo{}}
	svc := newVerificationService(t, repo)

	_, err := svc.VerificationStatus(
		asUser("owner1"),
		&settlementv1.VerificationStatusRequest{UserId: "owner1"},
	)
	if !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
