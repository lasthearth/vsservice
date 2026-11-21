package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/mail/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox/pagination"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

// ListMailsAdmin returns one page of mails for the admin list, each carrying
// the number of players who claimed it, plus the token for the next page.
//
// Newest first comes from the default `_id` descending sort: mongox mints `_id`
// from the creation time, so `_id` order and `created_at` order never disagree.
//
// The counters come from one aggregation over the claim rows of the page, not
// one query per mail. `mail_id` is the leftmost key of the unique
// (mail_id, player_id) index, so the $in is an index scan, and that uniqueness
// is what makes a row count equal a player count.
func (r *Repository) ListMailsAdmin(
	ctx context.Context,
	filter model.AdminMailFilter,
	pageToken string,
	pageSize int64,
) ([]*model.AdminMailEntry, string, error) {
	l := r.log.With(zap.String("method", "ListMailsAdmin"))

	mails, next, err := pagination.List(
		ctx, r.mailColl, pageToken, pageSize, mailFromDTO,
		pagination.WithFilter(adminMailFilter(filter)),
	)
	if err != nil {
		l.Error("failed to list mails", zap.Error(err))
		return nil, "", err
	}
	if len(mails) == 0 {
		return nil, next, nil
	}

	counts, err := r.claimedCounts(ctx, mails)
	if err != nil {
		l.Error("failed to count claims", zap.Error(err))
		return nil, "", err
	}

	entries := make([]*model.AdminMailEntry, len(mails))
	for i, m := range mails {
		entries[i] = model.NewAdminMailEntry(m, counts[m.Id])
	}
	return entries, next, nil
}

// adminMailFilter turns the domain filter into the Mongo predicate. Broadcast
// wins over an explicit recipient, matching AdminMailFilter's documented rule.
func adminMailFilter(f model.AdminMailFilter) bson.M {
	switch {
	case f.BroadcastOnly:
		return bson.M{"recipient": model.RecipientBroadcast}
	case f.Recipient != "":
		return bson.M{"recipient": f.Recipient}
	default:
		return bson.M{}
	}
}

// claimedCounts returns the number of claimed rows per mail id, for the ids of
// one page. Mails nobody claimed are absent from the map, which reads as zero.
func (r *Repository) claimedCounts(ctx context.Context, mails []*model.Mail) (map[string]int64, error) {
	ids := make([]string, len(mails))
	for i, m := range mails {
		ids[i] = m.Id
	}

	cursor, err := r.claimColl.Aggregate(ctx, claimedCountsPipeline(ids))
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := cursor.Close(ctx); cerr != nil {
			r.log.Error("cursor close failed", zap.Error(cerr))
		}
	}()

	var rows []struct {
		Id string `bson:"_id"`
		N  int64  `bson:"n"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}

	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.Id] = row.N
	}
	return counts, nil
}

// claimedCountsPipeline counts the claimed claim rows of the given mails,
// grouped by mail id. Only rows whose state is claimed are counted: a row that
// is merely read means the player opened the mail but took nothing.
func claimedCountsPipeline(mailIDs []string) bson.A {
	return bson.A{
		bson.D{{Key: "$match", Value: bson.M{
			"mail_id": bson.M{"$in": mailIDs},
			"state":   string(model.MailStateClaimed),
		}}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id": "$mail_id",
			"n":   bson.M{"$sum": 1},
		}}},
	}
}
