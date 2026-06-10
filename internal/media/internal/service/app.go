package service

import (
	"context"
	"slices"
	"strings"
	"time"

	mediav1 "github.com/lasthearth/vsservice/gen/media/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/storage"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ mediav1.MediaServiceServer = (*Service)(nil)

const presignExpiry = 15 * time.Minute

// purposeConfig describes where a purpose uploads and with what limits.
type purposeConfig struct {
	bucket       string
	maxSize      int64    // size limit, baked into the POST policy
	contentTypes []string // allowed MIME types (when the request sets content_type)
	scope        string   // required JWT scope; "" = any authenticated user
}

// purpose → config. Bucket names match the existing public domain buckets so
// previously stored URLs are not orphaned.
var purposes = map[mediav1.UploadPurpose]purposeConfig{
	mediav1.UploadPurpose_UPLOAD_PURPOSE_DONATE_SHOP: {
		bucket: "donate-shop", maxSize: 2 << 20,
		contentTypes: []string{"image/webp", "image/png", "image/jpeg"},
		scope:        "donate:shop:create",
	},
	mediav1.UploadPurpose_UPLOAD_PURPOSE_SETTLEMENT: {
		bucket: "settlementsreq", maxSize: 5 << 20,
		contentTypes: []string{"image/webp", "image/png", "image/jpeg"},
		scope:        "",
	},
	mediav1.UploadPurpose_UPLOAD_PURPOSE_NEWS: {
		bucket: "news", maxSize: 5 << 20,
		contentTypes: []string{"image/webp", "image/png", "image/jpeg"},
		scope:        "news:create",
	},
}

// extFromContentType returns the object extension for a MIME type (default .webp).
func extFromContentType(ct string) string {
	switch ct {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	default:
		return ".webp"
	}
}

// checkScope verifies the JWT scope for purposes that require one.
func (s *Service) checkScope(ctx context.Context, cfg purposeConfig) error {
	if cfg.scope == "" {
		return nil
	}
	claims, err := interceptor.GetClaims(ctx)
	if err != nil {
		return status.Error(codes.Unauthenticated, "missing claims")
	}
	if !slices.Contains(strings.Fields(claims.Scope), cfg.scope) {
		return status.Error(codes.PermissionDenied, "no permission for this upload purpose")
	}
	return nil
}

// Storage is the subset of pkg/storage.Storage used by the media service.
type Storage interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	CreateBucket(ctx context.Context, bucketName string) error
	MakeBucketPublic(ctx context.Context, bucketName string) error
	PresignedPostObject(
		ctx context.Context,
		bucketName, objectName string,
		expiry time.Duration,
		maxSize int64,
		contentType string,
	) (string, map[string]string, error)
}

var _ Storage = (*storage.Storage)(nil)

type Service struct {
	storage Storage
	cfg     config.Config
	log     logger.Logger
}

type Opts struct {
	fx.In
	Storage Storage
	Config  config.Config
	Logger  logger.Logger
}

func New(opts Opts) *Service {
	return &Service{storage: opts.Storage, cfg: opts.Config, log: opts.Logger}
}
