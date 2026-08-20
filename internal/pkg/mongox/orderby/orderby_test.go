package orderby

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The caller's field is the primary sort key; `_id` is the tiebreaker and must
// come last. With `_id` first the caller's order_by never broke a tie, so it was
// silently ignored.
func TestBuildSortOptionsKeyOrder(t *testing.T) {
	sort := BuildSortOptions(&Info{
		Field:      "created_at",
		Direction:  Asc,
		MongoField: "created_at",
	})

	want := bson.D{
		{Key: "created_at", Value: Asc},
		{Key: "_id", Value: Asc},
	}

	if len(sort) != len(want) {
		t.Fatalf("sort = %#v, want %#v", sort, want)
	}
	for i := range want {
		if sort[i] != want[i] {
			t.Fatalf("sort[%d] = %#v, want %#v", i, sort[i], want[i])
		}
	}
}

// The `_id` tiebreaker must follow the caller's direction, otherwise the cursor
// (an `_id` comparison) walks the collection the wrong way.
func TestBuildSortOptionsTiebreakerFollowsDirection(t *testing.T) {
	for _, dir := range []Direction{Asc, Desc} {
		sort := BuildSortOptions(&Info{MongoField: "coins", Direction: dir})
		if sort[1].Key != "_id" {
			t.Fatalf("sort = %#v, want _id last", sort)
		}
		if sort[1].Value != dir {
			t.Fatalf("_id direction = %v, want %v", sort[1].Value, dir)
		}
	}
}

func TestBuildSortOptionsNilFallsBackToID(t *testing.T) {
	sort := BuildSortOptions(nil)
	if len(sort) != 1 || sort[0].Key != "_id" || sort[0].Value != Desc {
		t.Fatalf("sort = %#v, want [{_id -1}]", sort)
	}
}

func TestParse(t *testing.T) {
	allowed := map[string]string{"created_at": "created_at"}
	fallback := &Info{Field: "created_at", Direction: Desc, MongoField: "created_at"}

	cases := map[string]struct {
		orderBy string
		want    *Info
		wantErr bool
	}{
		"empty falls back": {orderBy: "", want: fallback},
		"bare field is ascending": {
			orderBy: "created_at",
			want:    &Info{Field: "created_at", Direction: Asc, MongoField: "created_at"},
		},
		"explicit desc": {
			orderBy: "created_at desc",
			want:    &Info{Field: "created_at", Direction: Desc, MongoField: "created_at"},
		},
		"unknown field rejected": {orderBy: "coins", wantErr: true},
		"malformed rejected":     {orderBy: "created_at sideways", wantErr: true},
		"uppercase rejected":     {orderBy: "createdAt", wantErr: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.orderBy, allowed, fallback)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if *got != *tc.want {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
