package repository

import (
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func aboutPtr(s string) *string { return &s }

// TestBuildUpdateSetWritesTheDescriptionAsOneDecision pins that about and
// about_updated_at are written and removed together. MongoDB rejects an update
// that names one path in both $set and $unset, and it rejects the whole write:
// not only the description, but every other field of the same PlayerUpdate.
func TestBuildUpdateSetWritesTheDescriptionAsOneDecision(t *testing.T) {
	r := &Repository{}
	stamp := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)

	t.Run("a description and its stamp are set together", func(t *testing.T) {
		got := r.buildUpdateSet(model.PlayerUpdate{About: aboutPtr("лес"), AboutUpdatedAt: &stamp})

		set, ok := got["$set"].(bson.M)
		if !ok {
			t.Fatalf("$set = %v, want the written fields", got["$set"])
		}
		if set["about"] != "лес" || set["about_updated_at"] != stamp {
			t.Errorf("$set = %v, want the description and its stamp", set)
		}
		if unset, present := got["$unset"]; present {
			t.Errorf("$unset = %v, want none", unset)
		}
	})

	t.Run("a removal takes the stamp with it even when a caller passes one", func(t *testing.T) {
		got := r.buildUpdateSet(model.PlayerUpdate{About: aboutPtr(""), AboutUpdatedAt: &stamp})

		unset, ok := got["$unset"].(bson.M)
		if !ok {
			t.Fatalf("$unset = %v, want the removed fields", got["$unset"])
		}
		if _, present := unset["about"]; !present {
			t.Error("$unset does not remove about")
		}
		if _, present := unset["about_updated_at"]; !present {
			t.Error("$unset does not remove about_updated_at")
		}
		if set, present := got["$set"]; present {
			t.Errorf("$set = %v, want none: about_updated_at in both operators makes MongoDB reject the write", set)
		}
	})

	t.Run("a removal keeps the other fields of the same update", func(t *testing.T) {
		online := true
		got := r.buildUpdateSet(model.PlayerUpdate{About: aboutPtr(""), IsOnline: &online})

		set, ok := got["$set"].(bson.M)
		if !ok || set["is_online"] != true {
			t.Fatalf("$set = %v, want is_online", got["$set"])
		}
		if _, ok := got["$unset"].(bson.M); !ok {
			t.Error("$unset is missing, so the description would survive the hide")
		}
	})

	t.Run("an update that carries nothing sends no operator", func(t *testing.T) {
		// MongoDB rejects an update document with no operators, so the callers
		// skip the write instead of sending this.
		if got := r.buildUpdateSet(model.PlayerUpdate{}); len(got) != 0 {
			t.Errorf("update document = %v, want it empty", got)
		}
	})

	// A grant clears the recorded change; a rename writes one. Both in the same
	// document would name last_nickname_changed_at in $set and $unset, and
	// MongoDB rejects the whole update for the path conflict.
	t.Run("a grant removes the recorded change", func(t *testing.T) {
		got := r.buildUpdateSet(model.PlayerUpdate{ClearNicknameCooldown: true})

		unset, ok := got["$unset"].(bson.M)
		if !ok {
			t.Fatalf("$unset = %v, want the removed field", got["$unset"])
		}
		if _, present := unset["last_nickname_changed_at"]; !present {
			t.Error("$unset does not remove last_nickname_changed_at")
		}
		if set, present := got["$set"]; present {
			t.Errorf("$set = %v, want none: the field cannot be both written and removed", set)
		}
	})

	t.Run("a grant wins over a timestamp in the same update", func(t *testing.T) {
		got := r.buildUpdateSet(model.PlayerUpdate{
			ClearNicknameCooldown: true,
			LastNicknameChangedAt: &stamp,
		})

		if set, present := got["$set"]; present {
			t.Errorf("$set = %v, want none", set)
		}
		if _, ok := got["$unset"].(bson.M); !ok {
			t.Error("$unset is missing, so the cooldown would survive the grant")
		}
	})
}
