package service_test

import (
	"context"
	"testing"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// newOpService builds the settlement service over an observer core, so a test
// sees exactly the records the operations emit. The mail and the media
// validator are the pieces Submit and Approve touch on their way to a record.
func newOpService(t *testing.T, repo service.SettlementRepository) (*service.Service, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.InfoLevel)
	svc := service.New(service.Opts{
		DbRepo:   repo,
		Mapper:   &sermapper.MapperImpl{},
		Log:      logger.NewWithCore(core),
		Mail:     &fakeMail{},
		MediaURL: mediaurl.New(config.Config{CdnUrl: "https://cdn.test"}),
	})
	return svc, logs
}

// opString returns one record's string field, and fails the test when the field
// is absent: a missing assertion target is a broken record, not a pass.
func opString(t *testing.T, e observer.LoggedEntry, key string) string {
	t.Helper()
	for _, f := range e.Context {
		if f.Key == key && f.Type == zapcore.StringType {
			return f.String
		}
	}
	t.Fatalf("record %q carries no string field %q", e.Message, key)
	return ""
}

// opFieldPresent reports whether a key is on the record at all, whatever its
// type.
func opFieldPresent(e observer.LoggedEntry, key string) bool {
	for _, f := range e.Context {
		if f.Key == key {
			return true
		}
	}
	return false
}

// opEntryFor finds the one record with the given event key and fails the test
// when there is not exactly one of it: a duplicated fact is as wrong as a
// missing one.
func opEntryFor(t *testing.T, logs *observer.ObservedLogs, event string) observer.LoggedEntry {
	t.Helper()
	var found []observer.LoggedEntry
	for _, e := range logs.All() {
		for _, f := range e.Context {
			if f.Key == "event" && f.String == event {
				found = append(found, e)
			}
		}
	}
	if len(found) != 1 {
		t.Fatalf("records with event %q = %d, want exactly 1; all records: %v", event, len(found), logs.All())
	}
	return found[0]
}

// rejectRepo fakes only Reject. The embedded interface is nil, so any other
// call panics and says so, which pins the handler to that one write.
type rejectRepo struct {
	service.SettlementRepository
	id     string
	reason string
}

func (r *rejectRepo) Reject(_ context.Context, id, reason string) error {
	r.id, r.reason = id, reason
	return nil
}

// A rejection writes exactly one operation record: the event key, the request
// id, the moderator and the reason they gave, and no second line beside it.
func TestRejectEmitsOneRequestRejectedRecord(t *testing.T) {
	repo := &rejectRepo{}
	svc, logs := newOpService(t, repo)

	if _, err := svc.Reject(asUser("mod1"), &settlementv1.RejectRequest{
		Id:              "s1",
		RejectionReason: "скриншоты не показывают территорию",
	}); err != nil {
		t.Fatal(err)
	}

	if logs.Len() != 1 {
		t.Fatalf("records = %d, want exactly the rejection record: %v", logs.Len(), logs.All())
	}
	e := logs.All()[0]
	if e.Level != zapcore.InfoLevel {
		t.Fatalf("level = %s, want Info", e.Level)
	}
	if got := opString(t, e, "event"); got != "settlement.request.rejected" {
		t.Fatalf("event = %q, want settlement.request.rejected", got)
	}
	if got := opString(t, e, "component"); got != "op" {
		t.Fatalf("component = %q, want op", got)
	}
	if got := opString(t, e, "entity"); got != "settlement_request" {
		t.Fatalf("entity = %q, want settlement_request", got)
	}
	if got := opString(t, e, "entity_id"); got != "s1" {
		t.Fatalf("entity_id = %q, want s1", got)
	}
	if got := opString(t, e, "actor_id"); got != "mod1" {
		t.Fatalf("actor_id = %q, want mod1", got)
	}
	if got := opString(t, e, "reason"); got != "скриншоты не показывают территорию" {
		t.Fatalf("reason = %q, want the moderator's reason", got)
	}
	if got := opString(t, e, "from_state"); got != string(model.SettlementStatusPending) {
		t.Fatalf("from_state = %q, want pending", got)
	}
	if got := opString(t, e, "to_state"); got != string(model.SettlementStatusRejected) {
		t.Fatalf("to_state = %q, want rejected", got)
	}
	if repo.id != "s1" || repo.reason != "скриншоты не показывают территорию" {
		t.Fatalf("repository got (%q, %q), want the request the record names", repo.id, repo.reason)
	}
}

