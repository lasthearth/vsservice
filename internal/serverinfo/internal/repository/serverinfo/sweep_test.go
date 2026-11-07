package serverinfo

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// staleCutoff is the heartbeat boundary the sweeper's filter is asserted
// against, so a case compares a number rather than a reading of time.Now.
var staleCutoff = time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC).Add(-30 * time.Second).UnixMilli()

// orBranches returns the $or branches of a filter document.
func orBranches(t *testing.T, filter bson.M) []bson.M {
	t.Helper()

	raw, ok := filter["$or"].(bson.A)
	if !ok {
		t.Fatalf("filter carries no $or list: %v", filter)
	}

	branches := make([]bson.M, 0, len(raw))
	for _, entry := range raw {
		branch, ok := entry.(bson.M)
		if !ok {
			t.Fatalf("$or branch is %T, want bson.M", entry)
		}
		branches = append(branches, branch)
	}

	return branches
}

// hasBranch reports whether branches contains exactly want.
func hasBranch(branches []bson.M, want bson.M) bool {
	for _, branch := range branches {
		if reflect.DeepEqual(branch, want) {
			return true
		}
	}

	return false
}

// TestStaleOnlineFilterGuardsOnANonZeroCount pins the guard.
//
// total_online: {$ne: 0} is what keeps an idle pass from rewriting the document
// every 30 seconds. The game server writes it every 5 s, and a $set that lands
// between two of its writes would be a second writer racing the first for no
// reason.
func TestStaleOnlineFilterGuardsOnANonZeroCount(t *testing.T) {
	filter := buildStaleOnlineFilter(staleCutoff)

	want := bson.M{"$ne": 0}
	if got := filter["total_online"]; !reflect.DeepEqual(got, want) {
		t.Errorf("total_online = %v, want %v", got, want)
	}
}

// TestStaleOnlineFilterCarriesBothStalenessBranches pins the whole $or.
//
// Order is not asserted: MongoDB evaluates the branches as a set, and a test
// that fails on a swap would train the next reader to ignore it.
func TestStaleOnlineFilterCarriesBothStalenessBranches(t *testing.T) {
	branches := orBranches(t, buildStaleOnlineFilter(staleCutoff))

	want := []bson.M{
		{"heartbeat_at": bson.M{"$lt": staleCutoff}},
		{"heartbeat_at": bson.M{"$exists": false}},
	}
	for _, branch := range want {
		if !hasBranch(branches, branch) {
			t.Errorf("filter is missing the %v branch; it carries %v", branch, branches)
		}
	}

	if len(branches) != len(want) {
		t.Errorf("filter carries %d $or branches, want %d: %v", len(branches), len(want), branches)
	}
}

// TestStaleOnlineFilterCatchesADocumentThatNeverHeartbeat is the fix for a
// count that never goes down, asserted on its own.
//
// $lt does not match a document that lacks the field. Until the game server
// writes heartbeat_at for the first time the singleton document has no such
// field at all, so without the $exists branch the online count of a server that
// has been gone since before the rollout stays on the site forever.
func TestStaleOnlineFilterCatchesADocumentThatNeverHeartbeat(t *testing.T) {
	branches := orBranches(t, buildStaleOnlineFilter(staleCutoff))

	if !hasBranch(branches, bson.M{"heartbeat_at": bson.M{"$exists": false}}) {
		t.Fatalf("no {heartbeat_at: {$exists: false}} branch in %v: a document with no heartbeat is never zeroed", branches)
	}
}

// TestStaleOnlineUpdateOnlyZeroesTheCount pins the write.
//
// world_time is deliberately absent: the last reported time of day is more
// useful to the site than an empty one, and it is not what a dead server makes
// wrong. max_online is absent for the same reason — it is a property of the
// server's configuration, not of whether it is up.
func TestStaleOnlineUpdateOnlyZeroesTheCount(t *testing.T) {
	want := bson.M{"$set": bson.M{"total_online": 0}}

	if got := buildStaleOnlineUpdate(); !reflect.DeepEqual(got, want) {
		t.Errorf("update = %v, want %v", got, want)
	}
}
