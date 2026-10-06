package repository

import (
	"reflect"
	"strings"
	"testing"

	"github.com/lasthearth/vsservice/internal/event/internal/dto"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
)

// TestDTOCarriesTheVersionGuard pins the dto shape mongox.UpdateDoc relies on.
//
// UpdateDoc finds the optimistic-concurrency field by reflecting on an
// *anonymous* mongox.Model tagged inline. Writing the field as
// "Model mongox.Model" instead compiles, stores identical BSON and passes every
// other test — but the guard then looks for a "model.version" path that no
// document has, so every write takes the version-zero branch and the pin matches
// nothing. Two editors saving the same event would silently discard one edit
// again, which is the defect this repository was changed to fix.
func TestDTOCarriesTheVersionGuard(t *testing.T) {
	typ := reflect.TypeFor[dto.Event]()
	for f := range typ.Fields() {
		if !f.Anonymous || f.Type != reflect.TypeFor[mongox.Model]() {
			continue
		}
		if _, opts, _ := strings.Cut(f.Tag.Get("bson"), ","); strings.Contains(opts, "inline") {
			return
		}
	}

	t.Fatal(`dto.Event must embed mongox.Model with bson:",inline" so UpdateDoc can guard on version`)
}
