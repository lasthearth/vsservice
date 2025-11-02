package mongox

import (
	"github.com/go-faster/errors"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type ComputeOpts func(bson.M)

func ComputeUpdateBson(s any, opts ...ComputeOpts) (bson.M, error) {
	bytes, _ := bson.Marshal(s)
	var m bson.M
	err := bson.Unmarshal(bytes, &m)
	if err != nil {
		return nil, errors.Wrap(ErrFailToCompute, err.Error())
	}
	for _, opt := range opts {
		opt(m)
	}

	return m, nil
}

func WithoutFields(fields ...string) ComputeOpts {
	return func(m bson.M) {
		for _, field := range fields {
			delete(m, field)
		}
	}
}
