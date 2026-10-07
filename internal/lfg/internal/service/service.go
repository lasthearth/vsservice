package service

import (
	"context"
	"errors"
	"fmt"

	lfgv1 "github.com/lasthearth/vsservice/gen/lfg/v1"
	"github.com/lasthearth/vsservice/internal/lfg/internal/ierror"
	"github.com/lasthearth/vsservice/internal/lfg/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
)

const (
	defaultPageSize = 50
	maxPageSize     = 100
)

// CreatePost implements lfgv1.LfgServiceServer.
func (s *Service) CreatePost(ctx context.Context, req *lfgv1.CreatePostRequest) (*lfgv1.Post, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}

	now := s.now()
	post, err := model.NewPost(userID, s.mapper.CreateRequestToDetails(req), now)
	if err != nil {
		return nil, mapModelErr(err)
	}

	limit := model.MaxOpenSessionsPerAuthor
	if post.Kind == model.KindTeammate {
		limit = model.MaxOpenTeammatePerAuthor
	}
	open, err := s.repo.CountOpenByAuthor(ctx, userID, post.Kind, now)
	if err != nil {
		return nil, err
	}
	if open >= int64(limit) {
		return nil, ierror.ErrOpenPostLimit
	}

	created, err := s.repo.Create(ctx, post)
	if err != nil {
		return nil, err
	}

	return s.mapper.ToProto(*created), nil
}

// ListPosts implements lfgv1.LfgServiceServer.
func (s *Service) ListPosts(ctx context.Context, req *lfgv1.ListPostsRequest) (*lfgv1.ListPostsResponse, error) {
	limit := int(req.GetPageSize())
	if limit <= 0 {
		limit = defaultPageSize
	}
	limit = min(limit, maxPageSize)

	kind := KindFromProto(req.GetKind())
	if kind == "" {
		return nil, pkgerr.InvalidArgument("kind is required")
	}

	posts, err := s.repo.ListOpen(ctx, s.now(), kind, ActivityFromProto(req.GetActivity()), limit)
	if err != nil {
		return nil, err
	}

	return &lfgv1.ListPostsResponse{Posts: s.mapper.ToProtos(posts)}, nil
}

// GetPost implements lfgv1.LfgServiceServer.
func (s *Service) GetPost(ctx context.Context, req *lfgv1.GetPostRequest) (*lfgv1.Post, error) {
	post, err := s.repo.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	return s.mapper.ToProto(*post), nil
}

// RespondToPost implements lfgv1.LfgServiceServer.
func (s *Service) RespondToPost(ctx context.Context, req *lfgv1.RespondToPostRequest) (*lfgv1.RespondToPostResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}

	now := s.now()
	var joined bool
	updated, err := s.repo.UpdatePost(ctx, req.GetId(), func(_ context.Context, p *model.Post) (*model.Post, error) {
		in, err := p.Respond(userID, now)
		if err != nil {
			return nil, mapModelErr(err)
		}
		joined = in
		return p, nil
	})
	if err != nil {
		return nil, err
	}

	if joined {
		s.notifyAuthor(ctx, updated)
	}

	return &lfgv1.RespondToPostResponse{Joined: joined, Post: s.mapper.ToProto(*updated)}, nil
}

// ClosePost implements lfgv1.LfgServiceServer.
func (s *Service) ClosePost(ctx context.Context, req *lfgv1.ClosePostRequest) (*lfgv1.Post, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}

	now := s.now()
	updated, err := s.repo.UpdatePost(ctx, req.GetId(), func(_ context.Context, p *model.Post) (*model.Post, error) {
		if err := p.Close(userID, now); err != nil {
			return nil, mapModelErr(err)
		}
		return p, nil
	})
	if err != nil {
		return nil, err
	}

	return s.mapper.ToProto(*updated), nil
}

// RenewPost implements lfgv1.LfgServiceServer.
func (s *Service) RenewPost(ctx context.Context, req *lfgv1.RenewPostRequest) (*lfgv1.Post, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}

	now := s.now()
	updated, err := s.repo.UpdatePost(ctx, req.GetId(), func(_ context.Context, p *model.Post) (*model.Post, error) {
		if err := p.Renew(userID, now); err != nil {
			return nil, mapModelErr(err)
		}
		return p, nil
	})
	if err != nil {
		return nil, err
	}

	return s.mapper.ToProto(*updated), nil
}

// GetPostContact implements lfgv1.LfgServiceServer.
func (s *Service) GetPostContact(ctx context.Context, req *lfgv1.GetPostContactRequest) (*lfgv1.GetPostContactResponse, error) {
	if _, err := interceptor.GetUserID(ctx); err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}

	post, err := s.repo.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if post.ClosedAt != nil {
		return nil, ierror.ErrPostClosed
	}

	return &lfgv1.GetPostContactResponse{Contact: post.Contact}, nil
}

// notifyAuthor tells the author someone joined the session or is interested
// in the teammate post. The response stands even if the notification fails,
// so the failure is only logged.
func (s *Service) notifyAuthor(ctx context.Context, p *model.Post) {
	if s.cnuc == nil {
		return
	}

	title := "Новый отклик"
	text := fmt.Sprintf("К «%s» присоединился игрок (%d/%d)", p.Title, len(p.Responders), p.Slots)
	if p.Kind == model.KindTeammate {
		title = "Вашим объявлением заинтересовались"
		text = fmt.Sprintf("Игрок откликнулся на «%s». Всего откликов: %d", p.Title, len(p.Responders))
	}

	if err := s.cnuc.CreateNotification(
		ctx,
		title,
		text,
		notificationuc.WithUserId(p.AuthorId),
	); err != nil {
		s.logger.Error("failed to notify post author", zap.String("post_id", p.Id), zap.Error(err))
	}
}

// mapModelErr turns a model refusal into a typed domain error.
func mapModelErr(err error) error {
	switch {
	case errors.Is(err, model.ErrPostClosed):
		return ierror.ErrPostClosed
	case errors.Is(err, model.ErrOwnPost):
		return ierror.ErrOwnPost
	case errors.Is(err, model.ErrPostFull):
		return ierror.ErrPostFull
	case errors.Is(err, model.ErrNotAuthor):
		return ierror.ErrNotAuthor
	case errors.Is(err, model.ErrNotRenewable):
		return ierror.ErrNotRenewable
	case errors.Is(err, model.ErrRenewTooSoon):
		return ierror.ErrRenewTooSoon
	case errors.Is(err, model.ErrKindInvalid),
		errors.Is(err, model.ErrActivityInvalid),
		errors.Is(err, model.ErrTitleInvalid),
		errors.Is(err, model.ErrDescriptionTooLong),
		errors.Is(err, model.ErrSlotsInvalid),
		errors.Is(err, model.ErrStartInvalid),
		errors.Is(err, model.ErrScheduleInvalid),
		errors.Is(err, model.ErrExperienceInvalid),
		errors.Is(err, model.ErrContactInvalid):
		return pkgerr.InvalidArgument(err.Error())
	default:
		return err
	}
}
