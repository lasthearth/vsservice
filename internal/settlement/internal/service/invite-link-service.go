package service

import (
	"context"
	"time"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.uber.org/zap"
)

// CreateInviteLink implements settlementv1.SettlementServiceServer.
func (s *Service) CreateInviteLink(ctx context.Context, req *settlementv1.CreateInviteLinkRequest) (*settlementv1.InviteLink, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requirePermission(ctx, req.GetSettlementId(), uid, model.PermInviteMember); err != nil {
		return nil, err
	}

	now := time.Now()
	active, err := s.dbRepo.CountActiveInviteLinks(ctx, req.GetSettlementId(), now)
	if err != nil {
		return nil, err
	}
	if active >= model.MaxActiveInviteLinks {
		return nil, ierror.ErrInviteLinkLimit
	}

	link, err := model.NewInviteLink(
		req.GetSettlementId(),
		uid,
		time.Duration(req.GetTtlHours())*time.Hour,
		req.GetMaxUses(),
		now,
	)
	if err != nil {
		return nil, mapModelErr(err)
	}

	created, err := s.dbRepo.CreateInviteLink(ctx, link)
	if err != nil {
		return nil, err
	}

	return s.inviteLinkProto(*created, now), nil
}

// ListInviteLinks implements settlementv1.SettlementServiceServer.
func (s *Service) ListInviteLinks(ctx context.Context, req *settlementv1.ListInviteLinksRequest) (*settlementv1.ListInviteLinksResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requirePermission(ctx, req.GetSettlementId(), uid, model.PermInviteMember); err != nil {
		return nil, err
	}

	links, err := s.dbRepo.ListInviteLinks(ctx, req.GetSettlementId())
	if err != nil {
		return nil, err
	}

	now := time.Now()
	out := make([]*settlementv1.InviteLink, 0, len(links))
	for _, link := range links {
		out = append(out, s.inviteLinkProto(link, now))
	}
	return &settlementv1.ListInviteLinksResponse{InviteLinks: out}, nil
}

// RevokeInviteLink implements settlementv1.SettlementServiceServer.
func (s *Service) RevokeInviteLink(ctx context.Context, req *settlementv1.RevokeInviteLinkRequest) (*settlementv1.InviteLink, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requirePermission(ctx, req.GetSettlementId(), uid, model.PermInviteMember); err != nil {
		return nil, err
	}

	now := time.Now()
	revoked, err := s.dbRepo.UpdateInviteLink(ctx, req.GetSettlementId(), req.GetLinkId(),
		func(_ context.Context, link *model.InviteLink) (*model.InviteLink, error) {
			link.Revoke(now)
			return link, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return s.inviteLinkProto(*revoked, now), nil
}

// GetInviteLink implements settlementv1.SettlementServiceServer. Public.
func (s *Service) GetInviteLink(ctx context.Context, req *settlementv1.GetInviteLinkRequest) (*settlementv1.InviteLinkPreview, error) {
	link, err := s.dbRepo.GetInviteLinkByCode(ctx, req.GetCode())
	if err != nil {
		return nil, err
	}

	set, err := s.dbRepo.GetSettlement(ctx, link.SettlementId)
	if err != nil {
		return nil, err
	}

	proto := s.inviteLinkProto(*link, time.Now())
	return &settlementv1.InviteLinkPreview{
		Status:     proto.GetStatus(),
		ExpiresAt:  proto.GetExpiresAt(),
		MaxUses:    proto.GetMaxUses(),
		Uses:       proto.GetUses(),
		CreatedBy:  proto.GetCreatedBy(),
		Settlement: s.mapper.ToSettlementProto(*set),
	}, nil
}

// JoinByInviteLink implements settlementv1.SettlementServiceServer.
func (s *Service) JoinByInviteLink(ctx context.Context, req *settlementv1.JoinByInviteLinkRequest) (*settlementv1.JoinByInviteLinkResponse, error) {
	l := s.log.WithMethod("JoinByInviteLink")

	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	link, err := s.dbRepo.JoinByInviteLink(ctx, req.GetCode(), uid,
		func(_ context.Context, link *model.InviteLink) (*model.InviteLink, error) {
			if err := link.Use(now); err != nil {
				return nil, mapModelErr(err)
			}
			return link, nil
		},
	)
	if err != nil {
		return nil, err
	}

	// Tell whoever shared the link. A failed notification must not undo the join.
	name := link.SettlementId
	if set, gerr := s.dbRepo.GetSettlement(ctx, link.SettlementId); gerr == nil {
		name = set.Name
	}
	if nerr := s.notifier.CreateNotification(ctx,
		"Новый житель",
		"По вашей ссылке-приглашению в поселение «"+name+"» вступил новый житель",
		notificationuc.WithUserId(link.CreatedBy),
	); nerr != nil {
		l.Warn("failed to send invite-link notification", zap.Error(nerr), zap.String("user_id", link.CreatedBy))
	}

	return &settlementv1.JoinByInviteLinkResponse{SettlementId: link.SettlementId}, nil
}

// inviteLinkProto maps a link and fills in its status at now.
func (s *Service) inviteLinkProto(link model.InviteLink, now time.Time) *settlementv1.InviteLink {
	out := s.mapper.ToInviteLinkProto(link)
	out.Status = inviteLinkStatusToProto(link.Status(now))
	return out
}

// inviteLinkStatusToProto converts a model status to its proto enum.
func inviteLinkStatusToProto(st model.InviteLinkStatus) settlementv1.InviteLinkStatus {
	switch st {
	case model.InviteLinkActive:
		return settlementv1.InviteLinkStatus_INVITE_LINK_STATUS_ACTIVE
	case model.InviteLinkExpired:
		return settlementv1.InviteLinkStatus_INVITE_LINK_STATUS_EXPIRED
	case model.InviteLinkExhausted:
		return settlementv1.InviteLinkStatus_INVITE_LINK_STATUS_EXHAUSTED
	case model.InviteLinkRevoked:
		return settlementv1.InviteLinkStatus_INVITE_LINK_STATUS_REVOKED
	default:
		return settlementv1.InviteLinkStatus_INVITE_LINK_STATUS_UNSPECIFIED
	}
}
