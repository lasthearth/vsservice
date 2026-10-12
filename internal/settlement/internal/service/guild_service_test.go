package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/mail/mailcompose"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service"
	"github.com/lasthearth/vsservice/internal/settlement/model"
)

// isOwner reads the owner role from the members, which is where the model keeps it. The deprecated
// leader field is derived from the first owner on read by the repository, and the fake does not
// derive it, so a test that asserts on leader would assert on a stale copy.
func isOwner(set *settlementv1.Settlement, userID string) bool {
	for _, m := range set.GetMembers() {
		if m.GetUserId() == userID {
			return slices.Contains(m.GetRoleIds(), model.OwnerRoleId)
		}
	}
	return false
}

// newGuild is a guild that has only its founder, the way a creation leaves it.
func newGuild() *model.Settlement {
	owner := model.Member{UserId: "owner1", RoleIds: []string{model.OwnerRoleId}}
	return &model.Settlement{
		Id:      "g1",
		Name:    "Северный Ветер",
		Type:    model.SettlementTypeGuild,
		Leader:  owner,
		Members: []model.Member{owner, {UserId: "resident", RoleIds: []string{}}},
	}
}

// soloGuild is a guild whose founder is its only member: nobody to transfer the owner role to.
func soloGuild() *model.Settlement {
	owner := model.Member{UserId: "owner1", RoleIds: []string{model.OwnerRoleId}}
	return &model.Settlement{
		Id:      "g1",
		Name:    "Северный Ветер",
		Type:    model.SettlementTypeGuild,
		Leader:  owner,
		Members: []model.Member{owner},
	}
}

// guildSubmitRequest is a first guild submission. A guild takes the same fields as a camp and goes
// through the same moderation, so only the type tells the two apart.
func guildSubmitRequest() *settlementv1.SubmitRequest {
	return &settlementv1.SubmitRequest{
		Type:        settlementv1.SubmitRequest_GUILD,
		Name:        "Северный Ветер",
		Description: "Гильдия северных торговцев",
		Diplomacy:   "Нейтралитет",
		Coordinates: &settlementv1.Vector2{X: 10, Y: 20},
		Attachments: []*settlementv1.SubmitRequest_SubmitAttachment{{Url: "https://cdn.test/a.png", Description: "d"}},
	}
}

// A guild takes the same fields as a first settlement and is stored as its own request, so a player
// who holds nothing may found one.
func TestSubmitGuildCreatesItsOwnRequest(t *testing.T) {
	repo := &notifierRepo{}
	svc, _, _ := newNotifierService(t, repo)

	if _, err := svc.Submit(asUser("founder"), guildSubmitRequest()); err != nil {
		t.Fatal(err)
	}

	if repo.createdReq == nil {
		t.Fatal("no request was created")
	}
	if repo.createdReq.Type != model.SettlementTypeGuild {
		t.Errorf("request type = %q, want guild", repo.createdReq.Type)
	}
	// One read of what the player holds answers both rules the kind asks.
	if repo.membershipReads != 1 {
		t.Errorf("membership reads = %d, want 1", repo.membershipReads)
	}
}

// The owner of a guild belongs to no settlement, so a settlement member cannot found one: leaving
// the settlement comes first.
func TestSubmitGuildRefusesASettlementMember(t *testing.T) {
	repo := &notifierRepo{memberships: model.Memberships{InSettlement: true}}
	svc, _, _ := newNotifierService(t, repo)

	_, err := svc.Submit(asUser("resident"), guildSubmitRequest())
	if !errors.Is(err, ierror.ErrGuildOwnerInSettlement) {
		t.Fatalf("err = %v, want ErrGuildOwnerInSettlement", err)
	}
	if repo.createdReq != nil {
		t.Error("a request was stored for a submission that was refused")
	}
	if len(repo.membershipReadsFor) != 1 || repo.membershipReadsFor[0] != "resident" {
		t.Errorf("memberships read for %v, want the caller resident", repo.membershipReadsFor)
	}
}

