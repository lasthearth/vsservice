package service

import (
	"context"
	"errors"

	referralv1 "github.com/lasthearth/vsservice/gen/referral/v1"
	"github.com/lasthearth/vsservice/internal/referral/internal/ierror"
	"github.com/lasthearth/vsservice/internal/referral/internal/model"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GetMyReferralCode implements referralv1.ReferralServiceServer
func (s *Service) GetMyReferralCode(ctx context.Context, _ *emptypb.Empty) (*referralv1.GetMyReferralCodeResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	code, err := s.dbRepo.GetCodeByPlayerID(ctx, userID)
	if err == nil {
		return &referralv1.GetMyReferralCodeResponse{Code: code.Code}, nil
	}
	if !errors.Is(err, ierror.ErrNotFound) {
		return nil, err
	}

	// No code exists for this player yet, generate a new one.
	//
	// PlayerName is intentionally left empty here: there is no
	// player-display-name source available at this layer (JWT claims only
	// carry subject/scope, no name). When this referrer's code is eventually
	// redeemed, AddCoins will create/update their donate wallet - if the
	// wallet already exists from prior donate activity it already has the
	// correct name; if not, the wallet is created with an empty name, a
	// minor cosmetic gap that's explicitly accepted.
	newCode := model.GenerateCode(userID, "")

	persisted, err := s.dbRepo.UpsertCode(ctx, newCode)
	if err != nil {
		return nil, err
	}

	return &referralv1.GetMyReferralCodeResponse{Code: persisted.Code}, nil
}

// GetMyReferralStats implements referralv1.ReferralServiceServer
func (s *Service) GetMyReferralStats(ctx context.Context, _ *emptypb.Empty) (*referralv1.GetMyReferralStatsResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	totalReferrals, totalCoins, err := s.dbRepo.GetStatsByPlayerID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &referralv1.GetMyReferralStatsResponse{
		TotalReferrals:   totalReferrals,
		TotalCoinsEarned: totalCoins,
	}, nil
}

// UseReferralCode implements referralv1.ReferralServiceServer
func (s *Service) UseReferralCode(ctx context.Context, req *referralv1.UseReferralCodeRequest) (*emptypb.Empty, error) {
	refereeID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	code, err := s.dbRepo.GetCodeByCode(ctx, req.GetCode())
	if err != nil {
		return nil, err
	}

	if code.PlayerID == refereeID {
		return nil, ErrSelfReferral
	}

	hasReferee, err := s.dbRepo.HasReferee(ctx, refereeID)
	if err != nil {
		return nil, err
	}
	if hasReferee {
		return nil, ierror.ErrAlreadyReferred
	}

	event := model.NewReferralEvent(code.PlayerID, refereeID, s.cfg.ReferralCoinsReward)

	err = s.dbRepo.CreateEvent(ctx, event)
	if err != nil {
		return nil, err
	}

	err = s.donateUC.AddCoins(ctx, code.PlayerID, code.PlayerName, s.cfg.ReferralCoinsReward)
	if err != nil {
		// The referral event is already recorded; do not fail the RPC. This
		// means the referrer's coins were not actually credited despite the
		// event existing, and needs manual reconciliation.
		s.log.Error(
			"failed to add coins to referrer wallet after referral event was recorded",
			zap.String("referrer_player_id", code.PlayerID),
			zap.Error(err),
		)
	}

	return &emptypb.Empty{}, nil
}
