//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/player/internal/service/player
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/image"
	"github.com/lasthearth/vsservice/internal/player/internal/ierror"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/minio/minio-go/v7"
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
	GetUsersByIds(ctx context.Context, ids []string) ([]model.Player, error)
	SearchUsers(ctx context.Context, query string) ([]model.Player, error)
	UpdatePlayerNickname(
		ctx context.Context,
		userId string,
		newNickname string,
		previousNickname string,
		lastChangedAt time.Time,
	) error
	UpdateById(ctx context.Context, uid string, p model.PlayerUpdate) error
	ListPlayers(
		ctx context.Context,
		filter model.PlayerListFilter,
		page, size int,
		now time.Time,
	) (*model.PlayerListPage, error)
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
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimePtrToTimestamp
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields
	ToImageProto(*model.Avatar) *userv1.User_Image

	// goverter:ignore state sizeCache unknownFields
	ToUserProto(model.Player) *userv1.User
	ToUserProtos([]model.Player) []*userv1.User

	// goverter:ignore state sizeCache unknownFields
	ToPlayerListItemProto(model.PlayerListItem) *userv1.PlayerListItem
	ToPlayerListItemsProto([]model.PlayerListItem) []*userv1.PlayerListItem

	// goverter:ignore state sizeCache unknownFields
	ToLastSeenCountsProto(model.LastSeenCounts) *userv1.LastSeenCounts
}

// UpdateAvatar implements userv1.UserServiceServer.
func (s *Service) UpdateAvatar(ctx context.Context, req *userv1.UpdateAvatarRequest) (*emptypb.Empty, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if uid != req.GetUserId() {
		return nil, status.Error(codes.PermissionDenied, "user id mismatch, you are can't update another user's avatar")
	}

	file := req.GetAvatar()
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
	avatar := model.Avatar{
		Original: originalPath,
	}

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

		avatar.SetVariant(height, path)

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

	err = s.dbRepo.UpdateById(ctx, uid, model.PlayerUpdate{
		Avatar: &avatar,
	})
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// GetUser implements userv1.UserServiceServer.
func (s *Service) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.User, error) {
	u, err := s.dbRepo.GetUserById(ctx, req.GetUserId())
	if err != nil {
		if errors.Is(err, ierror.ErrNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	resp := s.mapper.ToUserProto(*u)

	return resp, nil
}

// BatchGetUsers implements userv1.UserServiceServer.
//
// Returns only the users that were found; missing IDs are silently omitted.
func (s *Service) BatchGetUsers(
	ctx context.Context,
	req *userv1.BatchGetUsersRequest,
) (*userv1.BatchGetUsersResponse, error) {
	users, err := s.dbRepo.GetUsersByIds(ctx, req.GetUserIds())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &userv1.BatchGetUsersResponse{
		Users: s.mapper.ToUserProtos(users),
	}, nil
}

// SearchUsers implements userv1.UserServiceServer.
func (s *Service) SearchUsers(ctx context.Context, req *userv1.SearchUsersRequest) (*userv1.SearchUsersResponse, error) {
	searched, err := s.dbRepo.SearchUsers(ctx, req.GetQuery())
	if err != nil {
		return nil, err
	}

	users := s.mapper.ToUserProtos(searched)

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

	if uid != req.GetUserId() {
		return nil, status.Error(
			codes.PermissionDenied,
			"you can only change your own nickname",
		)
	}

	// The shape is checked before the read, so a malformed name costs no query.
	if !model.ValidNickname(req.GetNewNickname()) {
		return nil, model.ErrNicknameInvalid
	}

	player, err := s.dbRepo.GetUserById(ctx, req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err := player.CheckNicknameChange(req.GetNewNickname()); err != nil {
		return nil, err
	}

	now := time.Now()

	// The cooldown is the model's, so a grant from an admin - which clears the
	// recorded change - lifts it on this path without a second rule here.
	if !player.CanChangeNickname(now) {
		return nil, status.Error(
			codes.FailedPrecondition,
			"nickname change is on cooldown, try again after: "+
				player.NicknameChangeAllowedAt().Format(time.RFC3339),
		)
	}

	oldNickname := player.UserGameName

	if err := player.SetNickname(req.GetNewNickname(), now); err != nil {
		return nil, err
	}

	err = s.dbRepo.UpdatePlayerNickname(
		ctx,
		req.GetUserId(),
		player.UserGameName,
		player.PreviousNickname,
		now,
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	go func() { //nolint:gosec // G118: deliberate — the notification must outlive the request scope
		bctx := context.Background()

		nerr := s.sendAdminNotification(
			bctx,
			oldNickname,
			player.UserGameName,
		)
		if nerr != nil {
			s.log.Error("Failed to send admin notification for nickname change",
				zap.String("player_id", req.GetUserId()),
				zap.String("old_nickname", oldNickname),
				zap.String("new_nickname", player.UserGameName),
				zap.Error(nerr),
			)
		}
	}()

	return &userv1.ChangeNicknameResponse{
		OldNickname: oldNickname,
		NewNickname: player.UserGameName,
	}, nil
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
