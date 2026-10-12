package service

import (
	"context"
	"time"

	wikiv1 "github.com/lasthearth/vsservice/gen/wiki/v1"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// defaultPageSize is the page size used when a request does not set one.
const defaultPageSize = 20

// maxPageSize bounds a page so one request cannot ask for the whole wiki.
const maxPageSize = 50

// CreateArticle implements wikiv1.WikiServiceServer.
func (s *Service) CreateArticle(ctx context.Context, req *wikiv1.CreateArticleRequest) (*wikiv1.Article, error) {
	author, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	input := s.fromProto(req.GetArticle())

	// The domain derives the slug from the title and rejects the rest, so the
	// collision check has to run on the finished article, not on the request.
	article, err := model.NewArticle(input, author, time.Now())
	if err != nil {
		return nil, err
	}

	// A new address must not collide with an existing page: the unique index is
	// the last line of defence, this is the readable error.
	if taken, err := s.slugTaken(ctx, article.Slug, ""); err != nil {
		return nil, err
	} else if taken {
		return nil, model.ErrSlugTaken
	}

	created, err := s.repo.Create(ctx, article)
	if err != nil {
		return nil, err
	}

	return s.toProto(created), nil
}

// UpdateArticle implements wikiv1.WikiServiceServer.
func (s *Service) UpdateArticle(ctx context.Context, req *wikiv1.UpdateArticleRequest) (*wikiv1.Article, error) {
	author, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	id := req.GetArticle().GetId()
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "article id is required")
	}

	stored, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	input := s.fromProto(req.GetArticle())
	if input.Slug == "" {
		input.Slug = model.Slugify(input.Title)
	}

	if taken, err := s.slugTaken(ctx, input.Slug, id); err != nil {
		return nil, err
	} else if taken {
		return nil, model.ErrSlugTaken
	}

	// The state being replaced is saved before the edit lands, so an editor can
	// bring the page back to what it was.
	stored.SaveRevision(req.GetNote(), time.Now())

	if err := stored.ApplyEdit(input, author, req.GetExpectedVersion(), time.Now()); err != nil {
		return nil, err
	}

	updated, err := s.repo.Save(ctx, stored, req.GetExpectedVersion())
	if err != nil {
		return nil, err
	}

	return s.toProto(updated), nil
}

