package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

// playersCollName is the player document collection. It belongs to
// internal/player: the game server and the site both write to it, and this
// domain only reads the two fields that say how far a referee has got, the way
// internal/appearance reads referral_events from here.
const playersCollName = "verification_requests"

// Referee statuses that mean "verification passed". They are repeated from
// internal/player/internal/model/verification rather than imported, because
// that package is internal to the player domain. "approved" is what an admin
// approval writes; "verified" is accepted too, so the day that state starts
// being written the payout follows it instead of silently stopping.
const (
	refereeStatusApproved = "approved"
	refereeStatusVerified = "verified"
)

// GetRefereeProgress reports whether refereePlayerID has passed verification on
// the site and whether they have ever joined the game server.
//
// Verification is the status on the referee's player document. A join is the
// last_heartbeat the game server writes on every join and every heartbeat and
// never clears — the presence sweeper only flips is_online back to false — so a
// field above zero is proof that the player has been in the game at least once.
//
// Both are counted rather than fetched: a user can own more than one player
// document, and one of them may carry the heartbeat while another carries the
// approval.
func (r *Repository) GetRefereeProgress(ctx context.Context, refereePlayerID string) (bool, bool, error) {
	l := r.log.With(zap.String("method", "GetRefereeProgress"), zap.String("referee_player_id", refereePlayerID))

	verifiedCount, err := r.playersColl.CountDocuments(ctx, bson.M{
		"user_id": refereePlayerID,
		"status":  bson.M{"$in": bson.A{refereeStatusApproved, refereeStatusVerified}},
	})
	if err != nil {
		l.Error("failed to read the referee verification status", zap.Error(err))
		return false, false, err
	}

	joinedCount, err := r.playersColl.CountDocuments(ctx, bson.M{
		"user_id":        refereePlayerID,
		"last_heartbeat": bson.M{"$gt": int64(0)},
	})
	if err != nil {
		l.Error("failed to read the referee join marker", zap.Error(err))
		return false, false, err
	}

	return verifiedCount > 0, joinedCount > 0, nil
}
