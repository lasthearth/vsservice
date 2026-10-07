//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/lfg/internal/service
package service

import (
	"context"
	"time"

	lfgv1 "github.com/lasthearth/vsservice/gen/lfg/v1"
	"github.com/lasthearth/vsservice/internal/lfg/internal/model"
	"github.com/lasthearth/vsservice/internal/lfg/internal/repository"
)

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend ActivityToProto
// goverter:extend ActivityFromProto
// goverter:extend KindToProto
// goverter:extend KindFromProto
// goverter:extend PlayTimeToProto
// goverter:extend PlayTimeFromProto
// goverter:extend ExperienceToProto
// goverter:extend ExperienceFromProto
// goverter:extend TimestampToTimePtr
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimePtrToTimestamp
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields
	// goverter:map ClosedAt Closed | IsClosed
	// goverter:map Contact HasContact | HasContact
	ToProto(model.Post) *lfgv1.Post
	ToProtos([]model.Post) []*lfgv1.Post

	// goverter:useZeroValueOnPointerInconsistency
	CreateRequestToDetails(*lfgv1.CreatePostRequest) model.Details
}

var _ Repository = (*repository.Repository)(nil)

// Repository stores posts.
type Repository interface {
	Create(ctx context.Context, post *model.Post) (*model.Post, error)
	Get(ctx context.Context, id string) (*model.Post, error)
	ListOpen(ctx context.Context, now time.Time, kind model.Kind, activity model.Activity, limit int) ([]model.Post, error)
	CountOpenByAuthor(ctx context.Context, authorID string, kind model.Kind, now time.Time) (int64, error)
	UpdatePost(
		ctx context.Context,
		id string,
		updateFn func(ctx context.Context, p *model.Post) (*model.Post, error),
	) (*model.Post, error)
}