// A first submission has no request id yet — the repository mints it — so the
// record names the actor and the kind, and there is exactly one of them.
func TestSubmitEmitsOneRequestSubmittedRecord(t *testing.T) {
	svc, logs := newOpService(t, &notifierRepo{})

	if _, err := svc.Submit(asUser("founder"), submitRequest(10, 20)); err != nil {
		t.Fatal(err)
	}

	if logs.Len() != 1 {
		t.Fatalf("records = %d, want exactly the submission record: %v", logs.Len(), logs.All())
	}
	e := opEntryFor(t, logs, "settlement.request.submitted")
	if e.Level != zapcore.InfoLevel {
		t.Fatalf("level = %s, want Info", e.Level)
	}
	if got := opString(t, e, "actor_id"); got != "founder" {
		t.Fatalf("actor_id = %q, want founder", got)
	}
	if got := opString(t, e, "kind"); got != string(model.KindSettlement) {
		t.Fatalf("kind = %q, want settlement", got)
	}
	if got := opString(t, e, "to_state"); got != string(model.SettlementStatusPending) {
		t.Fatalf("to_state = %q, want pending", got)
	}
	if opFieldPresent(e, "entity_id") {
		t.Fatal("entity_id on a first submission: the repository has not minted the request id yet")
	}
}

// An approved request that is submitted again moves up a tier: two facts, so two
// records — the resubmission and the level up — and the level up is written only
// after the request landed.
func TestSubmitLevelUpEmitsTheResubmissionAndTheLevelUp(t *testing.T) {
	repo := &notifierRepo{
		request: &model.SettlementVerification{
			Id:     "s1",
			Status: model.SettlementStatusApproved,
			Type:   model.SettlementTypeCamp,
		},
	}
	svc, logs := newOpService(t, repo)

	if _, err := svc.Submit(asUser("founder"), submitRequest(10, 20)); err != nil {
		t.Fatal(err)
	}
	if repo.updatedReq == nil {
		t.Fatal("the request was never written, so the level up must not be recorded")
	}

	if logs.Len() != 2 {
		t.Fatalf("records = %d, want the resubmission and the level up: %v", logs.Len(), logs.All())
	}

	submitted := opEntryFor(t, logs, "settlement.request.submitted")
	if got := opString(t, submitted, "entity_id"); got != "s1" {
		t.Fatalf("entity_id = %q, want s1", got)
	}
	if got := opString(t, submitted, "from_state"); got != string(model.SettlementStatusApproved) {
		t.Fatalf("from_state = %q, want approved", got)
	}

	levelUp := opEntryFor(t, logs, "settlement.level_up")
	if got := opString(t, levelUp, "entity_id"); got != "s1" {
		t.Fatalf("level up entity_id = %q, want s1", got)
	}
	if got := opString(t, levelUp, "from_state"); got != string(model.SettlementTypeCamp) {
		t.Fatalf("level up from_state = %q, want camp", got)
	}
	if got := opString(t, levelUp, "to_state"); got != string(model.SettlementTypeVillage) {
		t.Fatalf("level up to_state = %q, want village", got)
	}
	if got := opString(t, levelUp, "actor_id"); got != "founder" {
		t.Fatalf("level up actor_id = %q, want founder", got)
	}
}

// An approval records the settlement it created, the moderator who approved it
// and the status transition, once the transaction has committed.
func TestApproveEmitsOneRequestApprovedRecord(t *testing.T) {
	repo := &notifierRepo{
		request: &model.SettlementVerification{
			Id:        "s1",
			Status:    model.SettlementStatusPending,
			UpdatedAt: submittedAt,
		},
		approval: &service.ApprovalResult{
			Created:     true,
			Settlement:  *newSet(model.SettlementTypeCamp),
			RequestedAt: submittedAt,
		},
	}
	svc, logs := newOpService(t, repo)

	if _, err := svc.Approve(asUser("mod1"), &settlementv1.ApproveRequest{Id: "s1"}); err != nil {
		t.Fatal(err)
	}
	if !repo.txCommitted {
		t.Fatal("the transaction did not commit, so the approval must not be recorded")
	}

	e := opEntryFor(t, logs, "settlement.request.approved")
	if e.Level != zapcore.InfoLevel {
		t.Fatalf("level = %s, want Info", e.Level)
	}
	if got := opString(t, e, "entity"); got != "settlement" {
		t.Fatalf("entity = %q, want settlement", got)
	}
	if got := opString(t, e, "entity_id"); got != "s1" {
		t.Fatalf("entity_id = %q, want s1", got)
	}
	if got := opString(t, e, "actor_id"); got != "mod1" {
		t.Fatalf("actor_id = %q, want mod1", got)
	}
	if got := opString(t, e, "to_state"); got != string(model.SettlementStatusApproved) {
		t.Fatalf("to_state = %q, want approved", got)
	}
	if got := opString(t, e, "leader_id"); got != "owner1" {
		t.Fatalf("leader_id = %q, want owner1", got)
	}
}

