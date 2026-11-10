package repository

// The $set document Repository.Update sends.
//
// Update prepares an envelope-preserving document for its write and then handed
// the driver the raw mapped dto instead, so the prepared document was never
// referenced. Mapper declares 'goverter:ignore Model', which leaves the dto's
// mongox.Model at its zero value, and only Id is tagged omitempty: created_at,
// updated_at and version therefore reached the write as zero values on every
// approval, rejection and resubmission, and the stored application date was gone
// with no code path restoring it and no RPC reporting the loss. The derived
// entitlement followed, because Days collapses to 0 and locks the 30, 60 and 90
// day appearance items.
//
// Three tests, one per half of the defect. The payload test reads the document
// the real generated mapper produces, the premise test pins why the delete list
// is load-bearing, and the source test pins the wiring: a payload test alone
// still passes when Update goes back to sending the dto, because the helper it
// ignores stays correct.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strconv"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/verification/repository/repomapper"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// updateSourceName is the file the wiring test reads. Update lives in it.
const updateSourceName = "repository.go"

// envelopeKeys are the mongox.Model keys the stored document owns. None of them
// may appear in a $set this repository builds from a mapped dto.
var envelopeKeys = []string{"_id", "created_at", "version"}

// decidedVerification is an application the service hands to Update: approved,
// loaded from the collection, so its envelope carries what Create stamped.
func decidedVerification() verification.Verification {
	stored := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	return verification.Verification{
		Id:               "6650f0a1c1d2e3f4a5b6c7d8",
		UserId:           "user-1",
		UserName:         "Player Name",
		UserGameName:     "PlayerNick",
		Answers:          []verification.Answer{{Question: "q1", Answer: "yes"}, {Question: "q2", Answer: "no"}},
		Contacts:         "@player",
		Status:           verification.VerificationStatusApproved,
		VerificationCode: "CODE-1",
		CreatedAt:        stored,
		UpdatedAt:        stored.Add(time.Hour),
	}
}

// TestUpdateSetDocumentKeepsTheStoredEnvelope pins the write payload: the
// application's own fields and a fresh updated_at, and no envelope key.
func TestUpdateSetDocumentKeepsTheStoredEnvelope(t *testing.T) {
	v := decidedVerification()

	before := time.Now()
	doc, err := updateSetDocument(&repomapper.MapperImpl{}, v)
	after := time.Now()
	if err != nil {
		t.Fatalf("build the $set document: %v", err)
	}

	for _, key := range envelopeKeys {
		if got, ok := doc[key]; ok {
			t.Errorf("the $set document carries %s = %v: the mapper left it at its zero value, so the write resets what Create stamped on the stored document", key, got)
		}
	}

	// updated_at is the one envelope key this write owns: it is the time of the
	// decision, and dropping it would leave the document claiming the previous
	// one.
	stamp, ok := doc["updated_at"].(time.Time)
	if !ok {
		t.Fatalf("the $set document carries updated_at = %v (%T), want the time of this write", doc["updated_at"], doc["updated_at"])
	}
	if stamp.IsZero() {
		t.Error("the $set document stamps a zero updated_at, so the write erases the decision time")
	}
	if stamp.Before(before) || stamp.After(after) {
		t.Errorf("the $set document stamps updated_at = %v, want a time between %v and %v", stamp, before, after)
	}

	for field, want := range map[string]any{
		"user_id":           v.UserId,
		"user_name":         v.UserName,
		"user_game_name":    v.UserGameName,
		"contacts":          v.Contacts,
		"status":            string(v.Status),
		"verification_code": v.VerificationCode,
		"rejection_reason":  v.RejectionReason,
	} {
		if got := doc[field]; got != want {
			t.Errorf("the $set document carries %s = %v, want %v", field, got, want)
		}
	}

	answers, ok := doc["answers"].(bson.A)
	if !ok || len(answers) != len(v.Answers) {
		t.Errorf("the $set document carries answers = %v, want %d entries", doc["answers"], len(v.Answers))
	}

	// The whole key set, so an envelope key a future mapper revision starts
	// emitting is answered here instead of reaching the collection. Add a new
	// application field to this list; do not add a mongox.Model key.
	want := map[string]bool{
		"user_id": true, "user_name": true, "user_game_name": true, "answers": true,
		"contacts": true, "status": true, "verification_code": true,
		"rejection_reason": true, "updated_at": true,
	}
	if len(doc) != len(want) {
		extra := make([]string, 0, len(doc))
		for key := range doc {
			if !want[key] {
				extra = append(extra, key)
			}
		}
		t.Errorf("the $set document carries %d keys and %v is not expected: every unexpected key overwrites a field of the stored document", len(doc), extra)
	}
}

