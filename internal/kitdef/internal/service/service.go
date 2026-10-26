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

// UpdateKit sets the site-owned metadata of a captured kit. A nil request
// field means "leave unchanged"; the model methods own the validation (an
// empty title and an unknown category are refused there). NotFound when the
// kit was never captured (vsservice never inserts).
func (s *Service) UpdateKit(ctx context.Context, req *kitdefv1.UpdateKitRequest) (*kitdefv1.UpdateKitResponse, error) {
	l := s.log.With(zap.String("method", "UpdateKit"), zap.String("code", req.GetCode()))

	kit, err := s.repo.GetKit(ctx, req.GetCode())
	if err != nil {
		if errors.Is(err, kiterr.ErrNotFound) {
			return nil, kiterr.ErrNotFound
		}
		l.Error("failed to load kit", zap.Error(err))
		return nil, pkgerr.Internal("failed to update kit")
	}

	// Validate through the model: the mutations below are applied to the
	// in-memory copy only — the repo writes the named fields with a narrow
	// $set, so this pass is a rules check, not the write itself.
	if req.GetTitle() != "" || req.Title != nil {
		if err := kit.Rename(req.GetTitle()); err != nil {
			return nil, pkgerr.InvalidArgument(err.Error())
		}
	}
	if req.Category != nil {
		if err := kit.SetCategory(req.GetCategory()); err != nil {
			return nil, pkgerr.InvalidArgument(err.Error())
		}
	}

	updated, err := s.repo.UpdateKit(ctx, req.GetCode(), req.Title, req.Category)
	if err != nil {
		if errors.Is(err, kiterr.ErrNotFound) {
			return nil, kiterr.ErrNotFound
		}
		l.Error("failed to update kit", zap.Error(err))
		return nil, pkgerr.Internal("failed to update kit")
	}

	return &kitdefv1.UpdateKitResponse{Kit: s.mapper.ToKitDefProto(updated)}, nil
}

// RemoveKitItem removes the item at the given position of a captured kit. The
// index is validated against the kit as read (bounds, and the refusal to empty
// a kit) before the repo's pipeline update runs. NotFound when the kit was
// never captured.
func (s *Service) RemoveKitItem(ctx context.Context, req *kitdefv1.RemoveKitItemRequest) (*kitdefv1.RemoveKitItemResponse, error) {
	l := s.log.With(
		zap.String("method", "RemoveKitItem"),
		zap.String("code", req.GetCode()),
		zap.Int32("index", req.GetIndex()),
	)

	kit, err := s.repo.GetKit(ctx, req.GetCode())
	if err != nil {
		if errors.Is(err, kiterr.ErrNotFound) {
			return nil, kiterr.ErrNotFound
		}
		l.Error("failed to load kit", zap.Error(err))
		return nil, pkgerr.Internal("failed to remove kit item")
	}

	if err := kit.RemoveItemAt(int(req.GetIndex())); err != nil {
		switch {
		case errors.Is(err, model.ErrItemIndex):
			return nil, pkgerr.InvalidArgument(err.Error())
		case errors.Is(err, model.ErrLastItem):
			return nil, pkgerr.FailedPrecondition(err.Error())
		default:
			return nil, pkgerr.InvalidArgument(err.Error())
		}
	}

	updated, err := s.repo.RemoveKitItem(ctx, req.GetCode(), int(req.GetIndex()))
	if err != nil {
		if errors.Is(err, kiterr.ErrNotFound) {
			return nil, kiterr.ErrNotFound
		}
		l.Error("failed to remove kit item", zap.Error(err))
		return nil, pkgerr.Internal("failed to remove kit item")
	}

	return &kitdefv1.RemoveKitItemResponse{Kit: s.mapper.ToKitDefProto(updated)}, nil
}

// GetKit is the read port for other domains (e.g. mail's KitReader): it returns
// the full domain model with Items carrying attr_snapshot intact — this is the
// internal read path, distinct from the ListKits wire type which omits it.
// Returns ierror.ErrNotFound when the kit was never captured.
func (s *Service) GetKit(ctx context.Context, code string) (*model.KitDef, error) {
	return s.repo.GetKit(ctx, code)
}

// ListKitsModel is the bulk read port for other domains (e.g. donate's
// category join): every captured kit as the domain model, attr_snapshot
// included. The wire path is ListKits, which maps and drops it.
func (s *Service) ListKitsModel(ctx context.Context) ([]*model.KitDef, error) {
	return s.repo.ListKits(ctx)
}
