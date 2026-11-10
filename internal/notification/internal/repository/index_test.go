package repository

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/mongox/orderby"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// listSort is the sort ListNotifications ends up with: its default order is
// created_at desc, and pagination.List appends the _id tiebreaker in the same
// direction. Built through the same helpers the repository uses so a change to
// the default order moves this test with it.
func listSort(t *testing.T) bson.D {
	t.Helper()

	order, err := orderby.Parse(
		"",
		map[string]string{"created_at": "created_at"},
		&orderby.Info{Field: "created_at", Direction: orderby.Desc, MongoField: "created_at"},
	)
	if err != nil {
		t.Fatal(err)
	}
	return orderby.BuildSortOptions(order)
}

// dir normalizes an index or sort direction to an int: the sort comes from
// orderby.Direction, the index spec from a literal.
func dir(v any) int {
	switch d := v.(type) {
	case int:
		return d
	case int32:
		return int(d)
	case int64:
		return int(d)
	case orderby.Direction:
		return int(d)
	default:
		return 0
	}
}

func keysOf(im mongo.IndexModel) (bson.D, bool) {
	keys, ok := im.Keys.(bson.D)
	return keys, ok
}

func indexOptions(t *testing.T, im mongo.IndexModel) options.IndexOptions {
	t.Helper()

	var out options.IndexOptions
	if im.Options == nil {
		return out
	}
	for _, set := range im.Options.List() {
		if err := set(&out); err != nil {
			t.Fatalf("index options: %v", err)
		}
	}
	return out
}

// TestListQueryIsIndexServed pins the agreement between ListNotifications and
// the index that has to serve it.
//
// The failure is silent: MongoDB accepts the query and inserts a blocking
// in-memory SORT, so an inbox that pages 15 documents at a time degrades to a
// full scan plus a sort on a collection every domain writes to. The filter is
// $or [{user_id: <caller>}, {user_id: "broadcast"}], so each leg pins user_id by
// equality and the leg then reads created_at desc, _id desc straight out of the
// index; the index must lead with user_id and carry the whole sort after it, in
// order and direction.
func TestListQueryIsIndexServed(t *testing.T) {
	sort := listSort(t)

	want := append(bson.D{{Key: "user_id", Value: 1}}, sort...)

	served := slices.ContainsFunc(notificationIndexes, func(im mongo.IndexModel) bool {
		keys, ok := keysOf(im)
		if !ok || len(keys) != len(want) {
			return false
		}
		for i := range want {
			if keys[i].Key != want[i].Key || dir(keys[i].Value) != dir(want[i].Value) {
				return false
			}
		}
		return true
	})

	if !served {
		t.Errorf("no index in notificationIndexes serves the list filter on user_id with the sort %v; got %v", sort, notificationIndexes)
	}
}

// TestTTLIndexExpiresEveryDocument guards the only bound on the collection. No
// RPC deletes a notification, and the indexes are created one at a time with a
// failure only logged, so a dropped or narrowed TTL spec leaves notifications
// growing forever with no error anywhere.
//
// The partial filter is asserted absent on purpose. A TTL restricted to
// {state: "read"} never expires the documents an attacker writes into someone
// else's inbox, because those stay unread until the victim opens them: the
// collection would keep growing and keep being scanned, which is the finding
// this index closes.
func TestTTLIndexExpiresEveryDocument(t *testing.T) {
	wantKeys := bson.D{{Key: "created_at", Value: 1}}

	var ttl *mongo.IndexModel
	for i, im := range notificationIndexes {
		keys, ok := keysOf(im)
		if ok && slices.EqualFunc(keys, wantKeys, func(a, b bson.E) bool {
			return a.Key == b.Key && dir(a.Value) == dir(b.Value)
		}) {
			ttl = &notificationIndexes[i]
		}
	}
	if ttl == nil {
		t.Fatalf("notificationIndexes has no created_at TTL index, got %v", notificationIndexes)
	}

	opts := indexOptions(t, *ttl)
	if opts.ExpireAfterSeconds == nil {
		t.Fatalf("the created_at index is not a TTL index: %+v", opts)
	}
	if got, want := *opts.ExpireAfterSeconds, int32(90*24*3600); got != want {
		t.Errorf("expireAfterSeconds = %d, want %d", got, want)
	}
	if opts.PartialFilterExpression != nil {
		t.Errorf("the TTL must expire unread notifications too; partialFilterExpression = %v", opts.PartialFilterExpression)
	}
}

// TestIndexesHaveDistinctKeyPatterns protects the one-at-a-time creation: two
// indexes on the same key pattern with different options make MongoDB refuse the
// second one with an options conflict, and since a failure is only logged the
// collection would silently lose either the list index or the TTL.
func TestIndexesHaveDistinctKeyPatterns(t *testing.T) {
	seen := map[string]bool{}

	for _, im := range notificationIndexes {
		keys, ok := keysOf(im)
		if !ok {
			t.Fatalf("index keys must be bson.D, got %T", im.Keys)
		}
		if len(keys) == 0 {
			t.Fatal("an index with no keys creates nothing")
		}

		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k.Key+":"+strconv.Itoa(dir(k.Value)))
		}
		pattern := strings.Join(parts, ",")
		if seen[pattern] {
			t.Errorf("two indexes share the key pattern %q; MongoDB rejects the second one", pattern)
		}
		seen[pattern] = true
	}
}
