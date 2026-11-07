package repository

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// staleCutoff is the heartbeat boundary the sweeper's filter is asserted
// against, derived from the same pinned clock the roster tests use, so a case
// compares a number rather than a reading of time.Now.
var staleCutoff = fixedNow.Add(-time.Minute).UnixMilli()

// orBranches returns the $or branches of a filter document.
func orBranches(t *testing.T, filter bson.M) []bson.M {
	t.Helper()

	raw, ok := fieldM(filter, "$or").(bson.A)
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
		if equalAny(branch, want) {
			return true
		}
	}

	return false
}

// TestStalePresenceFilterGuardsOnTheOnlineFlag pins the guard.
//
// is_online: true is not a narrowing convenience: it is also the partial index's
// filter expression, so dropping it does not merely widen one pass, it makes
// playerIndexes' presence index unusable and turns every sweep into a scan of
// the whole verification_requests collection.
func TestStalePresenceFilterGuardsOnTheOnlineFlag(t *testing.T) {
	filter := buildStalePresenceFilter(staleCutoff)

	if got := fieldM(filter, "is_online"); got != true {
		t.Errorf("is_online = %v, want true", got)
	}
}

// TestStalePresenceFilterCarriesBothStalenessBranches pins the whole $or.
//
// Order is not asserted: MongoDB evaluates the branches as a set, and a test
// that fails on a swap would train the next reader to ignore it.
func TestStalePresenceFilterCarriesBothStalenessBranches(t *testing.T) {
	branches := orBranches(t, buildStalePresenceFilter(staleCutoff))

	want := []bson.M{
		{"last_heartbeat": bson.M{"$lt": staleCutoff}},
		{"last_heartbeat": bson.M{"$exists": false}},
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

// TestStalePresenceFilterCatchesAPlayerWhoNeverHeartbeat is the fix for "stuck
// online forever", asserted on its own.
//
// $lt does not match a document that lacks the field, so without the $exists
// branch a player whose last_heartbeat was never written stays online forever.
// That is the state every already-online document is in when the sweeper is
// first deployed: the flag was written by the old event path, the heartbeat by
// the new one. The branch is also what catches a game server that dies between
// setting is_online and writing its first beat.
func TestStalePresenceFilterCatchesAPlayerWhoNeverHeartbeat(t *testing.T) {
	branches := orBranches(t, buildStalePresenceFilter(staleCutoff))

	if !hasBranch(branches, bson.M{"last_heartbeat": bson.M{"$exists": false}}) {
		t.Fatalf("no {last_heartbeat: {$exists: false}} branch in %v: a document with no heartbeat is never taken offline", branches)
	}
}

// TestStalePresenceFilterCutsOffAtTheArgument pins that the boundary is the
// caller's, not the clock's: a filter that read time.Now itself would take live
// players offline, and a test could not tell.
func TestStalePresenceFilterCutsOffAtTheArgument(t *testing.T) {
	branches := orBranches(t, buildStalePresenceFilter(staleCutoff))

	if !hasBranch(branches, bson.M{"last_heartbeat": bson.M{"$lt": staleCutoff}}) {
		t.Errorf("no {last_heartbeat: {$lt: %d}} branch; the filter carries %v", staleCutoff, branches)
	}
}

// TestStalePresenceUpdateOnlyClearsTheFlag pins the write.
//
// last_heartbeat is deliberately left where the game put it: the roster's "last
// seen" and the next pass's cutoff both still read something meaningful, and a
// cleared heartbeat would make a swept player indistinguishable from one the
// game never reported.
func TestStalePresenceUpdateOnlyClearsTheFlag(t *testing.T) {
	want := bson.M{"$set": bson.M{"is_online": false}}

	if got := buildStalePresenceUpdate(); !equalAny(got, want) {
		t.Errorf("update = %v, want %v", got, want)
	}
}
