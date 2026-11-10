package dto_test

import (
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/lfg/internal/dto"
	"github.com/lasthearth/vsservice/internal/lfg/internal/model"
	"github.com/lasthearth/vsservice/internal/lfg/internal/repository/repomapper"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var now = time.Date(2026, 10, 7, 18, 0, 0, 0, time.UTC)

// persisted walks the whole write path a Respond goes through: model → mapper →
// BSON document → mapper → model. A break anywhere in it makes the notification
// dedupe a silent no-op, because Respond reads Notified back from the stored
// document on the next call.
func persisted(t *testing.T, p *model.Post) *model.Post {
	t.Helper()

	var m repomapper.MapperImpl
	doc := m.FromModel(*p)

	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back dto.Post
	if err := bson.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	loaded := m.ToModel(back)
	return &loaded
}

func sessionPost(t *testing.T) *model.Post {
	t.Helper()
	starts := now.Add(20 * time.Minute)
	p, err := model.NewPost("author", model.Details{
		Kind:       model.KindSession,
		Activities: []model.Activity{model.ActivityMining},
		Title:      "Идём в шахту",
		StartsAt:   &starts,
		Slots:      3,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestNotifiedSurvivesThePersistRoundTrip is the regression for the audit's
// finding that a model-only dedupe set is dropped on every write: the field has
// to reach the document under a stable key and come back out of it.
func TestNotifiedSurvivesThePersistRoundTrip(t *testing.T) {
	p := sessionPost(t)

	// join, withdraw, rejoin: Notified holds "a" once while Responders holds it
	// again after the rejoin.
	if _, notify, err := p.Respond("a", now); err != nil || !notify {
		t.Fatalf("first join: notify=%v err=%v", notify, err)
	}
	if _, _, err := p.Respond("a", now); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if _, notify, err := p.Respond("a", now); err != nil || notify {
		t.Fatalf("rejoin: notify=%v err=%v", notify, err)
	}
	if _, notify, err := p.Respond("b", now.Add(time.Minute)); err != nil || !notify {
		t.Fatalf("b joins: notify=%v err=%v", notify, err)
	}

	raw, err := bson.Marshal((&repomapper.MapperImpl{}).FromModel(*p))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bson.Raw(raw).LookupErr("notified"); err != nil {
		t.Fatalf("the stored document has no \"notified\" key: %v (%v)", raw, err)
	}

	loaded := persisted(t, p)
	if got := loaded.Notified; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("Notified did not survive the round trip: %v", got)
	}

	// The point of persisting it: the reloaded post must not notify the author
	// again for a responder it already announced, while a new responder still
	// does.
	if joined, _, err := loaded.Respond("a", now); err != nil || joined {
		t.Fatalf("a withdraws from the reloaded post: joined=%v err=%v", joined, err)
	}
	if joined, notify, err := loaded.Respond("a", now); err != nil || !joined || notify {
		t.Fatalf("a rejoins the reloaded post: joined=%v notify=%v err=%v", joined, notify, err)
	}
	if _, notify, err := loaded.Respond("c", now); err != nil || !notify {
		t.Fatalf("a new responder on the reloaded post: notify=%v err=%v", notify, err)
	}
}

// omitempty keeps the field out of the documents of every post nobody responded
// to, which is most of the collection.
func TestNotifiedIsOmittedWhenEmpty(t *testing.T) {
	p := sessionPost(t)

	raw, err := bson.Marshal((&repomapper.MapperImpl{}).FromModel(*p))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bson.Raw(raw).LookupErr("notified"); err == nil {
		t.Fatalf("an empty Notified must not be stored, got %v", raw)
	}
}

// Posts written before the field existed decode with Notified nil, and Respond
// then treats their responders as never announced: one more notification per
// already-seen responder, once, and never again.
func TestLegacyDocumentDecodesWithoutNotified(t *testing.T) {
	p := sessionPost(t)
	if _, _, err := p.Respond("a", now); err != nil {
		t.Fatal(err)
	}

	doc := (&repomapper.MapperImpl{}).FromModel(*p)
	doc.Notified = nil // what an old document holds
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	var back dto.Post
	if err := bson.Unmarshal(raw, &back); err != nil {
		t.Fatalf("legacy document must still decode: %v", err)
	}
	loaded := (&repomapper.MapperImpl{}).ToModel(back)

	if loaded.Notified != nil {
		t.Fatalf("legacy Notified: %v", loaded.Notified)
	}
	if !loaded.HasResponder("a") {
		t.Fatalf("legacy responders lost: %v", loaded.Responders)
	}
	if _, _, err := loaded.Respond("a", now); err != nil {
		t.Fatalf("legacy withdraw: %v", err)
	}
	if joined, notify, err := loaded.Respond("a", now); err != nil || !joined || !notify {
		t.Fatalf("legacy rejoin: joined=%v notify=%v err=%v", joined, notify, err)
	}
	if _, notify, err := loaded.Respond("a", now); err != nil || notify {
		t.Fatalf("legacy withdraw after the extra notification: notify=%v err=%v", notify, err)
	}
	if joined, notify, err := loaded.Respond("a", now); err != nil || !joined || notify {
		t.Fatalf("legacy second rejoin: joined=%v notify=%v err=%v", joined, notify, err)
	}
}
