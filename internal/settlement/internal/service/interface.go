package service

import (
	"context"

	settlementdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/settlement"
	"github.com/lasthearth/vsservice/internal/settlement/model"
)

type SettlementRepository interface {
	SettlementDbRepository
	SettlementRequestDbRepository
}

type SettlementDbRepository interface {
	Create(ctx context.Context, dto settlementdto.Settlement) error
	CountByLeaderID(ctx context.Context, id string) (int64, error)
	GetSettlement(ctx context.Context, id string) (*model.Settlement, error)
	GetSettlementsByLeader(ctx context.Context, leaderID string) ([]*model.Settlement, error)
	GetAllSettlements(ctx context.Context) ([]*model.Settlement, error)
	RemoveMember(ctx context.Context, settlementID string, userID string) error
	AddMember(ctx context.Context, settlementID string, member model.Member) error
}

type SettlementRequestDbRepository interface {
	Submit(ctx context.Context, opts SettlementOpts) error
	GetSettlementRequest(ctx context.Context, id string) (*model.SettlementVerification, error)
	GetSettlementRequestByLeader(ctx context.Context, leaderID string) (*model.SettlementVerification, error)
	GetPendingSettlements(ctx context.Context) ([]*model.SettlementVerification, error)
	Approve(ctx context.Context, id string) error
	Reject(ctx context.Context, id string, rejectionReason string) error
}
