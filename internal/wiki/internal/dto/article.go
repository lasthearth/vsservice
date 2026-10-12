package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Collection is the MongoDB collection holding wiki articles.
const Collection = "wiki_articles"

// Article is the stored shape of a wiki article.
//
// Revisions live inside the article document: they are always read and written
// together with it, and one document is what makes a save and the history entry
// it produces land atomically.
type Article struct {
	// Embedded, not named: mongox.UpdateDoc locates the version guard by
	// reflecting on an anonymous Model field, and it needs the promoted
	// envelope methods. The driver inlines _id/created_at/updated_at/version at
	// the top level either way.
	mongox.Model `bson:",inline"`

	Slug        string       `bson:"slug"`
	Title       string       `bson:"title"`
	Section     string       `bson:"section"`
	Lead        string       `bson:"lead,omitempty"`
	Cover       *Cover       `bson:"cover,omitempty"`
	Tags        []string     `bson:"tags,omitempty"`
	GameVersion string       `bson:"game_version,omitempty"`
	QuickAnswer *QuickAnswer `bson:"quick_answer,omitempty"`
	Related     []string     `bson:"related,omitempty"`
	Body        string       `bson:"body,omitempty"`
	Status      string       `bson:"status"`
	AuthorName  string       `bson:"author_name,omitempty"`
	Revisions   []Revision   `bson:"revisions,omitempty"`
}

// Cover is the stored shape of a cover image.
type Cover struct {
	Url   string `bson:"url"`
	Focus int32  `bson:"focus"`
}

// QuickAnswer is the stored shape of a quick answer.
type QuickAnswer struct {
	Question string `bson:"question"`
	Answer   string `bson:"answer"`
}

// Revision is the stored shape of a saved article state.
//
// The identifier is a plain hex string that the repository assigns when the
// revision is stored: a revision is an element of an array field, and MongoDB
// assigns _id only to a top-level document, so nothing else would ever fill it.
type Revision struct {
	Id         string    `bson:"_id,omitempty"`
	ArticleId  string    `bson:"article_id"`
	Article    Article   `bson:"article"`
	Note       string    `bson:"note,omitempty"`
	AuthorName string    `bson:"author_name,omitempty"`
	SavedAt    time.Time `bson:"saved_at"`
}

// Id returns the document identifier, which the pagination helper needs.
func (a Article) Id() bson.ObjectID {
	return a.Model.Id
}
