package service

import (
	"context"
	"errors"
	"testing"

	mailerr "github.com/lasthearth/vsservice/internal/mail/internal/ierror"
	"github.com/lasthearth/vsservice/internal/mail/internal/model"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
)

// fakeKitReader is a hand-written stand-in for KitReader.
type fakeKitReader struct {
	snap *KitSnapshot
	err  error
}

func (f fakeKitReader) GetKit(context.Context, string) (*KitSnapshot, error) {
	return f.snap, f.err
}

func TestExpandKitMapsItemsThrough(t *testing.T) {
	kits := fakeKitReader{snap: &KitSnapshot{Items: []KitItem{
		{GameCode: "game:bread", Type: "item", Quantity: 3, AttrSnapshot: "abc"},
		{GameCode: "game:log", Type: "block", Quantity: 1},
	}}}

	got, err := expandKit(context.Background(), kits, "foodkit")
	if err != nil {
		t.Fatalf("expandKit: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("attachments = %d, want 2", len(got))
	}
	a := got[0].Item
	if a == nil || a.GameCode != "game:bread" || a.Type != "item" || a.Quantity != 3 || a.AttrSnapshot != "abc" {
		t.Fatalf("attachment 0 = %+v, want game:bread item x3 attr=abc carried through as-is", a)
	}
	if got[1].Item == nil || got[1].Item.Type != "block" {
		t.Fatalf("attachment 1 = %+v, want the block type preserved", got[1].Item)
	}
}

// An empty (but captured) kit is fail-loud: ErrKitEmpty, never a claimless mail.
func TestExpandKitEmptyFailsLoud(t *testing.T) {
	kits := fakeKitReader{snap: &KitSnapshot{Items: nil}}

	_, err := expandKit(context.Background(), kits, "emptykit")
	if !errors.Is(err, mailerr.ErrKitEmpty) {
		t.Fatalf("expandKit: got %v, want ErrKitEmpty", err)
	}
}

// A kit that was never captured is normalized to ErrKitNotFound.
func TestExpandKitNotFound(t *testing.T) {
	kits := fakeKitReader{err: pkgerr.NotFound("kit not found")}

	_, err := expandKit(context.Background(), kits, "nope")
	if !errors.Is(err, mailerr.ErrKitNotFound) {
		t.Fatalf("expandKit: got %v, want ErrKitNotFound", err)
	}
}

// A non-NotFound reader error is passed through, not swallowed as not-found.
func TestExpandKitPassesThroughOtherErrors(t *testing.T) {
	sentinel := errors.New("db down")
	kits := fakeKitReader{err: sentinel}

	_, err := expandKit(context.Background(), kits, "foodkit")
	if !errors.Is(err, sentinel) {
		t.Fatalf("expandKit: got %v, want the raw reader error", err)
	}
}

// ensure model.Attachment is referenced (guards against an unused import if the
// mapping helpers change).
var _ = model.Attachment{}
