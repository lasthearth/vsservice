package cache

import (
	"github.com/dgraph-io/ristretto/v2"
	"github.com/go-faster/errors"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrSetFailed = errors.New("cannot set the value")
)

type Manager interface {
	Get(key string) (any, error)
	Set(key string, value any) error
	Delete(key string) error
}

type Ristretto struct {
	c     *ristretto.Cache[string, any]
	scope string
}

func NewRistretto(c *ristretto.Cache[string, any]) *Ristretto {
	return &Ristretto{c: c}
}

func (r *Ristretto) Get(key string) (any, error) {
	v, found := r.c.Get(key)
	if !found {
		return nil, errors.Wrap(ErrNotFound, r.scope)
	}

	return v, nil
}

func (r *Ristretto) Set(key string, value any) error {
	set := r.c.Set(key, value, 1)
	r.c.Wait()

	if !set {
		return errors.Wrap(ErrSetFailed, r.scope)
	}

	return nil
}

func (r *Ristretto) Delete(key string) error {
	r.c.Del(key)
	return nil
}
