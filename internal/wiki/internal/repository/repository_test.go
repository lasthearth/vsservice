package repository

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/regexsafe"
	"github.com/lasthearth/vsservice/internal/wiki/internal/dto"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestListFilterNeverWidensPastTheDraftGate pins the rule that keeps drafts off
// the public list: a status filter narrows the result, it never widens it.
func TestListFilterNeverWidensPastTheDraftGate(t *testing.T) {
	cases := []struct {
		name   string
		filter model.Filter
		want   any
	}{
		{"a reader asking for drafts gets the published pages", model.Filter{Status: model.StatusDraft}, model.StatusPublished},
		{"a reader asking for published gets published", model.Filter{Status: model.StatusPublished}, model.StatusPublished},
		{"a reader asking for nothing gets published", model.Filter{}, model.StatusPublished},
		{"an editor asking for drafts gets drafts", model.Filter{Status: model.StatusDraft, IncludeDrafts: true}, model.StatusDraft},
		{"an editor asking for published gets published", model.Filter{Status: model.StatusPublished, IncludeDrafts: true}, model.StatusPublished},
		{"an editor asking for nothing gets every status", model.Filter{IncludeDrafts: true}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := listFilter(tc.filter)["status"]
			if tc.want == nil {
				if ok {
					t.Errorf("status = %v, want no status filter", got)
				}

				return
			}
			if got != tc.want {
				t.Errorf("status = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestListFilterEscapesTheSearchQuery pins that a search reaches the regex
// escaped. ListArticles is in the interceptor's publicMethods, so an
// unauthenticated caller supplies this text; unescaped, it lets them build a
// pattern, and "(.*)*x" then makes MongoDB's regex engine backtrack until the
// worker threads are gone. The helper has its own test - this one is the call
// site, which is the part that can silently stop calling it.
func TestListFilterEscapesTheSearchQuery(t *testing.T) {
	const search = "ве.*т"

	ors, ok := listFilter(model.Filter{Search: search})["$or"].([]bson.M)
	if !ok || len(ors) == 0 {
		t.Fatalf("$or = %v, want one condition per searched field",
			listFilter(model.Filter{Search: search})["$or"])
	}

	escaped := regexsafe.Escape(search)
	if escaped == search {
		t.Fatal("the test query must contain metacharacters")
	}

	for _, condition := range ors {
		for fieldName, value := range condition {
			pattern, ok := value.(bson.M)
			if !ok {
				t.Fatalf("%s pattern = %T, want bson.M", fieldName, value)
			}
			if pattern["$regex"] != escaped {
				t.Errorf("%s $regex = %v, want the escaped %q", fieldName, pattern["$regex"], escaped)
			}
			if pattern["$options"] != "i" {
				t.Errorf("%s $options = %v, want i", fieldName, pattern["$options"])
			}
		}
	}
}

// TestAssignRevisionIdsAddressesEveryRevision pins that a stored revision can be
// named by a restore: MongoDB assigns no _id to an element of an array field.
func TestAssignRevisionIdsAddressesEveryRevision(t *testing.T) {
	stored := dto.Article{Revisions: []dto.Revision{{Id: "kept"}, {}, {}}}

	out := assignRevisionIds(stored)

	if out.Revisions[0].Id != "kept" {
		t.Errorf("existing id = %q, want it kept", out.Revisions[0].Id)
	}

	seen := make(map[string]struct{}, len(out.Revisions))
	for i, revision := range out.Revisions {
		if revision.Id == "" {
			t.Fatalf("revision %d has no id", i)
		}
		if _, duplicate := seen[revision.Id]; duplicate {
			t.Fatalf("revision %d repeats id %q", i, revision.Id)
		}
		seen[revision.Id] = struct{}{}
	}
}
