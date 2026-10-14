package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The roster bounds. The query is a nickname fragment, not a filter expression:
// a one-character fragment would answer with half the collection, and a long one
// is a client mistake rather than a search.
const (
	minRosterQueryRunes   = 2
	maxRosterQueryRunes   = 32
	defaultRosterPageSize = 24
	maxRosterPageSize     = 48
)

// ListPlayers implements userv1.UserServiceServer.
//
// The roster is public and carries nothing the leaderboard does not already
// publish: a nickname, an avatar, whether the game has the player now, the
// hours it recorded, and the description the player wrote.
func (s *Service) ListPlayers(
	ctx context.Context,
	req *userv1.ListPlayersRequest,
) (*userv1.ListPlayersResponse, error) {
	query := strings.TrimSpace(req.GetQuery())
	if utf8.RuneCountInString(query) > maxRosterQueryRunes {
		return nil, ierror.InvalidArgument("query is longer than 32 characters")
	}
	// Too short to narrow anything: the whole roster is the honest answer.
	if utf8.RuneCountInString(query) < minRosterQueryRunes {
		query = ""
	}

	page := max(int(req.GetPage()), 1)

	size := int(req.GetPageSize())
	if size < 1 {
		size = defaultRosterPageSize
	}
	if size > maxRosterPageSize {
		size = maxRosterPageSize
	}

	result, err := s.dbRepo.ListPlayers(
		ctx,
		model.PlayerListFilter{
			Query:    query,
			LastSeen: lastSeenWindow(req.GetLastSeen()),
			Sort:     playerSort(req.GetSort()),
		},
		page,
		size,
		time.Now(),
	)
	if err != nil {
		// The roster is public. The driver's text names collections, stages and
		// sometimes the server address, so the detail goes to the log and the
		// caller gets a plain failure.
		s.log.Error("failed to list players", zap.Error(err))

		return nil, status.Error(codes.Internal, "failed to list players")
	}

	// The answer carries the page and the size that were actually applied, so a
	// client paging through the roster does not have to repeat the clamping.
	return &userv1.ListPlayersResponse{
		Players:   s.mapper.ToPlayerListItemsProto(result.Players),
		TotalSize: result.TotalSize,
		Page:      result.Page,
		PageSize:  result.PageSize,
		Counts:    s.mapper.ToLastSeenCountsProto(result.Counts),
	}, nil
}

// lastSeenWindow maps the request's window onto the model's. An unrecognized
// value is the same as "every player", so a newer client cannot be locked out
// of the roster by an older server.
func lastSeenWindow(v userv1.LastSeen) model.LastSeenWindow {
	switch v {
	case userv1.LastSeen_LAST_SEEN_ONLINE:
		return model.LastSeenOnline
	case userv1.LastSeen_LAST_SEEN_DAY:
		return model.LastSeenDay
	case userv1.LastSeen_LAST_SEEN_WEEK:
		return model.LastSeenWeek
	case userv1.LastSeen_LAST_SEEN_MONTH:
		return model.LastSeenMonth
	case userv1.LastSeen_LAST_SEEN_INACTIVE:
		return model.LastSeenInactive
	default:
		return model.LastSeenAny
	}
}

// playerSort maps the request's order onto the model's. An unrecognized value
// sorts by when the players were last seen, which is what the page shows by
// default.
func playerSort(v userv1.PlayerSort) model.PlayerListSort {
	switch v {
	case userv1.PlayerSort_PLAYER_SORT_HOURS:
		return model.PlayerSortHours
	case userv1.PlayerSort_PLAYER_SORT_NICKNAME:
		return model.PlayerSortNickname
	default:
		return model.PlayerSortLastSeen
	}
}
