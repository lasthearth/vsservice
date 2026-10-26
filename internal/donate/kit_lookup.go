package donate

import (
	"context"

	"github.com/lasthearth/vsservice/internal/donate/internal/service"
	"github.com/lasthearth/vsservice/internal/kitdef/kitdefread"
)

// kitLookupAdapter adapts kitdef's public kitdefread.Reader port to donate's
// consumer-side service.KitLookup. This is the composition seam: donate
// imports kitdefread (public); kitdef never imports donate.
type kitLookupAdapter struct {
	inner kitdefread.Reader
}

func newKitLookupAdapter(inner kitdefread.Reader) service.KitLookup {
	return &kitLookupAdapter{inner: inner}
}

func (a *kitLookupAdapter) Get(ctx context.Context, code string) (service.KitSummary, error) {
	kit, err := a.inner.GetKit(ctx, code)
	if err != nil {
		return service.KitSummary{}, err
	}
	return toSummary(kit), nil
}

func (a *kitLookupAdapter) Catalog(ctx context.Context) (map[string]service.KitSummary, error) {
	kits, err := a.inner.ListKits(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]service.KitSummary, len(kits))
	for _, k := range kits {
		out[k.Code] = toSummary(k)
	}
	return out, nil
}

func toSummary(kit *kitdefread.KitDef) service.KitSummary {
	items := make([]service.KitItemView, len(kit.Items))
	for i, it := range kit.Items {
		items[i] = service.KitItemView{
			GameCode: it.GameCode,
			Quantity: it.Quantity,
			ImageURL: it.ImageURL,
		}
	}
	return service.KitSummary{Category: kit.Category, Items: items}
}
