//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/settlement/internal/service
package service

import (
	"context"

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
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToInt64
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:IntToInt32
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields
	ToVector2Proto(model.Vector2) *settlementv1.Vector2
	ToVector2Protos([]model.Vector2) []*settlementv1.Vector2

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

	// goverter:ignore state sizeCache unknownFields
	// goverter:map TagIds Tags
	ToSettlementProto(model.Settlement) *settlementv1.Settlement
	ToSettlementProtos([]model.Settlement) []*settlementv1.Settlement
	// goverter:ignore state sizeCache unknownFields
	// goverter:ignore Members Tags ImperialFavor Roles RolesEnabled ContactInfo
	VerifToSettlementProto(model.SettlementVerification) *settlementv1.Settlement
	VerifsToSettlementProtos([]model.SettlementVerification) []*settlementv1.Settlement

	// goverter:ignore state sizeCache unknownFields
	ToInvProto(model.Invitation) *settlementv1.Invitation
	ToInvProtos([]model.Invitation) []*settlementv1.Invitation

	// goverter:ignore state sizeCache unknownFields
	// goverter:map CreatedAt | github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToInt64
	ToImperialFavorLogProto(model.ImperialFavorLog) *settlementv1.ImperialFavorLog
	ToImperialFavorLogsProto([]model.ImperialFavorLog) []*settlementv1.ImperialFavorLog
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
}

type SettlementRequestDbRepository interface {
	CreateRequest(ctx context.Context, opts SettlementOpts) error
	UpdateRequest(ctx context.Context, opts SettlementOpts) error
	GetSettlementRequest(ctx context.Context, id string) (*model.SettlementVerification, error)
	GetSettlementRequestByLeader(ctx context.Context, leaderID string) (*model.SettlementVerification, error)
	GetPendingSettlements(ctx context.Context) ([]model.SettlementVerification, error)
	Approve(ctx context.Context, id string) error
	Reject(ctx context.Context, id string, rejectionReason string) error
}
