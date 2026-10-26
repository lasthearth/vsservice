package model

import (
	"errors"
	"testing"
	"time"
)

func TestKitDefRename(t *testing.T) {
	k := ReconstituteKitDef("starter", "Old", KitCategoryKit, nil, time.Time{}, time.Time{})
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

func TestKitDefSetCategory(t *testing.T) {
	k := ReconstituteKitDef("starter", "Title", KitCategoryKit, nil, time.Time{}, time.Time{})

	for _, c := range []string{KitCategoryKit, KitCategoryPrivilege, KitCategorySpecial} {
		if err := k.SetCategory(c); err != nil {
			t.Fatalf("SetCategory(%q): %v", c, err)
		}
		if k.Category != c {
			t.Fatalf("category = %q, want %q", k.Category, c)
		}
	}

	for _, c := range []string{"", "Kit", "privileges", "special "} {
		before := k.Category
		if err := k.SetCategory(c); !errors.Is(err, ErrUnknownCategory) {
			t.Fatalf("SetCategory(%q): got %v, want ErrUnknownCategory", c, err)
		}
		if k.Category != before {
			t.Fatalf("category changed on a failed set: %q", k.Category)
		}
	}
}

func TestKitDefRemoveItemAt(t *testing.T) {
	items := []KitItem{
		{GameCode: "game:stick"},
		{GameCode: "game:stone"},
		{GameCode: "game:ingot-copper"},
	}
	k := ReconstituteKitDef("starter", "Title", KitCategoryKit, items, time.Time{}, time.Time{})

	// Middle: order of the rest is preserved.
	if err := k.RemoveItemAt(1); err != nil {
		t.Fatalf("RemoveItemAt(1): %v", err)
	}
	if len(k.Items) != 2 || k.Items[0].GameCode != "game:stick" || k.Items[1].GameCode != "game:ingot-copper" {
		t.Fatalf("items = %+v, want stick and ingot-copper", k.Items)
	}

	// Bounds.
	for _, idx := range []int{-1, 2, 100} {
		if err := k.RemoveItemAt(idx); !errors.Is(err, ErrItemIndex) {
			t.Fatalf("RemoveItemAt(%d): got %v, want ErrItemIndex", idx, err)
		}
	}

	// Last item is not removable: an empty kit is treated as missing.
	if err := k.RemoveItemAt(0); err != nil {
		t.Fatalf("RemoveItemAt(0): %v", err)
	}
	if err := k.RemoveItemAt(0); !errors.Is(err, ErrLastItem) {
		t.Fatalf("RemoveItemAt(0) on the last item: got %v, want ErrLastItem", err)
	}
	if len(k.Items) != 1 {
		t.Fatalf("items = %d, want the last one kept", len(k.Items))
	}
}
