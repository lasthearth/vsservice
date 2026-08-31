//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/mail/internal/service
package service

import (
	mailv1 "github.com/lasthearth/vsservice/gen/mail/v1"
	"github.com/lasthearth/vsservice/internal/mail/internal/model"
)

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimePtrToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/mail/internal/goverter:AttachmentModelToProto
type Mapper interface {
	// State is per-player and derived at read, so the mapper ignores it —
	// Service stamps it from the caller's claim row.
	// goverter:ignore state sizeCache unknownFields State
	ToMailProto(*model.Mail) *mailv1.Mail
	ToMailsProto([]*model.Mail) []*mailv1.Mail
}
