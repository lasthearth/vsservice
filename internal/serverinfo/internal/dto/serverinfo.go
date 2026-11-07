package dto

import "github.com/lasthearth/vsservice/internal/pkg/mongox"

type ServerInfo struct {
	mongox.Model `bson:",inline"`
	WorldTime    string `bson:"world_time"`
	TotalOnline  int    `bson:"total_online"`
	MaxOnline    int    `bson:"max_online"`
	// HeartbeatAt is the game server's own clock, in unix milliseconds: it is
	// the mark the sweeper compares against, so it stays an int64 rather than a
	// time.Time. created_at/updated_at keep the BSON datetime type because
	// mongox.Model decodes them as time.Time, and one field cannot hold both.
	HeartbeatAt int64 `bson:"heartbeat_at"`
}
