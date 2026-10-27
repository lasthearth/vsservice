package service

import (
	"testing"

	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
)

// captured stands for a captured kit's items in game order; "game:bread"
// appears twice on purpose, because kit item codes repeat.
func capturedKit() []KitItemView {
	return []KitItemView{
		{GameCode: "game:sword", Quantity: 1, ImageURL: "https://cdn.test/sword.png"},
		{GameCode: "game:bread", Quantity: 2, ImageURL: "https://cdn.test/bread.png"},
		{GameCode: "game:bread", Quantity: 5, ImageURL: "https://cdn.test/bread2.png"},
	}
}

// Without stored rows the витрина is the captured kit, in kit order.
func TestDisplayKitEntriesWithoutRowsIsTheCapturedKit(t *testing.T) {
	got := displayKitEntries(nil, capturedKit())
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].GetName() != "game:sword" || got[1].GetQuantity() != 2 || got[2].GetQuantity() != 5 {
		t.Fatalf("unexpected captured view: %v", got)
	}
}

// Stored rows set the order and the shown name; contents stay the kit's.
func TestDisplayKitEntriesFollowsStoredOrderAndNames(t *testing.T) {
	stored := []*donatev1.KitEntry{
		{GameCode: "game:bread", Name: "Хлеб", Quantity: 99},
		{GameCode: "game:sword", Name: "Меч", Description: "Острый"},
	}
	got := displayKitEntries(stored, capturedKit())
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].GetName() != "Хлеб" || got[0].GetGameCode() != "game:bread" || got[0].GetQuantity() != 2 {
		t.Fatalf("row 0 = %v, want bread named Хлеб with the kit's quantity 2", got[0])
	}
	if got[1].GetName() != "Меч" || got[1].GetDescription() != "Острый" {
		t.Fatalf("row 1 = %v", got[1])
	}
	// The second bread item has no row: it follows in kit order, by its code.
	if got[2].GetName() != "game:bread" || got[2].GetQuantity() != 5 {
		t.Fatalf("row 2 = %v", got[2])
	}
}

// A row for an item the kit no longer holds disappears; an empty image falls
// back to the captured icon.
func TestDisplayKitEntriesDropsGoneItemsAndFallsBackToIcon(t *testing.T) {
	stored := []*donatev1.KitEntry{
		{GameCode: "game:gone", Name: "Исчезло"},
		{GameCode: "game:sword", Name: "Меч"},
	}
	got := displayKitEntries(stored, capturedKit())
	if got[0].GetGameCode() != "game:sword" {
		t.Fatalf("first row = %v, want sword", got[0])
	}
	if got[0].GetImageUrl() != "https://cdn.test/sword.png" {
		t.Fatalf("image = %q, want the captured icon", got[0].GetImageUrl())
	}
	for _, e := range got {
		if e.GetGameCode() == "game:gone" {
			t.Fatalf("gone item must not be shown: %v", e)
		}
	}
}

// The write side never fails a save over a витрина row: rows an older client
// sends without game_code are dropped (they were ignored before), a foreign
// image host is cleared so the row falls back to the captured icon, and the
// order the admin set is kept.
func TestKitEntriesFromProtoIsLenientAndKeepsOrder(t *testing.T) {
	s := &Service{mediaUrl: mediaurl.New(config.Config{CdnUrl: "https://cdn.test"})}
	got := s.kitEntriesFromProto([]*donatev1.KitEntry{
		{GameCode: "game:bread", Name: "Хлеб", ImageUrl: "https://evil.test/x.png", Quantity: 7},
		{Name: "Без кода"},
		{GameCode: "game:sword", Name: "Меч", ImageUrl: "https://cdn.test/sword.png"},
	})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (the row without game_code dropped)", len(got))
	}
	if got[0].GameCode != "game:bread" || got[1].GameCode != "game:sword" {
		t.Fatalf("order = %v, want bread then sword", got)
	}
	if got[0].ImageURL != "" {
		t.Fatalf("foreign image kept: %q", got[0].ImageURL)
	}
	if got[0].Quantity != 0 {
		t.Fatalf("quantity stored: %d, want 0 (it comes from the kit)", got[0].Quantity)
	}
	if got[1].ImageURL != "https://cdn.test/sword.png" {
		t.Fatalf("cdn image dropped: %q", got[1].ImageURL)
	}
}
