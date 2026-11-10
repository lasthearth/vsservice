package repository

// Attribution of appearance unlocks to a game identity.
//
// player() read verification_requests by user_id alone and treated the stored user_game_name as the
// caller's identity. Nothing on the write path binds that name to the caller who typed it, so a
// pending or rejected applicant inherited the hours, kills, deaths and ranks of any player whose
// name it claimed, and with them every catalog item that gates on those numbers.
//
// These tests fail on a players read that drops or widens the approved-status window, on an
// attribution with no name to attribute by, and on a projection that loses created_at, which
// Standing.Days is computed from. The guard reads the package source, so it also fails on a players
// read this file does not know about.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// approvedWindow is the roster's status window
// (internal/player/internal/repository/player/repository/list.go:163-167), spelled out and sorted.
// The player domain's status constants are internal to it, so this list is where a drift on either
// side shows up.
var approvedWindow = []string{"approved", "verified"}

// fixedDay is the application date a case carries, so the standing's date fields are numbers
// instead of time.Now.
var fixedDay = time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)

func TestPlayerStandingFilterIsTheRostersApprovedWindow(t *testing.T) {
	got := playerStandingFilter("user-1")

	if id, ok := got["user_id"].(string); !ok || id != "user-1" {
		t.Errorf("filter user_id = %#v, want \"user-1\": a standing reads the caller's own application", got["user_id"])
	}

	window := statusWindowValue(t, got["status"])
	if !slices.Equal(window, approvedWindow) {
		t.Errorf("filter status window = %v, want %v; a pending or rejected application carries no approved game identity", window, approvedWindow)
	}
}

func TestAttributionNeedsAnApprovedGameIdentity(t *testing.T) {
	// The rows the game wrote for players who never applied under the caller's user id, plus the
	// row the unbounded stats group builds for the documents with no user_game_name at all.
	all := []totals{
		{Name: "Victim", Hours: 900, Kills: 120, Deaths: 30},
		{Name: "Second", Hours: 100, Kills: 10, Deaths: 5},
		{Name: "", Hours: 500, Kills: 7, Deaths: 2},
	}

	cases := []struct {
		name       string
		who        player
		wantMine   totals
		wantHours  int
		wantKills  int
		wantInStat bool
	}{
		{
			// This is exactly what player() hands back for a pending or a rejected application:
			// the status window excludes the document, so nothing decodes.
			name: "an unapproved applicant inherits no totals",
			who:  player{},
		},
		{
			name: "a blank name attributes nothing, not even the nameless stats row",
			who:  player{CreatedAt: fixedDay},
		},
		{
			name:       "an approved applicant is attributed by the approved name",
			who:        player{Name: "Victim", CreatedAt: fixedDay},
			wantMine:   totals{Name: "Victim", Hours: 900, Kills: 120, Deaths: 30},
			wantHours:  1,
			wantKills:  1,
			wantInStat: true,
		},
		{
			name:       "a second approved applicant is placed behind the first",
			who:        player{Name: "Second", CreatedAt: fixedDay},
			wantMine:   totals{Name: "Second", Hours: 100, Kills: 10, Deaths: 5},
			wantHours:  3,
			wantKills:  2,
			wantInStat: true,
		},
		{
			name: "an approved name with no stats rows stays at zero",
			who:  player{Name: "Newcomer", CreatedAt: fixedDay},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mine, hoursRank, killsRank := attribution(tc.who, all)

			if mine != tc.wantMine {
				t.Errorf("totals = %#v, want %#v", mine, tc.wantMine)
			}
			if hoursRank != tc.wantHours || killsRank != tc.wantKills {
				t.Errorf("ranks = hours %d, kills %d; want hours %d, kills %d",
					hoursRank, killsRank, tc.wantHours, tc.wantKills)
			}
			if inStat := mine.Name != ""; inStat != tc.wantInStat {
				t.Errorf("the standing names %q, want a named row only for an approved identity", mine.Name)
			}
		})
	}
}

func TestPlaceRefusesANamelessLookup(t *testing.T) {
	all := []totals{
		{Name: "", Hours: 500, Kills: 7, Deaths: 2},
		{Name: "Victim", Hours: 900, Kills: 120, Deaths: 30},
	}

	mine, hoursRank, killsRank := place(all, "")

	if mine != (totals{}) || hoursRank != 0 || killsRank != 0 {
		t.Errorf("place(all, \"\") = %#v, hours %d, kills %d; want zero totals and no places, because the nameless stats row belongs to no player",
			mine, hoursRank, killsRank)
	}
}

// playerRead is one call this package makes against the verification_requests collection.
type playerRead struct {
	where  string // file:line, for the failure message
	fn     string // enclosing function
	op     string // collection method
	filter ast.Expr
	args   []ast.Expr
}

