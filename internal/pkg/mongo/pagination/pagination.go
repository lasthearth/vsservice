package pagination

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Options struct {
	limit  int64
	sort   bson.D
	filter bson.M
	next   token
}

type token struct {
	Oid bson.ObjectID `json:"oid"`
}

func decodeToken(st string) (token, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(st)
	if err != nil {
		return token{}, err
	}

	var t token
	err = json.Unmarshal(decoded, &t)
	return t, err
}

func encodeToken(t token) (string, error) {
	encoded, err := json.Marshal(t)
	if err != nil {
		return "", err
	}

	return base64.RawStdEncoding.EncodeToString(encoded), nil
}

type OptionFn func(*Options) error

func WithLimit(limit int64) OptionFn {
	return func(o *Options) error {
		o.limit = limit
		return nil
	}
}

func WithSort(sort bson.D) OptionFn {
	return func(o *Options) error {
		o.sort = sort
		return nil
	}
}

func WithFilter(filter bson.M) OptionFn {
	return func(o *Options) error {
		o.filter = filter
		return nil
	}
}

func WithNext(next string) OptionFn {
	return func(o *Options) error {
		t, err := decodeToken(next)
		if err != nil {
			return err
		}

		o.next = t

		if t.Oid.IsZero() {
			return nil
		}

		o.filter["_id"] = bson.M{"$lt": t.Oid}
		return nil
	}
}

func defaultOptions() Options {
	sort := bson.D{
		{Key: "_id", Value: -1},
	}

	return Options{
		limit:  25,
		sort:   sort,
		filter: bson.M{},
		next:   token{},
	}
}

type Response[T any] struct {
	Data T
	Next string
}

type Identifiable interface {
	Id() bson.ObjectID
}

func Find[T Identifiable](
	ctx context.Context,
	coll *mongo.Collection,
	opts ...OptionFn,
) (*Response[[]T], error) {
	pgOpts := defaultOptions()
	for _, opt := range opts {
		opt(&pgOpts)
	}

	findOpts := options.Find().SetSort(pgOpts.sort).SetLimit(int64(pgOpts.limit))
	cursor, err := coll.Find(ctx, pgOpts.filter, findOpts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var datas []T
	if err := cursor.All(ctx, &datas); err != nil {
		return nil, err
	}

	if len(datas) == 0 {
		return nil, errors.New("no data found")
	}

	next := ""
	if len(datas) == int(pgOpts.limit) {
		id := datas[len(datas)-1].Id()

		next, err = encodeToken(token{Oid: id})
		if err != nil {
			return nil, err
		}
	}

	return &Response[[]T]{
		Data: datas,
		Next: next,
	}, nil
}
