package service

import (
	"context"

	reactionv1 "github.com/lasthearth/vsservice/gen/reaction/v1"
	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/reaction/internal/model"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
)

// ListReactions implements reactionv1.ReactionServiceServer.
func (s *Service) ListReactions(ctx context.Context, req *reactionv1.ListReactionsRequest) (*reactionv1.ListReactionsResponse, error) {
	targets, err := model.ValidateTargets(req.GetTargets())
	if err != nil {
		return nil, err
	}

	counts, err := s.repo.Counts(ctx, targets)
	if err != nil {
		return nil, err
	}

	return &reactionv1.ListReactionsResponse{Reactions: groupCounts(targets, counts)}, nil
}

// ListMyReactions implements reactionv1.ReactionServiceServer.
func (s *Service) ListMyReactions(ctx context.Context, req *reactionv1.ListMyReactionsRequest) (*reactionv1.ListMyReactionsResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, ierror.Unauthenticated(err.Error())
	}

	targets, err := model.ValidateTargets(req.GetTargets())
	if err != nil {
		return nil, err
	}

	mine, err := s.repo.UserEmojis(ctx, userID, targets)
	if err != nil {
		return nil, err
	}

	out := make([]*reactionv1.MyReactions, 0, len(mine))
	for _, target := range targets {
		emojis, ok := mine[target]
		if !ok {
			continue
		}
		out = append(out, &reactionv1.MyReactions{Target: target, Emojis: ordered(emojis)})
	}

	return &reactionv1.ListMyReactionsResponse{Reactions: out}, nil
}

// ToggleReaction implements reactionv1.ReactionServiceServer.
func (s *Service) ToggleReaction(ctx context.Context, req *reactionv1.ToggleReactionRequest) (*reactionv1.ToggleReactionResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, ierror.Unauthenticated(err.Error())
	}

	if err := model.ValidateTarget(req.GetTarget()); err != nil {
		return nil, err
	}
	if err := model.ValidateEmoji(req.GetEmoji()); err != nil {
		return nil, err
	}

	active, err := s.repo.Toggle(ctx, req.GetTarget(), userID, req.GetEmoji())
	if err != nil {
		return nil, err
	}

	// The toggle has already committed, so a failed follow-up read must not
	// turn the call into an error: the client would retry and invert the
	// reaction the player just set. active is the authoritative part of the
	// response; the counts are a convenience that ListReactions also serves.
	counts, err := s.repo.Counts(ctx, []string{req.GetTarget()})
	if err != nil {
		s.logger.Error("failed to read reaction counts after toggle",
			zap.String("target", req.GetTarget()), zap.Error(err))
		return &reactionv1.ToggleReactionResponse{Active: active}, nil
	}

	return &reactionv1.ToggleReactionResponse{
		Active:    active,
		Reactions: groupCounts([]string{req.GetTarget()}, counts)[0],
	}, nil
}

// groupCounts returns one entry per target in request order, with counts in
// the fixed emoji order and zero counts left out.
func groupCounts(targets []string, counts []model.Count) []*reactionv1.TargetReactions {
	byTarget := make(map[string]map[string]int64, len(targets))
	for _, c := range counts {
		if byTarget[c.Target] == nil {
			byTarget[c.Target] = make(map[string]int64)
		}
		byTarget[c.Target][c.Emoji] = c.Count
	}

	out := make([]*reactionv1.TargetReactions, 0, len(targets))
	for _, target := range targets {
		entry := &reactionv1.TargetReactions{Target: target, Counts: []*reactionv1.ReactionCount{}}
		for _, emoji := range model.Emojis {
			if n := byTarget[target][emoji]; n > 0 {
				entry.Counts = append(entry.Counts, &reactionv1.ReactionCount{Emoji: emoji, Count: n})
			}
		}
		out = append(out, entry)
	}

	return out
}

// ordered sorts emojis into the fixed display order, dropping unknown ones.
func ordered(emojis []string) []string {
	have := make(map[string]struct{}, len(emojis))
	for _, e := range emojis {
		have[e] = struct{}{}
	}

	out := make([]string, 0, len(emojis))
	for _, e := range model.Emojis {
		if _, ok := have[e]; ok {
			out = append(out, e)
		}
	}
	return out
}
