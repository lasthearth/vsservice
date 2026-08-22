package mongox

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// --- fixtures ---------------------------------------------------------------

type testDoc struct {
	Model `bson:",inline"`
	Name  string `bson:"name"`
}

type nestedDoc struct {
	Model
	Name string `bson:"name"`
}

type testModel struct {
	Name      string
	UpdatedAt time.Time
}

func (m *testModel) Touch(now time.Time) { m.UpdatedAt = now }

func toTestModel(d testDoc) *testModel {
	return &testModel{Name: d.Name, UpdatedAt: d.UpdatedAt}
}

func fromTestModel(m *testModel) testDoc {
	return testDoc{Name: m.Name}
}

// fakeStore replays a scripted sequence of ReplaceOne outcomes and records what
// it was asked to do, so UpdateDoc's decision logic runs without a server.
type fakeStore struct {
	doc      testDoc
	findErr  error
	matched  []int64 // MatchedCount per ReplaceOne call, last value repeats
	replErr  error
	writes   []testDoc
	filters  []bson.M
	findCall int
	replCall int
}

func (f *fakeStore) FindOne(_ context.Context, _ any, _ ...options.Lister[options.FindOneOptions]) *mongo.SingleResult {
	f.findCall++
	return mongo.NewSingleResultFromDocument(f.doc, f.findErr, nil)
}

func (f *fakeStore) ReplaceOne(
	_ context.Context,
	filter any,
	replacement any,
	_ ...options.Lister[options.ReplaceOptions],
) (*mongo.UpdateResult, error) {
	f.replCall++
	f.filters = append(f.filters, filter.(bson.M))
	f.writes = append(f.writes, replacement.(testDoc))
	if f.replErr != nil {
		return nil, f.replErr
	}
	matched := int64(1)
	if len(f.matched) > 0 {
		matched = f.matched[min(f.replCall, len(f.matched))-1]
	}
	return &mongo.UpdateResult{MatchedCount: matched}, nil
}

var errNotFound = errors.New("not found")

func storedDoc() testDoc {
	return testDoc{
		Model: Model{
			Id:        bson.NewObjectID(),
			CreatedAt: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
			UpdatedAt: time.Date(2021, 6, 7, 8, 9, 10, 0, time.UTC),
			Version:   7,
		},
		Name: "before",
	}
}

func rename(to string) func(context.Context, *testModel) (*testModel, error) {
	return func(_ context.Context, m *testModel) (*testModel, error) {
		m.Name = to
		return m, nil
	}
}

// --- tests ------------------------------------------------------------------

func TestUpdateDocPersistsCallbackResult(t *testing.T) {
	stored := storedDoc()
	f := &fakeStore{doc: stored}
	before := time.Now().UTC().Add(-time.Millisecond)

	got, err := UpdateDoc(context.Background(), f, bson.M{"name": "before"}, errNotFound,
		toTestModel, fromTestModel, rename("after"))
	if err != nil {
		t.Fatalf("UpdateDoc: %v", err)
	}
	if got.Name != "after" {
		t.Errorf("returned model Name = %q, want %q", got.Name, "after")
	}
	if len(f.writes) != 1 {
		t.Fatalf("ReplaceOne called %d times, want 1", len(f.writes))
	}

	w := f.writes[0]
	if w.Name != "after" {
		t.Errorf("persisted Name = %q, want %q — the callback's model is not what got written", w.Name, "after")
	}
	if w.Id != stored.Id {
		t.Errorf("persisted _id = %v, want %v (envelope must survive)", w.Id, stored.Id)
	}
	if !w.CreatedAt.Equal(stored.CreatedAt) {
		t.Errorf("persisted created_at = %v, want %v (envelope must survive)", w.CreatedAt, stored.CreatedAt)
	}
	if !w.UpdatedAt.After(before) {
		t.Errorf("persisted updated_at = %v, want a fresh stamp after %v", w.UpdatedAt, before)
	}
	if w.UpdatedAt.Truncate(time.Millisecond) != w.UpdatedAt {
		t.Errorf("persisted updated_at = %v, want millisecond precision so it round-trips through BSON", w.UpdatedAt)
	}
	if !got.UpdatedAt.Equal(w.UpdatedAt) {
		t.Errorf("returned model updated_at = %v, want the persisted %v", got.UpdatedAt, w.UpdatedAt)
	}
}

func TestGuardFieldMatchesEncodedDocument(t *testing.T) {
	// The guard names a field path; if that path is not where the DTO actually
	// encodes updated_at, every replace silently matches nothing.
	for _, tc := range []struct {
		name string
		doc  any
		path string
	}{
		{"inline envelope", testDoc{Model: Model{Version: 1}}, versionField[testDoc, *testDoc]()},
		{"untagged embedded envelope", nestedDoc{Model: Model{Version: 1}}, versionField[nestedDoc, *nestedDoc]()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := bson.Marshal(tc.doc)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if _, err := bson.Raw(raw).LookupErr(strings.Split(tc.path, ".")...); err != nil {
				t.Errorf("guard reads %q, which the encoded document does not have: %v", tc.path, err)
			}
		})
	}
}

func TestUpdateDocNotFound(t *testing.T) {
	f := &fakeStore{doc: storedDoc(), findErr: mongo.ErrNoDocuments}

	_, err := UpdateDoc(context.Background(), f, bson.M{}, errNotFound,
		toTestModel, fromTestModel, rename("after"))
	if !errors.Is(err, errNotFound) {
		t.Fatalf("err = %v, want the caller's notFound error", err)
	}
	if len(f.writes) != 0 {
		t.Errorf("wrote %d documents, want 0", len(f.writes))
	}
}

