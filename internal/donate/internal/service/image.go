package service

import (
	"bytes"
	"context"
	"fmt"
	"mime"

	"github.com/google/uuid"
)

const imageExt = ".webp"

// uploadImage uploads raw image bytes to MinIO and returns the public CDN URL.
func (s *Service) uploadImage(ctx context.Context, data []byte) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}

	objectName := fmt.Sprintf("%s%s", id.String(), imageExt)
	contentType := mime.TypeByExtension(imageExt)

	_, err = s.storage.UploadObject(
		ctx,
		bucketName,
		objectName,
		bytes.NewReader(data),
		int64(len(data)),
		contentType,
	)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%s/%s", s.cfg.CdnUrl, bucketName, objectName), nil
}
