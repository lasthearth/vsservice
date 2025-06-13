package storage

import (
	"github.com/minio/minio-go/v7"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In
	Client *minio.Client
}

type Storage struct {
	client *minio.Client
}

func New(opts Opts) *Storage {
	return &Storage{
		client: opts.Client,
	}
}
