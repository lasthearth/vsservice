package repository

import (
	"errors"
	"testing"

	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
)

// donate_shop_items carries a unique index on code (setupIndexes) and a hidden
// entry still holds its code, so writing a taken code fails E11000 — on the
// insert of a new entry and on the replace that moves an entry onto another
// code. That driver error used to travel up unchanged and the service answered
// 500 "failed to create shop item" or "failed to update shop item", which says
// nothing about the code. It must arrive as the domain error the service turns
// into 409.
func TestDuplicateCodeErrMapsTheUniqueIndexViolation(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{
			name: "insert of a taken code",
			err: mgo.WriteException{WriteErrors: mgo.WriteErrors{{
				Code:    11000,
				Message: `E11000 duplicate key error collection: donate.donate_shop_items index: code_1 dup key: { code: "foodkit" }`,
			}}},
		},
		{
			name: "update onto a taken code",
			err: mgo.WriteException{WriteErrors: mgo.WriteErrors{{
				Code:    11001,
				Message: `E11001 duplicate key error collection: donate.donate_shop_items index: code_1 dup key: { code: "foodkit" }`,
			}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !mgo.IsDuplicateKeyError(tc.err) {
				t.Fatal("the fixture is not a duplicate-key error, so the test proves nothing")
			}

			got := duplicateCodeErr(tc.err)
			if !errors.Is(got, ierror.ErrShopCodeExists) {
				t.Fatalf("duplicateCodeErr(%v) = %v, want ErrShopCodeExists", tc.err, got)
			}
		})
	}
}

// Anything else stays the caller's error: the repository logs it and the service
// answers 500, because only a taken code is something the caller can fix.
func TestDuplicateCodeErrLeavesOtherFailuresAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "no write errors", err: mgo.WriteException{}},
		{name: "another write error", err: mgo.WriteException{
			WriteErrors: mgo.WriteErrors{{Code: 121, Message: "Document failed validation"}},
		}},
		{name: "plain error", err: errors.New("server selection timeout")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := duplicateCodeErr(tc.err); got != nil {
				t.Fatalf("duplicateCodeErr(%v) = %v, want nil", tc.err, got)
			}
		})
	}
}
