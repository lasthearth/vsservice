package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
)

func TestMarkIssuedIsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	repo.withPurchase("pu1", "p1", "Bob", 30)
	uc := newPurchases(repo)

	first, err := uc.MarkIssued(context.Background(), "pu1", "admin-1")
	if err != nil {
		t.Fatalf("first MarkIssued: %v", err)
	}
	if first.IssuedAt == nil || first.IssuedBy == nil || *first.IssuedBy != "admin-1" {
		t.Fatalf("purchase = issued_at %v issued_by %v, want stamped for admin-1", first.IssuedAt, first.IssuedBy)
	}
	issuedAt, issuedBy := *first.IssuedAt, *first.IssuedBy

	second, err := uc.MarkIssued(context.Background(), "pu1", "admin-2")
	if err != nil {
		t.Fatalf("second MarkIssued: %v", err)
	}
	if !second.IssuedAt.Equal(issuedAt) || *second.IssuedBy != issuedBy {
		t.Fatalf("re-issue rewrote the record: %v by %v, want %v by %v",
			second.IssuedAt, *second.IssuedBy, issuedAt, issuedBy)
	}
}

func TestMarkIssuedOnRefundedPurchaseRejected(t *testing.T) {
	repo := newFakeRepo()
	p := repo.withPurchase("pu1", "p1", "Bob", 30)
	if err := p.Refund(); err != nil {
		t.Fatalf("seeding a refunded purchase: %v", err)
	}

	_, err := newPurchases(repo).MarkIssued(context.Background(), "pu1", "admin-1")
	if !errors.Is(err, ierror.ErrCannotIssueRefunded) {
		t.Fatalf("MarkIssued: got %v, want ErrCannotIssueRefunded", err)
	}
	if repo.purchases["pu1"].IsIssued() {
		t.Fatal("a refunded purchase was marked issued")
	}
}

func TestMarkIssuedMissingPurchaseRejected(t *testing.T) {
	repo := newFakeRepo()

	if _, err := newPurchases(repo).MarkIssued(context.Background(), "nope", "admin-1"); !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("MarkIssued: got %v, want ErrNotFound", err)
	}
}
