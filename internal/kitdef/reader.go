package kitdef

import (
	"context"

	"github.com/lasthearth/vsservice/internal/kitdef/internal/service"
	"github.com/lasthearth/vsservice/internal/kitdef/kitdefread"
)

// reader adapts the internal kitdef service to the public kitdefread.Reader
// port: it maps the domain KitDef (with attr_snapshot on the internal read
// path) to the primitive kitdefread.KitDef consumers see. This is the
// composition-seam adapter — it lives in the outer kitdef package, the only
// place that may see both the internal model and the public port.
type reader struct {
	svc *service.Service
}

func (r *reader) GetKit(ctx context.Context, code string) (*kitdefread.KitDef, error) {
	kit, err := r.svc.GetKit(ctx, code)
	if err != nil {
		return nil, err
	}
	items := make([]kitdefread.KitItem, len(kit.Items))
	for i, it := range kit.Items {
		items[i] = kitdefread.KitItem{
			GameCode:     it.GameCode,
			Type:         it.Type,
			Quantity:     it.Quantity,
			AttrSnapshot: it.AttrSnapshot,
		}
	}
	return &kitdefread.KitDef{Code: kit.Code, Items: items}, nil
}
