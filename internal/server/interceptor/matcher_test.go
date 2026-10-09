package interceptor

import (
	"context"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/lasthearth/vsservice/internal/pkg/config"
)

func TestAuthMatcherPublicMethods(t *testing.T) {
	public := []string{
		"/user.v1.UserService/GetUser",
		"/user.v1.UserService/BatchGetUsers",
		"/settlement.v1.SettlementService/List",
		"/settlement.v1.SettlementService/GetInviteLink",
		"/settlement.v1.SettlementTagService/GetTag",
		"/settlement.v1.SettlementTagService/GetTags",
		"/settlement.v1.SettlementTagService/GetTagsByIds",
		"/event.v1.EventService/GetEvent",
		"/event.v1.EventService/ListEvents",
		"/event.v1.EventService/ListAttendees",
		"/reaction.v1.ReactionService/ListReactions",
		"/lfg.v1.LfgService/ListPosts",
		"/lfg.v1.LfgService/GetPost",
		"/appearance.v1.AppearanceService/ListAppearances",
	}
	protected := []string{
		"/user.v1.UserService/SearchUsers",
		"/user.v1.UserService/ChangeNickname",
		"/settlement.v1.SettlementTagService/CreateTag",
		"/event.v1.EventService/CreateEvent",
		"/event.v1.EventService/UpdateEvent",
		"/event.v1.EventService/DeleteEvent",
		// SetAttendance writes a per-player row, so it must never be public.
		"/event.v1.EventService/SetAttendance",
		"/event.v1.EventService/ListMyEvents",
		"/reaction.v1.ReactionService/ListMyReactions",
		"/reaction.v1.ReactionService/ToggleReaction",
		"/settlement.v1.SettlementService/CreateInviteLink",
		"/settlement.v1.SettlementService/ListInviteLinks",
		"/settlement.v1.SettlementService/RevokeInviteLink",
		"/settlement.v1.SettlementService/JoinByInviteLink",
		"/lfg.v1.LfgService/CreatePost",
		"/lfg.v1.LfgService/RespondToPost",
		"/lfg.v1.LfgService/ClosePost",
		"/lfg.v1.LfgService/RenewPost",
		// GetPostContact returns the author's contact, so it must never be public.
		"/lfg.v1.LfgService/GetPostContact",
		// The look, the standing and the purchases are the caller's own.
		"/appearance.v1.AppearanceService/UpdateMyAppearance",
		"/appearance.v1.AppearanceService/ResetMyAppearance",
		"/appearance.v1.AppearanceService/GetMyStanding",
		"/appearance.v1.AppearanceService/BuyBanner",
	}

	split := func(m string) interceptors.CallMeta {
		i := strings.LastIndexByte(m, '/')
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
