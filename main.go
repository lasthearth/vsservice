package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/go-faster/errors"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/ripls56/vsservice/cache"
	"github.com/ripls56/vsservice/config"
	"github.com/ripls56/vsservice/event"
	"github.com/ripls56/vsservice/logger"
	"github.com/ripls56/vsservice/server"
	"github.com/ripls56/vsservice/service"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"log"
	"os"
)

const (
	envDev  = "dev"
	envProd = "prod"
)

func main() {
	a := fx.New(
		fx.Provide(
			config.New,
			event.NewBus,
			setupLogger,
			setupRabbitMq,
			fx.Annotate(setupCache, fx.As(new(service.CacheManager)), fx.As(new(cache.Manager))),
		),

		server.App,

		fx.Invoke(func(lc fx.Lifecycle, conn *amqp.Connection, bus *event.Bus, manager cache.Manager) {
			lc.Append(
				fx.Hook{
					OnStart: func(ctx context.Context) error {
						go func() {
							ch, err := conn.Channel()
							defer ch.Close()

							if err != nil {
								log.Println(err)
							}
							queue, err := ch.QueueDeclare(
								"vs_event_bus",
								true,
								false,
								false,
								false,
								nil,
							)
							if err != nil {
								fmt.Println(errors.Wrap(err, "Failed to declare a queue"))
							}

							msgs, err := ch.Consume(
								queue.Name,
								"",
								false,
								false,
								false,
								false,
								nil,
							)
							if err != nil {
								fmt.Println(errors.Wrap(err, "Failed to read"))
							}

							for msg := range msgs {
								var ev event.Event
								err = json.Unmarshal(msg.Body, &ev)
								if err != nil {
									panic(err)
								}

								if ev.Data == nil {
									continue
								}

								err = manager.Set(string(ev.Type), ev.Data)
								if err != nil {
									return
								}

								bus.Dispatch(ev)

								err = msg.Ack(false)
								if err != nil {
									return
								}
							}
						}()
						return nil
					},
					OnStop: func(ctx context.Context) error {
						return nil
					},
				})
		}),
	)

	a.Run()

	defer func(app *fx.App, ctx context.Context) {
		err := app.Stop(ctx)
		panic(err)
	}(a, context.Background())

	<-a.Done()
}

func setupLogger(c config.Config) (logger.Logger, error) {
	var zc zap.Config

	switch c.AppEnv {
	case envDev:
		zc = zap.NewDevelopmentConfig()
	case envProd:
		zc = zap.NewProductionConfig()
	default:
		zc = zap.NewDevelopmentConfig()
	}

	zc.OutputPaths = []string{"stdout"}
	zc.ErrorOutputPaths = []string{"stderr"}

	l, err := logger.New(&zc)
	if err != nil {
		return nil, err
	}
	return l, err
}

func setupRabbitMq(c config.Config) (*amqp.Connection, error) {
	file, err := os.ReadFile(c.RabbitMqUrlFile)
	if err != nil {
		panic(err)
	}
	return amqp.Dial(string(file))
}

func setupCache() *cache.Ristretto {
	c, err := ristretto.NewCache(&ristretto.Config[string, any]{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		panic(err)
	}

	return cache.NewRistretto(c)
}
