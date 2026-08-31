//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/kitdef/internal/service
package service

import (
	kitdefv1 "github.com/lasthearth/vsservice/gen/kitdef/v1"
	"github.com/lasthearth/vsservice/internal/kitdef/internal/model"
)

// Mapper converts domain KitDef ↔ protobuf. attr_snapshot has no field on the
// proto KitItem, so the KitItem extend simply drops it — the wire type can never
// leak it.
//
// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/kitdef/internal/goverter:KitItemModelToProto
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields
	ToKitDefProto(*model.KitDef) *kitdefv1.KitDef
	ToKitDefsProto([]*model.KitDef) []*kitdefv1.KitDef
}
