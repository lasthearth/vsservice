package service

import (
	modv1 "github.com/lasthearth/vsservice/gen/mod/v1"
	"github.com/lasthearth/vsservice/internal/mod/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/goverter"
)

// The proto enums this domain serves. An unknown model value falls through to
// UNSPECIFIED, which is the same "not said" the wire enum starts at.
var audiences = map[model.Audience]modv1.ModAudience{
	model.AudienceUnspecified: modv1.ModAudience_MOD_AUDIENCE_UNSPECIFIED,
	model.AudienceRequired:    modv1.ModAudience_MOD_AUDIENCE_REQUIRED,
	model.AudienceOptional:    modv1.ModAudience_MOD_AUDIENCE_OPTIONAL,
	model.AudienceHidden:      modv1.ModAudience_MOD_AUDIENCE_HIDDEN,
}

var sides = map[model.Side]modv1.ModSide{
	model.SideUnspecified: modv1.ModSide_MOD_SIDE_UNSPECIFIED,
	model.SideBoth:        modv1.ModSide_MOD_SIDE_BOTH,
	model.SideClient:      modv1.ModSide_MOD_SIDE_CLIENT,
	model.SideServer:      modv1.ModSide_MOD_SIDE_SERVER,
}

// ReleaseToProto converts one release.
//
// It is written out rather than generated because the two sides spell the file
// URL differently (FileURL against the generated FileUrl), and a mapping method
// that carried that one difference could not be reused inside the mod
// conversion — goverter refuses a method with field settings nested in another.
func ReleaseToProto(release model.Release) modv1.ModRelease {
	return modv1.ModRelease{
		Version:      release.Version,
		GameVersions: release.GameVersions,
		FileUrl:      release.FileURL,
		FileName:     release.FileName,
		FileSize:     release.FileSize,
		ReleasedAt:   goverter.TimeToTimestamp(release.ReleasedAt),
	}
}

// ToReleaseProtos maps a list of releases.
func ToReleaseProtos(releases []model.Release) []*modv1.ModRelease {
	out := make([]*modv1.ModRelease, 0, len(releases))
	for _, release := range releases {
		mapped := ReleaseToProto(release)
		out = append(out, &mapped)
	}

	return out
}

// AudienceToProto converts who needs the mod into its wire enum.
func AudienceToProto(audience model.Audience) modv1.ModAudience { return audiences[audience] }

// SideToProto converts where the mod runs into its wire enum.
func SideToProto(side model.Side) modv1.ModSide { return sides[side] }

// audienceOf reads who needs the mod from its wire enum.
func audienceOf(audience modv1.ModAudience) model.Audience {
	for mine, theirs := range audiences {
		if theirs == audience {
			return mine
		}
	}

	return model.AudienceUnspecified
}

// sideOf reads where the mod runs from ModDB's word for it.
func sideOf(side string) model.Side {
	switch side {
	case "both":
		return model.SideBoth
	case "client":
		return model.SideClient
	case "server":
		return model.SideServer
	default:
		return model.SideUnspecified
	}
}
