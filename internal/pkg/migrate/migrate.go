// Package migrate wires the settlement service's versioned MongoDB migrations
// on top of github.com/xakep666/mongo-migrate — a small, tested, widely-used
// (mongo-go-driver v2) migration runner. We keep a thin Run() wrapper so the
// rest of the codebase depends on our package, not the library directly.
package migrate

import (
	"context"

	migrate "github.com/xakep666/mongo-migrate"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// logger adapts our needs to the library's Logger interface (Printf-style).
type logger interface {
	Printf(format string, args ...any)
}

// Run applies every registered migration that is newer than the database's
// current version, in order. It is safe to call on every startup: the library
// records applied versions in the "migrations" collection and skips them.
//
// A failed migration returns an error so fx startup aborts — the service never
// serves against a half-migrated schema.
func Run(ctx context.Context, db *mongo.Database, log logger) error {
	m := migrate.NewMigrate(db, migrations...)
	if log != nil {
		m.SetLogger(log)
	}
	return m.Up(ctx, migrate.AllAvailable)
}