// TestMappedDtoCarriesAZeroEnvelope pins the premise the delete list rests on.
//
// _id is absent because mongox.Model tags it omitempty and the mapper leaves it
// zero, which is why MongoDB accepts the write instead of refusing an
// immutable-field change. The other three carry no omitempty, so they are always
// written, and always as zero values.
func TestMappedDtoCarriesAZeroEnvelope(t *testing.T) {
	v := decidedVerification()

	dto := (&repomapper.MapperImpl{}).FromVerification(v)
	raw, err := bson.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal the mapped dto: %v", err)
	}
	var rawMap bson.M
	if err := bson.Unmarshal(raw, &rawMap); err != nil {
		t.Fatalf("unmarshal the mapped dto: %v", err)
	}

	if got, ok := rawMap["_id"]; ok {
		t.Errorf("the mapped dto carries _id = %v, want it dropped by omitempty: the write would fail as an immutable-field change instead of resetting the envelope", got)
	}
	for _, key := range []string{"created_at", "updated_at", "version"} {
		if _, ok := rawMap[key]; !ok {
			t.Errorf("the mapped dto carries no %s key, so the delete list in updateSetDocument has nothing to drop for it", key)
		}
	}

	// The zero time reaches the wire as a BSON datetime, not as a missing key:
	// year 1, which overwrites the application date Create stamped.
	if got := decodedTime(t, rawMap, "created_at"); !got.IsZero() {
		t.Errorf("the mapped dto carries created_at = %v, want the zero time: the model holds %v and the mapper ignores it, so this key overwrites the stored application date", got, v.CreatedAt)
	}
	switch got := rawMap["version"].(type) {
	case int32:
		if got != 0 {
			t.Errorf("the mapped dto carries version = %d, want 0", got)
		}
	case int64:
		if got != 0 {
			t.Errorf("the mapped dto carries version = %d, want 0", got)
		}
	default:
		t.Errorf("the mapped dto carries version = %v (%T), want the number 0", rawMap["version"], rawMap["version"])
	}
}

// decodedTime reads a datetime key back out of a document decoded into any,
// which the driver answers as time.Time or as bson.DateTime depending on the
// value it read.
func decodedTime(t *testing.T, doc bson.M, key string) time.Time {
	t.Helper()

	switch got := doc[key].(type) {
	case time.Time:
		return got
	case bson.DateTime:
		return got.Time()
	default:
		t.Errorf("%s = %v (%T), want a BSON datetime", key, doc[key], doc[key])
		return time.Time{}
	}
}

// TestUpdateSendsThePreparedDocument reads repository.go and fails when the
// UpdateOne in Update sends anything other than the document updateSetDocument
// returned.
func TestUpdateSendsThePreparedDocument(t *testing.T) {
	file := parseSource(t, updateSourceName)
	fn := findMethod(t, file, "Update")

	prepared := assignedFromCall(fn.Body, "updateSetDocument")
	if len(prepared) == 0 {
		t.Fatal("Update never calls updateSetDocument, so it builds no envelope-preserving document to send")
	}

	values := setValuesInUpdates(fn.Body)
	if len(values) == 0 {
		t.Fatal("Update makes no UpdateOne call this test can read, so it proves nothing")
	}
	for _, value := range values {
		id, ok := value.(*ast.Ident)
		if !ok || !prepared[id.Name] {
			t.Errorf("Update sends %s as the $set value, want the document updateSetDocument returned: the mapped dto carries a zero created_at, updated_at and version, so the write resets the envelope Create stamped",
				types.ExprString(value))
		}
	}
}

func parseSource(t *testing.T, name string) *ast.File {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}

	return file
}

func findMethod(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
	t.Helper()

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv != nil && fn.Body != nil && fn.Name.Name == name {
			return fn
		}
	}

	t.Fatalf("%s declares no method %s", file.Name.Name, name)

	return nil
}

// assignedFromCall maps every local name assigned from a call to callee, so the
// test reads the write whichever name the document is held in.
func assignedFromCall(body *ast.BlockStmt, callee string) map[string]bool {
	names := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 || callName(assign.Rhs[0]) != callee {
			return true
		}
		for _, lhs := range assign.Lhs {
			if id, ok := lhs.(*ast.Ident); ok {
				names[id.Name] = true
			}
		}
		return true
	})

	return names
}

// setValuesInUpdates returns the $set value of every UpdateOne call in body.
func setValuesInUpdates(body *ast.BlockStmt) []ast.Expr {
	locals := localCompositeLiterals(body)

	var out []ast.Expr
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || callName(call) != "UpdateOne" || len(call.Args) < 3 {
			return true
		}

		doc := call.Args[2]
		if id, ok := doc.(*ast.Ident); ok {
			if resolved, found := locals[id.Name]; found {
				doc = resolved
			}
		}
		out = append(out, setValueOf(doc)...)

		return true
	})

	return out
}

// setValueOf reads the $set value out of a bson.D or a bson.M update document.
func setValueOf(doc ast.Expr) []ast.Expr {
	lit, ok := doc.(*ast.CompositeLit)
	if !ok {
		return nil
	}

	var out []ast.Expr
	for _, elt := range lit.Elts {
		switch e := elt.(type) {
		case *ast.KeyValueExpr: // bson.M{"$set": ...}
			if stringLit(e.Key) == "$set" {
				out = append(out, e.Value)
			}
		case *ast.CompositeLit: // bson.D{{Key: "$set", Value: ...}}
			if key, value := keyedElement(e); key == "$set" && value != nil {
				out = append(out, value)
			}
		}
	}

	return out
}

// localCompositeLiterals maps a local name to the literal assigned to it, so an
// update document built in a variable is still read.
func localCompositeLiterals(body *ast.BlockStmt) map[string]ast.Expr {
	locals := make(map[string]ast.Expr)

	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		id, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		if lit, ok := assign.Rhs[0].(*ast.CompositeLit); ok {
			locals[id.Name] = lit
		}
		return true
	})

	return locals
}

// keyedElement reads one bson.E literal into its Key string and its Value.
func keyedElement(lit *ast.CompositeLit) (string, ast.Expr) {
	var key string
	var value ast.Expr

	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch name.Name {
		case "Key":
			key = stringLit(kv.Value)
		case "Value":
			value = kv.Value
		}
	}

	return key, value
}

func callName(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

func stringLit(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return value
}