// DeleteArticle implements wikiv1.WikiServiceServer.
func (s *Service) DeleteArticle(ctx context.Context, req *wikiv1.DeleteArticleRequest) (*emptypb.Empty, error) {
	if err := s.repo.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// GetArticle implements wikiv1.WikiServiceServer.
func (s *Service) GetArticle(ctx context.Context, req *wikiv1.GetArticleRequest) (*wikiv1.Article, error) {
	article, err := s.repo.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	return s.toProto(article), nil
}

// GetArticleBySlug implements wikiv1.WikiServiceServer.
func (s *Service) GetArticleBySlug(ctx context.Context, req *wikiv1.GetArticleBySlugRequest) (*wikiv1.Article, error) {
	article, err := s.repo.GetBySlug(ctx, req.GetSlug())
	if err != nil {
		return nil, err
	}

	return s.toProto(article), nil
}

// ListArticles implements wikiv1.WikiServiceServer.
func (s *Service) ListArticles(ctx context.Context, req *wikiv1.ListArticlesRequest) (*wikiv1.ListArticlesResponse, error) {
	filter := model.Filter{
		Section: req.GetSection(),
		Status:  ProtoToStatus(req.GetStatus()),
		Search:  req.GetSearch(),
		Tag:     req.GetTag(),
	}

	// The list is public, so an unpublished page never leaves the service
	// unless the caller asked for drafts and is allowed to write.
	filter.IncludeDrafts = filter.Status == model.StatusDraft && s.canEdit(ctx)

	limit := int64(req.GetPageSize())
	if limit <= 0 {
		limit = defaultPageSize
	}
	limit = min(limit, maxPageSize)

	page, err := s.repo.List(ctx, filter, req.GetPageToken(), limit)
	if err != nil {
		return nil, err
	}

	articles := make([]*wikiv1.Article, 0, len(page.Articles))
	for _, article := range page.Articles {
		articles = append(articles, s.toProto(article))
	}

	return &wikiv1.ListArticlesResponse{
		Articles:      articles,
		NextPageToken: page.Next,
		TotalSize:     int32(page.Total),
	}, nil
}

// ListRevisions implements wikiv1.WikiServiceServer.
func (s *Service) ListRevisions(ctx context.Context, req *wikiv1.ListRevisionsRequest) (*wikiv1.ListRevisionsResponse, error) {
	article, err := s.repo.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	revisions := make([]*wikiv1.Revision, 0, len(article.Revisions))
	for _, revision := range article.Revisions {
		revisions = append(revisions, s.mapper.ToRevisionProto(revision))
	}

	return &wikiv1.ListRevisionsResponse{Revisions: revisions}, nil
}

// RestoreRevision implements wikiv1.WikiServiceServer.
func (s *Service) RestoreRevision(ctx context.Context, req *wikiv1.RestoreRevisionRequest) (*wikiv1.Article, error) {
	author, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	article, err := s.repo.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	target, found := findRevision(article.Revisions, req.GetRevisionId())
	if !found {
		return nil, model.ErrRevisionNotFound
	}

	if err := article.Restore(target, author, time.Now()); err != nil {
		return nil, err
	}

	restored, err := s.repo.Save(ctx, article, 0)
	if err != nil {
		return nil, err
	}

	return s.toProto(restored), nil
}

// slugTaken reports whether the slug belongs to another article.
func (s *Service) slugTaken(ctx context.Context, slug string, exceptId string) (bool, error) {
	if slug == "" {
		return false, status.Error(codes.InvalidArgument, "slug is required")
	}

	return s.repo.ExistsSlug(ctx, slug, exceptId)
}

// canEdit reports whether the caller may see unpublished pages.
//
// The token is optional on a public read, so a missing one is not an error: it
// only means the caller is a reader.
func (s *Service) canEdit(ctx context.Context) bool {
	_, err := interceptor.GetUserID(ctx)

	return err == nil
}

// toProto converts an article, keeping the logger out of the call sites.
func (s *Service) toProto(article *model.Article) *wikiv1.Article {
	if article == nil {
		return nil
	}

	return s.mapper.ToProto(*article)
}

// fromProto converts the editable fields of a request into a domain article.
//
// The fields are collected here and handed to the domain in one call: the
// service reads the request, and the model decides what an article is made of.
func (s *Service) fromProto(input *wikiv1.Article) model.Article {
	if input == nil {
		return model.Article{}
	}

	cover := input.GetCover()
	answer := input.GetQuickAnswer()

	return model.Article{
		Slug:        input.GetSlug(),
		Title:       input.GetTitle(),
		Section:     input.GetSection(),
		Lead:        input.GetLead(),
		Tags:        input.GetTags(),
		GameVersion: input.GetGameVersion(),
		Related:     input.GetRelated(),
		Body:        input.GetBody(),
		Status:      ProtoToStatus(input.GetStatus()),
		Cover:       coverOf(cover),
		QuickAnswer: answerOf(answer),
	}
}

// coverOf converts the protobuf cover into the domain type, if there is one.
func coverOf(cover *wikiv1.Cover) *model.Cover {
	if cover == nil {
		return nil
	}

	return &model.Cover{Url: cover.GetUrl(), Focus: cover.GetFocus()}
}

// answerOf converts the protobuf quick answer into the domain type, if there is
// one.
func answerOf(answer *wikiv1.QuickAnswer) *model.QuickAnswer {
	if answer == nil {
		return nil
	}

	return &model.QuickAnswer{Question: answer.GetQuestion(), Answer: answer.GetAnswer()}
}

// findRevision looks a revision up by its identifier.
func findRevision(revisions []model.Revision, id string) (model.Revision, bool) {
	for _, revision := range revisions {
		if revision.Id == id {
			return revision, true
		}
	}

	return model.Revision{}, false
}

// logValidation keeps the logger in use at the validation boundary, where a
// rejected edit is worth a line: it is the difference between a typo and a
// broken client.
func (s *Service) logValidation(err error) {
	s.logger.Warn("wiki article rejected", zap.Error(err))
}
