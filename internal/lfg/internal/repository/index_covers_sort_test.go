package repository

import (
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestBoardSortsAreIndexServed pins the agreement between the two board sorts
// and the indexes that must serve them.
//
// The failure is silent: MongoDB accepts the query and just inserts a blocking
// in-memory SORT stage, so a public board endpoint degrades to latency and then,
// past the sort memory limit, to an error for every caller — with nothing in the
// code pointing at the cause.
func TestBoardSortsAreIndexServed(t *testing.T) {
	sorts := map[string]bson.D{
		"session board":  sortSessions,
		"teammate board": sortTeammates,
	}

	for name, sort := range sorts {
		served := slices.ContainsFunc(lfgIndexes, func(im mongo.IndexModel) bool {
			keys, ok := im.Keys.(bson.D)
			return ok && servesSort(keys, sort)
		})
		if !served {
			t.Errorf("%s sorts on %v, which no index in lfgIndexes serves; the planner will add a blocking in-memory sort", name, sort)
		}
	}
}

// servesSort reports whether idx can yield sort without a SORT stage. ListOpen
// constrains kind by equality, so idx must lead with kind and then carry every
// sort key immediately after it, in the same order and direction. A range field
// such as expires_at sitting between them breaks the ordering just as surely as
// a missing key does.
func servesSort(idx, sort bson.D) bool {
	if len(idx) != len(sort)+1 || idx[0].Key != "kind" {
		return false
	}
	for i, s := range sort {
		if idx[i+1].Key != s.Key || idx[i+1].Value != s.Value {
			return false
		}
	}
	return true
}

// TestTTLIndexIsDeclared guards the only thing that deletes expired posts. The
// indexes are created one at a time and a failure is only logged, so a dropped
// or renamed delete_at spec would leave lfg_posts growing forever with no error
// anywhere.
func TestTTLIndexIsDeclared(t *testing.T) {
	want := bson.D{{Key: "delete_at", Value: 1}}

	if !slices.ContainsFunc(lfgIndexes, func(im mongo.IndexModel) bool {
		keys, ok := im.Keys.(bson.D)
		return ok && slices.Equal(keys, want)
	}) {
		t.Errorf("lfgIndexes must keep a delete_at index for the TTL monitor, got %v", lfgIndexes)
	}
}
