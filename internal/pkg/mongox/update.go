package mongox

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// updateAttempts bounds the load-modify-persist cycles before giving up.
const updateAttempts = 3

// ErrConflict reports that a document kept being modified by someone else while
// UpdateDoc was trying to persist its own change.
var ErrConflict = ierror.FailedPrecondition("document was modified concurrently, retry")

// DocStore is the slice of *mongo.Collection that UpdateDoc needs, so its
// decision logic can be exercised without a live server.
type DocStore interface {
	FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) *mongo.SingleResult
	ReplaceOne(ctx context.Context, filter any, replacement any, opts ...options.Lister[options.ReplaceOptions]) (*mongo.UpdateResult, error)
}

// enveloped is satisfied by any *DTO whose struct embeds Model — and, because
// its methods are unexported and promoted from this package, by nothing else.
type enveloped interface {
	envelope() Model
	setEnvelope(Model)
}

func (m *Model) envelope() Model     { return *m }
func (m *Model) setEnvelope(e Model) { *m = e }

// UpdateDoc runs a read-modify-persist cycle over the document matched by
// filter: load the DTO, convert it to its domain model, hand that to fn,
// convert the result back and replace the stored document.
//
// It owns what every caller used to repeat by hand: the loaded document's Model
// envelope (_id, created_at) is carried over, updated_at is stamped once, and
// mongo.ErrNoDocuments becomes notFound. When the model exposes Touch(time.Time)
// the returned model is stamped too, so callers see the persisted value.
//
// The replace is guarded by the updated_at that was read, so a concurrent writer
// is never silently overwritten: the cycle is retried against fresh state
// instead, and ErrConflict is returned if it keeps losing.
func UpdateDoc[D any, PD interface {
	*D
	enveloped
}, M any](
	ctx context.Context,
	coll DocStore,
	filter bson.M,
	notFound error,
	toModel func(D) *M,
	fromModel func(*M) D,
	fn func(context.Context, *M) (*M, error),
) (*M, error) {
	guardField := updatedAtField[D, PD]()

	for range updateAttempts {
		var d D
		if err := coll.FindOne(ctx, filter).Decode(&d); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, notFound
			}
			return nil, err
		}
		loaded := PD(&d).envelope()

		updated, err := fn(ctx, toModel(d))
		if err != nil {
			return nil, err
		}

		// BSON datetime keeps milliseconds, so a truncated stamp round-trips
		// exactly and can serve as the next writer's guard value.
		now := time.Now().UTC().Truncate(time.Millisecond)
		if t, ok := any(updated).(interface{ Touch(time.Time) }); ok {
			t.Touch(now)
		}

		out := fromModel(updated)
		PD(&out).setEnvelope(Model{
			Id:        loaded.Id,
			CreatedAt: loaded.CreatedAt,
			UpdatedAt: now,
		})

		res, err := coll.ReplaceOne(ctx, guard(filter, guardField, loaded.UpdatedAt), out)
		if err != nil {
			return nil, err
		}
		if res.MatchedCount > 0 {
			return updated, nil
		}
		// Nothing matched: the document moved on (or was deleted) since the read.
		// Reload and reapply fn to the fresh state.
	}

	return nil, ErrConflict
}

// guard pins the replace to the document state that was read. Documents written
// before updated_at was maintained carry no such field, and a nil match covers
// both missing and null, so no backfill is required.
func guard(filter bson.M, field string, loaded time.Time) bson.M {
	pin := bson.M{field: loaded}
	if loaded.IsZero() {
		pin = bson.M{"$or": bson.A{bson.M{field: nil}, bson.M{field: loaded}}}
	}
	return bson.M{"$and": bson.A{filter, pin}}
}

// updatedAtField locates the envelope's updated_at in the stored document.
// Most DTOs embed Model with `bson:",inline"` and keep it at the top level; a
// DTO that embeds Model untagged nests it under the field name instead.
func updatedAtField[D any, PD interface {
	*D
	enveloped
}]() string {
	t := reflect.TypeFor[D]()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.Anonymous || f.Type != reflect.TypeFor[Model]() {
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("bson"), ",")
		if strings.Contains(opts, "inline") {
			break
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		return name + ".updated_at"
	}
	return "updated_at"
}
