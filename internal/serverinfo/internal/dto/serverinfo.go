package dto

import "github.com/lasthearth/vsservice/internal/pkg/mongox"

type ServerInfo struct {
	mongox.Model `bson:",inline"`
	WorldTime    string `bson:"world_time"`
	TotalOnline  int    `bson:"total_online"`
	MaxOnline    int    `bson:"max_online"`
}
