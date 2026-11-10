package service_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	lfgv1 "github.com/lasthearth/vsservice/gen/lfg/v1"
)

// TestRespondRejoinKeepsJoinedTrue pins the proto contract the dedupe must not
// break. RespondToPostResponse.joined answers "are you in the post now", so a
// rejoin reports true even though the author is not notified a second time.
// Reporting false would tell a player they lost a slot they hold, and the client
// toggles its button off while the post still lists them.
func TestRespondRejoinKeepsJoinedTrue(t *testing.T) {
	svc, repo := newService(t)
	seedPost(t, repo, 2)
	req := &lfgv1.RespondToPostRequest{Id: "p1"}

	join, err := svc.RespondToPost(as("a"), req)
	if err != nil || !join.GetJoined() {
		t.Fatalf("join: joined=%v err=%v", join.GetJoined(), err)
	}

	leave, err := svc.RespondToPost(as("a"), req)
	if err != nil || leave.GetJoined() {
		t.Fatalf("withdraw: joined=%v err=%v", leave.GetJoined(), err)
	}

	rejoin, err := svc.RespondToPost(as("a"), req)
	if err != nil {
		t.Fatalf("rejoin: %v", err)
	}
	if !rejoin.GetJoined() {
		t.Fatal("rejoin must report joined=true; only the notification is deduped")
	}
	if got := rejoin.GetPost().GetResponders(); len(got) != 1 || got[0] != "a" {
		t.Fatalf("rejoined responder list: %v", got)
	}

	// Another cycle, to be sure the answer does not depend on the cycle count.
	if _, err := svc.RespondToPost(as("a"), req); err != nil {
		t.Fatal(err)
	}
	again, err := svc.RespondToPost(as("a"), req)
	if err != nil || !again.GetJoined() {
		t.Fatalf("second rejoin: joined=%v err=%v", again.GetJoined(), err)
	}
}

// TestAuthorNotificationIsGatedOnTheDedupeFlag is the guard on the bug itself:
// RespondToPost used to notify the author whenever Respond reported a join, and
// since Respond is a toggle with no cooldown, one responder could write one
// notification per withdraw/rejoin cycle into an inbox they cannot delete from.
//
// The gate has to read the model's second result — "the author has not been told
// about this responder yet" — and not the first. Nothing observable at the RPC
// boundary shows which one it reads: the notification sink is a concrete
// *notificationuc.Create whose repository type lives in another domain's internal
// package, so a test double cannot implement it. The source is read instead, the
// way nats_guard_test.go reads imports: parse RespondToPost, follow the value
// returned in second position by the Respond call through its assignments, and
// require the notifyAuthor call to sit under an if on that value.
func TestAuthorNotificationIsGatedOnTheDedupeFlag(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "service.go", nil, 0)
	if err != nil {
		t.Fatalf("parse service.go: %v", err)
	}

	fn := funcDecl(file, "RespondToPost")
	if fn == nil {
		t.Fatal("RespondToPost is gone from service.go; move this guard with it")
	}

	dedupe := dedupeFlagNames(fn)
	if len(dedupe) == 0 {
		t.Fatal("no call to the model's Respond found in RespondToPost; the guard cannot tell which flag gates the notification")
	}

	gate := notifyAuthorCondition(fn)
	if gate == "" {
		t.Fatal("no if statement guards the notifyAuthor call in RespondToPost")
	}
	if !dedupe[gate] {
		t.Errorf("notifyAuthor is gated on %q, which is not the model's notify flag %v; the author is notified once per join again", gate, keys(dedupe))
	}
}

// funcDecl returns the named method or function of the file.
func funcDecl(file *ast.File, name string) *ast.FuncDecl {
	for _, d := range file.Decls {
		if f, ok := d.(*ast.FuncDecl); ok && f.Name.Name == name {
			return f
		}
	}
	return nil
}

// dedupeFlagNames collects the identifiers that hold the model's second Respond
// result, following plain assignments so a rename inside the update callback
// still tracks: `in, tell, err := p.Respond(...)` seeds tell, and
// `joined, notify = in, tell` then seeds notify.
func dedupeFlagNames(fn *ast.FuncDecl) map[string]bool {
	names := map[string]bool{}

	for range 4 { // assignments chain, so a few passes settle them
		ast.Inspect(fn, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}

			if call, ok := assign.Rhs[len(assign.Rhs)-1].(*ast.CallExpr); ok && isRespondCall(call) {
				if len(assign.Lhs) == 3 && len(assign.Rhs) == 1 {
					if id, ok := assign.Lhs[1].(*ast.Ident); ok {
						names[id.Name] = true
					}
				}
				return true
			}

			if len(assign.Lhs) != len(assign.Rhs) {
				return true
			}
			for i, rhs := range assign.Rhs {
				id, ok := rhs.(*ast.Ident)
				if !ok || !names[id.Name] {
					continue
				}
				if lhs, ok := assign.Lhs[i].(*ast.Ident); ok {
					names[lhs.Name] = true
				}
			}
			return true
		})
	}

	return names
}

// isRespondCall reports whether call invokes a method named Respond.
func isRespondCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Respond"
}

// notifyAuthorCondition returns the name of the single identifier a notifyAuthor
// call is guarded by, or "" when no such guard exists.
func notifyAuthorCondition(fn *ast.FuncDecl) string {
	found := ""

	ast.Inspect(fn, func(n ast.Node) bool {
		stmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		cond, ok := stmt.Cond.(*ast.Ident)
		if !ok {
			return true
		}
		callsNotifyAuthor := false
		ast.Inspect(stmt.Body, func(inner ast.Node) bool {
			if call, ok := inner.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "notifyAuthor" {
					callsNotifyAuthor = true
				}
			}
			return true
		})
		if callsNotifyAuthor {
			found = cond.Name
		}
		return true
	})

	return found
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
