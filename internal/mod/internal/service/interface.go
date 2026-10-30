//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/mod/internal/service
package service

import (
	"context"

	modv1 "github.com/lasthearth/vsservice/gen/mod/v1"
	"github.com/lasthearth/vsservice/internal/mod/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/moddb"
)

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend AudienceToProto
// goverter:extend SideToProto
// goverter:extend ReleaseToProto
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:IntToInt32
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields
	// goverter:map ModId Id
	// goverter:map LogoURL LogoUrl
	// goverter:map ModDBURL ModdbUrl
	ToModProto(model.Mod) *modv1.Mod
	ToModProtos([]model.Mod) []*modv1.Mod
}

// Repository stores the server's mods.
type Repository interface {
	List(ctx context.Context) ([]model.Mod, error)
	Get(ctx context.Context, modId string) (*model.Mod, error)
	Create(ctx context.Context, mod *model.Mod) error
	Save(
		ctx context.Context,
		modId string,
		fn func(ctx context.Context, mod *model.Mod) (*model.Mod, error),
	) (*model.Mod, error)
	Delete(ctx context.Context, modId string) error
	Reorder(ctx context.Context, ids []string) error
	// Sync writes the server-sourced part of the list: the mods in mods are
	// stored by id (a new one after the last position) and removeIds are
	// deleted. Everything else stays untouched.
	Sync(ctx context.Context, mods []model.Mod, removeIds []string) error
}

// RequiredMods reads the mods the "download everything" archive packs.
type RequiredMods interface {
	ListRequired(ctx context.Context) ([]model.Mod, error)
}

// ModDB reads a mod from the Vintage Story mod database: what it says about the
// mod, and how large the file behind one of its releases is. It is an interface
// so the mod list can be tested without reaching the network, and
// *moddb.Client is the implementation in production.
type ModDB interface {
	GetMod(ctx context.Context, ref string) (*moddb.Mod, error)
	FileSize(ctx context.Context, fileURL string) int64
}
