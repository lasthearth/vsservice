package repository

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/wiki/internal/dto"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
)

func TestEscapeRegexNeutralizesMetacharacters(t *testing.T) {
	cases := map[string]string{
		"поселения":   "поселения",
		"a.b":         "a\\.b",
		"a*b":         "a\\*b",
		"(group)":     "\\(group\\)",
		"[class]":     "\\[class\\]",
		"a|b":         "a\\|b",
		"^start$":     "\\^start\\$",
		"back\\slash": "back\\\\slash",
	}

	for input, want := range cases {
		if got := escapeRegex(input); got != want {
			t.Errorf("escapeRegex(%q) = %q, want %q", input, got, want)
		}
	}
}

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
