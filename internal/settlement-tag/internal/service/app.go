//go:generate goverter gen github.com/lasthearth/vsservice/internal/settlement-tag/internal/service
package service

import (
	"context"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/settlement-tag/internal/model"
	"go.uber.org/fx"
	"google.golang.org/genproto/googleapis/type/color"
)

var _ settlementv1.SettlementTagServiceServer = (*Service)(nil)

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend FloatValueToFloat32
// goverter:extend Float32ToFloatValue
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields
	TagToProto(tag model.Tag) *settlementv1.SettlementTag
	TagsToProto(tags []model.Tag) []*settlementv1.SettlementTag
	// goverter:ignore state sizeCache unknownFields
	ColorToProto(color model.Color) *color.Color
}

type Repository interface {
	GetTags(ctx context.Context) ([]model.Tag, error)
	CreateTag(ctx context.Context, tag *model.Tag) (*model.Tag, error)
	GetTagById(ctx context.Context, id string) (*model.Tag, error)
	GetTagsByIds(ctx context.Context, ids []string) ([]model.Tag, error)
	SoftDeleteTag(ctx context.Context, id string) error
}

type Opts struct {
	fx.In
	Repository Repository
	Mapper     Mapper
}

type Service struct {
	repo   Repository
	mapper Mapper
}

func New(opts Opts) *Service {
	return &Service{
		repo:   opts.Repository,
		mapper: opts.Mapper,
	}
}
