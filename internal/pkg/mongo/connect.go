package mongo

import (
	"context"
	"os"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func New(cfg config.Config) *mongo.Client {
	file, err := os.ReadFile(cfg.MongoUrlFile)
	if err != nil {
		panic(err)
	}

	client, err := mongo.Connect(options.Client().ApplyURI(string(file)))
	if err != nil {
		panic(err)
	}

	err = client.Ping(context.Background(), nil)
	if err != nil {
		panic(err)
	}
	return client
}

func NewDatabase(c *mongo.Client) *mongo.Database {
	return c.Database("lsp")
}
