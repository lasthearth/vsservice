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
// The replace is guarded by the version that was read and writes version+1, so a
// concurrent writer is never silently overwritten: the cycle is retried against
// fresh state instead, and ErrConflict is returned if it keeps losing.
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
	guardField := versionField[D, PD]()

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
		// exactly. It is reported to callers, but it is the version that guards
		// the write.
		now := time.Now().UTC().Truncate(time.Millisecond)
		if t, ok := any(updated).(interface{ Touch(time.Time) }); ok {
			t.Touch(now)
		}

		out := fromModel(updated)
		PD(&out).setEnvelope(Model{
			Id:        loaded.Id,
			CreatedAt: loaded.CreatedAt,
			UpdatedAt: now,
			Version:   loaded.Version + 1,
		})

		res, err := coll.ReplaceOne(ctx, guard(filter, guardField, loaded.Version), out)
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

// guard pins the replace to the document version that was read. Documents
// written before version was maintained carry no such field and decode as 0, and
// a nil match covers both missing and zero, so no backfill is required.
func guard(filter bson.M, field string, loaded int64) bson.M {
	pin := bson.M{field: loaded}
	if loaded == 0 {
		pin = bson.M{"$or": bson.A{bson.M{field: nil}, bson.M{field: int64(0)}}}
	}
	return bson.M{"$and": bson.A{filter, pin}}
}

// versionField locates the envelope's version in the stored document.
// Most DTOs embed Model with `bson:",inline"` and keep it at the top level; a
// DTO that embeds Model untagged nests it under the field name instead.
func versionField[D any, PD interface {
	*D
	enveloped
}]() string {
	for f := range reflect.TypeFor[D]().Fields() {
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
		return name + ".version"
	}
	return "version"
}
