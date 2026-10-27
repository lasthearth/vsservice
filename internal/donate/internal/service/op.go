package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
)

// Event keys of donate's domain-operation records (the plan's L2 layer). One
// key names one fact and never changes: dashboards and alerts filter on it,
// never on the message.
//
// The shape is <domain>.<entity>.<action>. A refusal — a rule that denied the
// action, so nothing changed — takes its own ".refused" key at Warn with
// Reason set, because a refusal is a different fact from the action it blocked.
// A failed write reuses the action key at Error with the cause attached, the
// way grpc.call.finish carries its code.
const (
	// Wallet balance changes. The wallet is addressed by player id: that is the
	// key the repository upserts on, and one player holds one wallet.
	opWalletCredited     = "donate.wallet.credited"
	opWalletDebited      = "donate.wallet.debited"
	opWalletDebitRefused = "donate.wallet.debit.refused"

	// Shop purchases and their afterlife. The purchase id is the entity id: a
	// refund, an issue and the ledger entry all point at it.
	opPurchaseCompleted     = "donate.purchase.completed"
	opPurchaseRefused       = "donate.purchase.refused"
	opPurchaseRefunded      = "donate.purchase.refunded"
	opPurchaseRefundRefused = "donate.purchase.refund.refused"
	opPurchaseIssued        = "donate.purchase.issued"
	opPurchaseIssueRefused  = "donate.purchase.issue.refused"
)

// coins is the only balance donate moves: the wallet model exposes Coins and
// nothing else. Naming it on every record keeps a replay query across domains
// answerable without reading the model.
const coins = "coins"

// Entity kinds.
const (
	entityWallet   = "wallet"
	entityPurchase = "purchase"
)

// opFor starts one domain-operation record: logger.OpFrom supplies the ctx
// correlation ids (request_id, user_id) and component=op, and the actor is the
// interceptor's subject. Handlers that already read the subject themselves do
// not call Actor again, so the record never carries actor_id twice.
//
// A missing subject is a normal answer, not an error: AddCoins also runs
// server-to-server for a referral grant, where no user is signed in. A logging
// detail must not fail the operation, and the access log already holds the auth
// outcome of every call that does have a subject.
func (s *Service) opFor(ctx context.Context) *logger.Op {
	op := logger.OpFrom(ctx, s.log)
	if uid, err := interceptor.GetUserID(ctx); err == nil && uid != "" {
		op = op.Actor(uid)
	}
	return op
}
