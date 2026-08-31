package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/kitdef/internal/model"
)

// KitRepository is the persistence interface for the kitdef domain over the
// shared `kits` collection. The implementation lives in
// internal/kitdef/internal/repository/mongo. vsservice is a read + metadata
// writer: it never inserts a kit and never writes content (items/code), only
// the title.
type KitRepository interface {
	// ListKits returns every captured kit. attr_snapshot is carried on the
	// model but is the caller's responsibility to keep off the wire.
	//
	// ponytail: no pagination — mongox/pagination is not wired. Returns the
	// whole catalog in one call; graduate to AIP-132 paging if it grows.
	ListKits(ctx context.Context) ([]*model.KitDef, error)

	// GetKit returns the kit by code, or ierror.ErrNotFound if never captured.
	GetKit(ctx context.Context, code string) (*model.KitDef, error)

	// RenameKit sets the title metadata with a $set (upsert:false). Returns
	// ierror.ErrNotFound when no kit matches the code — vsservice never inserts.
	RenameKit(ctx context.Context, code, title string) (*model.KitDef, error)
}
