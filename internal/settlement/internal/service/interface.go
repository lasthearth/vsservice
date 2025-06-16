package service

import (
	"context"
	"io"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	settlementdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/settlement"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"github.com/minio/minio-go/v7"
)

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend TypeToProto
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
	ToSettlementProto(model.Settlement) *settlementv1.Settlement
	ToSettlementProtos([]model.Settlement) []*settlementv1.Settlement
	// goverter:ignore state sizeCache unknownFields
	// goverter:ignore Members
	VerifToSettlementProto(model.SettlementVerification) *settlementv1.Settlement
	VerifsToSettlementProtos([]model.SettlementVerification) []*settlementv1.Settlement

	// goverter:ignore state sizeCache unknownFields
	ToInvProto(model.Invitation) *settlementv1.GetInvitationsResponse_Invitation
	ToInvProtos([]model.Invitation) []*settlementv1.GetInvitationsResponse_Invitation
}

type Storage interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	MakeBucketPublic(ctx context.Context, bucketName string) error
	CreateBucket(ctx context.Context, bucketName string) error
	UploadObject(
		ctx context.Context,
		bucketName, objectName string,
		reader io.Reader,
		size int64,
		contentType string,
	) (*minio.UploadInfo, error)
}

type SettlementRepository interface {
	SettlementDbRepository
	SettlementRequestDbRepository
}

type SettlementDbRepository interface {
	Create(ctx context.Context, dto settlementdto.Settlement) error
	CountByLeaderID(ctx context.Context, id string) (int64, error)
	GetSettlement(ctx context.Context, id string) (*model.Settlement, error)
	GetSettlementsByLeader(ctx context.Context, leaderID string) ([]model.Settlement, error)
	GetAllSettlements(ctx context.Context) ([]model.Settlement, error)

	IsMemberOrLeader(ctx context.Context, settlementID, userID string) error
	IsLeaderOfSettlement(ctx context.Context, settlementID, userID string) error

	RemoveMember(ctx context.Context, settlementID, userID string) error
	InviteMember(ctx context.Context, settlementID, userID string) error
	RevokeInvitation(ctx context.Context, invID string) error
	GetInvitations(ctx context.Context, settlementID string) ([]model.Invitation, error)
}

type SettlementRequestDbRepository interface {
	Submit(ctx context.Context, opts SettlementOpts) error
	GetSettlementRequest(ctx context.Context, id string) (*model.SettlementVerification, error)
	GetSettlementRequestByLeader(ctx context.Context, leaderID string) (*model.SettlementVerification, error)
	GetPendingSettlements(ctx context.Context) ([]model.SettlementVerification, error)
	Approve(ctx context.Context, id string) error
	Reject(ctx context.Context, id string, rejectionReason string) error
}