// One guild per player, so a guild member cannot found a second one.
func TestSubmitGuildRefusesAMemberOfAnotherGuild(t *testing.T) {
	repo := &notifierRepo{memberships: model.Memberships{InGuild: true}}
	svc, _, _ := newNotifierService(t, repo)

	_, err := svc.Submit(asUser("resident"), guildSubmitRequest())
	if !errors.Is(err, ierror.ErrAlreadyInGuild) {
		t.Fatalf("err = %v, want ErrAlreadyInGuild", err)
	}
	if repo.createdReq != nil {
		t.Error("a request was stored for a submission that was refused")
	}
}

// A guild member may found a settlement: only guild ownership binds, not guild membership.
func TestSubmitSettlementAllowsAGuildMember(t *testing.T) {
	repo := &notifierRepo{memberships: model.Memberships{InGuild: true}}
	svc, _, _ := newNotifierService(t, repo)

	if _, err := svc.Submit(asUser("resident"), submitRequest(10, 20)); err != nil {
		t.Fatal(err)
	}
	if repo.createdReq == nil || repo.createdReq.Type != model.SettlementTypeCamp {
		t.Fatalf("created request = %+v, want a camp", repo.createdReq)
	}
}

// A guild owner cannot found a settlement, and a settlement member cannot found a second one. Both
// used to be swallowed here and to fail later, at the approval, on the unique index.
func TestSubmitSettlementRefusesAGuildOwnerAndAMember(t *testing.T) {
	cases := []struct {
		name        string
		memberships model.Memberships
		wantErr     error
	}{
		{"guild owner", model.Memberships{InGuild: true, OwnsGuild: true}, ierror.ErrGuildOwnerInSettlement},
		{"settlement member", model.Memberships{InSettlement: true}, ierror.ErrAlreadyMember},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &notifierRepo{memberships: tc.memberships}
			svc, _, _ := newNotifierService(t, repo)

			_, err := svc.Submit(asUser("resident"), submitRequest(10, 20))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if repo.createdReq != nil {
				t.Error("a request was stored for a submission that was refused")
			}
		})
	}
}

// Each kind keeps its own request, so a pending settlement request neither hides nor blocks a guild
// submission. Before the split, one request per leader meant the guild submission was answered with
// "already pending" or, worse, levelled the settlement up.
func TestSubmitGuildIsIndependentOfAPendingSettlementRequest(t *testing.T) {
	repo := &notifierRepo{request: &model.SettlementVerification{
		Id: "s1", Status: model.SettlementStatusPending, Type: model.SettlementTypeCamp,
	}}
	svc, _, _ := newNotifierService(t, repo)

	if _, err := svc.Submit(asUser("founder"), guildSubmitRequest()); err != nil {
		t.Fatal(err)
	}
	if repo.createdReq == nil || repo.createdReq.Type != model.SettlementTypeGuild {
		t.Fatalf("created request = %+v, want a guild", repo.createdReq)
	}
	if repo.updatedReq != nil {
		t.Error("the pending settlement request was rewritten by a guild submission")
	}
}

// Approving a guild mails its founder the guild herald, in the same transaction as the group it
// creates, exactly like a settlement gets its tier block.
func TestApproveGuildMailsTheGuildHerald(t *testing.T) {
	guild := newGuild()
	repo := &notifierRepo{
		request: &model.SettlementVerification{
			Id: "g1", Status: model.SettlementStatusPending, Type: model.SettlementTypeGuild,
			Leader: model.Member{UserId: "owner1"}, UpdatedAt: submittedAt,
		},
		approval: &service.ApprovalResult{Created: true, Settlement: *guild, RequestedAt: submittedAt},
	}
	svc, mail, notices := newNotifierService(t, repo)

	if _, err := svc.Approve(context.Background(), &settlementv1.ApproveRequest{Id: "g1"}); err != nil {
		t.Fatal(err)
	}

	if len(mail.sent) != 1 {
		t.Fatalf("mails = %+v, want exactly the block mail", mail.sent)
	}
	m := mail.sent[0]
	if m.recipient != "owner1" || m.sender != mailcompose.SenderSettlement {
		t.Errorf("mail = %+v, want the guild herald to owner1", m)
	}
	if len(m.items) != 1 || m.items[0].GameCode != "lhgui:guild-notifier" ||
		m.items[0].Type != model.NotifierBlockKind || m.items[0].Quantity != 1 {
		t.Errorf("attachment = %+v, want one lhgui:guild-notifier block", m.items)
	}
	if !m.inTx || !repo.txCommitted {
		t.Errorf("mail written in transaction = %v, committed = %v; want both true", m.inTx, repo.txCommitted)
	}
	if len(notices.notified()) != 0 {
		t.Errorf("creation must not send site notifications, got %v", notices.notified())
	}
}

