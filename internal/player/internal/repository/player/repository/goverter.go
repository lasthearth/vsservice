package repository

import (
	"fmt"

	dto "github.com/lasthearth/vsservice/internal/player/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
)

// goverter:context avatarPrefix
func AvatarWithPrefix(avatar *dto.Avatar, avatarPrefix string) *model.Avatar {
	if avatar == nil {
		return nil
	}
	return &model.Avatar{
		Original: fmt.Sprintf("%s/%s", avatarPrefix, avatar.Original),
		X96:      fmt.Sprintf("%s/%s", avatarPrefix, avatar.X96),
		X48:      fmt.Sprintf("%s/%s", avatarPrefix, avatar.X48),
	}
}
