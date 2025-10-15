//go:generate goverter gen github.com/lasthearth/vsservice/internal/player/internal/service/player
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"regexp"
	"strings"
	"time"

	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/image"
	"github.com/lasthearth/vsservice/internal/player/internal/ierror"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/minio/minio-go/v7"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type SsoRepository interface {
	UpdateUserAvatar(ctx context.Context, userID, avatarURL string) error
	GetAdminUsers(ctx context.Context) ([]string, error)
}

type DbRepository interface {
	GetUserById(ctx context.Context, id string) (*model.Player, error)
	SearchUsers(ctx context.Context, query string) ([]model.Player, error)
	UpdatePlayerNickname(
		ctx context.Context,
		userId string,
		newNickname string,
		previousNickname string,
		lastChangedAt time.Time,
	) error
}

type Storage interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	MakeBucketPublic(ctx context.Context, bucketName string) error
	CreateBucket(ctx context.Context, bucketName string) error
	UploadObject(
		ctx context.Context,
		bucketName, objectName string,
		reader io.Reader,
		size int64,
		contentType string,
	) (*minio.UploadInfo, error)
}

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields Avatar
	ToUserProto(model.Player) *userv1.User
	ToUserProtos([]model.Player) []*userv1.User
}

// UpdateAvatar implements userv1.UserServiceServer.
func (s *Service) UpdateAvatar(ctx context.Context, req *userv1.UpdateAvatarRequest) (*emptypb.Empty, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if uid != req.UserId {
		return nil, status.Error(codes.PermissionDenied, "user id mismatch, you are can't update another user's avatar")
	}

	file := req.Avatar
	bucketName := "avatars"
	filename := "avatar"
	ext := ".webp"
	mb := 1024 * 1024

	fmt.Printf("Updating avatar for user")

	if file == nil {
		return nil, status.Error(codes.InvalidArgument, "avatar is empty")
	}

	sizeLimit := mb * 3
	if len(file) > sizeLimit {
		return nil, status.Error(codes.InvalidArgument, "avatar size is too large")
	}

	valid, err := image.IsSizeValid(file, 512, 512)
	if err != nil {
		return nil, err
	}

	if !valid {
		return nil, status.Error(codes.InvalidArgument, "avatar size is too large")
	}

	webp, err := image.ConvertToWebp(file)
	if err != nil {
		return nil, err
	}

	originalPath := fmt.Sprintf("%s/%s%s", uid, filename, ext)
	_, err = s.storage.UploadObject(
		ctx,
		bucketName,
		originalPath,
		bytes.NewReader(webp),
		int64(len(webp)),
		mime.TypeByExtension(ext),
	)
	if err != nil {
		return nil, err
	}

	heights := []int{96, 48}
	imgs := make(map[string][]byte)

	for _, height := range heights {
		img, err := image.ProcessImage(webp, height, height)
		if err != nil {
			return nil, err
		}

		fileName := fmt.Sprintf("%s_%d%s", filename, height, ext)
		path := strings.Join([]string{
			uid,
			fileName,
		}, "/")

		imgs[path] = img
	}

	for path, img := range imgs {
		_, err = s.storage.UploadObject(
			ctx,
			bucketName,
			path,
			bytes.NewReader(img),
			int64(len(img)),
			mime.TypeByExtension(ext),
		)
		if err != nil {
			return nil, err
		}
	}

	url := fmt.Sprintf("%s/%s/%s", s.cfg.CdnUrl, bucketName, originalPath)
	err = s.ssoRepo.UpdateUserAvatar(ctx, uid, url)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// GetUser implements userv1.UserServiceServer.
func (s *Service) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.User, error) {
	u, err := s.dbRepo.GetUserById(ctx, req.UserId)
	if err != nil {
		if errors.Is(err, ierror.ErrNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	resp := s.mapper.ToUserProto(*u)

	return resp, nil
}

// SearchUsers implements userv1.UserServiceServer.
func (s *Service) SearchUsers(ctx context.Context, req *userv1.SearchUsersRequest) (*userv1.SearchUsersResponse, error) {
	searched, err := s.dbRepo.SearchUsers(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	bucketName := "avatars"

	users := lo.Map(searched, func(player model.Player, _ int) *userv1.User {
		url := fmt.Sprintf("%s/%s/%s", s.cfg.CdnUrl, bucketName, player.UserId)
		original := fmt.Sprintf("%s/avatar.webp", url)
		x96 := fmt.Sprintf("%s/avatar_96.webp", url)
		x48 := fmt.Sprintf("%s/avatar_48.webp", url)
		return &userv1.User{
			UserId:           player.UserId,
			UserGameName:     player.UserGameName,
			PreviousNickname: player.PreviousNickname,
			Avatar: &userv1.User_Image{
				Original: original,
				X96:      x96,
				X48:      x48,
			},
		}
	})

	return &userv1.SearchUsersResponse{
		Users: users,
	}, nil
}

// ChangeNickname changes the user's nickname with cooldown validation and creates admin notification
func (s *Service) ChangeNickname(ctx context.Context, req *userv1.ChangeNicknameRequest) (*userv1.ChangeNicknameResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if uid != req.UserId {
		return nil, status.Error(
			codes.PermissionDenied,
			"you can only change your own nickname",
		)
	}

	if !isValidNickname(req.NewNickname) {
		return nil, status.Error(codes.InvalidArgument, "invalid nickname format")
	}

	player, err := s.dbRepo.GetUserById(ctx, req.UserId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if player.UserGameName == req.NewNickname {
		return nil, status.Error(
			codes.InvalidArgument,
			"new nickname must be different from the current one",
		)
	}

	now := time.Now()

	// Check cooldown period - 6 months = 183 days = 183 * 24 * 60 * 60 seconds
	cd := time.Hour * 24 * 183

	nextChangeAllowed := player.LastNicknameChangedAt.Add(cd)
	if now.Before(nextChangeAllowed) {
		return nil, status.Error(
			codes.FailedPrecondition,
			fmt.Sprintf(
				"nickname change is on cooldown, try again after: %s",
				nextChangeAllowed.Format(time.RFC3339),
			),
		)
	}

	oldNickname := player.UserGameName

	err = s.dbRepo.UpdatePlayerNickname(
		ctx,
		req.UserId,
		req.NewNickname,
		oldNickname,
		now,
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	go func() {
		bctx := context.Background()

		nerr := s.sendAdminNotification(
			bctx,
			oldNickname,
			req.NewNickname,
		)
		if nerr != nil {
			s.log.Error("Failed to send admin notification for nickname change",
				zap.String("player_id", req.UserId),
				zap.String("old_nickname", oldNickname),
				zap.String("new_nickname", req.NewNickname),
				zap.Error(nerr),
			)
		}
	}()

	return &userv1.ChangeNicknameResponse{
		OldNickname: oldNickname,
		NewNickname: req.NewNickname,
	}, nil
}

// isValidNickname validates that the nickname is 15 characters or less and contains alphanumeric characters and special characters
func isValidNickname(nickname string) bool {
	if len(nickname) == 0 || len(nickname) > 15 {
		return false
	}

	matched, err := regexp.MatchString(`^[a-zA-Z0-9_\-]+$`, nickname)
	if err != nil || !matched {
		return false
	}

	return true
}

// sendAdminNotification sends a notification to all admin users about a nickname change
func (s *Service) sendAdminNotification(
	ctx context.Context,
	oldNickname, newNickname string,
) error {
	title := "Смена ника"
	message := fmt.Sprintf(
		"Игрок изменил свой никнейм с '%s' на '%s'",
		oldNickname,
		newNickname,
	)

	adminUids, err := s.ssoRepo.GetAdminUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve admin users from logto: %w", err)
	}

	for _, adminUid := range adminUids {
		err := s.cnuc.CreateNotification(
			ctx,
			title,
			message,
			notificationuc.WithUserId(adminUid),
		)
		if err != nil {
			s.log.Error(
				"Failed to send notification to admin",
				zap.String("admin_user_id", adminUid),
				zap.Error(err),
			)
		}
	}

	return nil
}
