package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/ticket/internal/dto"
	"github.com/lasthearth/vsservice/internal/ticket/internal/ierror"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

func (r *Repository) replyToModel(doc dto.SavedReply) *model.SavedReply {
	m := r.mapper.ReplyToModel(doc)
	return &m
}

func (r *Repository) replyToDTO(m *model.SavedReply) dto.SavedReply {
	return r.mapper.ReplyFromModel(*m)
}

// ListReplies returns every saved reply, by title.
func (r *Repository) ListReplies(ctx context.Context) ([]model.SavedReply, error) {
	cursor, err := r.replies.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "title", Value: 1}}))
	if err != nil {
		r.logger.WithMethod("ListReplies").Error("failed to list saved replies", zap.Error(err))
		return nil, err
	}
	var docs []dto.SavedReply
	if err := cursor.All(ctx, &docs); err != nil {
		r.logger.WithMethod("ListReplies").Error("failed to decode saved replies", zap.Error(err))
		return nil, err
	}
	out := make([]model.SavedReply, 0, len(docs))
	for _, d := range docs {
		out = append(out, *r.replyToModel(d))
	}
	return out, nil
}

// CountReplies counts the saved replies.
func (r *Repository) CountReplies(ctx context.Context) (int64, error) {
	n, err := r.replies.CountDocuments(ctx, bson.M{})
	if err != nil {
		r.logger.WithMethod("CountReplies").Error("failed to count saved replies", zap.Error(err))
	}
	return n, err
}

// CreateReply stores a new saved reply.
func (r *Repository) CreateReply(ctx context.Context, m *model.SavedReply) (*model.SavedReply, error) {
	doc := r.replyToDTO(m)
	doc.Model = mongox.NewModel()
	if _, err := r.replies.InsertOne(ctx, doc); err != nil {
		r.logger.WithMethod("CreateReply").Error("failed to insert saved reply", zap.Error(err))
		return nil, err
	}
	return r.replyToModel(doc), nil
}

// UpdateReply loads a saved reply, lets fn change it and stores it.
func (r *Repository) UpdateReply(
	ctx context.Context,
	id string,
	fn func(ctx context.Context, m *model.SavedReply) (*model.SavedReply, error),
) (*model.SavedReply, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrReplyNotFound
	}
	updated, err := mongox.UpdateDoc(ctx, r.replies, bson.M{"_id": oid}, ierror.ErrReplyNotFound, r.replyToModel, r.replyToDTO, fn)
	if err != nil && !isDomainErr(err) {
		r.logger.WithMethod("UpdateReply").Error("failed to update saved reply", zap.String("id", id), zap.Error(err))
	}
	return updated, err
}

// DeleteReply removes a saved reply.
func (r *Repository) DeleteReply(ctx context.Context, id string) error {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return ierror.ErrReplyNotFound
	}
	res, err := r.replies.DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		r.logger.WithMethod("DeleteReply").Error("failed to delete saved reply", zap.String("id", id), zap.Error(err))
		return err
	}
	if res.DeletedCount == 0 {
		return ierror.ErrReplyNotFound
	}
	return nil
}