// filterArgIndex maps the collection methods whose filter this guard reads to the position of that
// argument. A method outside the map builds its condition in a shape the guard cannot read, so it
// fails the test instead of passing silently.
var filterArgIndex = map[string]int{
	"FindOne":           1,
	"Find":              1,
	"CountDocuments":    1,
	"FindOneAndUpdate":  1,
	"FindOneAndReplace": 1,
	"FindOneAndDelete":  1,
	"UpdateOne":         1,
	"UpdateMany":        1,
	"DeleteOne":         1,
	"DeleteMany":        1,
	"ReplaceOne":        1,
	"Distinct":          2,
}

func TestEveryPlayersReadKeepsTheApprovedStatusWindow(t *testing.T) {
	scan := scanPackage(t)
	reads := scan.reads

	if len(reads) == 0 {
		t.Fatal("the scan found no read of the players collection, so it proves nothing")
	}

	// The site this guard was written for, named so a rename or a move shows up as a lost site
	// instead of a silently smaller scan.
	if !slices.ContainsFunc(reads, func(r playerRead) bool { return r.fn == "player" }) {
		t.Error("no players read was found in player(); the scan missed a known site")
	}

	for _, r := range reads {
		argPos, readable := filterArgIndex[r.op]
		if !readable {
			t.Errorf("%s: %s on the players collection carries no filter this guard can read; a read of another domain's collection must stay inside the approved-status window, so answer for it here", r.where, r.op)
			continue
		}
		if len(r.args) <= argPos || r.filter == nil {
			t.Errorf("%s: %s on the players collection passes no filter this guard can read", r.where, r.op)
			continue
		}

		window, ok := scan.statusWindow(r.filter)
		if !ok {
			t.Errorf("%s: %s filters the players collection with no status $in window this guard can read", r.where, r.op)
			continue
		}
		if !slices.Equal(window, approvedWindow) {
			t.Errorf("%s: %s allows the statuses %v, want %v; every other status carries a self-declared user_game_name that no moderator approved, and its totals would be attributed to the caller",
				r.where, r.op, window, approvedWindow)
		}

		scan.checkProjection(t, r)
	}
}

// statusWindowValue reads the statuses a built filter condition allows, sorted.
func statusWindowValue(t *testing.T, condition any) []string {
	t.Helper()

	doc, ok := condition.(bson.M)
	if !ok {
		t.Fatalf("status condition is %#v (%T), want bson.M{\"$in\": ...}", condition, condition)
	}
	list, ok := doc["$in"].(bson.A)
	if !ok {
		t.Fatalf("status condition %#v carries no $in list", doc)
	}

	var window []string
	for _, status := range list {
		name, ok := status.(string)
		if !ok {
			t.Fatalf("status window holds %#v (%T), want a string", status, status)
		}
		window = append(window, name)
	}
	slices.Sort(window)
	return window
}

// packageScan is this package's parsed source: its constants, its filter builders and the players
// reads a guard takes out of it.
type packageScan struct {
	fset     *token.FileSet
	consts   map[string]string   // package-level constant name -> string value
	builders map[string]ast.Expr // package-level function name -> the document literal it returns
	reads    []playerRead
}

func scanPackage(t *testing.T) *packageScan {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the package directory: %v", err)
	}

	scan := &packageScan{
		fset:     token.NewFileSet(),
		consts:   map[string]string{},
		builders: map[string]ast.Expr{},
	}

	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(scan.fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		t.Fatal("the package directory holds no source file")
	}

	// Constants and builders come first: a read may name either one.
	for _, file := range files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				scan.collectConsts(d)
			case *ast.FuncDecl:
				if d.Recv == nil {
					scan.collectBuilder(d)
				}
			}
		}
	}
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				scan.collectReads(fn)
			}
		}
	}

	return scan
}

func (s *packageScan) collectConsts(decl *ast.GenDecl) {
	if decl.Tok != token.CONST {
		return
	}
	for _, spec := range decl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range vs.Names {
			if i >= len(vs.Values) {
				continue
			}
			if value := stringLiteral(vs.Values[i]); value != "" {
				s.consts[name.Name] = value
			}
		}
	}
}

// collectBuilder remembers the document a package-level function returns, so a filter built by a
// function is still read.
func (s *packageScan) collectBuilder(fn *ast.FuncDecl) {
	if fn.Body == nil {
		return
	}

	var returned ast.Expr
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return true
		}
		if _, ok := ret.Results[0].(*ast.CompositeLit); ok {
			returned = ret.Results[0]
			return false
		}
		return true
	})
	if returned != nil {
		s.builders[fn.Name.Name] = returned
	}
}

func (s *packageScan) collectReads(fn *ast.FuncDecl) {
	locals := localExprs(fn.Body)

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isPlayersCollection(sel.X) {
			return true
		}

		read := playerRead{
			where: s.pos(call),
			fn:    fn.Name.Name,
			op:    sel.Sel.Name,
			args:  call.Args,
		}
		if argPos, readable := filterArgIndex[read.op]; readable && len(call.Args) > argPos {
			read.filter = s.resolve(call.Args[argPos], locals)
		}
		s.reads = append(s.reads, read)
		return true
	})
}

