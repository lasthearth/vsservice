package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.uber.org/zap"
)

// Event keys of settlement's domain-operation records (the plan's L2 layer).
// One key names one fact and never changes: dashboards and alerts filter on it,
// never on the message.
//
// The shape is <domain>.<entity>.<action>. A refusal — a rule that denied the
// action, so nothing changed — takes its own ".refused" key at Warn with Reason
// set, because a refusal is a different fact from the action it blocked. A
// failed write reuses the action key at Error with the cause attached, the way
// grpc.call.finish carries its code.
const (
	// The verification request: submitted, approved, rejected, and moved up a
	// tier by a resubmission. The request id is also the settlement id.
	opRequestSubmitted      = "settlement.request.submitted"
	opRequestSubmissionDeny = "settlement.request.submission.refused"
	opRequestApproved       = "settlement.request.approved"
	opRequestApprovalDeny   = "settlement.request.approval.refused"
	opRequestRejected       = "settlement.request.rejected"
	opLevelUp               = "settlement.level_up"

	// Membership: who joined, who was removed, who left, and which group ended
	// because its last owner left.
	opMemberJoined      = "settlement.member.joined"
	opMemberLeft        = "settlement.member.left"
	opMemberRemoved     = "settlement.member.removed"
	opMemberRemovalDeny = "settlement.member.removal.refused"
	opDisbanded         = "settlement.disbanded"
	opLeaveDeny         = "settlement.leave.refused"

	// Invitations, direct and by link. A link's code is the credential that
	// lets a player in, so the records name the link id and never the code.
	opInvitationSent     = "settlement.invitation.sent"
	opInvitationDeny     = "settlement.invitation.refused"
	opInvitationAccepted = "settlement.invitation.accepted"
	opInvitationRejected = "settlement.invitation.rejected"
	opInvitationRevoked  = "settlement.invitation.revoked"

	// Imperial favor is a balance an admin moves, so every change carries the
	// amount and the balance it left behind.
	opFavorGranted       = "settlement.favor.granted"
	opFavorDeducted      = "settlement.favor.deducted"
	opFavorDeductRefused = "settlement.favor.deduction.refused"
	opFavorTransferred   = "settlement.favor.transferred"

	// Control of the group: ownership moving between members, and an admin
	// deleting the group outright.
	opOwnershipTransferred = "settlement.ownership.transferred"
	opDeleted              = "settlement.deleted"
)

// favor is the currency the imperial-favor operations move. Naming it keeps a
// replay query across domains answerable without reading the model.
const favor = "imperial_favor"

// Entity kinds.
const (
	entitySettlement = "settlement"
	entityRequest    = "settlement_request"
	entityInvitation = "invitation"
)

// opFor starts one domain-operation record: logger.OpFrom supplies the ctx
// correlation ids (request_id, user_id) and component=op, and the actor is the
// interceptor's subject. Handlers that already read the subject themselves pass
// it to Actor only when it can differ, so the record never carries actor_id
// twice.
//
// A missing subject is a normal answer, not an error: a logging detail must not
// fail the operation, and the access log already holds the auth outcome of
// every call.
func (s *Service) opFor(ctx context.Context) *logger.Op {
	op := logger.OpFrom(ctx, s.log)
	if uid, err := interceptor.GetUserID(ctx); err == nil && uid != "" {
		op = op.Actor(uid)
	}
	return op
}

// requestEntity names the request a record is about, when the service has read
// one. A first submission has no request yet: the repository mints its id and
// answers only an error, so those records name the actor and the kind instead,
// and every later record — the approval, the rejection, the level up — carries
// the id.
func requestEntity(op *logger.Op, found *model.SettlementVerification) *logger.Op {
	if found == nil {
		return op
	}
	return op.Entity(entityRequest, found.Id)
}

// submissionFields are the facts of a submission that name no document: the
// kind, the title and how many attachments came with it. Attachment urls stay
// out of the log — the count is what a replay needs.
func submissionFields(kind model.SettlementKind, name string, attachments int) []zap.Field {
	return []zap.Field{
		zap.String("kind", string(kind)),
		zap.String("name", name),
		zap.Int("attachments", attachments),
	}
}
