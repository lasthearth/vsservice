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

// SourceToString stores where the record comes from. An unspecified source is
// stored as an empty string, which is what a document written before the sync
// existed carries and what the model reads back as unspecified.
func SourceToString(source model.Source) string {
	switch source {
	case model.SourceServer:
		return "server"
	case model.SourceAdmin:
		return "admin"
	default:
		return ""
	}
}

// SourceFromString reads where the record comes from; an unknown word is
// unspecified, which makes the model classify the document by its audience.
func SourceFromString(source string) model.Source {
	switch source {
	case "server":
		return model.SourceServer
	case "admin":
		return model.SourceAdmin
	default:
		return model.SourceUnspecified
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
