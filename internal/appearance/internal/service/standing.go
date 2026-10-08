package service

import (
	"context"
	"errors"
	"sort"
	"time"

	appearancev1 "github.com/lasthearth/vsservice/gen/appearance/v1"
	"github.com/lasthearth/vsservice/internal/appearance/internal/ierror"
	"github.com/lasthearth/vsservice/internal/appearance/internal/model"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
)

// refundTimeout bounds the shard refund after a failed purchase record: the
// request that started the buy may already be gone, so the refund cannot use
// its context.
const refundTimeout = 5 * time.Second

// GetMyStanding implements appearancev1.AppearanceServiceServer.
func (s *Service) GetMyStanding(
	ctx context.Context,
	_ *appearancev1.GetMyStandingRequest,
) (*appearancev1.Standing, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}
	return s.standingOf(ctx, userID)
}

// BuyBanner implements appearancev1.AppearanceServiceServer.
//
// The order: check the banner is sold and not owned, take the shards, record
// the purchase. If the record fails the shards go back; a second click racing
// the first loses on the unique index and is refunded the same way.
func (s *Service) BuyBanner(
	ctx context.Context,
	req *appearancev1.BuyBannerRequest,
) (*appearancev1.Standing, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}
	bannerID := req.GetBannerId()

	price, err := model.BannerPrice(bannerID)
	if err != nil {
		return nil, mapModelErr(err)
	}

	standing, err := s.standings.Standing(ctx, userID)
	if err != nil {
		return nil, err
	}
	if standing.Owns(bannerID) {
		return nil, ierror.Owned("banner " + bannerID + " is already bought")
	}

	reason := "appearance: banner " + bannerID
	if err := s.wallet.Debit(ctx, userID, price, reason); err != nil {
		if errors.Is(err, donateuc.ErrInsufficientFunds) {
			return nil, ierror.NoFunds
		}
		return nil, err
	}

	if err := s.repo.AddPurchase(ctx, userID, bannerID, price, s.now()); err != nil {
		// Detached context: whatever canceled the request between the debit
		// and the refund must not also cancel the refund, or the shards are
		// simply gone.
		refundCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refundTimeout)
		defer cancel()
		if cerr := s.wallet.Credit(refundCtx, userID, "", price, "refund: "+reason); cerr != nil {
			s.logger.WithMethod("BuyBanner").Error(
				"shards were taken but neither the purchase nor the refund was saved",
				zap.String("user_id", userID), zap.String("banner_id", bannerID),
				zap.Int64("price", price), zap.Error(err), zap.NamedError("refund_error", cerr),
			)
		}
		return nil, mapModelErr(err)
	}

	return s.standingOf(ctx, userID)
}

func (s *Service) standingOf(ctx context.Context, userID string) (*appearancev1.Standing, error) {
	st, err := s.standings.Standing(ctx, userID)
	if err != nil {
		return nil, err
	}
	return standingToProto(st), nil
}

// standingToProto lists the purchases in a stable order.
func standingToProto(st model.Standing) *appearancev1.Standing {
	purchased := make([]string, 0, len(st.Purchased))
	for id, owned := range st.Purchased {
		if owned {
			purchased = append(purchased, id)
		}
	}
	sort.Strings(purchased)

	return &appearancev1.Standing{
		Hours:              st.Hours,
		Kills:              int32(st.Kills),
		Deaths:             int32(st.Deaths),
		HoursRank:          int32(st.HoursRank),
		KillsRank:          int32(st.KillsRank),
		SettlementRole:     string(st.SettlementRole),
		HungerGamesWins:    int32(st.HungerGamesWins),
		Referrals:          int32(st.Referrals),
		Events:             int32(st.Events),
		Days:               int32(st.Days),
		PurchasedBannerIds: purchased,
	}
}
