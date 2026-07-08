//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/discord/internal/service

package service

import (
	discordv1 "github.com/lasthearth/vsservice/gen/discord/v1"
	"github.com/lasthearth/vsservice/internal/discord/internal/model"
)

// goverter:converter
// goverter:output:file sermapper/mapper.go
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields Id
	// goverter:map Author AuthorName
	ToProtoMessage(model.Message) *discordv1.Message
	ToProtoMessages([]model.Message) []*discordv1.Message
	// goverter:ignore state sizeCache unknownFields Id
	// goverter:map Author AuthorName
	ToProtoImage(model.Image) *discordv1.Image
	ToProtoImages([]model.Image) []*discordv1.Image
}
