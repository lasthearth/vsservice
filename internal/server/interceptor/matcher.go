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
	case "/vintage.v1.VintageService/GetOnlinePlayersCount":
		return false
	case "/vintage.v1.VintageService/GetGameTime":
		return false
	case "/leaderboard.v1.LeaderboardService/ListEntries":
		return false
	case "/verification.v1.VerificationService/VerifyCode":
		return false
	case "/verification.v1.VerificationService/VerifyStatusByName":
		return false
	case "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo":
		return false
	default:
		return true
	}
}
