package service

import (
	"context"
	"errors"

	kitdefv1 "github.com/lasthearth/vsservice/gen/kitdef/v1"
	kiterr "github.com/lasthearth/vsservice/internal/kitdef/internal/ierror"
	"github.com/lasthearth/vsservice/internal/kitdef/internal/model"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"go.uber.org/zap"
)

// ListKits returns every captured kit. attr_snapshot is dropped by the mapper,
// so it never reaches the wire.
func (s *Service) ListKits(ctx context.Context, _ *kitdefv1.ListKitsRequest) (*kitdefv1.ListKitsResponse, error) {
	l := s.log.With(zap.String("method", "ListKits"))

	kits, err := s.repo.ListKits(ctx)
	if err != nil {
		l.Error("failed to list kits", zap.Error(err))
		return nil, pkgerr.Internal("failed to list kits")
	}

	return &kitdefv1.ListKitsResponse{Kits: s.mapper.ToKitDefsProto(kits)}, nil
}

// RenameKit sets the display title of a captured kit. Metadata-only; NotFound
// when the kit was never captured (vsservice never inserts).
func (s *Service) RenameKit(ctx context.Context, req *kitdefv1.RenameKitRequest) (*kitdefv1.RenameKitResponse, error) {
	l := s.log.With(zap.String("method", "RenameKit"), zap.String("code", req.GetCode()))

	if _, err := s.repo.RenameKit(ctx, req.GetCode(), req.GetTitle()); err != nil {
		if errors.Is(err, kiterr.ErrNotFound) {
			return nil, kiterr.ErrNotFound
		}
		l.Error("failed to rename kit", zap.Error(err))
		return nil, pkgerr.Internal("failed to rename kit")
	}

	return &kitdefv1.RenameKitResponse{}, nil
}

// GetKit is the read port for other domains (e.g. mail's KitReader): it returns
// the full domain model with Items carrying attr_snapshot intact — this is the
// internal read path, distinct from the ListKits wire type which omits it.
// Returns ierror.ErrNotFound when the kit was never captured.
func (s *Service) GetKit(ctx context.Context, code string) (*model.KitDef, error) {
	return s.repo.GetKit(ctx, code)
}
