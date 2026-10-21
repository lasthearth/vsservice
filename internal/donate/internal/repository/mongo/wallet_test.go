package repository

import (
	"sort"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// operatorPaths flattens an update document into the field paths each operator
// touches, in the order the operators appear.
func operatorPaths(t *testing.T, update bson.D) map[string][]string {
	t.Helper()

	paths := make(map[string][]string, len(update))
	for _, op := range update {
		fields, ok := op.Value.(bson.D)
		if !ok {
			t.Fatalf("operator %s holds %T, want bson.D", op.Key, op.Value)
		}
		if _, dup := paths[op.Key]; dup {
			t.Fatalf("operator %s appears twice in the update document", op.Key)
		}
		names := make([]string, 0, len(fields))
		for _, f := range fields {
			names = append(names, f.Key)
		}
		paths[op.Key] = names
	}
	return paths
}

// TestAddCoinsUpdateHasNoPathConflict guards the failure that took every credit
// down: version sat in both $inc and $setOnInsert, and MongoDB rejects such a
// document with "Updating the path 'version' would create a conflict at
// 'version'" — on the update path as well as the insert path, so AddCoinsToWallet
// returned an Internal error for every player, old wallets included.
func TestAddCoinsUpdateHasNoPathConflict(t *testing.T) {
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	id := bson.ObjectID{0x01}

	for _, tc := range []struct {
		name       string
		playerName string
	}{
		{name: "named credit", playerName: "Ugalqg"},
		{name: "credit without a name", playerName: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := make(map[string]string)
			for op, fields := range operatorPaths(t, addCoinsUpdate(tc.playerName, 500, id, now)) {
				for _, f := range fields {
					if prev, dup := seen[f]; dup {
						t.Fatalf("path %q is set by both %s and %s; MongoDB rejects the update", f, prev, op)
					}
					seen[f] = op
				}
			}
		})
	}
}

// TestAddCoinsUpdateShape pins what a credit writes: the balance and the guard
// counter move, the envelope fields appear only on insert, and player_name goes
// where the caller's intent puts it.
func TestAddCoinsUpdateShape(t *testing.T) {
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	id := bson.ObjectID{0x02}

	t.Run("a name overwrites the stored one", func(t *testing.T) {
		paths := operatorPaths(t, addCoinsUpdate("Ugalqg", 500, id, now))

		assertFields(t, paths["$inc"], "coins", "version")
		assertFields(t, paths["$set"], "updated_at", "player_name")
		assertFields(t, paths["$setOnInsert"], "_id", "created_at")
	})

	t.Run("no name leaves the stored one alone", func(t *testing.T) {
		paths := operatorPaths(t, addCoinsUpdate("", 500, id, now))

		assertFields(t, paths["$inc"], "coins", "version")
		assertFields(t, paths["$set"], "updated_at")
		assertFields(t, paths["$setOnInsert"], "_id", "created_at", "player_name")
	})

	t.Run("the increment seeds the guard counter on insert", func(t *testing.T) {
		update := addCoinsUpdate("Ugalqg", 500, id, now)

		var inc bson.D
		for _, op := range update {
			if op.Key == "$inc" {
				inc, _ = op.Value.(bson.D)
			}
		}
		for _, f := range inc {
			switch f.Key {
			case "coins":
				if got, ok := f.Value.(int64); !ok || got != 500 {
					t.Fatalf("$inc coins = %#v, want int64(500)", f.Value)
				}
			case "version":
				if got, ok := f.Value.(int64); !ok || got != 1 {
					t.Fatalf("$inc version = %#v, want int64(1)", f.Value)
				}
			}
		}
	})
}

func assertFields(t *testing.T, got []string, want ...string) {
	t.Helper()

	sortedGot := append([]string(nil), got...)
	sort.Strings(sortedGot)
	sortedWant := append([]string(nil), want...)
	sort.Strings(sortedWant)

	if len(sortedGot) != len(sortedWant) {
		t.Fatalf("fields = %v, want %v", got, want)
	}
	for i := range sortedGot {
		if sortedGot[i] != sortedWant[i] {
			t.Fatalf("fields = %v, want %v", got, want)
		}
	}
}
