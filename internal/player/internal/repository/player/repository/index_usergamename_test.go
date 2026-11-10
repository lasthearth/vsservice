package repository

// The uniqueness of the in-game name, and the way the indexes are built.
//
// user_game_name is the key the roster, the leaderboard, the game mod's presence
// write and the appearance standing attribute stats by, and the repository
// already answers a duplicate with ErrNickAlreadyExists on the strength of an
// index that did not exist. These tests pin the declaration and the build order,
// because both halves fail silently: an absent unique index turns
// ErrNickAlreadyExists into dead code, and a CreateMany batch reports one error
// for the whole set, so the index that cannot build takes the presence index
// with it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// userGameNameIndexModel returns the declared index that leads on user_game_name.
func userGameNameIndexModel(t *testing.T) mongo.IndexModel {
	t.Helper()

	for _, im := range playerIndexes {
		keys, ok := im.Keys.(bson.D)
		if !ok || len(keys) == 0 || keys[0].Key != "user_game_name" {
			continue
		}

		return im
	}

	t.Fatal("playerIndexes declares no index leading on user_game_name, so nothing enforces one name per player and the roster's $lookup scans stats per player")

	return mongo.IndexModel{}
}

// TestUserGameNameIndexIsUnique pins the index the repository's
// ErrNickAlreadyExists answer assumes. A non-unique index lets a second
// verification document claim a name that is already attributed to another
// user_id, and every name-keyed read then answers from two documents.
func TestUserGameNameIndexIsUnique(t *testing.T) {
	im := userGameNameIndexModel(t)

	wantKeys := bson.D{{Key: "user_game_name", Value: 1}}
	if keys, ok := im.Keys.(bson.D); !ok || !equalAny(keys, wantKeys) {
		t.Errorf("user_game_name index keys = %v, want %v", im.Keys, wantKeys)
	}

	opts := declaredIndexOptions(t, im)
	if opts.Unique == nil || !*opts.Unique {
		t.Error("the user_game_name index is not unique, so two players can hold the same name and the repository's duplicate-key answer is unreachable")
	}
	if opts.Name == nil || *opts.Name != "uniq_user_game_name" {
		t.Errorf("the user_game_name index is named %v, want uniq_user_game_name: an operator dropping the superseded plain index needs a stable name", opts.Name)
	}
}

// TestUserGameNameIndexIsPartialOnANonEmptyName pins the partial filter.
//
// Documents with no in-game name exist: the roster filters them out with
// $nin ["", nil]. A plain unique index would refuse the second such document and
// break verification submissions for every player who has not set a name yet.
// The filter must therefore keep only a non-empty string, which excludes a
// missing field, null and "".
func TestUserGameNameIndexIsPartialOnANonEmptyName(t *testing.T) {
	got := declaredIndexOptions(t, userGameNameIndexModel(t)).PartialFilterExpression

	partial, ok := got.(bson.M)
	if !ok {
		t.Fatalf("user_game_name index partialFilterExpression = %v (%T), want a bson document", got, got)
	}

	want := bson.M{"user_game_name": bson.M{"$type": "string", "$gt": ""}}
	if !equalAny(partial, want) {
		t.Fatalf("user_game_name index partialFilterExpression = %v, want %v", partial, want)
	}

	// Read the two conditions back separately, so the failure names the
	// condition that was lost rather than reporting a document mismatch.
	cond, ok := partial["user_game_name"].(bson.M)
	if !ok {
		t.Fatalf("the partial filter does not constrain user_game_name with a document: %v", partial)
	}
	if !equalAny(cond["$type"], "string") {
		t.Errorf("the partial filter takes $type %v, want \"string\": a missing or null name would enter the index and collide with the next one", cond["$type"])
	}
	if !equalAny(cond["$gt"], "") {
		t.Errorf("the partial filter takes $gt %v, want \"\": an empty name would enter the index and collide with the next one", cond["$gt"])
	}
}

// TestSetupIndexesCreatesOneAtIndexAtATime reads app.go and fails when
// setupIndexes goes back to a batch call.
//
// The unique index cannot build until an operator de-duplicates the existing
// rows and drops the superseded plain index. Under CreateMany that one failure
// aborts the whole batch, so the presence index the sweeper depends on would
// not be created either, and the single logged error would not say which spec
// failed.
func TestSetupIndexesCreatesOneAtIndexAtATime(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "app.go", nil, 0)
	if err != nil {
		t.Fatalf("parse app.go: %v", err)
	}

	var body *ast.BlockStmt
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "setupIndexes" {
			body = fn.Body
		}
	}
	if body == nil {
		t.Fatal("app.go declares no setupIndexes, so nothing creates the verification_requests indexes")
	}

	calls := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			calls[sel.Sel.Name] = true
		}
		return true
	})

	if calls["CreateMany"] {
		t.Error("setupIndexes calls CreateMany: one spec that cannot build aborts the batch and the presence index is lost with it")
	}
	if !calls["CreateOne"] {
		t.Error("setupIndexes does not call CreateOne, so a single failing index spec is not isolated")
	}
}
