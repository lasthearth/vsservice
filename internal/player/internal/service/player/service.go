//go:generate goverter gen github.com/lasthearth/vsservice/internal/player/internal/service/player
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"

	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/pkg/image"
	"github.com/lasthearth/vsservice/internal/player/internal/ierror"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/minio/minio-go/v7"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type SsoRepository interface {
	UpdateUserAvatar(ctx context.Context, userID, avatarURL string) error
}

type DbRepository interface {
	GetUserById(ctx context.Context, id string) (*model.Player, error)
	SearchUsers(ctx context.Context, query string) ([]model.Player, error)
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
			UserId:       player.UserId,
			UserGameName: player.UserGameName,
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
