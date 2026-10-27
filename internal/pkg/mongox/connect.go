package mongox

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/migrate"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// New builds the MongoDB client and attaches the command monitor that makes
// driver operations visible in the application log. It still panics on
// failure — fx startup cannot continue without storage — but the reason is
// logged first, because a panic during boot otherwise reaches the operator
// without the fields the record would have carried. The connection string is
// never logged: it holds credentials.
func New(cfg config.Config, log logger.Logger) *mongo.Client {
	l := log.WithComponent(componentMongo)

	file, err := os.ReadFile(cfg.MongoUrlFile)
	if err != nil {
		l.Error("read mongo url file failed", zap.String("path", cfg.MongoUrlFile), zap.Error(err))
		panic(err)
	}

	monitor := newCommandMonitor(log, cfg.MongoSlowMS)

	client, err := mongo.Connect(
		options.Client().
			ApplyURI(string(file)).
			SetMonitor(monitor.monitor()),
	)
	if err != nil {
		l.Error("mongo connect failed", zap.Error(err))
		panic(err)
	}

	err = client.Ping(context.Background(), nil)
	if err != nil {
		l.Error("mongo ping failed", zap.Error(err))
		panic(err)
	}
	return client
}

// NewDatabase resolves the application database and runs pending schema
// migrations before returning. Repositories depend on *mongo.Database, so this
// runs exactly once, before any repository builds its indexes. A failed
// migration returns an error and aborts fx startup — the service never serves
// against an un-migrated schema.
func NewDatabase(c *mongo.Client, log logger.Logger) (*mongo.Database, error) {
	db := c.Database("lsp")

	l := log.WithComponent("migrate")
	l.Info("running migrations")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := migrate.Run(ctx, db, &migrateLogger{l}); err != nil {
		l.Error("migration failed", zap.Error(err))
		return nil, err
	}
	l.Info("migrations up to date")
	return db, nil
}

// migrateLogger adapts our zap-based logger to the migration runner's
// Printf-style Logger interface.
type migrateLogger struct{ l logger.Logger }

func (m *migrateLogger) Printf(format string, args ...any) {
	m.l.Info(fmt.Sprintf(format, args...))
}
