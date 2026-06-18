package model

type TalentNode struct {
	Id          string
	Name        string
	Description string
	Effect      string
	CostBi      int64
}

type TalentEdge struct {
	From string
	To   string
}

type TalentTree struct {
	Id          string
	Name        string
	Description string
	Nodes       []TalentNode
	Edges       []TalentEdge
}

func ReconstituteTalentTree(id, name, description string, nodes []TalentNode, edges []TalentEdge) *TalentTree {
	return &TalentTree{
		Id:          id,
		Name:        name,
		Description: description,
		Nodes:       nodes,
		Edges:       edges,
	}
}

type TalentPreset struct {
	Id      string
	Name    string
	TreeIds []string
}

func ReconstituteTalentPreset(id, name string, treeIds []string) *TalentPreset {
	return &TalentPreset{Id: id, Name: name, TreeIds: treeIds}
}
