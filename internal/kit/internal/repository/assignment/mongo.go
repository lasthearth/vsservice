package assignment

import (
	"context"

	dto "github.com/lasthearth/vsservice/internal/kit/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/kit/internal/ierror"
	"github.com/lasthearth/vsservice/internal/kit/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

// CreateAssignment creates a new assignment in the database
func (r *Repository) CreateAssignment(ctx context.Context, assignment *model.KitAssignment) (*model.KitAssignment, error) {
	l := r.log.With(
		zap.String("method", "CreateAssignment"),
		zap.String("assignment_id", assignment.Id),
		zap.String("user_id", assignment.UserId),
		zap.String("kit_name", assignment.KitName),
	)

	l.Info("creating assignment")

	dtoObj := r.mapper.FromAssignment(*assignment)

	result, err := r.coll.InsertOne(ctx, dtoObj)
	if err != nil {
		l.Error("failed to insert assignment", zap.Error(err))
		return nil, err
	}

	oid, err := mongox.ParseAnyObjectID(result.InsertedID)
	if err != nil {
		l.Error("failed to parse inserted ID", zap.Error(err))
		return nil, err
	}

	assignment.Id = oid.Hex()

	l.Info("successfully created assignment")
	return assignment, nil
}

// GetAssignment retrieves an assignment by ID from the database
func (r *Repository) GetAssignment(ctx context.Context, assignmentID string) (*model.KitAssignment, error) {
	l := r.log.With(
		zap.String("method", "GetAssignment"),
		zap.String("assignment_id", assignmentID),
	)

	l.Info("retrieving assignment")

	var dtoObj dto.Assignment
	err := r.coll.FindOne(ctx, bson.M{"_id": assignmentID}).Decode(&dtoObj)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			l.Info("assignment not found", zap.String("assignment_id", assignmentID))
			return nil, ierror.ErrNotFound
		}
		l.Error("failed to find assignment", zap.Error(err))
		return nil, err
	}

	assignment := r.mapper.ToAssignment(dtoObj)
	l.Info("successfully retrieved assignment")
	return &assignment, nil
}

// UpdateAssignment updates an assignment using the provided function
func (r *Repository) UpdateAssignment(
	ctx context.Context,
	assignmentID string,
	updateFn func(ctx context.Context, assignment *model.KitAssignment) (*model.KitAssignment, error),
) error {
	l := r.log.With(
		zap.String("method", "UpdateAssignment"),
		zap.String("assignment_id", assignmentID),
	)

	l.Info("updating assignment")

	// Get the current assignment
	var dtoObj dto.Assignment
	err := r.coll.FindOne(ctx, bson.M{"_id": assignmentID}).Decode(&dtoObj)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			l.Info("assignment not found", zap.String("assignment_id", assignmentID))
			return ierror.ErrNotFound
		}
		l.Error("failed to find assignment", zap.Error(err))
		return err
	}

	// Convert DTO to model for processing
	assignment := r.mapper.ToAssignment(dtoObj)
	assignmentPtr := &assignment

	// Apply the update function
	updatedAssignment, err := updateFn(ctx, assignmentPtr)
	if err != nil {
		l.Error("failed to update assignment in update function", zap.Error(err))
		return err
	}

	// Convert back to DTO
	dtoToUpdate := r.mapper.FromAssignment(*updatedAssignment)

	// Update in database
	_, err = r.coll.ReplaceOne(ctx, bson.M{"_id": assignmentID}, dtoToUpdate)
	if err != nil {
		l.Error("failed to update assignment in database", zap.Error(err))
		return err
	}

	l.Info("successfully updated assignment")
	return nil
}

// GetAssignmentsByUserID retrieves all assignments for a specific user
func (r *Repository) GetAssignmentsByUserID(ctx context.Context, userID string) ([]*model.KitAssignment, error) {
	l := r.log.With(
		zap.String("method", "GetAssignmentsByUserID"),
		zap.String("user_id", userID),
	)

	l.Info("retrieving assignments by user ID")

	cursor, err := r.coll.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		l.Error("failed to find assignments", zap.Error(err))
		return nil, err
	}
	defer cursor.Close(ctx)

	var dtos []dto.Assignment
	if err := cursor.All(ctx, &dtos); err != nil {
		l.Error("failed to decode assignments", zap.Error(err))
		return nil, err
	}

	// Convert DTOs to models
	assignments := r.mapper.ToAssignments(dtos)
	// Convert to []*model.KitAssignment slice
	result := make([]*model.KitAssignment, len(assignments))
	for i := range assignments {
		result[i] = &assignments[i]
	}

	l.Info("successfully retrieved assignments", zap.Int("count", len(result)))
	return result, nil
}
