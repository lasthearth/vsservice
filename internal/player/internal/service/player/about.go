package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	domainerr "github.com/lasthearth/vsservice/internal/player/internal/ierror"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// isVerified reports whether a player may keep a description: an application
// the moderators approved, or one whose code the player has already entered.
// Everyone else has no profile the site renders, so nothing to write on.
func isVerified(st verification.Status) bool {
	return st == verification.VerificationStatusApproved ||
		st == verification.VerificationStatusVerified
}

// UpdateMyAbout implements userv1.UserServiceServer.
//
// The description belongs to the caller: the JWT subject is the only player
// this can write.
func (s *Service) UpdateMyAbout(
	ctx context.Context,
	req *userv1.UpdateMyAboutRequest,
) (*userv1.User, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		s.log.Error("a request the interceptor authenticated carries no user id", zap.Error(err))

		return nil, status.Error(codes.Internal, "the request carries no user id")
	}

	player, err := s.dbRepo.GetUserById(ctx, uid)
	if err != nil {
		if errors.Is(err, domainerr.ErrNotFound) {
			return nil, ierror.FailedPrecondition("player is not verified")
		}

		s.log.Error("failed to read the player", zap.String("user_id", uid), zap.Error(err))

		return nil, status.Error(codes.Internal, "failed to read the player")
	}

	if !isVerified(player.Verification.Status) {
		return nil, ierror.FailedPrecondition("player is not verified")
	}

	about, err := model.NormalizeAbout(req.GetAbout())
	if err != nil {
		return nil, err
	}

	// An empty description is a removal: the repository unsets both fields
	// instead of storing an empty string.
	update := model.PlayerUpdate{About: &about}
	if about != "" {
		written := time.Now()
		update = model.PlayerUpdate{About: &about, AboutUpdatedAt: &written}
	}

	if err := s.dbRepo.UpdateById(ctx, uid, update); err != nil {
		s.log.Error("failed to save the description", zap.String("user_id", uid), zap.Error(err))

		return nil, status.Error(codes.Internal, "failed to save the description")
	}

	// The write is committed and both halves of it are already in hand, so the
	// answer is built from them: the caller sees the normalized text it now owns,
	// not the text it sent. Reading the player back cost a round trip and reported
	// a transient failure of that read as an error on a write that had already
	// succeeded, which invites the client to submit the same text again.
	player.SetAbout(about, update.AboutUpdatedAt)

	return s.mapper.ToUserProto(*player), nil
}

// HideAbout implements userv1.UserServiceServer.
//
// A moderator removes a description the site must not show. The text itself is
// dropped rather than flagged, so the player is told why and may write a new
// one.
func (s *Service) HideAbout(
	ctx context.Context,
	req *userv1.HideAboutRequest,
) (*emptypb.Empty, error) {
	// The reason reaches the player as a notification and the audit log as a
	// field, and the two have to read the same, so it is bounded and stripped of
	// the characters a description is refused for. The byte bound comes first: it
	// is what keeps a multi-megabyte body from being copied and rewritten before
	// the answer says it is too long.
	if len(req.GetReason()) > model.MaxReasonRunes*utf8.UTFMax {
		return nil, ierror.InvalidArgument("reason is longer than 500 characters")
	}

	reason := strings.TrimSpace(model.SanitizeReason(req.GetReason()))
	if reason == "" {
		return nil, ierror.InvalidArgument("reason is empty")
	}
	if utf8.RuneCountInString(reason) > model.MaxReasonRunes {
		return nil, ierror.InvalidArgument("reason is longer than 500 characters")
	}

	player, err := s.dbRepo.GetUserById(ctx, req.GetUserId())
	if err != nil {
		if errors.Is(err, domainerr.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "player not found")
		}

		s.log.Error("failed to read the player", zap.String("user_id", req.GetUserId()), zap.Error(err))

		return nil, status.Error(codes.Internal, "failed to read the player")
	}

	// A hide that removes nothing is not a hide. Without this the player is told
	// their description was hidden and the audit log records a moderation action,
	// while no document changed - and a second click in the panel tells them
	// again.
	if player.About == "" {
		return nil, ierror.FailedPrecondition("the player has no description")
	}

	if err := s.dbRepo.UpdateById(
		ctx,
		req.GetUserId(),
		model.PlayerUpdate{About: new(string)},
	); err != nil {
		s.log.Error("failed to remove the description",
			zap.String("user_id", req.GetUserId()), zap.Error(err))

		return nil, status.Error(codes.Internal, "failed to remove the description")
	}

	recipient := player.UserId
	if recipient == "" {
		recipient = req.GetUserId()
	}

	// The removal is committed, so a notification that fails must not undo it:
	// the moderator's decision stands and the failure is only logged. The context
	// outlives the request, because a moderator who hangs up after the write would
	// otherwise leave the player with no explanation of it. WithoutCancel keeps
	// the call on this goroutine, so the answer still waits for it.
	switch {
	case s.cnuc == nil:
		s.log.Warn("no notifier is wired, the player is not told why the description was hidden",
			zap.String("user_id", recipient))
	default:
		notifyErr := s.cnuc.CreateNotification(
			context.WithoutCancel(ctx),
			"Описание профиля скрыто",
			fmt.Sprintf("Модератор скрыл ваше описание: %s. Вы можете написать новое.", reason),
			notificationuc.WithUserId(recipient),
		)

		if notifyErr != nil {
			s.log.Warn("failed to notify player about a hidden description",
				zap.String("user_id", recipient),
				zap.Error(notifyErr))
		}
	}

	moderator, _ := interceptor.GetUserID(ctx)

	s.log.Info("player about hidden",
		zap.String("moderator_id", moderator),
		zap.String("user_id", recipient),
		zap.String("reason", reason))

	return &emptypb.Empty{}, nil
}
