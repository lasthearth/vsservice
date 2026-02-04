package mongodto

type Entry struct {
	Name        string  `bson:"user_game_name"`
	TotalHours  float64 `bson:"total_hours"`
	TotalDeaths int     `bson:"total_deaths"`
	TotalKills  int     `bson:"total_kills"`
}
