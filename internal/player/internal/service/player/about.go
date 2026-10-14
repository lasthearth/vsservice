package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
		return nil, status.Error(codes.Internal, err.Error())
	}

	player, err := s.dbRepo.GetUserById(ctx, uid)
	if err != nil {
		if errors.Is(err, domainerr.ErrNotFound) {
			return nil, ierror.FailedPrecondition("player is not verified")
		}

		return nil, status.Error(codes.Internal, err.Error())
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
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Read back what was written, the way GetUser answers: the caller sees the
	// normalized text it now owns, not the text it sent.
	saved, err := s.dbRepo.GetUserById(ctx, uid)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return s.mapper.ToUserProto(*saved), nil
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
	reason := strings.TrimSpace(req.GetReason())
	if reason == "" {
		return nil, ierror.InvalidArgument("reason is empty")
	}

	player, err := s.dbRepo.GetUserById(ctx, req.GetUserId())
	if err != nil {
		if errors.Is(err, domainerr.ErrNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	if err := s.dbRepo.UpdateById(
		ctx,
		req.GetUserId(),
		model.PlayerUpdate{About: new(string)},
	); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	recipient := player.UserId
	if recipient == "" {
		recipient = req.GetUserId()
	}

	// The removal is committed, so a notification that fails must not undo it:
	// the moderator's decision stands and the failure is only logged.
	notifyErr := s.cnuc.CreateNotification(
		ctx,
		"Описание профиля скрыто",
		fmt.Sprintf("Модератор скрыл ваше описание: %s. Вы можете написать новое.", reason),
		notificationuc.WithUserId(recipient),
	)

	if notifyErr != nil {
		s.log.Warn("failed to notify player about a hidden description",
			zap.String("user_id", recipient),
			zap.Error(notifyErr))
	}

	moderator, _ := interceptor.GetUserID(ctx)

	s.log.Info("player about hidden",
		zap.String("moderator_id", moderator),
		zap.String("user_id", recipient),
		zap.String("reason", reason))

	return &emptypb.Empty{}, nil
}
