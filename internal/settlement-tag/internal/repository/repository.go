package repository

import (
	"context"
	"errors"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/settlement-tag/internal/dto"
	"github.com/lasthearth/vsservice/internal/settlement-tag/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement-tag/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// GetTagsByIds implements service.Repository.
func (r *Repository) GetTagsByIds(ctx context.Context, ids []string) ([]model.Tag, error) {
	oids := make([]bson.ObjectID, len(ids))
	for i, id := range ids {
		oid, err := mongox.ParseObjectID(id)
		if err != nil {
			continue
		}
		oids[i] = oid
	}
	var dtos []dto.Tag
	filter := bson.M{"_id": bson.M{"$in": oids}}
	cursor, err := r.tagsColl.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	defer cursor.Close(ctx)
	err = cursor.All(ctx, &dtos)
	if err != nil {
		return nil, err
	}

	tags := r.mapper.FromTagDtos(dtos)
	return tags, nil
}

// GetTags implements service.Repository.
func (r *Repository) GetTags(ctx context.Context) ([]model.Tag, error) {
	var dtos []dto.Tag
	filter := bson.M{"is_active": true}
	cursor, err := r.tagsColl.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	defer cursor.Close(ctx)
	err = cursor.All(ctx, &dtos)
	if err != nil {
		return nil, err
	}

	tags := r.mapper.FromTagDtos(dtos)
	return tags, nil
}

// CreateTag creates a new tag in the database
func (r *Repository) CreateTag(ctx context.Context, tag *model.Tag) (*model.Tag, error) {
	existingTag, err := r.GetTagByName(ctx, tag.Name)
	if err != nil {
		return nil, err
	}
	if existingTag != nil && existingTag.IsActive {
		return nil, ierror.ErrAlreadyExists
	}

	tagDto := r.mapper.ToTagDto(*tag)

	inserted, err := r.tagsColl.InsertOne(ctx, tagDto)
	if err != nil {
		return nil, errors.Join(ierror.ErrConstraintViolation, err)
	}
	oid, err := mongox.ParseAnyObjectID(inserted.InsertedID)
	if err != nil {
		return nil, err
	}
	tag.Id = oid.Hex()

	return tag, nil
}

// GetTagById retrieves a tag by its ID
func (r *Repository) GetTagById(ctx context.Context, id string) (*model.Tag, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, errors.Join(ierror.ErrInvalidArgument, err)
	}

	var dto dto.Tag
	filter := bson.M{"_id": oid}
	err = r.tagsColl.FindOne(ctx, filter).Decode(&dto)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ierror.ErrNotFound
		}
		return nil, err
	}

	tag := r.mapper.FromTagDto(dto)
	return &tag, nil
}

// GetTagByName retrieves a tag by its name
func (r *Repository) GetTagByName(ctx context.Context, name string) (*model.Tag, error) {
	var tag model.Tag
	filter := bson.M{"name": name}
	err := r.tagsColl.FindOne(ctx, filter).Decode(&tag)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Return nil if not found, not an error
		}
		return nil, err
	}

	return &tag, nil
}

// GetAllTags retrieves all tags with an option to filter only active ones
func (r *Repository) GetAllTags(ctx context.Context, onlyActive bool) ([]*model.Tag, error) {
	filter := bson.M{}
	if onlyActive {
		filter["is_active"] = true
	}

	cursor, err := r.tagsColl.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var tags []*model.Tag
	if err = cursor.All(ctx, &tags); err != nil {
		return nil, err
	}

	return tags, nil
}

// SoftDeleteTag marks a tag as inactive (soft delete)
func (r *Repository) SoftDeleteTag(ctx context.Context, id string) error {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return errors.Join(ierror.ErrInvalidArgument, err)
	}

	filter := bson.M{"_id": oid}
	update := bson.M{"$set": bson.M{
		"is_active":  false,
		"updated_at": time.Now(),
	}}

	result, err := r.tagsColl.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return ierror.ErrNotFound
	}

	return nil
}
