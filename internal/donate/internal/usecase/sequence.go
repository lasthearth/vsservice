// Package usecase holds donate's write rules: the operations that move coins
// between a wallet, a purchase record and the transaction ledger. They live
// here rather than in the Mongo repository so the rules can be read and tested
// without a database, and rather than in donateuc because they are internal to
// the donate domain — donateuc is the primitive-typed surface other modules
// consume.
package usecase

import "context"

// Sequence runs a series of writes in order, stopping at the first failure.
//
// It is deliberately NOT called a transaction: the deployed MongoDB is a
// standalone, so no adapter can roll anything back. What the Mongo adapter buys
// is a single session, i.e. read-your-own-writes across the steps — nothing
// more. Every caller therefore owns its ordering and its own compensation, and
// documents what a mid-sequence failure leaves behind.
type Sequence interface {
	Do(ctx context.Context, steps ...func(context.Context) error) error
}

// Inline is the Sequence without a database attached: it runs the steps on the
// caller's context. Used by tests, and a valid production fallback since the
// Mongo adapter adds session scoping only.
type Inline struct{}

func (Inline) Do(ctx context.Context, steps ...func(context.Context) error) error {
	for _, step := range steps {
		if err := step(ctx); err != nil {
			return err
		}
	}
	return nil
}
