package messaging

import (
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/nats-io/nats.go"
)

func SetupConn(cfg config.Config) (*nats.Conn, error) {
	nc, err := nats.Connect(cfg.NatsUrl)
	if err != nil {
		return nil, err
	}

	return nc, nil
}
