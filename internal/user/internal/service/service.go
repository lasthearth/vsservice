package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"strings"

	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/pkg/image"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/verification/model"
	"github.com/minio/minio-go/v7"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type SsoRepository interface {
	UpdateUserAvatar(ctx context.Context, userID, avatarURL string) error
}

type DbRepository interface {
	GetVerificationStatus(ctx context.Context, userID string) (model.VerificationStatus, error)
	GetVerificationStatusByUserGameName(ctx context.Context, userGameName string) (model.VerificationStatus, error)
	GetVerificationCode(ctx context.Context, userID string) (string, error)
	VerifyCode(ctx context.Context, userGameName string, code string) error
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

// UpdateAvatar implements userv1.UserServiceServer.
func (s *Service) UpdateAvatar(ctx context.Context, req *userv1.UpdateAvatarRequest) (*emptypb.Empty, error) {
	file := req.Avatar
	bucketName := "avatars"
	filename := "avatar"
	ext := ".webp"
	mb := 1 << (10 * 2)

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

	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
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

// VerifyStatus implements userv1.UserServiceServer
func (s *Service) VerifyStatusByName(ctx context.Context, req *userv1.VerifyStatusByNameRequest) (*userv1.VerifyStatusResponse, error) {
	if req.UserGameName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "user name is required")
	}

	status, err := s.dbRepo.GetVerificationStatusByUserGameName(ctx, req.UserGameName)
	if err != nil {
		return nil, err
	}

	return &userv1.VerifyStatusResponse{
		Status: string(status),
	}, nil
}

// VerifyStatus implements userv1.UserServiceServer
func (s *Service) VerifyStatus(ctx context.Context, req *userv1.VerifyStatusRequest) (*userv1.VerifyStatusResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	status, err := s.dbRepo.GetVerificationStatus(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &userv1.VerifyStatusResponse{
		Status: string(status),
	}, nil
}

// GetVerifyCode implements userv1.UserServiceServer.
func (s *Service) GetVerifyCode(ctx context.Context, req *userv1.GetVerifyCodeRequest) (*userv1.GetVerifyCodeResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	code, err := s.dbRepo.GetVerificationCode(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetVerifyCodeResponse{
		Code: code,
	}, nil
}

// VerifyCode implements userv1.UserServiceServer.
func (s *Service) VerifyCode(ctx context.Context, req *userv1.VerifyCodeRequest) (*userv1.VerifyCodeResponse, error) {
	err := s.dbRepo.VerifyCode(ctx, req.UserGameName, req.Code)
	if err != nil {
		return nil, err
	}

	return &userv1.VerifyCodeResponse{}, nil
}
