//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/wiki/internal/repository
package repository

import (
	"github.com/lasthearth/vsservice/internal/wiki/internal/dto"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
)

// Mapper converts between domain articles and their stored shape.
//
// The identifier, timestamps and version live on the stored document and are
// copied onto the domain envelope by the repository, so what is converted here
// is only the page. The envelope is ignored when reading a document back; the
// document envelope itself is written by the repository.
//
// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:ignoreUnexported
type Mapper interface {
	// FromArticle converts a stored document into a domain article.
	// goverter:ignore Envelope
	FromArticle(dto.Article) model.Article
	// ToArticle converts a domain article into its stored shape.
	// goverter:ignore Model
	ToArticle(model.Article) dto.Article

	// FromRevision converts a stored revision into a domain revision.
	// goverter:ignore Id
	FromRevision(dto.Revision) model.Revision
	// ToRevision converts a domain revision into its stored shape.
	ToRevision(model.Revision) dto.Revision

	// FromSuggestion converts a stored proposal into a domain proposal.
	// goverter:ignore Id CreatedAt Envelope
	FromSuggestion(dto.Suggestion) model.Suggestion
	// ToSuggestion converts a domain proposal into its stored shape.
	// goverter:ignore Model
	ToSuggestion(model.Suggestion) dto.Suggestion
}
