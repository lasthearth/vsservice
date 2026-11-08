package repository

import (
	"reflect"
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestListSortsAreIndexServed pins the agreement between every (equality
// prefix, sort) pair List and ListAutoCloseDue can build and the indexes that
// must serve them.
//
// The failure is silent: MongoDB accepts the query and just inserts a blocking
// in-memory SORT stage, so a list endpoint degrades to latency and then, past
// the sort memory limit, to an error for every caller — with nothing in the
// code pointing at the cause.
func TestListSortsAreIndexServed(t *testing.T) {
	// The two sorts List builds: the player list by the last message they
	// can see, the staff inbox by the last message of all.
	authorSort := bson.D{{Key: "last_public_at", Value: -1}, {Key: "_id", Value: -1}}
	staffSort := bson.D{{Key: "last_message_at", Value: -1}, {Key: "_id", Value: -1}}

	tests := []struct {
		name   string
		prefix []string
		sort   bson.D
	}{
		{"player list", []string{"author_id"}, authorSort},
		{"staff inbox, no filter", nil, staffSort},
		{"staff inbox by status", []string{"status"}, staffSort},
		{"staff inbox by author", []string{"author_id"}, staffSort},
		{"staff inbox by assignee", []string{"assignee_id"}, staffSort},
		{"staff inbox by status and assignee", []string{"status", "assignee_id"}, staffSort},
		{"auto-close due list", []string{"status"}, bson.D{{Key: "answered_at", Value: 1}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			served := slices.ContainsFunc(ticketIndexes, func(im mongo.IndexModel) bool {
				keys, ok := im.Keys.(bson.D)
				return ok && serves(keys, tc.prefix, tc.sort)
			})
			if !served {
				t.Errorf("prefix %v sorted by %v: no index in ticketIndexes serves it; the planner will add a blocking in-memory sort", tc.prefix, tc.sort)
			}
		})
	}
}

// serves reports whether idx can yield a query that matches every prefix
// field by equality and then sorts by sort, without a SORT stage: the index
// must lead with the prefix fields (equality, so their direction does not
// matter) and carry the sort keys immediately after them, in the same order
// and direction. A different field between them breaks the ordering just as
// surely as a missing key does.
func serves(idx bson.D, prefix []string, sort bson.D) bool {
	if len(idx) != len(prefix)+len(sort) {
		return false
	}
	for i, field := range prefix {
		if idx[i].Key != field {
			return false
		}
	}
	for i, e := range sort {
		if idx[len(prefix)+i].Key != e.Key || idx[len(prefix)+i].Value != e.Value {
			return false
		}
	}
	return true
}

// TestStaffCountersOrLegsHaveIndexes guards the $or of the staff badge
// aggregation: a top-level $or only uses indexes when EVERY leg has one, so
// dropping either index silently degrades the counters to a collection scan.
func TestStaffCountersOrLegsHaveIndexes(t *testing.T) {
	statusLeg := slices.ContainsFunc(ticketIndexes, func(im mongo.IndexModel) bool {
		keys, ok := im.Keys.(bson.D)
		return ok && len(keys) > 0 && keys[0].Key == "status"
	})
	if !statusLeg {
		t.Error("the {status: open} leg of the StaffCounters $or has no index leading with status")
	}

	unreadLeg := slices.ContainsFunc(ticketIndexes, func(im mongo.IndexModel) bool {
		keys, ok := im.Keys.(bson.D)
		if !ok || len(keys) != 1 || keys[0].Key != "staff_unread" {
			return false
		}
		// A partial index only serves the leg when its filter matches the
		// leg's predicate {staff_unread: true}.
		var o options.IndexOptions
		if im.Options != nil {
			for _, set := range im.Options.List() {
				if err := set(&o); err != nil {
					t.Fatalf("index options: %v", err)
				}
			}
		}
		return reflect.DeepEqual(o.PartialFilterExpression, bson.M{"staff_unread": true})
	})
	if !unreadLeg {
		t.Error("the {staff_unread: true} leg of the StaffCounters $or has no partial index with the matching filter")
	}
}
