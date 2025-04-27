package interceptor

import (
	"context"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
)

func AuthMatcher(ctx context.Context, c interceptors.CallMeta) bool {
	switch c.FullMethod() {
	case "/vintage.v1.VintageService/GetOnlinePlayersCount":
		return false
	case "/vintage.v1.VintageService/GetGameTime":
		return false
	case "/leaderboard.v1.LeaderboardService/ListEntries":
		return false
	case "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo":
		return false
	default:
		return true
	}
}
