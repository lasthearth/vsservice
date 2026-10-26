package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	domainerr "github.com/lasthearth/vsservice/internal/player/internal/ierror"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AdminChangeNickname implements userv1.UserServiceServer.
//
// A moderator replaces another player's nickname without waiting out the
// cooldown. The write is an ordinary nickname change: the old name is kept, the
// player's cooldown starts again, and the player is told who changed what and
// why. The caller's scope is checked by the interceptor.
func (s *Service) AdminChangeNickname(
	ctx context.Context,
	req *userv1.AdminChangeNicknameRequest,
) (*userv1.AdminChangeNicknameResponse, error) {
	// The reason is validated before the read: a bad body costs no query.
	reason, err := normalizeReason(req.GetReason(), false)
	if err != nil {
		return nil, err
	}

	if !model.ValidNickname(req.GetNewNickname()) {
		return nil, model.ErrNicknameInvalid
	}

	player, err := s.playerForAdmin(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	if err := player.CheckNicknameChange(req.GetNewNickname()); err != nil {
		return nil, err
	}

	now := time.Now()
	oldNickname := player.UserGameName

	if err := player.SetNickname(req.GetNewNickname(), now); err != nil {
		return nil, err
	}

	if err := s.dbRepo.UpdatePlayerNickname(
		ctx,
		req.GetUserId(),
		player.UserGameName,
		player.PreviousNickname,
		now,
	); err != nil {
		s.log.Error("failed to change a nickname as an admin",
			zap.String("user_id", req.GetUserId()), zap.Error(err))

		return nil, status.Error(codes.Internal, "failed to change the nickname")
	}

	s.notifyPlayer(
		ctx,
		player.UserId,
		req.GetUserId(),
		"Никнейм изменён администратором",
		adminNicknameMessage(oldNickname, player.UserGameName, reason),
	)

	moderator, _ := interceptor.GetUserID(ctx)

	s.log.Info("nickname changed by an admin",
		zap.String("moderator_id", moderator),
		zap.String("user_id", req.GetUserId()),
		zap.String("old_nickname", oldNickname),
		zap.String("new_nickname", player.UserGameName),
		zap.String("reason", reason))

	return &userv1.AdminChangeNicknameResponse{
		OldNickname:         oldNickname,
		NewNickname:         player.UserGameName,
		NextChangeAllowedAt: timestamppb.New(player.NicknameChangeAllowedAt()),
	}, nil
}

// AdminGrantNicknameChange implements userv1.UserServiceServer.
//
// The recorded change is cleared, so the cooldown stops holding and the player
// may change the nickname themselves at once. Refusing a grant that gives
// nothing keeps the player from being told about a permission they already had.
func (s *Service) AdminGrantNicknameChange(
	ctx context.Context,
	req *userv1.AdminGrantNicknameChangeRequest,
) (*emptypb.Empty, error) {
	reason, err := normalizeReason(req.GetReason(), false)
	if err != nil {
		return nil, err
	}

	player, err := s.playerForAdmin(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	if err := player.GrantNicknameChange(); err != nil {
		return nil, err
	}

	if err := s.dbRepo.UpdateById(
		ctx,
		req.GetUserId(),
		model.PlayerUpdate{ClearNicknameCooldown: true},
	); err != nil {
		s.log.Error("failed to grant a nickname change",
			zap.String("user_id", req.GetUserId()), zap.Error(err))

		return nil, status.Error(codes.Internal, "failed to grant the nickname change")
	}

	message := "Администратор разрешил вам сменить никнейм раньше срока."
	if reason != "" {
		message += " Причина: " + reason
	}

	s.notifyPlayer(
		ctx,
		player.UserId,
		req.GetUserId(),
		"Смена никнейма разрешена",
		message,
	)

	moderator, _ := interceptor.GetUserID(ctx)

	s.log.Info("nickname change granted by an admin",
		zap.String("moderator_id", moderator),
		zap.String("user_id", req.GetUserId()),
		zap.String("reason", reason))

	return &emptypb.Empty{}, nil
}

// playerForAdmin reads the player a moderator acts on. A missing player is
// NOT_FOUND rather than INTERNAL, because a mistyped id is the likely cause and
// the panel has to be able to tell it apart from a database failure.
func (s *Service) playerForAdmin(ctx context.Context, userID string) (*model.Player, error) {
	player, err := s.dbRepo.GetUserById(ctx, userID)
	if err != nil {
		if errors.Is(err, domainerr.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "player not found")
		}

		s.log.Error("failed to read the player", zap.String("user_id", userID), zap.Error(err))

		return nil, status.Error(codes.Internal, "failed to read the player")
	}

	return player, nil
}

// notifyPlayer tells a player what a moderator changed about them. A failure is
// only logged: the write is committed, so reporting it as an error would invite
// the moderator to repeat an action that already happened. WithoutCancel keeps
// the call on this goroutine, so the answer waits for the notification, while a
// client that hangs up still lets it through.
func (s *Service) notifyPlayer(
	ctx context.Context,
	recipient, fallbackRecipient, title, message string,
) {
	// A player document may lack the SSO id; the id the moderator addressed is
	// then the only way to reach the same person.
	if recipient == "" {
		recipient = fallbackRecipient
	}

	if s.cnuc == nil {
		s.log.Warn("no notifier is wired, the player is not told about a moderator change",
			zap.String("user_id", recipient))

		return
	}

	err := s.cnuc.CreateNotification(
		context.WithoutCancel(ctx),
		title,
		message,
		notificationuc.WithUserId(recipient),
	)
	if err != nil {
		s.log.Warn("failed to notify the player about a moderator change",
			zap.String("user_id", recipient),
			zap.Error(err))
	}
}

// adminNicknameMessage is what the player reads after a moderator renamed them.
func adminNicknameMessage(oldNickname, newNickname, reason string) string {
	message := fmt.Sprintf(
		"Администратор изменил ваш никнейм с '%s' на '%s'.",
		oldNickname,
		newNickname,
	)
	if reason != "" {
		message += " Причина: " + reason
	}

	return message
}
