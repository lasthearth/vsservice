// Package pagination implements cursor (keyset) pagination over MongoDB
// collections.
//
// List is the entry point. The cursor it hands out identifies the last document
// of the page by its primary sort key plus its `_id`, so paging stays exact
// even when the caller sorts by a non-unique field. List fetches one document
// more than requested to decide whether a next page exists, and converts the
// DTOs it decoded into domain models. An empty page is an empty slice and a nil
// error, never an error.
package pagination

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"maps"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Limits applied to every listing. A non-positive requested limit falls back to
// DefaultLimit; anything above MaxLimit is clamped.
const (
	DefaultLimit int64 = 25
	MaxLimit     int64 = 100
)

type Options struct {
	limit  int64
	sort   bson.D
	filter bson.M
	next   string
}

// token is the decoded page cursor: the `_id` of the last document of the
// previous page and, when the caller sorted by another field, that field's
// name and value on the same document.
type token struct {
	Oid bson.ObjectID `bson:"oid"`
	Key string        `bson:"k,omitempty"`
	Val bson.RawValue `bson:"v,omitempty"`
}

func encodeToken(t token) (string, error) {
	encoded, err := bson.Marshal(t)
	if err != nil {
		return "", err
	}

	return base64.RawStdEncoding.EncodeToString(encoded), nil
}

func decodeToken(st string) (token, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(st)
	if err != nil {
		return token{}, err
	}

	// Cursors used to be base64(JSON). A BSON document starts with its own
	// little-endian length, so anything else is a token handed out before the
	// format change — keep reading those so in-flight pagination survives a
	// deploy instead of erroring.
	if !isBSONDocument(decoded) {
		var legacy struct {
			Oid bson.ObjectID `json:"oid"`
		}
		if err := json.Unmarshal(decoded, &legacy); err != nil {
			return token{}, err
		}
		return token{Oid: legacy.Oid}, nil
	}

	var t token
	err = bson.Unmarshal(decoded, &t)
	return t, err
}

func isBSONDocument(b []byte) bool {
	if len(b) < 5 {
		return false
	}

	length := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16 | int32(b[3])<<24
	return int(length) == len(b)
}

type OptionFn func(*Options) error

// WithLimit requests a page size. Resolved against DefaultLimit/MaxLimit.
func WithLimit(limit int64) OptionFn {
	return func(o *Options) error {
		o.limit = limit
		return nil
	}
}

// WithSort sets the sort order. An `_id` tiebreaker is appended if absent; the
// cursor is built from the resolved sort, so any order pages correctly.
func WithSort(sort bson.D) OptionFn {
	return func(o *Options) error {
		o.sort = sort
		return nil
	}
}

// WithFilter sets the query filter. The map is copied before the cursor is
// folded in, so the caller's map is never mutated.
func WithFilter(filter bson.M) OptionFn {
	return func(o *Options) error {
		o.filter = filter
		return nil
	}
}

// WithNext carries the page token. It only records the token — the cursor
// comparison is built once every option has been applied, so WithNext and
// WithFilter are order-independent.
func WithNext(next string) OptionFn {
	return func(o *Options) error {
		o.next = next
		return nil
	}
}

func defaultOptions() Options {
	return Options{
		limit:  DefaultLimit,
		sort:   bson.D{{Key: "_id", Value: -1}},
		filter: bson.M{},
	}
}

type Identifiable interface {
	Id() bson.ObjectID
}

// query is the fully resolved MongoDB query: cursor folded into the filter,
// limit clamped, `_id` guaranteed present in the sort.
type query struct {
	filter bson.M
	sort   bson.D
	limit  int64
}

func resolve(opts []OptionFn) (query, error) {
	o := defaultOptions()
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return query{}, err
		}
	}

	limit := o.limit
	switch {
	case limit <= 0:
		limit = DefaultLimit
	case limit > MaxLimit:
		limit = MaxLimit
	}

	sort := withIDTiebreaker(o.sort)

	filter := bson.M{}
	maps.Copy(filter, o.filter)

	if o.next != "" {
		t, err := decodeToken(o.next)
		if err != nil {
			return query{}, err
		}

		if !t.Oid.IsZero() {
			cursor := cursorFilter(sort, t)
			if len(filter) > 0 {
				// $and instead of merging keys: the caller's filter may already
				// use $or or constrain _id itself.
				filter = bson.M{"$and": bson.A{filter, cursor}}
			} else {
				filter = cursor
			}
		}
	}

	return query{filter: filter, sort: sort, limit: limit}, nil
}

