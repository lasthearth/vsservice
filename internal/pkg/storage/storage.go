package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

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

func (s *Storage) PresignedPutURL(
	ctx context.Context,
	bucketName, objectName string,
	expiry time.Duration,
) (string, error) {
	u, err := s.client.PresignedPutObject(ctx, bucketName, objectName, expiry)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// GetObject opens one stored object for reading. The caller closes it.
//
// The object is stat-ed before it is returned, because minio's GetObject is
// lazy: without the stat a missing object would surface only on the first read
// as a less clear error.
func (s *Storage) GetObject(
	ctx context.Context,
	bucketName, objectName string,
) (io.ReadCloser, error) {
	object, err := s.client.GetObject(ctx, bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}

	if _, err := object.Stat(); err != nil {
		if closeErr := object.Close(); closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}

		return nil, err
	}

	return object, nil
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

// PresignedPostObject возвращает presigned POST URL и поля формы для прямой
// загрузки в S3. Размер ограничивается через POST-policy (S3 отклоняет
// превышение). Если contentType пуст — разрешается любой image/*.
func (s *Storage) PresignedPostObject(
	ctx context.Context,
	bucketName, objectName string,
	expiry time.Duration,
	maxSize int64,
	contentType string,
) (string, map[string]string, error) {
	policy := minio.NewPostPolicy()
	if err := policy.SetBucket(bucketName); err != nil {
		return "", nil, err
	}
	if err := policy.SetKey(objectName); err != nil {
		return "", nil, err
	}
	if err := policy.SetExpires(time.Now().UTC().Add(expiry)); err != nil {
		return "", nil, err
	}
	if err := policy.SetContentLengthRange(1, maxSize); err != nil {
		return "", nil, err
	}
	if contentType != "" {
		if err := policy.SetContentType(contentType); err != nil {
			return "", nil, err
		}
	} else if err := policy.SetContentTypeStartsWith("image/"); err != nil {
		return "", nil, err
	}

	u, formData, err := s.client.PresignedPostPolicy(ctx, policy)
	if err != nil {
		return "", nil, err
	}
	return u.String(), formData, nil
}