// The rules are read again at the approval, inside the transaction: the author may have joined a
// settlement in the hours between the submission and the moderation decision.
func TestApproveGuildRefusesAnAuthorWhoJoinedASettlement(t *testing.T) {
	repo := &notifierRepo{
		request: &model.SettlementVerification{
			Id: "g1", Status: model.SettlementStatusPending, Type: model.SettlementTypeGuild,
			Leader: model.Member{UserId: "owner1"},
		},
		memberships: model.Memberships{InSettlement: true},
		approval:    &service.ApprovalResult{Created: true, Settlement: *newGuild()},
	}
	svc, mail, _ := newNotifierService(t, repo)

	_, err := svc.Approve(context.Background(), &settlementv1.ApproveRequest{Id: "g1"})
	if !errors.Is(err, ierror.ErrGuildOwnerInSettlement) {
		t.Fatalf("err = %v, want ErrGuildOwnerInSettlement", err)
	}
	if repo.approved {
		t.Error("the request was approved although its author lives in a settlement")
	}
	if len(mail.sent) != 0 {
		t.Errorf("mails = %+v, want none", mail.sent)
	}
	if repo.txCommitted {
		t.Error("transaction committed although the approval was refused")
	}
	// The rules are read for the author of the request, who is not the admin that approves it.
	if len(repo.membershipReadsFor) != 1 || repo.membershipReadsFor[0] != "owner1" {
		t.Errorf("memberships read for %v, want the author owner1", repo.membershipReadsFor)
	}
}

// The same rule from the other side: a guild owner cannot be approved into a settlement they found.
func TestApproveSettlementRefusesAnAuthorWhoOwnsAGuild(t *testing.T) {
	repo := &notifierRepo{
		request: &model.SettlementVerification{
			Id: "s1", Status: model.SettlementStatusPending, Type: model.SettlementTypeCamp,
			Leader: model.Member{UserId: "owner1"},
		},
		memberships: model.Memberships{InGuild: true, OwnsGuild: true},
		approval:    &service.ApprovalResult{Created: true, Settlement: *newSet(model.SettlementTypeCamp)},
	}
	svc, _, _ := newNotifierService(t, repo)

	_, err := svc.Approve(context.Background(), &settlementv1.ApproveRequest{Id: "s1"})
	if !errors.Is(err, ierror.ErrGuildOwnerInSettlement) {
		t.Fatalf("err = %v, want ErrGuildOwnerInSettlement", err)
	}
	if repo.approved {
		t.Error("the request was approved although its author owns a guild")
	}
}

// An upgrade changes no membership, so it asks no membership question: the author already belongs to
// the group it raises, and refusing them would make every upgrade impossible.
func TestApproveUpgradeSkipsTheMembershipRules(t *testing.T) {
	set := memberSet(model.SettlementTypeCamp)
	repo := &notifierRepo{
		set: set,
		request: &model.SettlementVerification{
			Id: "s1", Status: model.SettlementStatusPending, Type: model.SettlementTypeVillage,
			Leader: model.Member{UserId: "owner1"},
		},
		// A settled player: a member of the settlement being raised.
		memberships: model.Memberships{InSettlement: true},
		approval: &service.ApprovalResult{
			Settlement:   *memberSet(model.SettlementTypeVillage),
			PreviousType: model.SettlementTypeCamp,
			RequestedAt:  submittedAt,
		},
	}
	svc, _, _ := newNotifierService(t, repo)

	if _, err := svc.Approve(context.Background(), &settlementv1.ApproveRequest{Id: "s1"}); err != nil {
		t.Fatal(err)
	}
	if repo.membershipReads != 0 || len(repo.joinChecks) != 0 || repo.ownGuildChecks != 0 {
		t.Errorf("membership checks on an upgrade = %d reads, %v joins, %d ownership; want none",
			repo.membershipReads, repo.joinChecks, repo.ownGuildChecks)
	}
}

