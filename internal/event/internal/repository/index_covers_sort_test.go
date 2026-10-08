package repository

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestListSortsAreIndexServed pins the agreement between the list sorts and the
// declared indexes.
//
// The failure is silent: MongoDB accepts the query and just inserts a blocking
// in-memory SORT stage. On a public endpoint that shows up as latency and, past
// the sort memory limit, as an error for every caller — with nothing in the code
// pointing at the cause. This is exactly how {starts_at, _id} came to be sorted
// against an index on {starts_at} alone.
func TestListSortsAreIndexServed(t *testing.T) {
	sorts := map[string]bson.D{
		"ListUpcoming": sortUpcoming,
		"ListPast":     sortPast,
	}

	for name, sort := range sorts {
		if !indexServes(sort) {
			t.Errorf("%s sorts on %v, which no index in eventIndexes serves; the planner will add a blocking in-memory sort", name, sort)
		}
	}
}

// indexServes reports whether some declared index yields the sort order, scanned
// either forward or backward.
func indexServes(sort bson.D) bool {
	for _, im := range eventIndexes {
		keys, ok := im.Keys.(bson.D)
		if !ok {
			continue
		}
		if isPrefix(keys, sort) || isPrefix(reverseKeys(keys), sort) {
			return true
		}
	}
	return false
}

// isPrefix reports whether sort matches the leading keys of index exactly, in
// both field and direction. A sort shorter than the index is still served: the
// extra trailing keys only break ties the sort does not care about.
func isPrefix(index, sort bson.D) bool {
	if len(index) < len(sort) {
		return false
	}
	for i, s := range sort {
		if index[i].Key != s.Key || index[i].Value != s.Value {
			return false
		}
	}
	return true
}

// reverseKeys negates every direction in an index key pattern, which is the
// order a backward scan yields. The key order is unchanged: a backward scan of
// {until: 1, _id: 1} produces {until: -1, _id: -1}, not {_id: -1, until: -1}.
func reverseKeys(keys bson.D) bson.D {
	out := make(bson.D, len(keys))
	for i, k := range keys {
		dir, _ := k.Value.(int)
		out[i] = bson.E{Key: k.Key, Value: -dir}
	}
	return out
}
