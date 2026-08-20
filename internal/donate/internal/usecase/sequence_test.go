package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lasthearth/vsservice/internal/donate/internal/usecase"
)

// Inline runs the steps in order and stops at the first failure — the contract
// the Mongo adapter also has to keep.
func TestInlineSequenceStopsAtTheFirstFailure(t *testing.T) {
	var ran []int
	boom := errors.New("boom")

	err := usecase.Inline{}.Do(context.Background(),
		func(context.Context) error { ran = append(ran, 1); return nil },
		func(context.Context) error { ran = append(ran, 2); return boom },
		func(context.Context) error { ran = append(ran, 3); return nil },
	)
	if !errors.Is(err, boom) {
		t.Fatalf("Do: got %v, want boom", err)
	}
	if len(ran) != 2 || ran[0] != 1 || ran[1] != 2 {
		t.Fatalf("ran %v, want steps 1 and 2 only", ran)
	}
}
