package mongodto

import "time"

type Entry struct {
	Name        string  `bson:"user_game_name"`
	TotalHours  float64 `bson:"total_hours"`
	TotalDeaths int     `bson:"total_deaths"`
	TotalKills  int     `bson:"total_kills"`
	// UserId is joined from verification_requests by user_game_name.
	UserId string `bson:"user_id"`
	// LastOnline is the latest last_online mark across the player's stats
	// documents; nil when the game never recorded one. The C# writer stores unix
	// milliseconds as a NumberLong (e.g. 1770642047406), which the driver
	// decodes into a time. entry_decode_test.go pins that assumption, because
	// $max above compares the raw values numerically.
	LastOnline *time.Time `bson:"last_online"`
}