// The public list answers one kind, and an unspecified kind keeps answering settlements so no guild
// appears on the map.
func TestListFiltersByKind(t *testing.T) {
	repo := &notifierRepo{
		set:    memberSet(model.SettlementTypeCamp),
		others: map[string]*model.Settlement{"g1": newGuild()},
	}
	svc, _, _ := newNotifierService(t, repo)

	res, err := svc.List(context.Background(), &settlementv1.ListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetSettlements()) != 1 || res.GetSettlements()[0].GetId() != "s1" {
		t.Fatalf("unspecified kind listed %+v, want the settlement only", res.GetSettlements())
	}

	res, err = svc.List(context.Background(), &settlementv1.ListRequest{
		Kind: settlementv1.GroupKind_GROUP_KIND_GUILD,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetSettlements()) != 1 || res.GetSettlements()[0].GetId() != "g1" {
		t.Fatalf("guild kind listed %+v, want the guild only", res.GetSettlements())
	}
	if got := res.GetSettlements()[0].GetType(); got != settlementv1.SettlementType_GUILD {
		t.Errorf("listed type = %v, want GUILD", got)
	}

	res, err = svc.List(context.Background(), &settlementv1.ListRequest{
		Kind: settlementv1.GroupKind_GROUP_KIND_SETTLEMENT,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetSettlements()) != 1 || res.GetSettlements()[0].GetId() != "s1" {
		t.Fatalf("settlement kind listed %+v, want the settlement only", res.GetSettlements())
	}
}

// The moderation queue must never hide a guild request, so an unspecified kind lists both.
func TestListPendingDefaultsToBothKinds(t *testing.T) {
	repo := &notifierRepo{pending: []model.SettlementVerification{
		{Id: "s1", Status: model.SettlementStatusPending, Type: model.SettlementTypeCamp},
		{Id: "g1", Status: model.SettlementStatusPending, Type: model.SettlementTypeGuild},
	}}
	svc, _, _ := newNotifierService(t, repo)

	res, err := svc.ListPending(context.Background(), &settlementv1.ListPendingRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetSettlements()) != 2 {
		t.Fatalf("unspecified kind listed %d requests, want both kinds", len(res.GetSettlements()))
	}

	res, err = svc.ListPending(context.Background(), &settlementv1.ListPendingRequest{
		Kind: settlementv1.GroupKind_GROUP_KIND_GUILD,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetSettlements()) != 1 || res.GetSettlements()[0].GetId() != "g1" {
		t.Fatalf("guild kind listed %+v, want the guild request only", res.GetSettlements())
	}
}

// A player who holds both gets both from one call, which is what the site renders the profile from.
func TestGetUserMembershipsReturnsBothGroups(t *testing.T) {
	repo := &notifierRepo{
		set:    memberSet(model.SettlementTypeCamp),
		others: map[string]*model.Settlement{"g1": newGuild()},
	}
	svc, _, _ := newNotifierService(t, repo)

	res, err := svc.GetUserMemberships(context.Background(), &settlementv1.GetUserMembershipsRequest{UserId: "resident"})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetSettlement().GetId() != "s1" {
		t.Errorf("settlement = %+v, want s1", res.GetSettlement())
	}
	if res.GetGuild().GetId() != "g1" {
		t.Errorf("guild = %+v, want g1", res.GetGuild())
	}
	// The guild pays no notifier read: its herald carries no settlement binding and writes no
	// placement, so there is nothing to fill notifier_position from.
	if repo.notifierReads != 1 {
		t.Errorf("notifier reads = %d, want one for the settlement only", repo.notifierReads)
	}
}

// A guild founder holds no settlement, and that is a normal answer rather than an error: both
// fields stay set or unset, and the call succeeds.
func TestGetUserMembershipsForAGuildFounder(t *testing.T) {
	repo := &notifierRepo{others: map[string]*model.Settlement{"g1": newGuild()}}
	svc, _, _ := newNotifierService(t, repo)

	res, err := svc.GetUserMemberships(context.Background(), &settlementv1.GetUserMembershipsRequest{UserId: "owner1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetSettlement() != nil {
		t.Errorf("settlement = %+v, want unset for a guild founder", res.GetSettlement())
	}
	if res.GetGuild().GetId() != "g1" {
		t.Errorf("guild = %+v, want g1", res.GetGuild())
	}
	if repo.notifierReads != 0 {
		t.Errorf("notifier reads = %d, want none", repo.notifierReads)
	}
}

// Get answers a guild without paying the notifier read and leaves notifier_position unset, exactly as
// GetUserMemberships does: one place decides it, so the two rpcs cannot disagree even when a
// placement happens to stand under the guild's id.
func TestGetGuildSkipsTheNotifierRead(t *testing.T) {
	repo := &notifierRepo{
		set:       newGuild(),
		placement: &model.NotifierPlacement{SettlementId: "g1", Position: model.Vector3{X: 1, Y: 2, Z: 3}},
	}
	svc, _, _ := newNotifierService(t, repo)

	res, err := svc.Get(context.Background(), &settlementv1.GetRequest{Id: "g1"})
	if err != nil {
		t.Fatal(err)
	}
	if repo.notifierReads != 0 {
		t.Errorf("notifier reads = %d, want none for a guild", repo.notifierReads)
	}
	if res.GetSettlement().GetNotifierPosition() != nil {
		t.Errorf("notifier_position = %v, want unset for a guild", res.GetSettlement().GetNotifierPosition())
	}
	if res.GetSettlement().GetType() != settlementv1.SettlementType_GUILD {
		t.Errorf("type = %v, want GUILD", res.GetSettlement().GetType())
	}
}

// GetByUserId answers the settlement only, so a guild never reaches a caller that expects one group
// on the map.
func TestGetByUserIdSkipsTheGuild(t *testing.T) {
	repo := &notifierRepo{others: map[string]*model.Settlement{"g1": newGuild()}}
	svc, _, _ := newNotifierService(t, repo)

	_, err := svc.GetByUserId(context.Background(), &settlementv1.GetByUserIdRequest{UserId: "owner1"})
	if !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound for a player who holds a guild only", err)
	}
}

// VerificationStatus answers one kind of request, and an unspecified kind keeps answering the
// settlement one.
func TestVerificationStatusReadsTheKindOfRequest(t *testing.T) {
	repo := &notifierRepo{request: &model.SettlementVerification{
		Id: "g1", Status: model.SettlementStatusPending, Type: model.SettlementTypeGuild,
	}}
	svc, _, _ := newNotifierService(t, repo)

	_, err := svc.VerificationStatus(context.Background(), &settlementv1.VerificationStatusRequest{UserId: "owner1"})
	if !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("unspecified kind: err = %v, want ErrNotFound while only a guild request exists", err)
	}

	res, err := svc.VerificationStatus(context.Background(), &settlementv1.VerificationStatusRequest{
		UserId: "owner1",
		Kind:   settlementv1.GroupKind_GROUP_KIND_GUILD,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetStatus() != string(model.SettlementStatusPending) {
		t.Errorf("status = %q, want pending", res.GetStatus())
	}
}

// Ownership of a guild cannot move to a member who lives in a settlement: the target would break the
// rule the moment the role moved. The check must read the TARGET and not the caller — both are
// members of this guild here, so a guard pointed at the wrong id would pass unnoticed.
func TestTransferGuildOwnershipRefusesAMemberOfASettlement(t *testing.T) {
	repo := &notifierRepo{
		set: newGuild(),
		// The caller owns the guild and holds no settlement; the target lives in one.
		memberships: model.Memberships{InGuild: true, OwnsGuild: true},
		membershipsOf: map[string]model.Memberships{
			"resident": {InGuild: true, InSettlement: true},
		},
	}
	svc, _, _ := newNotifierService(t, repo)

	_, err := svc.TransferOwnership(asUser("owner1"), &settlementv1.TransferOwnershipRequest{
		SettlementId: "g1",
		ToUserId:     "resident",
	})
	if !errors.Is(err, ierror.ErrGuildOwnerInSettlement) {
		t.Fatalf("err = %v, want ErrGuildOwnerInSettlement", err)
	}
	if len(repo.ownGuildChecksFor) != 1 || repo.ownGuildChecksFor[0] != "resident" {
		t.Errorf("ownership checked for %v, want the target resident", repo.ownGuildChecksFor)
	}
}

// The same transfer to a member who holds no settlement goes through.
func TestTransferGuildOwnershipToAFreeMemberSucceeds(t *testing.T) {
	guild := newGuild()
	guild.AddMember("builder")
	repo := &notifierRepo{
		set:         guild,
		memberships: model.Memberships{InGuild: true, OwnsGuild: true},
		membershipsOf: map[string]model.Memberships{
			"resident": {InGuild: true, InSettlement: true},
			"builder":  {InGuild: true},
		},
	}
	svc, _, _ := newNotifierService(t, repo)

	res, err := svc.TransferOwnership(asUser("owner1"), &settlementv1.TransferOwnershipRequest{
		SettlementId: "g1",
		ToUserId:     "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !isOwner(res.GetSettlement(), "builder") || isOwner(res.GetSettlement(), "owner1") {
		t.Errorf("members = %+v, want builder to hold the owner role alone", res.GetSettlement().GetMembers())
	}
}

// Granting the owner role of a guild answers the same rule as taking it, and reads the member the
// role is granted to.
func TestAddOwnerOnAGuildRefusesAMemberOfASettlement(t *testing.T) {
	repo := &notifierRepo{
		set: newGuild(),
		membershipsOf: map[string]model.Memberships{
			"resident": {InGuild: true, InSettlement: true},
		},
	}
	svc, _, _ := newNotifierService(t, repo)

	_, err := svc.AddOwner(context.Background(), &settlementv1.AddOwnerRequest{
		SettlementId: "g1",
		UserId:       "resident",
	})
	if !errors.Is(err, ierror.ErrGuildOwnerInSettlement) {
		t.Fatalf("err = %v, want ErrGuildOwnerInSettlement", err)
	}
	if len(repo.ownGuildChecksFor) != 1 || repo.ownGuildChecksFor[0] != "resident" {
		t.Errorf("ownership checked for %v, want the target resident", repo.ownGuildChecksFor)
	}
}

// A free member of the guild is granted the role, and a settlement asks no such question at all: its
// owners are already its members, so the rule has nothing to add there.
func TestAddOwnerGrantsAFreeGuildMemberAndSkipsSettlements(t *testing.T) {
	guild := newGuild()
	guild.AddMember("builder")
	repo := &notifierRepo{set: guild}
	svc, _, _ := newNotifierService(t, repo)

	res, err := svc.AddOwner(context.Background(), &settlementv1.AddOwnerRequest{
		SettlementId: "g1",
		UserId:       "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !isOwner(res.GetSettlement(), "builder") {
		t.Errorf("members = %+v, want builder granted the owner role", res.GetSettlement().GetMembers())
	}
	if repo.ownGuildChecks != 1 {
		t.Errorf("ownership checks = %d, want one for the guild", repo.ownGuildChecks)
	}

	repo.set = memberSet(model.SettlementTypeCamp)
	if _, err := svc.AddOwner(context.Background(), &settlementv1.AddOwnerRequest{
		SettlementId: "s1",
		UserId:       "builder",
	}); err != nil {
		t.Fatal(err)
	}
	if repo.ownGuildChecks != 1 {
		t.Errorf("ownership checks = %d, want no further check for a settlement", repo.ownGuildChecks)
	}
}

// The same transfer inside a settlement is untouched by the guild rule.
func TestTransferSettlementOwnershipIsUnaffected(t *testing.T) {
	repo := &notifierRepo{
		set:         memberSet(model.SettlementTypeCamp),
		memberships: model.Memberships{InSettlement: true, InGuild: true},
	}
	svc, _, _ := newNotifierService(t, repo)

	res, err := svc.TransferOwnership(asUser("owner1"), &settlementv1.TransferOwnershipRequest{
		SettlementId: "s1",
		// A plain member: owner2 already owns, and the model refuses a second owner role here.
		ToUserId: "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetSettlement().GetMembers()) != 4 {
		t.Errorf("members = %+v, want the four the settlement held", res.GetSettlement().GetMembers())
	}
	if repo.ownGuildChecks != 0 {
		t.Errorf("guild-ownership checks = %d, want none for a settlement", repo.ownGuildChecks)
	}
}

// The founder of a guild who is its only member cannot leave it and has nobody to transfer it to, so
// the leave disbands the guild — but only on an explicit confirmation, because the guild and
// everything that points at it go with it.
func TestLeaveSoloGuildNeedsTheDisbandConfirmation(t *testing.T) {
	repo := &notifierRepo{set: soloGuild()}
	svc, _, _ := newNotifierService(t, repo)

	_, err := svc.LeaveSettlement(asUser("owner1"), &settlementv1.LeaveSettlementRequest{SettlementId: "g1"})
	if !errors.Is(err, ierror.ErrDisbandRequired) {
		t.Fatalf("err = %v, want ErrDisbandRequired", err)
	}
	if len(repo.disbanded) != 0 {
		t.Errorf("disbanded %v without a confirmation, want nothing", repo.disbanded)
	}

	res, err := svc.LeaveSettlement(asUser("owner1"), &settlementv1.LeaveSettlementRequest{
		SettlementId: "g1",
		Disband:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.GetDisbanded() {
		t.Error("disbanded = false, want true")
	}
	if len(repo.disbanded) != 1 || repo.disbanded[0] != "g1/owner1" {
		t.Errorf("disbanded = %v, want the guild g1 and its last owner", repo.disbanded)
	}
}

// A guild with other members is never disbanded: its owner role has somewhere to go, and deleting the
// guild would throw those members out with it.
func TestLeaveGuildWithMembersRefusesTheLastOwner(t *testing.T) {
	repo := &notifierRepo{set: newGuild()}
	svc, _, _ := newNotifierService(t, repo)

	_, err := svc.LeaveSettlement(asUser("owner1"), &settlementv1.LeaveSettlementRequest{
		SettlementId: "g1",
		Disband:      true,
	})
	if !errors.Is(err, ierror.ErrLastOwner) {
		t.Fatalf("err = %v, want ErrLastOwner", err)
	}
	if len(repo.disbanded) != 0 {
		t.Errorf("disbanded %v, want nothing while the guild has another member", repo.disbanded)
	}
}

// A plain member of a guild leaves it the ordinary way and stays a member of nothing else.
func TestGuildMemberLeavesWithoutDisbanding(t *testing.T) {
	repo := &notifierRepo{set: newGuild()}
	svc, _, _ := newNotifierService(t, repo)

	res, err := svc.LeaveSettlement(asUser("resident"), &settlementv1.LeaveSettlementRequest{SettlementId: "g1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetDisbanded() {
		t.Error("disbanded = true for a plain member's leave")
	}
	if len(repo.disbanded) != 0 {
		t.Errorf("disbanded = %v, want nothing", repo.disbanded)
	}
	if got := len(repo.set.Members); got != 1 {
		t.Errorf("members = %d, want the owner alone after the leave", got)
	}
}

// Disbanding is a guild rule. The last owner of a settlement still cannot leave, confirmation or
// not: a settlement stands on the map, and an admin deletes it.
func TestLeaveSettlementNeverDisbands(t *testing.T) {
	repo := &notifierRepo{set: newSet(model.SettlementTypeCamp)}
	svc, _, _ := newNotifierService(t, repo)

	_, err := svc.LeaveSettlement(asUser("owner1"), &settlementv1.LeaveSettlementRequest{
		SettlementId: "s1",
		Disband:      true,
	})
	if !errors.Is(err, ierror.ErrLastOwner) {
		t.Fatalf("err = %v, want ErrLastOwner", err)
	}
	if len(repo.disbanded) != 0 {
		t.Errorf("disbanded %v, want nothing for a settlement", repo.disbanded)
	}
}

// The delete itself is conditional in the repository, so a player who joined since the service read
// the guild turns the disband into an answer instead of deleting them too.
func TestLeaveSoloGuildAnswersTheConflictFromTheRepository(t *testing.T) {
	repo := &notifierRepo{set: soloGuild(), disbandErr: ierror.ErrDisbandConflict}
	svc, _, _ := newNotifierService(t, repo)

	_, err := svc.LeaveSettlement(asUser("owner1"), &settlementv1.LeaveSettlementRequest{
		SettlementId: "g1",
		Disband:      true,
	})
	if !errors.Is(err, ierror.ErrDisbandConflict) {
		t.Fatalf("err = %v, want ErrDisbandConflict", err)
	}
}