// checkProjection fails on a players read that projects the game name but drops created_at:
// Standing.Days is computed from that field, so a projection without it silently zeroes every
// day-gated catalog item for every caller.
func (s *packageScan) checkProjection(t *testing.T, r playerRead) {
	t.Helper()

	for _, arg := range r.args {
		ast.Inspect(arg, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "SetProjection" || len(call.Args) != 1 {
				return true
			}

			keys := documentKeys(call.Args[0])
			if slices.Contains(keys, "user_game_name") && !slices.Contains(keys, "created_at") {
				t.Errorf("%s: %s projects user_game_name without created_at, which zeroes Standing.Days for every caller", r.where, r.op)
			}
			return false
		})
	}
}

// statusWindow reads the statuses a filter allows. It answers false when the filter is not a
// document this guard can read, or when the document carries no status $in window.
func (s *packageScan) statusWindow(expr ast.Expr) ([]string, bool) {
	doc, ok := s.resolveDocument(expr).(*ast.CompositeLit)
	if !ok {
		return nil, false
	}

	condition, ok := documentValue(doc, "status").(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	in, ok := documentValue(condition, "$in").(*ast.CompositeLit)
	if !ok {
		return nil, false
	}

	var window []string
	for _, elt := range in.Elts {
		if name, ok := elt.(*ast.Ident); ok {
			value, found := s.consts[name.Name]
			if !found {
				return nil, false
			}
			window = append(window, value)
			continue
		}
		value := stringLiteral(elt)
		if value == "" {
			return nil, false
		}
		window = append(window, value)
	}
	if len(window) == 0 {
		return nil, false
	}

	slices.Sort(window)
	return window, true
}

// resolveDocument turns a filter argument into the document literal it evaluates to: the literal
// itself, the literal a builder function returns, or nothing this guard can read.
func (s *packageScan) resolveDocument(expr ast.Expr) ast.Expr {
	for range 8 {
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			return expr
		}
		name, ok := call.Fun.(*ast.Ident)
		if !ok {
			return expr
		}
		built, found := s.builders[name.Name]
		if !found {
			return expr
		}
		expr = built
	}
	return expr
}

// resolve follows a local name to the expression it holds.
func (s *packageScan) resolve(expr ast.Expr, locals map[string]ast.Expr) ast.Expr {
	for range 8 {
		name, ok := expr.(*ast.Ident)
		if !ok {
			return expr
		}
		held, found := locals[name.Name]
		if !found {
			return expr
		}
		expr = held
	}
	return expr
}

// localExprs maps a local name to the expression it was assigned, so a filter built in a variable
// is still read.
func localExprs(body *ast.BlockStmt) map[string]ast.Expr {
	locals := make(map[string]ast.Expr)

	ast.Inspect(body, func(n ast.Node) bool {
		stmt, ok := n.(*ast.AssignStmt)
		if !ok || len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
			return true
		}
		if name, ok := stmt.Lhs[0].(*ast.Ident); ok {
			locals[name.Name] = stmt.Rhs[0]
		}
		return true
	})

	return locals
}

// isPlayersCollection reports whether the receiver of a call is the verification_requests
// collection, s.players.
func isPlayersCollection(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "players"
}

func (s *packageScan) pos(n ast.Node) string {
	p := s.fset.Position(n.Pos())
	return filepath.Base(p.Filename) + ":" + strconv.Itoa(p.Line)
}

// documentValue reads one key of a bson.M or a bson.D literal.
func documentValue(doc *ast.CompositeLit, key string) ast.Expr {
	for _, elt := range doc.Elts {
		switch e := elt.(type) {
		case *ast.KeyValueExpr: // bson.M{"status": ...}
			if stringLiteral(e.Key) == key {
				return e.Value
			}
		case *ast.CompositeLit: // bson.D{{Key: "status", Value: ...}}
			if name, value := keyedElement(e); name == key {
				return value
			}
		}
	}
	return nil
}

// documentKeys lists the keys a bson.M or a bson.D literal carries.
func documentKeys(expr ast.Expr) []string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}

	var keys []string
	for _, elt := range lit.Elts {
		switch e := elt.(type) {
		case *ast.KeyValueExpr:
			if key := stringLiteral(e.Key); key != "" {
				keys = append(keys, key)
			}
		case *ast.CompositeLit:
			if key, _ := keyedElement(e); key != "" {
				keys = append(keys, key)
			}
		}
	}
	return keys
}

// keyedElement reads one bson.E literal into its key and its value.
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
			key = stringLiteral(kv.Value)
		case "Value":
			value = kv.Value
		}
	}
	return key, value
}

func stringLiteral(expr ast.Expr) string {
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