func TestUpdateDocFindErrorIsPropagated(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeStore{doc: storedDoc(), findErr: boom}

	_, err := UpdateDoc(context.Background(), f, bson.M{}, errNotFound,
		toTestModel, fromTestModel, rename("after"))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if len(f.writes) != 0 {
		t.Errorf("wrote %d documents, want 0", len(f.writes))
	}
}

func TestUpdateDocCallbackErrorAbortsWithoutWriting(t *testing.T) {
	boom := errors.New("no")
	f := &fakeStore{doc: storedDoc()}

	_, err := UpdateDoc(context.Background(), f, bson.M{}, errNotFound,
		toTestModel, fromTestModel,
		func(context.Context, *testModel) (*testModel, error) { return nil, boom },
	)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if len(f.writes) != 0 {
		t.Errorf("wrote %d documents, want 0", len(f.writes))
	}
}

func TestUpdateDocGuardsReplaceWithLoadedUpdatedAt(t *testing.T) {
	stored := storedDoc()
	f := &fakeStore{doc: stored}

	if _, err := UpdateDoc(context.Background(), f, bson.M{"_id": stored.Id}, errNotFound,
		toTestModel, fromTestModel, rename("after")); err != nil {
		t.Fatalf("UpdateDoc: %v", err)
	}

	want := guard(bson.M{"_id": stored.Id}, "version", stored.Version)
	if got := f.filters[0]; !sameFilter(got, want) {
		t.Errorf("replace filter = %v, want %v", got, want)
	}
}

func TestUpdateDocRetriesWhenGuardRejectsStaleWrite(t *testing.T) {
	f := &fakeStore{doc: storedDoc(), matched: []int64{0, 1}}
	calls := 0

	got, err := UpdateDoc(context.Background(), f, bson.M{}, errNotFound,
		toTestModel, fromTestModel,
		func(_ context.Context, m *testModel) (*testModel, error) {
			calls++
			m.Name = "after"
			return m, nil
		},
	)
	if err != nil {
		t.Fatalf("UpdateDoc: %v", err)
	}
	if got.Name != "after" {
		t.Errorf("returned Name = %q, want %q", got.Name, "after")
	}
	if calls != 2 || f.findCall != 2 {
		t.Errorf("callback ran %d times over %d reads, want 2 and 2 — a rejected write must reload and reapply", calls, f.findCall)
	}
}

func TestUpdateDocReturnsConflictWhenGuardKeepsRejecting(t *testing.T) {
	f := &fakeStore{doc: storedDoc(), matched: []int64{0}}

	_, err := UpdateDoc(context.Background(), f, bson.M{}, errNotFound,
		toTestModel, fromTestModel, rename("after"))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	if f.replCall != updateAttempts {
		t.Errorf("attempted %d writes, want %d", f.replCall, updateAttempts)
	}
}

func TestUpdateDocReplaceErrorIsPropagated(t *testing.T) {
	boom := errors.New("write failed")
	f := &fakeStore{doc: storedDoc(), replErr: boom}

	_, err := UpdateDoc(context.Background(), f, bson.M{}, errNotFound,
		toTestModel, fromTestModel, rename("after"))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestGuard(t *testing.T) {
	filter := bson.M{"player_id": "p1"}

	t.Run("known version pins equality", func(t *testing.T) {
		got := guard(filter, "version", 7)
		want := bson.M{"$and": bson.A{filter, bson.M{"version": int64(7)}}}
		if !sameFilter(got, want) {
			t.Errorf("guard = %v, want %v", got, want)
		}
	})

	t.Run("zero version also matches a missing field", func(t *testing.T) {
		// Documents predating the counter carry no version at all; the guard has
		// to accept those without a migration.
		got := guard(filter, "version", 0)
		want := bson.M{"$and": bson.A{filter, bson.M{"$or": bson.A{
			bson.M{"version": nil},
			bson.M{"version": int64(0)},
		}}}}
		if !sameFilter(got, want) {
			t.Errorf("guard = %v, want %v", got, want)
		}
	})
}

func TestVersionField(t *testing.T) {
	if got := versionField[testDoc, *testDoc](); got != "version" {
		t.Errorf("inline envelope field = %q, want %q", got, "version")
	}
	if got := versionField[nestedDoc, *nestedDoc](); got != "model.version" {
		t.Errorf("untagged embedded envelope field = %q, want %q", got, "model.version")
	}
}

// The guard must be a monotonic counter, not a millisecond timestamp. With a
// timestamp, two writes inside one millisecond leave updated_at unchanged, so a
// third writer holding that same value still matches and its stale write lands
// (ABA). A version always advances, so the loser is always rejected.
func TestUpdateDocAdvancesVersionSoAStaleGuardCannotMatch(t *testing.T) {
	stored := storedDoc()
	f := &fakeStore{doc: stored}

	if _, err := UpdateDoc(context.Background(), f, bson.M{"_id": stored.Id}, errNotFound,
		toTestModel, fromTestModel, rename("after")); err != nil {
		t.Fatalf("UpdateDoc: %v", err)
	}

	w := f.writes[0]
	if w.Version != stored.Version+1 {
		t.Errorf("persisted version = %d, want %d (the guard must advance)", w.Version, stored.Version+1)
	}

	// The next writer's guard is the version it read. Whatever this write
	// persisted must not equal what it guarded on, or a same-millisecond
	// interleaving could satisfy it twice.
	guarded := guard(bson.M{"_id": stored.Id}, "version", stored.Version)
	if sameFilter(guarded, guard(bson.M{"_id": stored.Id}, "version", w.Version)) {
		t.Error("guard value did not change across the write: a stale writer could still match")
	}
}

// sameFilter compares two filters structurally.
func sameFilter(a, b bson.M) bool {
	return reflect.DeepEqual(a, b)
}
