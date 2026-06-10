package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	mediav1 "github.com/lasthearth/vsservice/gen/media/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) CreateUploadUrls(ctx context.Context, req *mediav1.CreateUploadUrlsRequest) (*mediav1.CreateUploadUrlsResponse, error) {
	l := s.log.With(zap.String("method", "CreateUploadUrls"))

	if req.Count < 1 || req.Count > 20 {
		return nil, status.Error(codes.InvalidArgument, "count must be between 1 and 20")
	}

	cfg, ok := purposes[req.Purpose]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "unknown upload purpose")
	}

	if err := s.checkScope(ctx, cfg); err != nil {
		return nil, err
	}

	contentType := req.ContentType
	if contentType != "" && !slices.Contains(cfg.contentTypes, contentType) {
		return nil, status.Error(codes.InvalidArgument, "unsupported content_type")
	}
	ext := extFromContentType(contentType)
	cdnBase := strings.TrimRight(s.cfg.CdnUrl, "/")

	targets := make([]*mediav1.UploadTarget, 0, req.Count)
	for i := int32(0); i < req.Count; i++ {
		id, err := uuid.NewV7()
		if err != nil {
			l.Error("failed to generate uuid", zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to generate object name")
		}
		objectName := id.String() + ext

		postURL, fields, err := s.storage.PresignedPostObject(ctx, cfg.bucket, objectName, presignExpiry, cfg.maxSize, contentType)
		if err != nil {
			l.Error("failed to generate presigned post", zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to generate upload url")
		}

		publicURL := fmt.Sprintf("%s/%s/%s", cdnBase, cfg.bucket, objectName)
		targets = append(targets, &mediav1.UploadTarget{
			PostUrl:   postURL,
			Fields:    fields,
			PublicUrl: publicURL,
		})
	}

	return &mediav1.CreateUploadUrlsResponse{Targets: targets}, nil
}
