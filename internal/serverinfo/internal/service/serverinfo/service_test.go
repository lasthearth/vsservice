package serverinfo

import (
	"reflect"
	"testing"
)

// TestRepositoryContractIsReadOnly pins that the service's repository interface
// carries no document update.
//
// The game server is the only writer of serverinfo now. The one exception is the
// sweeper's ClearStaleOnline, which is a filtered bulk write of a single flag
// rather than a read-modify-write of the document. A general Update creeping
// back into the interface would compile, wire through fx, and quietly restore
// the second writer that used to pin max_online to 75 on every touch — a defect
// nothing in the running service would report.
func TestRepositoryContractIsReadOnly(t *testing.T) {
	repo := reflect.TypeFor[Repository]()

	if _, ok := repo.MethodByName("Update"); ok {
		t.Error("Repository declares Update: the game server owns this document, so the service must not write it back")
	}

	for m := range repo.Methods() {
		switch m.Name {
		case "GetServerInfo", "ClearStaleOnline":
		default:
			t.Errorf("Repository declares %s; only GetServerInfo and the sweeper's ClearStaleOnline may be on this interface", m.Name)
		}
	}

	if repo.NumMethod() == 0 {
		t.Fatal("Repository declares no methods, so the check above proved nothing")
	}
}
