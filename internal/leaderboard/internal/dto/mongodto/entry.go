package mongodto

import "go.mongodb.org/mongo-driver/v2/bson"

type Entry struct {
	Name        string  `bson:"user_game_name"`
	TotalHours  float64 `bson:"total_hours"`
	TotalDeaths int     `bson:"total_deaths"`
	TotalKills  int     `bson:"total_kills"`
	// UserId is joined from verification_requests by user_game_name.
	UserId string `bson:"user_id"`
	// LastOnline is written by the game; kept raw because its BSON type
	// (date, string or number) is owned by the C# writer.
	LastOnline bson.RawValue `bson:"last_online"`
}
