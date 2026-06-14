package model

import (
	"testing"
	"time"
)

func TestNewKitAssignment(t *testing.T) {
	a := NewKitAssignment("user1", "Player1", "starter", "admin")

	if a.UserId != "user1" {
		t.Errorf("UserId = %v, want user1", a.UserId)
	}
	if a.UserGameName != "Player1" {
		t.Errorf("UserGameName = %v, want Player1", a.UserGameName)
	}
	if a.KitName != "starter" {
		t.Errorf("KitName = %v, want starter", a.KitName)
	}
	if a.AssignedBy != "admin" {
		t.Errorf("AssignedBy = %v, want admin", a.AssignedBy)
	}
	if a.Status != AssignmentStatusPending {
		t.Errorf("Status = %v, want PENDING", a.Status)
	}
	if a.DeliveredAt != nil {
		t.Errorf("DeliveredAt should be nil on creation")
	}
	if a.ClaimedAt != nil {
		t.Errorf("ClaimedAt should be nil on creation")
	}
}

func TestKitAssignment_TransitionTo(t *testing.T) {
	tests := []struct {
		name    string
		from    AssignmentStatus
		to      AssignmentStatus
		wantErr bool
	}{
		{"pending to delivered is valid", AssignmentStatusPending, AssignmentStatusDelivered, false},
		{"delivered to claimed is valid", AssignmentStatusDelivered, AssignmentStatusClaimed, false},
		{"pending to claimed is invalid", AssignmentStatusPending, AssignmentStatusClaimed, true},
		{"claimed to pending is invalid", AssignmentStatusClaimed, AssignmentStatusPending, true},
		{"claimed to delivered is invalid", AssignmentStatusClaimed, AssignmentStatusDelivered, true},
		{"delivered to pending is invalid", AssignmentStatusDelivered, AssignmentStatusPending, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &KitAssignment{Status: tc.from}
			err := a.TransitionTo(tc.to)
			if (err != nil) != tc.wantErr {
				t.Errorf("TransitionTo(%v) error = %v, wantErr %v", tc.to, err, tc.wantErr)
			}
			if err == nil && a.Status != tc.to {
				t.Errorf("TransitionTo() status = %v, want %v", a.Status, tc.to)
			}
		})
	}
}

func TestKitAssignment_Validate(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	past := now.Add(-2 * time.Hour)
	future := now.Add(1 * time.Hour)
	delivered := now.Add(-1 * time.Hour)
	beforeAssigned := now.Add(-3 * time.Hour)

	base := func() *KitAssignment {
		return &KitAssignment{
			UserId:       "user1",
			UserGameName: "Player1",
			KitName:      "starter",
			AssignedBy:   "admin",
			Status:       AssignmentStatusPending,
			AssignedAt:   past,
		}
	}

	tests := []struct {
		name    string
		build   func() *KitAssignment
		wantErr bool
	}{
		{
			"valid pending assignment",
			base,
			false,
		},
		{
			"empty UserId",
			func() *KitAssignment { a := base(); a.UserId = ""; return a },
			true,
		},
		{
			"empty UserGameName",
			func() *KitAssignment { a := base(); a.UserGameName = ""; return a },
			true,
		},
		{
			"empty KitName",
			func() *KitAssignment { a := base(); a.KitName = ""; return a },
			true,
		},
		{
			"empty AssignedBy",
			func() *KitAssignment { a := base(); a.AssignedBy = ""; return a },
			true,
		},
		{
			"AssignedAt in future",
			func() *KitAssignment { a := base(); a.AssignedAt = future; return a },
			true,
		},
		{
			"DeliveredAt before AssignedAt",
			func() *KitAssignment {
				a := base()
				a.Status = AssignmentStatusDelivered
				a.AssignedAt = delivered        // T-1h
				a.DeliveredAt = &beforeAssigned // T-3h — impossible
				return a
			},
			true,
		},
		{
			"ClaimedAt before DeliveredAt",
			func() *KitAssignment {
				a := base()
				a.Status = AssignmentStatusClaimed
				a.DeliveredAt = &delivered // T-1h
				a.ClaimedAt = &past        // T-2h — before delivery
				return a
			},
			true,
		},
		{
			"valid delivered assignment",
			func() *KitAssignment {
				a := base()
				a.Status = AssignmentStatusDelivered
				a.DeliveredAt = &delivered
				return a
			},
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.build().Validate(now)
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestKitAssignment_IsDelivered(t *testing.T) {
	tests := []struct {
		status AssignmentStatus
		want   bool
	}{
		{AssignmentStatusPending, false},
		{AssignmentStatusDelivered, true},
		{AssignmentStatusClaimed, true},
	}
	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			a := &KitAssignment{Status: tc.status}
			if got := a.IsDelivered(); got != tc.want {
				t.Errorf("IsDelivered() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestKitAssignment_IsClaimed(t *testing.T) {
	tests := []struct {
		status AssignmentStatus
		want   bool
	}{
		{AssignmentStatusPending, false},
		{AssignmentStatusDelivered, false},
		{AssignmentStatusClaimed, true},
	}
	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			a := &KitAssignment{Status: tc.status}
			if got := a.IsClaimed(); got != tc.want {
				t.Errorf("IsClaimed() = %v, want %v", got, tc.want)
			}
		})
	}
}
