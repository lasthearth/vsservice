package interceptor

import (
	"context"
	"testing"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/lasthearth/vsservice/internal/pkg/config"
)

func TestAuthMatcherPublicMethods(t *testing.T) {
	public := []string{
		"/user.v1.UserService/GetUser",
		"/user.v1.UserService/BatchGetUsers",
		"/settlement.v1.SettlementService/List",
		"/settlement.v1.SettlementTagService/GetTag",
		"/settlement.v1.SettlementTagService/GetTags",
		"/settlement.v1.SettlementTagService/GetTagsByIds",
	}
	protected := []string{
		"/user.v1.UserService/SearchUsers",
		"/user.v1.UserService/ChangeNickname",
		"/settlement.v1.SettlementTagService/CreateTag",
	}

	split := func(m string) interceptors.CallMeta {
		i := len(m) - 1
		for ; i >= 0 && m[i] != '/'; i-- {
		}
		return interceptors.CallMeta{Service: m[1:i], Method: m[i+1:]}
	}

	for _, m := range public {
		if AuthMatcher(context.Background(), split(m), config.Config{}) {
			t.Errorf("%s must be public", m)
		}
	}
	for _, m := range protected {
		if !AuthMatcher(context.Background(), split(m), config.Config{}) {
			t.Errorf("%s must require auth", m)
		}
	}
}
