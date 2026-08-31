package mail

import (
	"context"

	"github.com/lasthearth/vsservice/internal/kitdef/kitdefread"
	"github.com/lasthearth/vsservice/internal/mail/internal/service"
)

// kitReaderAdapter adapts kitdef's public kitdefread.Reader port to mail's
// consumer-side service.KitReader. This is the composition seam: mail imports
// the public kitdefread package (allowed — kitdef never imports mail), and the
// two read-model types are translated here so neither domain depends on the
// other's internals.
type kitReaderAdapter struct {
	inner kitdefread.Reader
}

func newKitReaderAdapter(inner kitdefread.Reader) service.KitReader {
	return &kitReaderAdapter{inner: inner}
}

func (a *kitReaderAdapter) GetKit(ctx context.Context, code string) (*service.KitSnapshot, error) {
	kit, err := a.inner.GetKit(ctx, code)
	if err != nil {
		return nil, err
	}
	items := make([]service.KitItem, len(kit.Items))
	for i, it := range kit.Items {
		items[i] = service.KitItem{
			GameCode:     it.GameCode,
			Type:         it.Type,
			AttrSnapshot: it.AttrSnapshot,
			Quantity:     it.Quantity,
		}
	}
	return &service.KitSnapshot{Items: items}, nil
}
