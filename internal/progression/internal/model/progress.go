package model

import (
	"errors"
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

// RollbackLast removes the last purchased node and returns it.
// AddNode rejects out-of-order purchases, so the last element is always the most
// recent one. Returns false if no nodes are purchased.
func (p *TalentProgress) RollbackLast() (PurchasedNode, bool) {
	if len(p.PurchasedNodes) == 0 {
		return PurchasedNode{}, false
	}
	last := p.PurchasedNodes[len(p.PurchasedNodes)-1]
	p.PurchasedNodes = p.PurchasedNodes[:len(p.PurchasedNodes)-1]
	return last, true
}

// AddNode appends a newly purchased node to the progress record.
// It rejects a node older than the current last one: RollbackLast pops from the
// tail and would otherwise discard the wrong purchase.
func (p *TalentProgress) AddNode(node PurchasedNode) error {
	if n := len(p.PurchasedNodes); n > 0 && node.PurchasedAt.Before(p.PurchasedNodes[n-1].PurchasedAt) {
		return errors.New("purchased node is older than the last recorded purchase")
	}
	p.PurchasedNodes = append(p.PurchasedNodes, node)
	return nil
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
