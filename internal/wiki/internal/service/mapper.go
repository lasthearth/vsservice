//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/wiki/internal/service
package service

import (
	wikiv1 "github.com/lasthearth/vsservice/gen/wiki/v1"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
)

// Mapper converts between the wiki domain and its protobuf representation.
//
// The generated protobuf structs keep runtime state in unexported fields, which
// is what `ignoreUnexported` covers. The identifier, timestamps and version live
// on the domain envelope while the protobuf message keeps them flat, so each is
// mapped from its source explicitly. The publication state travels as an enum
// and is converted by StatusToProto.
//
// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:ignoreUnexported
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimePtrToTimestamp
// goverter:extend StatusToProto
// goverter:extend ProtoToSuggestionStatus
// goverter:extend SuggestionStatusToProto
// goverter:extend SuggestionKindToProto
type Mapper interface {
	// ToProto converts one article.
	// goverter:map Envelope.Id Id
	// goverter:map Envelope.CreatedAt CreatedAt
	// goverter:map Envelope.UpdatedAt UpdatedAt
	// goverter:map Envelope.Version Version
	ToProto(model.Article) *wikiv1.Article
	// ToProtos converts a list of articles.
	ToProtos([]model.Article) []*wikiv1.Article

	// ToSuggestionProto converts one player proposal. The address lives on the
	// proposed article while the message keeps it flat, so it is mapped out.
	// goverter:map Proposal.Slug Slug
	ToSuggestionProto(model.Suggestion) *wikiv1.Suggestion

	// ToRevisionProto converts one revision.
	ToRevisionProto(model.Revision) *wikiv1.Revision
	// ToRevisionProtos converts a list of revisions.
	ToRevisionProtos([]model.Revision) []*wikiv1.Revision
}
