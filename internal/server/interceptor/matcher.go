package interceptor

import (
	"context"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/lasthearth/vsservice/internal/pkg/config"
)

func AuthMatcher(ctx context.Context, c interceptors.CallMeta, cfg config.Config) bool {
	if cfg.DisableAuthMatcher {
		return false
	}
	switch c.FullMethod() {
	case "/serverinfo.v1.ServerInfoService/WorldTime":
		return false
	case "/serverinfo.v1.ServerInfoService/TotalOnline":
		return false
	case "/leaderboard.v1.LeaderboardService/ListEntries":
		return false
	case "/hungergames.v1.HungerGamesService/ListLeaderboard":
		return false
	case "/hungergames.v1.HungerGamesService/ListSeasons":
		return false
	case "/hungergames.v1.HungerGamesService/GetSeasonLeaderboard":
		return false
	case "/hungergames.v1.HungerGamesService/GetPlayerStats":
		return false
	case "/verification.v1.VerificationService/VerifyCode":
		return false
	case "/verification.v1.VerificationService/VerifyStatusByName":
		return false
	case "/news.v1.NewsService/ListNews":
		return false
	case "/news.v1.NewsService/GetNews":
		return false
	case "/donate.v1.DonateService/ListShopItems":
		return false
	case "/settlement.v1.SettlementService/Get":
		return false
	case "/settlement.v1.SettlementService/List":
		return false
	case "/user.v1.UserService/GetUser":
		return false
	case "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo":
		return false
	default:
		return true
	}
}
