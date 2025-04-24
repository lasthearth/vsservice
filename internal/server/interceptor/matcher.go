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
	case "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo":
		return false
	default:
		return true
	}
}

func loginSkip(_ context.Context, c interceptors.CallMeta) bool {
	return c.FullMethod() != "/stats.v1.StatsService/GetOnlinePlayers"
}

func signUpSkip(_ context.Context, c interceptors.CallMeta) bool {
	return c.FullMethod() != "/v1.auth.Auth/SignUp"
}

func refreshSkip(_ context.Context, c interceptors.CallMeta) bool {
	return c.FullMethod() != "/v1.auth.Auth/RefreshToken"
}

func reflectionSkip(_ context.Context, c interceptors.CallMeta) bool {
	return c.FullMethod() != "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo"
}
