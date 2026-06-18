package dto

import "github.com/lasthearth/vsservice/internal/pkg/mongox"

type TalentNode struct {
	Id          string `bson:"id"`
	Name        string `bson:"name"`
	Description string `bson:"description"`
	Effect      string `bson:"effect"`
	CostBi      int64  `bson:"cost_bi"`
}

type TalentEdge struct {
	From string `bson:"from"`
	To   string `bson:"to"`
}

type TalentTree struct {
	mongox.Model `bson:",inline"`
	Name         string       `bson:"name"`
	Description  string       `bson:"description"`
	Nodes        []TalentNode `bson:"nodes"`
	Edges        []TalentEdge `bson:"edges"`
}

type TalentPreset struct {
	mongox.Model `bson:",inline"`
	Name         string   `bson:"name"`
	TreeIds      []string `bson:"tree_ids"`
}
