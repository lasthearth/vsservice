package repository

import (
	"reflect"
	"testing"

	"github.com/lasthearth/vsservice/internal/mail/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// The filter decides which mails an admin sees, and "only broadcasts" must not
// silently become "everything".
func TestAdminMailFilter(t *testing.T) {
	cases := []struct {
		name   string
		filter model.AdminMailFilter
		want   bson.M
	}{
		{
			name:   "no filter lists every mail",
			filter: model.NewAdminMailFilter("", false),
			want:   bson.M{},
		},
		{
			name:   "an explicit recipient",
			filter: model.NewAdminMailFilter("user-1", false),
			want:   bson.M{"recipient": "user-1"},
		},
		{
			name:   "broadcasts only",
			filter: model.NewAdminMailFilter("", true),
			want:   bson.M{"recipient": model.RecipientBroadcast},
		},
		{
			name:   "broadcast wins over an explicit recipient",
			filter: model.NewAdminMailFilter("user-1", true),
			want:   bson.M{"recipient": model.RecipientBroadcast},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := adminMailFilter(c.filter)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("filter = %v, want %v", got, c.want)
			}
		})
	}
}

// The counter must skip rows that are only read, and must count rows of the
// page's mails only.
func TestClaimedCountsPipeline(t *testing.T) {
	got := claimedCountsPipeline([]string{"m1", "m2"})
	if len(got) != 2 {
		t.Fatalf("stages = %d, want $match then $group", len(got))
	}

	match, ok := got[0].(bson.D)
	if !ok || len(match) != 1 || match[0].Key != "$match" {
		t.Fatalf("first stage = %v, want $match", got[0])
	}
	wantMatch := bson.M{
		"mail_id": bson.M{"$in": []string{"m1", "m2"}},
		"state":   string(model.MailStateClaimed),
	}
	if !reflect.DeepEqual(match[0].Value, wantMatch) {
		t.Fatalf("$match = %v, want %v", match[0].Value, wantMatch)
	}

	group, ok := got[1].(bson.D)
	if !ok || len(group) != 1 || group[0].Key != "$group" {
		t.Fatalf("second stage = %v, want $group", got[1])
	}
	wantGroup := bson.M{"_id": "$mail_id", "n": bson.M{"$sum": 1}}
	if !reflect.DeepEqual(group[0].Value, wantGroup) {
		t.Fatalf("$group = %v, want %v", group[0].Value, wantGroup)
	}
}
