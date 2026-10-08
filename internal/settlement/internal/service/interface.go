//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/settlement/internal/service
package service

import (
	"context"
	"time"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	settlementdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/settlement"
	"github.com/lasthearth/vsservice/internal/settlement/model"
)

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend TypeToProto
// goverter:extend TagIdsToProto
// goverter:extend PermissionToProto
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimePtrToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToInt64
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:IntToInt32
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields
	ToVector2Proto(model.Vector2) *settlementv1.Vector2
	ToVector2Protos([]model.Vector2) []*settlementv1.Vector2

	// goverter:ignore state sizeCache unknownFields
	ToVector3Proto(model.Vector3) *settlementv1.Vector3

	// goverter:ignore state sizeCache unknownFields
	ToAttachmentProto(model.Attachment) *settlementv1.Attachment
	ToAttachmentsProto([]model.Attachment) []*settlementv1.Attachment

	// goverter:ignore state sizeCache unknownFields
	ToMemberProto(model.Member) *settlementv1.Member
	ToMembersProto([]model.Member) []*settlementv1.Member

	// goverter:ignore state sizeCache unknownFields
	ToRoleProto(model.Role) *settlementv1.Role
	ToRolesProto([]model.Role) []*settlementv1.Role

	// goverter:ignore state sizeCache unknownFields
	ToJoinRequestProto(model.JoinRequest) *settlementv1.JoinRequest
	ToJoinRequestsProto([]model.JoinRequest) []*settlementv1.JoinRequest

	// NotifierPosition lives in another collection; the service fills it in.
	// goverter:ignore state sizeCache unknownFields NotifierPosition
	// goverter:map TagIds Tags
	ToSettlementProto(model.Settlement) *settlementv1.Settlement
	ToSettlementProtos([]model.Settlement) []*settlementv1.Settlement
	// goverter:ignore state sizeCache unknownFields
	// goverter:ignore Members Tags ImperialFavor Roles RolesEnabled ContactInfo NotifierPosition
	VerifToSettlementProto(model.SettlementVerification) *settlementv1.Settlement
	VerifsToSettlementProtos([]model.SettlementVerification) []*settlementv1.Settlement

	// goverter:ignore state sizeCache unknownFields
	ToInvProto(model.Invitation) *settlementv1.Invitation
	ToInvProtos([]model.Invitation) []*settlementv1.Invitation

	// goverter:ignore state sizeCache unknownFields
	// goverter:map CreatedAt | github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToInt64
	ToImperialFavorLogProto(model.ImperialFavorLog) *settlementv1.ImperialFavorLog
	ToImperialFavorLogsProto([]model.ImperialFavorLog) []*settlementv1.ImperialFavorLog

	// Status depends on the moment of the read; the service fills it in.
	// goverter:ignore state sizeCache unknownFields Status
	ToInviteLinkProto(model.InviteLink) *settlementv1.InviteLink
}

type SettlementRepository interface {
	SettlementDbRepository
	SettlementRequestDbRepository
}

