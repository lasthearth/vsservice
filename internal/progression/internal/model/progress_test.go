package model

import (
	"testing"
	"time"
)

func TestAddNodeRejectsOutOfOrderPurchases(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &TalentProgress{}

	if err := p.AddNode(PurchasedNode{NodeId: "n1", PurchasedAt: t0}); err != nil {
		t.Fatalf("first AddNode: %v", err)
	}
	if err := p.AddNode(PurchasedNode{NodeId: "n2", PurchasedAt: t0.Add(time.Hour)}); err != nil {
		t.Fatalf("later AddNode: %v", err)
	}
	// Equal timestamps are accepted: they cannot break the tail-pop invariant.
	if err := p.AddNode(PurchasedNode{NodeId: "n3", PurchasedAt: t0.Add(time.Hour)}); err != nil {
		t.Fatalf("same-instant AddNode: %v", err)
	}
	if err := p.AddNode(PurchasedNode{NodeId: "n0", PurchasedAt: t0}); err == nil {
		t.Fatal("AddNode accepted a purchase older than the last one")
	}
	if len(p.PurchasedNodes) != 3 {
		t.Fatalf("purchased nodes = %d, want 3", len(p.PurchasedNodes))
	}

	// RollbackLast must return the newest purchase, which is what the invariant buys.
	node, ok := p.RollbackLast()
	if !ok || node.NodeId != "n3" {
		t.Fatalf("RollbackLast = (%v, %v), want (n3, true)", node.NodeId, ok)
	}
}
