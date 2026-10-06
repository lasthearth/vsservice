package mongodto

import "time"

type Entry struct {
	Name        string  `bson:"user_game_name"`
	TotalHours  float64 `bson:"total_hours"`
	TotalDeaths int     `bson:"total_deaths"`
	TotalKills  int     `bson:"total_kills"`
	// UserId is joined from verification_requests by user_game_name.
	UserId string `bson:"user_id"`
	// LastOnline is the latest `last_online` date across the player's stats
	// documents. The game writes it as a BSON date; nil when never recorded.
	LastOnline *time.Time `bson:"last_online"`
}