// withIDTiebreaker guarantees the sort ends with `_id`, the unique key that
// makes the order — and therefore the cursor — deterministic. Its direction
// follows the last sort key.
func withIDTiebreaker(sort bson.D) bson.D {
	for _, e := range sort {
		if e.Key == "_id" {
			return sort
		}
	}

	dir := -1
	if len(sort) > 0 && ascending(sort[len(sort)-1].Value) {
		dir = 1
	}

	return append(append(bson.D{}, sort...), bson.E{Key: "_id", Value: dir})
}

// primaryKey returns the first sort key that is not `_id`, i.e. the field the
// caller actually ordered by. Absent when sorting by `_id` alone.
func primaryKey(sort bson.D) (bson.E, bool) {
	for _, e := range sort {
		if e.Key != "_id" {
			return e, true
		}
	}
	return bson.E{}, false
}

func idDirection(sort bson.D) any {
	for _, e := range sort {
		if e.Key == "_id" {
			return e.Value
		}
	}
	return -1
}

// op picks the comparison that walks the collection in sort order: `$gt` for an
// ascending key, `$lt` for a descending one.
func op(direction any) string {
	if ascending(direction) {
		return "$gt"
	}
	return "$lt"
}

// cursorFilter turns a token into the "everything after the previous page"
// clause. With a primary sort key it is a compound keyset comparison — skipping
// straight to `_id` would drop documents that sort after the cursor on the
// primary key but happen to have a smaller `_id`.
func cursorFilter(sort bson.D, t token) bson.M {
	idClause := bson.M{"_id": bson.M{op(idDirection(sort)): t.Oid}}

	primary, ok := primaryKey(sort)
	if !ok || t.Key != primary.Key || t.Val.IsZero() {
		return idClause
	}

	return bson.M{"$or": bson.A{
		bson.M{t.Key: bson.M{op(primary.Value): t.Val}},
		bson.M{t.Key: t.Val, "_id": idClause["_id"]},
	}}
}

// sortValue reads the primary sort field off a decoded document. Nested or
// absent fields yield a zero value, which degrades the cursor to `_id` only.
func sortValue(doc any, key string) bson.RawValue {
	raw, err := bson.Marshal(doc)
	if err != nil {
		return bson.RawValue{}
	}

	rv, err := bson.Raw(raw).LookupErr(key)
	if err != nil {
		return bson.RawValue{}
	}

	return rv
}

// page trims a limit+1 lookahead to the requested size and emits a cursor only
// when the extra document actually came back, so an exhausted list never hands
// the client a token that answers with an empty page.
func page[T Identifiable](datas []T, limit int64, sort bson.D) ([]T, string, error) {
	if int64(len(datas)) <= limit {
		return datas, "", nil
	}

	datas = datas[:limit]
	last := datas[len(datas)-1]

	t := token{Oid: last.Id()}
	if primary, ok := primaryKey(sort); ok {
		t.Key = primary.Key
		t.Val = sortValue(last, primary.Key)
	}

	next, err := encodeToken(t)
	if err != nil {
		return nil, "", err
	}

	return datas, next, nil
}

// List returns one page of coll converted to domain models, plus the token for
// the next page (empty when there is none). An empty page is not an error.
func List[D Identifiable, M any](
	ctx context.Context,
	coll *mongo.Collection,
	next string,
	limit int64,
	convert func(D) M,
	opts ...OptionFn,
) ([]M, string, error) {
	all := make([]OptionFn, 0, len(opts)+2)
	all = append(all, opts...)
	all = append(all, WithLimit(limit), WithNext(next))

	q, err := resolve(all)
	if err != nil {
		return nil, "", err
	}

	// One extra document tells us whether a next page exists.
	findOpts := options.Find().SetSort(q.sort).SetLimit(q.limit + 1)
	cursor, err := coll.Find(ctx, q.filter, findOpts)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var datas []D
	if err := cursor.All(ctx, &datas); err != nil {
		return nil, "", err
	}

	datas, nextToken, err := page(datas, q.limit, q.sort)
	if err != nil {
		return nil, "", err
	}

	items := make([]M, len(datas))
	for i, d := range datas {
		items[i] = convert(d)
	}

	return items, nextToken, nil
}

// ascending reports whether a bson sort direction means ascending. Directions
// arrive either as plain ints or as named types such as orderby.Direction,
// hence the reflection.
func ascending(v any) bool {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() > 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() > 0
	default:
		return false
	}
}
