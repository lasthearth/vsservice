package repository

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/notification/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// matches replays the equality semantics of the filter markReadFilter builds:
// a document matches when every key in the filter holds the same value. It
// exists so the ownership rule is asserted as behaviour, not as a map shape.
func matches(filter bson.M, doc bson.M) bool {
	for key, want := range filter {
		got, ok := doc[key]
		if !ok || got != want {
			return false
		}
	}
	return true
}

// The ownership predicate is the whole point of the filter: without user_id a
// caller who knows an id marks anyone's notification read.
func TestMarkReadFilterIsBoundToItsRecipient(t *testing.T) {
	oid := bson.NewObjectID()
	other := bson.NewObjectID()

	tests := []struct {
		name   string
		filter bson.M
		doc    bson.M
		want   bool
	}{
		{
			name:   "the recipient's own notification",
			filter: markReadFilter(oid, "u1"),
			doc:    bson.M{"_id": oid, "user_id": "u1", "state": model.NotificationStateUnread},
			want:   true,
		},
		{
			name:   "another player's notification",
			filter: markReadFilter(oid, "u1"),
			doc:    bson.M{"_id": oid, "user_id": "u2", "state": model.NotificationStateUnread},
			want:   false,
		},
		{
			name:   "a broadcast belongs to everybody, so to nobody in particular",
			filter: markReadFilter(oid, "u1"),
			doc:    bson.M{"_id": oid, "user_id": model.BroadcastUserId, "state": model.NotificationStateUnread},
			want:   false,
		},
		{
			name:   "an empty caller matches nothing",
			filter: markReadFilter(oid, ""),
			doc:    bson.M{"_id": oid, "user_id": "u1", "state": model.NotificationStateUnread},
			want:   false,
		},
		{
			name:   "another notification of the same player",
			filter: markReadFilter(oid, "u1"),
			doc:    bson.M{"_id": other, "user_id": "u1", "state": model.NotificationStateUnread},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matches(tt.filter, tt.doc); got != tt.want {
				t.Fatalf("matches(%v, %v) = %v, want %v", tt.filter, tt.doc, got, tt.want)
			}
		})
	}
}

// A caller must not be able to reach the broadcast document by claiming to be
// its owner: the broadcast user id is a fixed constant, not a JWT subject.
func TestMarkReadFilterNeverTargetsTheBroadcastDocument(t *testing.T) {
	oid := bson.NewObjectID()
	doc := bson.M{"_id": oid, "user_id": model.BroadcastUserId}

	for _, caller := range []string{"", "u1", "broadcast-alias"} {
		if matches(markReadFilter(oid, caller), doc) {
			t.Fatalf("caller %q matched the broadcast document", caller)
		}
	}
}