type SettlementDbRepository interface {
	Create(ctx context.Context, dto settlementdto.Settlement) error
	CountByLeaderID(ctx context.Context, id string) (int64, error)
	GetSettlement(ctx context.Context, id string) (*model.Settlement, error)
	GetSettlementByUserId(ctx context.Context, userId string) (*model.Settlement, error)
	GetAllSettlements(ctx context.Context) ([]model.Settlement, error)

	IsMemberOfAnySettlement(ctx context.Context, userID string) error
	IsLeaderOfSettlement(ctx context.Context, settlementID, userID string) error

	UpdateSettlement(
		ctx context.Context,
		id string,
		updateFn func(ctx context.Context, s *model.Settlement) (*model.Settlement, error),
	) (*model.Settlement, error)

	DeleteSettlement(ctx context.Context, settlementID string) error

	// GetNotifier returns where the settlement's notifier block stands; found is
	// false while none is placed. The collection is written by the game server;
	// vsservice only reads it.
	GetNotifier(ctx context.Context, settlementID string) (placement model.NotifierPlacement, found bool, err error)

	// InTransaction runs fn in one MongoDB transaction: every repository call and
	// every mail written with the ctx fn receives commits or rolls back together.
	// fn may run more than once on transient errors, so it must be idempotent and
	// must return, never swallow, the errors of the calls it makes.
	InTransaction(ctx context.Context, fn func(ctx context.Context) error) error

	AddTag(ctx context.Context, settlementID, tagID string) error
	RemoveTag(ctx context.Context, settlementID, tagID string) error

	CreateFavorLog(ctx context.Context, log model.ImperialFavorLog) error
	ListFavorLogs(ctx context.Context, settlementID, adminID, orderBy, nextToken string) ([]model.ImperialFavorLog, string, error)

	RemoveMember(ctx context.Context, settlementID, userID string) error
	CreateInvitation(ctx context.Context, settlementID, userID string) error
	DeleteInvitationForUser(ctx context.Context, invitationID, userID string) error
	DeleteInvitationForLeader(ctx context.Context, invitationID, settlementID string) error
	AcceptInvitation(ctx context.Context, invID, userID string) error
	GetInvitations(ctx context.Context, settlementID string) ([]model.Invitation, error)
	GetUserInvitations(ctx context.Context, userID string) ([]model.Invitation, error)

	CountUserJoinRequests(ctx context.Context, userID string) (int64, error)
	CreateJoinRequest(ctx context.Context, settlementID, userID string) error
	GetJoinRequest(ctx context.Context, joinRequestID string) (*model.JoinRequest, error)
	GetJoinRequests(ctx context.Context, settlementID string) ([]model.JoinRequest, error)
	GetUserJoinRequests(ctx context.Context, userID string) ([]model.JoinRequest, error)
	DeleteJoinRequestForUser(ctx context.Context, joinRequestID, userID string) error
	// ApproveJoinRequest adds the applicant to the settlement, deletes the
	// request and all other pending requests of that user, in one transaction.
	ApproveJoinRequest(ctx context.Context, joinRequestID string) (userID string, err error)
	DeleteJoinRequest(ctx context.Context, joinRequestID, settlementID string) error

	CreateInviteLink(ctx context.Context, link *model.InviteLink) (*model.InviteLink, error)
	// CountActiveInviteLinks counts links still usable at now.
	CountActiveInviteLinks(ctx context.Context, settlementID string, now time.Time) (int64, error)
	// ListInviteLinks returns links that were not revoked, newest first.
	ListInviteLinks(ctx context.Context, settlementID string) ([]model.InviteLink, error)
	GetInviteLinkByCode(ctx context.Context, code string) (*model.InviteLink, error)
	UpdateInviteLink(
		ctx context.Context,
		settlementID, linkID string,
		updateFn func(ctx context.Context, link *model.InviteLink) (*model.InviteLink, error),
	) (*model.InviteLink, error)
	// JoinByInviteLink spends a use of the link through useFn and adds userID
	// to its settlement; the use is given back if the player cannot be added.
	JoinByInviteLink(
		ctx context.Context,
		code, userID string,
		useFn func(ctx context.Context, link *model.InviteLink) (*model.InviteLink, error),
	) (*model.InviteLink, error)
}

type SettlementRequestDbRepository interface {
	CreateRequest(ctx context.Context, opts SettlementOpts) error
	UpdateRequest(ctx context.Context, opts SettlementOpts) error
	GetSettlementRequest(ctx context.Context, id string) (*model.SettlementVerification, error)
	GetSettlementRequestByLeader(ctx context.Context, leaderID string) (*model.SettlementVerification, error)
	GetPendingSettlements(ctx context.Context) ([]model.SettlementVerification, error)
	// Approve marks the request approved and creates the settlement from it
	// (the settlement id is the request id), or upgrades the settlement when it
	// already exists. Call it inside InTransaction so the caller can write the
	// mails and notices that belong to the approval atomically.
	Approve(ctx context.Context, id string) (*ApprovalResult, error)
	Reject(ctx context.Context, id string, rejectionReason string) error
}

// ApprovalResult is what an approval changed.
type ApprovalResult struct {
	// Created is true when the approval created the settlement and false when
	// it updated an existing one.
	Created bool
	// PreviousType is the settlement type before an update; empty when Created.
	PreviousType model.SettlementType
	// Settlement is the settlement as stored after the approval.
	Settlement model.Settlement
	// RequestedAt is when the approved request was last submitted. It tells two
	// upgrades to the same tier apart.
	RequestedAt time.Time
}
