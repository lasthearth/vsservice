package model

import (
	"testing"
	"time"
)

func TestKitDefRename(t *testing.T) {
	k := ReconstituteKitDef("starter", "Old", nil, time.Time{}, time.Time{})
	if err := k.Rename(""); err == nil {
		t.Fatal("expected error on empty title")
	}
	if k.Title != "Old" {
		t.Fatalf("title changed on failed rename: %q", k.Title)
	}

	if err := k.Rename("New"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if k.Title != "New" {
		t.Fatalf("title not updated: %q", k.Title)
	}
}
