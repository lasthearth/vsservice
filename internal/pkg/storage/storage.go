package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
)

func (s *Storage) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	return s.client.BucketExists(ctx, bucketName)
}

func (s *Storage) MakeBucketPublic(ctx context.Context, bucketName string) error {
	policy := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{
				"Effect":    "Allow",
				"Principal": map[string]string{"AWS": "*"},
				"Action":    []string{"s3:GetObject"},
				"Resource":  []string{fmt.Sprintf("arn:aws:s3:::%s/*", bucketName)},
			},
		},
	}

	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("failed to marshal policy: %w", err)
	}

	return s.client.SetBucketPolicy(ctx, bucketName, string(policyJSON))
}

func (s *Storage) CreateBucket(ctx context.Context, bucketName string) error {
	return s.client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
}

func (s *Storage) DeleteBucket(ctx context.Context, bucketName string) error {
	return s.client.RemoveBucket(ctx, bucketName)
}

func (s *Storage) UploadObject(
	ctx context.Context,
	bucketName, objectName string,
	reader io.Reader,
	size int64,
	contentType string,
) (*minio.UploadInfo, error) {
	info, err := s.client.PutObject(
		ctx,
		bucketName,
		objectName,
		reader,
		size,
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return nil, err
	}
	return &info, nil
}
