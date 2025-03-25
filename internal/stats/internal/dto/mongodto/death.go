package mongodto

type Death struct {
	Cause      string `bson:"cause"`
	EntityName string `bson:"entity_name"`
}
