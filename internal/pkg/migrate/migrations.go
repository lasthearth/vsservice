package migrate

import (
	"context"
	"fmt"

	migrate "github.com/xakep666/mongo-migrate"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// migrations is the ordered, append-only list of schema changes. Never reorder,
// renumber, or edit an already-shipped entry — add a new one with the next
// Version. The library records the applied version in the "migrations"
// collection and runs only newer ones.
var migrations = []migrate.Migration{
	{
		Version:     1,
		Description: "settlement: fold leader into members with owner role",
		Up:          settlementMultiOwnerUp,
		// Down is intentionally a no-op: the transformation is not safely
		// reversible (owner role membership carries information the old single
		// leader field cannot represent). Roll back by restoring a DB snapshot.
		Down: func(context.Context, *mongo.Database) error { return nil },
	},
}

// settlementMultiOwnerUp folds the single `leader` field into `members` with
// the built-in "owner" role, backfills the roles fields, and drops the obsolete
// indexes so the app can recreate the new ones on startup. Idempotent.
func settlementMultiOwnerUp(ctx context.Context, db *mongo.Database) error {
	coll := db.Collection("settlements")
	reqColl := db.Collection("settlement_requests")

	// Guard: the new unique index is on members.user_id. If any user already
	// appears in two settlements (across the old leader/members split), the
	// index build would fail — abort loudly instead.
	dupes, err := coll.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$project", Value: bson.M{"ids": bson.M{"$setUnion": bson.A{bson.A{"$leader.user_id"}, "$members.user_id"}}}}},
		{{Key: "$unwind", Value: "$ids"}},
		{{Key: "$group", Value: bson.M{"_id": "$ids", "n": bson.M{"$sum": 1}}}},
		{{Key: "$match", Value: bson.M{"n": bson.M{"$gt": 1}}}},
	})
	if err != nil {
		return err
	}
	var dupeDocs []bson.M
	if err := dupes.All(ctx, &dupeDocs); err != nil {
		return err
	}
	if len(dupeDocs) > 0 {
		return fmt.Errorf("cannot migrate: %d user_id(s) belong to more than one settlement: %v", len(dupeDocs), dupeDocs)
	}

	// Fold leader into members with the owner role.
	cur, err := coll.Find(ctx, bson.M{"leader": bson.M{"$exists": true}})
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()

	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			return err
		}

		members := normalizeMembers(doc["members"])
		if leader, ok := doc["leader"].(bson.M); ok {
			if lid, ok := leader["user_id"].(string); ok && lid != "" {
				members = ensureOwner(members, lid)
			}
		}

		rolesEnabled := true
		if v, ok := doc["roles_enabled"].(bool); ok {
			rolesEnabled = v
		}
		roles := doc["roles"]
		if roles == nil {
			roles = bson.A{}
		}
		contact := ""
		if v, ok := doc["contact_info"].(string); ok {
			contact = v
		}

		if _, err := coll.UpdateByID(ctx, doc["_id"], bson.M{
			"$set": bson.M{
				"members":       members,
				"roles":         roles,
				"roles_enabled": rolesEnabled,
				"contact_info":  contact,
			},
			"$unset": bson.M{"leader": ""},
		}); err != nil {
			return err
		}
	}
	if err := cur.Err(); err != nil {
		return err
	}

	// Drop obsolete indexes; the app recreates the replacements on startup.
	// Missing-index errors (code 27, IndexNotFound) are ignored.
	dropIndex(ctx, coll, "leader.user_id_1_members.user_id_1")
	dropIndex(ctx, reqColl, "leader.user_id_-1")
	return nil
}

func normalizeMembers(raw any) bson.A {
	out := bson.A{}
	arr, ok := raw.(bson.A)
	if !ok {
		return out
	}
	for _, item := range arr {
		m, ok := item.(bson.M)
		if !ok {
			continue
		}
		roleIds := bson.A{}
		if existing, ok := m["role_ids"].(bson.A); ok {
			roleIds = existing
		}
		out = append(out, bson.M{"user_id": m["user_id"], "role_ids": roleIds})
	}
	return out
}

func ensureOwner(members bson.A, leaderID string) bson.A {
	for _, item := range members {
		m := item.(bson.M)
		if m["user_id"] == leaderID {
			if !containsStr(m["role_ids"], "owner") {
				m["role_ids"] = append(toA(m["role_ids"]), "owner")
			}
			return members
		}
	}
	return append(bson.A{bson.M{"user_id": leaderID, "role_ids": bson.A{"owner"}}}, members...)
}

func toA(v any) bson.A {
	if a, ok := v.(bson.A); ok {
		return a
	}
	return bson.A{}
}

func containsStr(v any, target string) bool {
	for _, x := range toA(v) {
		if s, ok := x.(string); ok && s == target {
			return true
		}
	}
	return false
}

func dropIndex(ctx context.Context, coll *mongo.Collection, name string) {
	// Best-effort: ignore "index not found".
	_ = coll.Indexes().DropOne(ctx, name)
}
