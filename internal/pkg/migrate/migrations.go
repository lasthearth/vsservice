package migrate

import (
	"context"
	"errors"
	"fmt"
	"slices"

	migrate "github.com/xakep666/mongo-migrate"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ownerRoleID mirrors model.OwnerRoleId. Migrations deliberately do NOT import
// the live domain model: domain types evolve, but a migration must forever
// decode the data shape as it existed when the migration was written. Frozen
// local constants/types are the correct, intentional choice here.
const ownerRoleID = "owner"

// kindSettlement mirrors model.KindSettlement, frozen for the same reason. Every document written
// before the guild existed is a settlement.
const kindSettlement = "settlement"

// The names MongoDB gave the two unique indexes that counted a membership and an open request of
// any kind as one. The per-kind indexes that replace them carry explicit names.
const (
	legacyMemberIndex = "members.user_id_1"
	legacyLeaderIndex = "leader.user_id_1"
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
	{
		Version:     2,
		Description: "settlement: split membership uniqueness by kind",
		Up:          settlementKindUp,
		// Down is intentionally a no-op: the dropped index allowed one membership of any kind per
		// player, so rebuilding it fails as soon as one player holds a settlement and a guild.
		//
		// Rolling the BINARY back is not safe either, and no migration can undo it: the previous
		// build maps a settlement without the kind field, so its whole-document replace drops `kind`
		// from every settlement it touches, and those documents then answer to neither unique index.
		// Deploy this build forward only. Roll back by restoring a DB snapshot.
		Down: func(context.Context, *mongo.Database) error { return nil },
	},
	{
		Version:     3,
		Description: "wiki: backfill created_by from the oldest revision",
		Up:          wikiCreatedByUp,
		// Down is intentionally a no-op: the backfill only fills a field that did
		// not exist before, so a rollback that leaves it in place reads the same
		// article — while unsetting it would lose which player wrote the page.
		Down: func(context.Context, *mongo.Database) error { return nil },
	},
}

// settlementKindUp writes the kind every settlement and every request was created without, then
// drops the two unique indexes that treated a guild membership and a settlement membership as the
// same slot. The application recreates them as one partial unique index per kind on startup, right
// after this migration returns.
//
// The backfill runs first on purpose. A document without `kind` answers to neither partial index,
// so dropping the old index before the backfill would leave a window where a player could take two
// settlements.
func settlementKindUp(ctx context.Context, db *mongo.Database) error {
	// Join requests carry a copy of the kind of the group they apply to, so that clearing them
	// after a join can leave the other kind alone. Every request written before the guild existed
	// applies to a settlement.
	for _, name := range []string{"settlements", "settlement_requests", "settlement_join_requests"} {
		if _, err := db.Collection(name).UpdateMany(ctx,
			bson.M{"kind": nil},
			bson.M{"$set": bson.M{"kind": kindSettlement}},
		); err != nil {
			return fmt.Errorf("backfill kind in %s: %w", name, err)
		}
	}

	if err := dropLegacyIndex(ctx, db.Collection("settlements"), legacyMemberIndex); err != nil {
		return err
	}
	return dropLegacyIndex(ctx, db.Collection("settlement_requests"), legacyLeaderIndex)
}

// dropLegacyIndex removes an index by name and reports every failure except a missing one, which is
// the normal case on a database that never had it. Unlike the best-effort dropIndex below, this one
// fails the migration: an old unique index left standing would refuse exactly the write the new
// schema exists to allow, and the service must not come up in that state.
func dropLegacyIndex(ctx context.Context, coll *mongo.Collection, name string) error {
	err := coll.Indexes().DropOne(ctx, name)
	if err == nil {
		return nil
	}
	// The server answers a missing index with IndexNotFound and a collection that does not exist
	// yet with NamespaceNotFound. Both mean there is nothing to drop, which is what a fresh database
	// answers, and a fresh database must still start.
	var srvErr mongo.ServerError
	if errors.As(err, &srvErr) &&
		(srvErr.HasErrorCode(indexNotFoundCode) || srvErr.HasErrorCode(namespaceNotFoundCode)) {
		return nil
	}
	return fmt.Errorf("drop index %s on %s: %w", name, coll.Name(), err)
}

// The server's answers for "there is nothing to drop".
const (
	namespaceNotFoundCode = 26
	indexNotFoundCode     = 27
)

// oldMember / oldSettlement are frozen schema snapshots decoded with typed BSON
// tags. This is the load-bearing safety fix: the previous version decoded into
// bson.M and did item.(bson.M) assertions, which silently fail because the v2
// driver materialises nested documents as bson.D. Typed decoding turns any
// shape mismatch into a hard decode error instead of silently-dropped data.
type oldMember struct {
	UserID  string   `bson:"user_id"`
	RoleIDs []string `bson:"role_ids"`
}

type oldSettlement struct {
	ID      any         `bson:"_id"`
	Leader  *oldMember  `bson:"leader"`
	Members []oldMember `bson:"members"`
}

// settlementMultiOwnerUp folds the single `leader` field into `members` with
// the built-in "owner" role, backfills the roles fields, and drops the obsolete
// indexes so the app can recreate the new ones on startup. Idempotent, and
// guarded so it can never remove a member (see the invariant checks below).
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

	// Drop obsolete unique indexes FIRST. The old compound index
	// leader.user_id_1_members.user_id_1 is unique; unsetting `leader` below
	// collapses every touched document to (null, null) and would trip it with
	// E11000. Missing-index errors are ignored (best-effort).
	dropIndex(ctx, coll, "leader.user_id_1_members.user_id_1")
	dropIndex(ctx, reqColl, "leader.user_id_-1")

	// Only documents that still carry `leader` are pre-feature and need folding.
	cur, err := coll.Find(ctx, bson.M{"leader": bson.M{"$exists": true}})
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()

	for cur.Next(ctx) {
		var s oldSettlement
		if err := cur.Decode(&s); err != nil {
			return fmt.Errorf("decode settlement: %w", err)
		}

		newMembers := foldMembers(&s)

		// Invariant: the fold may only ever ADD the owner. If it would produce
		// fewer members than the source, something is wrong — abort the whole
		// migration so fx refuses to start, rather than writing a data loss.
		// This is the check that makes the original wipe structurally
		// impossible.
		if len(newMembers) < len(s.Members) {
			return fmt.Errorf("refusing to write settlement %v: member count would drop %d -> %d", s.ID, len(s.Members), len(newMembers))
		}
		if s.Leader != nil && s.Leader.UserID != "" && !hasOwner(newMembers, s.Leader.UserID) {
			return fmt.Errorf("refusing to write settlement %v: leader %q not folded into members", s.ID, s.Leader.UserID)
		}

		// Docs matched by the filter still have `leader`, so they predate the
		// roles feature: initialise the new fields to their zero-state.
		if _, err := coll.UpdateByID(ctx, s.ID, bson.M{
			"$set": bson.M{
				"members":       newMembers,
				"roles":         bson.A{},
				"roles_enabled": true,
				"contact_info":  "",
			},
			"$unset": bson.M{"leader": ""},
		}); err != nil {
			return err
		}
	}
	return cur.Err()
}

