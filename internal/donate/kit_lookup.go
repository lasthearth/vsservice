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

func (a *kitLookupAdapter) GetCategory(ctx context.Context, code string) (string, error) {
	kit, err := a.inner.GetKit(ctx, code)
	if err != nil {
		return "", err
	}
	return kit.Category, nil
}

func (a *kitLookupAdapter) Categories(ctx context.Context) (map[string]string, error) {
	kits, err := a.inner.ListKits(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(kits))
	for _, k := range kits {
		out[k.Code] = k.Category
	}
	return out, nil
}
