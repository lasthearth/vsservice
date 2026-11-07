package repository

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// presenceIndexModel returns the declared index that leads on is_online.
func presenceIndexModel(t *testing.T) mongo.IndexModel {
	t.Helper()

	for _, im := range playerIndexes {
		keys, ok := im.Keys.(bson.D)
		if !ok || len(keys) == 0 || keys[0].Key != "is_online" {
			continue
		}

		return im
	}

	t.Fatal("playerIndexes declares no index leading on is_online, so every presence sweep scans the collection")

	return mongo.IndexModel{}
}

// declaredIndexOptions applies an index's option builder and returns the result.
// IndexOptionsBuilder keeps its settings as closures, so the only way to read
// back what an index was declared with is to run them.
func declaredIndexOptions(t *testing.T, im mongo.IndexModel) *options.IndexOptions {
	t.Helper()

	if im.Options == nil {
		t.Fatal("the index was declared with no options, so it is not partial")
	}

	opts := &options.IndexOptions{}
	for _, apply := range im.Options.Opts {
		if err := apply(opts); err != nil {
			t.Fatalf("applying the index options: %v", err)
		}
	}

	return opts
}

// TestPresenceIndexIsDeclaredPartialOnTheOnlineFlag pins the index itself.
//
// Partial because the collection holds every verification request ever written
// while the players claiming to be online are dozens: an index over all of them
// would be almost entirely dead weight.
func TestPresenceIndexIsDeclaredPartialOnTheOnlineFlag(t *testing.T) {
	im := presenceIndexModel(t)

	wantKeys := bson.D{{Key: "is_online", Value: 1}, {Key: "last_heartbeat", Value: 1}}
	if keys, ok := im.Keys.(bson.D); !ok || !equalAny(keys, wantKeys) {
		t.Errorf("presence index keys = %v, want %v", im.Keys, wantKeys)
	}

	got := declaredIndexOptions(t, im).PartialFilterExpression
	partial, ok := got.(bson.M)
	if !ok {
		t.Fatalf("presence index partialFilterExpression = %v (%T), want a bson document", got, got)
	}
	if !equalAny(partial, bson.M{"is_online": true}) {
		t.Errorf("presence index partialFilterExpression = %v, want {is_online: true}", partial)
	}
}

// TestPresenceIndexServesTheSweeperFilter pins the agreement between the declared
// index and the filter buildStalePresenceFilter builds.
//
// Both halves of the failure are silent. MongoDB refuses to use a partial index
// unless the query provably matches only indexed documents, so a filter that
// drops or widens is_online: true is answered by a collection scan with nothing
// in the response saying so. And an index that does not lead with the equality
// field cannot serve the range that follows it.
func TestPresenceIndexServesTheSweeperFilter(t *testing.T) {
	im := presenceIndexModel(t)
	keys, ok := im.Keys.(bson.D)
	if !ok {
		t.Fatalf("presence index keys are %T, want bson.D", im.Keys)
	}

	filter := buildStalePresenceFilter(staleCutoff)

	partial, ok := declaredIndexOptions(t, im).PartialFilterExpression.(bson.M)
	if !ok {
		t.Fatal("the presence index is not partial, so it indexes the whole collection")
	}

	// Every field the partial expression constrains must be constrained the same
	// way by the filter, or the index is invisible to the query.
	for field, want := range partial {
		if got := fieldM(filter, field); !equalAny(got, want) {
			t.Errorf("the sweeper filters %s by %v, but the index is partial on %v: MongoDB will not use a partial index the query does not imply", field, got, want)
		}
	}

	if len(keys) < 2 {
		t.Fatalf("presence index keys = %v, want is_online then last_heartbeat", keys)
	}
	if keys[0].Key != "is_online" {
		t.Errorf("presence index leads on %s, want is_online: the filter's equality field must come first", keys[0].Key)
	}
	if keys[1].Key != "last_heartbeat" {
		t.Errorf("presence index carries %s second, want last_heartbeat: the range both $or branches apply must follow the equality", keys[1].Key)
	}

	// Each staleness branch constrains a field the index carries. A branch on a
	// field it does not would be evaluated per document off the index entries.
	for _, branch := range orBranches(t, filter) {
		for field := range branch {
			if field != keys[1].Key {
				t.Errorf("the sweeper's $or constrains %s, which the presence index does not carry", field)
			}
		}
	}
}