// foldMembers returns the source members with the leader guaranteed present and
// holding the owner role. Pure and typed — never drops a member.
func foldMembers(s *oldSettlement) []oldMember {
	members := make([]oldMember, len(s.Members))
	copy(members, s.Members)
	for i := range members {
		if members[i].RoleIDs == nil {
			members[i].RoleIDs = []string{}
		}
	}
	if s.Leader != nil && s.Leader.UserID != "" {
		members = ensureOwner(members, s.Leader.UserID)
	}
	return members
}

// ensureOwner guarantees leaderID is a member with the owner role, adding the
// role or prepending the member as needed. Idempotent.
func ensureOwner(members []oldMember, leaderID string) []oldMember {
	for i := range members {
		if members[i].UserID == leaderID {
			if !slices.Contains(members[i].RoleIDs, ownerRoleID) {
				members[i].RoleIDs = append(members[i].RoleIDs, ownerRoleID)
			}
			return members
		}
	}
	return append([]oldMember{{UserID: leaderID, RoleIDs: []string{ownerRoleID}}}, members...)
}

func hasOwner(members []oldMember, userID string) bool {
	for i := range members {
		if members[i].UserID == userID {
			return slices.Contains(members[i].RoleIDs, ownerRoleID)
		}
	}
	return false
}

func dropIndex(ctx context.Context, coll *mongo.Collection, name string) {
	// Best-effort: ignore "index not found".
	_ = coll.Indexes().DropOne(ctx, name)
}

// oldWikiRevision / oldWikiArticle are frozen schema snapshots of a wiki article
// as it looked before created_by existed, decoded with typed BSON tags for the
// same reason as the settlement snapshots above: a mismatch must be a hard
// decode error, never silently dropped data.
type oldWikiRevision struct {
	AuthorName string `bson:"author_name"`
}

type oldWikiArticle struct {
	ID         any               `bson:"_id"`
	AuthorName string            `bson:"author_name"`
	Revisions  []oldWikiRevision `bson:"revisions"`
}

// wikiCreatedByUp fills created_by on every article written before the field
// existed. The history is stored newest first, so the author the page was
// created with is the last revision's; a page without history falls back to the
// last editor the document names.
//
// Idempotent: the filter matches only documents still missing the field, so a
// second run finds nothing to write.
func wikiCreatedByUp(ctx context.Context, db *mongo.Database) error {
	coll := db.Collection("wiki_articles")

	cur, err := coll.Find(ctx, bson.M{"created_by": bson.M{"$exists": false}})
	if err != nil {
		return fmt.Errorf("find wiki articles without created_by: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	for cur.Next(ctx) {
		var article oldWikiArticle
		if err := cur.Decode(&article); err != nil {
			return fmt.Errorf("decode wiki article: %w", err)
		}

		author := article.AuthorName
		if len(article.Revisions) > 0 {
			if oldest := article.Revisions[len(article.Revisions)-1].AuthorName; oldest != "" {
				author = oldest
			}
		}

		// A document that names nobody is left as it is: writing an empty value
		// would make it look backfilled and hide that nothing was known.
		if author == "" {
			continue
		}

		if _, err := coll.UpdateOne(ctx, bson.M{"_id": article.ID},
			bson.M{"$set": bson.M{"created_by": author}},
		); err != nil {
			return fmt.Errorf("backfill created_by of %v: %w", article.ID, err)
		}
	}

	return cur.Err()
}
