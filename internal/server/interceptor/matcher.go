package interceptor

import (
	"context"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/lasthearth/vsservice/internal/pkg/config"
)

// publicMethods are the full gRPC method names served without authentication.
var publicMethods = map[string]struct{}{
	"/serverinfo.v1.ServerInfoService/WorldTime":                     {},
	"/serverinfo.v1.ServerInfoService/TotalOnline":                   {},
	"/leaderboard.v1.LeaderboardService/ListEntries":                 {},
	"/hungergames.v1.HungerGamesService/ListLeaderboard":             {},
	"/hungergames.v1.HungerGamesService/ListSeasons":                 {},
	"/hungergames.v1.HungerGamesService/GetSeasonLeaderboard":        {},
	"/hungergames.v1.HungerGamesService/GetPlayerStats":              {},
	"/verification.v1.VerificationService/VerifyCode":                {},
	"/verification.v1.VerificationService/VerifyStatusByName":        {},
	"/news.v1.NewsService/ListNews":                                  {},
	"/news.v1.NewsService/GetNews":                                   {},
	"/donate.v1.DonateService/ListShopItems":                         {},
	"/settlement.v1.SettlementService/Get":                           {},
	"/settlement.v1.SettlementService/List":                          {},
	"/user.v1.UserService/GetUser":                                   {},
	"/user.v1.UserService/BatchGetUsers":                             {},
	"/settlement.v1.SettlementTagService/GetTag":                     {},
	"/settlement.v1.SettlementTagService/GetTags":                    {},
	"/settlement.v1.SettlementTagService/GetTagsByIds":               {},
	"/discord.v1.DiscordService/ListMessages":                        {},
	"/discord.v1.DiscordService/ListImages":                          {},
	"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo": {},
}

func AuthMatcher(ctx context.Context, c interceptors.CallMeta, cfg config.Config) bool {
	if cfg.DisableAuthMatcher {
		return false
	}

	_, public := publicMethods[c.FullMethod()]
	return !public
}
