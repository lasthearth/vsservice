package cache

import (
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
