package model

import (
	"slices"
	"time"
)

type OwnerType string

const (
	OwnerTypeSettlement OwnerType = "settlement"
	OwnerTypePointSide  OwnerType = "point_side"
)

type PurchasedNode struct {
	NodeId                string
	PurchasedAt           time.Time
	PurchasedBySettlement string
}

type TalentProgress struct {
	Id             string
	OwnerType      OwnerType
	SettlementId   string // set when OwnerType == OwnerTypeSettlement
	PointId        string // set when OwnerType == OwnerTypePointSide
	Side           string // "east" | "west" — set when OwnerType == OwnerTypePointSide
	TreeId         string
	PurchasedNodes []PurchasedNode
}

func ReconstituteTalentProgress(id string, ownerType OwnerType, settlementId, pointId, side, treeId string, nodes []PurchasedNode) *TalentProgress {
	return &TalentProgress{
		Id:             id,
		OwnerType:      ownerType,
		SettlementId:   settlementId,
		PointId:        pointId,
		Side:           side,
		TreeId:         treeId,
		PurchasedNodes: nodes,
	}
}

// RollbackLast removes the most recently purchased node (by PurchasedAt) and returns it.
// Returns false if no nodes are purchased.
func (p *TalentProgress) RollbackLast() (PurchasedNode, bool) {
	if len(p.PurchasedNodes) == 0 {
		return PurchasedNode{}, false
	}
	lastIdx := 0
	for i := 1; i < len(p.PurchasedNodes); i++ {
		if p.PurchasedNodes[i].PurchasedAt.After(p.PurchasedNodes[lastIdx].PurchasedAt) {
			lastIdx = i
		}
	}
	last := p.PurchasedNodes[lastIdx]
	p.PurchasedNodes = slices.Delete(p.PurchasedNodes, lastIdx, lastIdx+1)
	return last, true
}

// AddNode appends a newly purchased node to the progress record.
func (p *TalentProgress) AddNode(node PurchasedNode) {
	p.PurchasedNodes = append(p.PurchasedNodes, node)
}

// HasNode reports whether nodeId is already purchased.
func (p *TalentProgress) HasNode(nodeId string) bool {
	for _, n := range p.PurchasedNodes {
		if n.NodeId == nodeId {
			return true
		}
	}
	return false
}
