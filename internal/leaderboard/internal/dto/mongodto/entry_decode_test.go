package mongodto

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestLastOnlineDecodesFromNumberLong pins the wire format the C# game
// actually writes. A production stats document carries
// last_online: NumberLong(1770642047406) — unix milliseconds, not a BSON date.
func TestLastOnlineDecodesFromNumberLong(t *testing.T) {
	doc := bson.D{
		{Key: "user_game_name", Value: "VladKiller13"},
		{Key: "hours_played", Value: 1.72},
		{Key: "last_online", Value: int64(1770642047406)},
		{Key: "death_count", Value: int32(1)},
	}

	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	var e Entry
	if err := bson.Unmarshal(raw, &e); err != nil {
		t.Fatalf("decoding the real stored shape failed: %v", err)
	}
	if e.LastOnline == nil {
		t.Fatal("last_online decoded to nil")
	}
	t.Logf("decoded last_online = %s", e.LastOnline)
}