// A solo guild that its last owner disbands records the end of the group, with
// the owner as the actor.
func TestLeaveSoloGuildEmitsOneDisbandRecord(t *testing.T) {
	svc, logs := newOpService(t, &notifierRepo{set: soloGuild()})

	res, err := svc.LeaveSettlement(asUser("owner1"), &settlementv1.LeaveSettlementRequest{
		SettlementId: "g1",
		Disband:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.GetDisbanded() {
		t.Fatal("disbanded = false, want the guild gone")
	}

	e := opEntryFor(t, logs, "settlement.disbanded")
	if e.Level != zapcore.InfoLevel {
		t.Fatalf("level = %s, want Info", e.Level)
	}
	if got := opString(t, e, "entity_id"); got != "g1" {
		t.Fatalf("entity_id = %q, want g1", got)
	}
	if got := opString(t, e, "actor_id"); got != "owner1" {
		t.Fatalf("actor_id = %q, want owner1", got)
	}
	if got := opString(t, e, "reason"); got == "" {
		t.Fatal("reason is empty, want why the group ended")
	}
	// The disband replaces the leave: one fact, one record.
	if logs.Len() != 1 {
		t.Fatalf("records = %d, want only the disband record: %v", logs.Len(), logs.All())
	}
}

// A leave without the disband confirmation changes nothing, so the record is a
// Warn that carries the rule, not an Error.
func TestLeaveSoloGuildWithoutConfirmationEmitsARefusalWarn(t *testing.T) {
	svc, logs := newOpService(t, &notifierRepo{set: soloGuild()})

	_, err := svc.LeaveSettlement(asUser("owner1"), &settlementv1.LeaveSettlementRequest{SettlementId: "g1"})
	if err == nil {
		t.Fatal("LeaveSettlement: got nil error, want the disband requirement")
	}

	e := opEntryFor(t, logs, "settlement.leave.refused")
	if e.Level != zapcore.WarnLevel {
		t.Fatalf("level = %s, want Warn", e.Level)
	}
	if got := opString(t, e, "entity_id"); got != "g1" {
		t.Fatalf("entity_id = %q, want g1", got)
	}
	if got := opString(t, e, "reason"); got == "" {
		t.Fatal("reason is empty, want the missing confirmation")
	}
	// A rule that denied the action is not a failure, so it carries a reason and
	// no error.
	if opFieldPresent(e, "error") {
		t.Fatal("error field on a refusal record, want the reason alone")
	}
	if logs.Len() != 1 {
		t.Fatalf("records = %d, want only the refusal: %v", logs.Len(), logs.All())
	}
}

// An eviction the caller has no right to is a Warn that names the settlement,
// the member it targeted and the rule that denied it.
func TestRemoveMemberDeniedEmitsARefusalWarn(t *testing.T) {
	svc, logs := newOpService(t, &notifierRepo{set: newGuild()})

	_, err := svc.RemoveMember(asUser("resident"), &settlementv1.RemoveMemberRequest{
		SettlementId: "g1",
		UserId:       "owner1",
	})
	if err == nil {
		t.Fatal("RemoveMember: got nil error, want a permission denial")
	}

	e := opEntryFor(t, logs, "settlement.member.removal.refused")
	if e.Level != zapcore.WarnLevel {
		t.Fatalf("level = %s, want Warn", e.Level)
	}
	if got := opString(t, e, "entity_id"); got != "g1" {
		t.Fatalf("entity_id = %q, want g1", got)
	}
	if got := opString(t, e, "actor_id"); got != "resident" {
		t.Fatalf("actor_id = %q, want the caller resident", got)
	}
	if got := opString(t, e, "member_id"); got != "owner1" {
		t.Fatalf("member_id = %q, want the eviction target owner1", got)
	}
	if got := opString(t, e, "reason"); got == "" {
		t.Fatal("reason is empty, want the rule that denied the removal")
	}
}
