package repository

import "github.com/lasthearth/vsservice/internal/mod/internal/model"

// The stored words of the two enums. They are spelled out rather than numbered
// so a document an admin reads in the database says what it means, and so a
// value inserted by hand cannot silently mean another audience.

// AudienceToString stores who needs the mod.
func AudienceToString(audience model.Audience) string {
	switch audience {
	case model.AudienceRequired:
		return "required"
	case model.AudienceOptional:
		return "optional"
	case model.AudienceHidden:
		return "hidden"
	default:
		return "unspecified"
	}
}

// AudienceFromString reads who needs the mod; an unknown value is UNSPECIFIED,
// which keeps a mod nobody can classify out of the roster and out of the
// archive.
func AudienceFromString(audience string) model.Audience {
	switch audience {
	case "required":
		return model.AudienceRequired
	case "optional":
		return model.AudienceOptional
	case "hidden":
		return model.AudienceHidden
	default:
		return model.AudienceUnspecified
	}
}

// SideToString stores where the mod runs.
func SideToString(side model.Side) string {
	switch side {
	case model.SideBoth:
		return "both"
	case model.SideClient:
		return "client"
	case model.SideServer:
		return "server"
	default:
		return "unspecified"
	}
}

// SideFromString reads where the mod runs.
func SideFromString(side string) model.Side {
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
