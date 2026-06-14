//go:generate goverter gen github.com/lasthearth/vsservice/internal/kit/internal/service
package service

import (
	"context"
	"time"

	kitv1 "github.com/lasthearth/vsservice/gen/kit/v1"
	"github.com/lasthearth/vsservice/internal/kit/internal/model"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type KitRepository interface {
	GetKits(ctx context.Context) ([]string, error)
}

type AssignmentRepository interface {
	CreateAssignment(ctx context.Context, assignment *model.KitAssignment) (*model.KitAssignment, error)
	GetAssignment(ctx context.Context, assignmentID string) (*model.KitAssignment, error)
	UpdateAssignment(
		ctx context.Context,
		assignmentID string,
		updateFn func(ctx context.Context, assignment *model.KitAssignment) (*model.KitAssignment, error),
	) error
	GetAssignmentsByUserID(ctx context.Context, userID string) ([]*model.KitAssignment, error)
}

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimePtrToTimestamp
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields
	ToAssignmentProto(*model.KitAssignment) *kitv1.Assignment
	ToAssignmentsProto([]*model.KitAssignment) []*kitv1.Assignment
}

// GetAvailableKits implements kitv1.KitServiceServer.
func (s *Service) GetAvailableKits(ctx context.Context, req *kitv1.GetAvailableKitsRequest) (*kitv1.GetAvailableKitsResponse, error) {
	l := s.log.With(zap.String("method", "get_available_kits"))

	l.Info("retrieving available kits")

	kits, err := s.kitRepo.GetKits(ctx)
	if err != nil {
		l.Error("failed to get active kits", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	l.Info("successfully retrieved available kits", zap.Int("count", len(kits)))

	return &kitv1.GetAvailableKitsResponse{
		Kits: kits,
	}, nil
}

// AssignKitToUser implements kitv1.KitServiceServer.
func (s *Service) AssignKitToUser(ctx context.Context, req *kitv1.AssignKitToUserRequest) (*kitv1.AssignKitToUserResponse, error) {
	requesterID, err := interceptor.GetUserID(ctx)
	if err != nil {
		s.log.Error("failed to get user ID from context", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	l := s.log.With(
		zap.String("method", "AssignKitToUser"),
		zap.String("requester_id", requesterID),
		zap.String("user_id", req.GetUserId()),
		zap.String("kit_name", req.GetKitName()),
	)

	l.Info("assigning kit to user")

	newAssignment := model.NewKitAssignment(
		req.GetUserId(),
		req.GetUserGameName(),
		req.GetKitName(),
		requesterID,
	)

	if err := newAssignment.Validate(time.Now()); err != nil {
		l.Error("assignment validation failed", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "assignment validation failed")
	}
	l.Info("assignment validation passed")

	l.Info("saving assignment to database")

	created, err := s.assignmentRepo.CreateAssignment(ctx, newAssignment)
	if err != nil {
		l.Error("failed to create assignment", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create assignment")
	}

	l.Info("publishing assignment notification event")

	if err := s.bus.kitGrantedPub.Publish(ctx, KitGrantedEvent{
		AssignmentID: created.Id,
		KitName:      req.GetKitName(),
		UserGameName: created.UserGameName,
		UserID:       req.GetUserId(),
	}); err != nil {
		l.Error("failed to publish kit granted event", zap.Error(err))
	}

	l.Info("kit assignment completed successfully")

	return &kitv1.AssignKitToUserResponse{}, nil
}

// ListUserAssignments implements kitv1.KitServiceServer.
func (s *Service) ListUserAssignments(ctx context.Context, req *kitv1.ListUserAssignmentsRequest) (*kitv1.ListUserAssignmentsResponse, error) {
	l := s.log.With(
		zap.String("method", "list-user-assignments"),
		zap.String("user_id", req.GetUserId()),
	)

	requesterID, err := interceptor.GetUserID(ctx)
	if err != nil {
		l.Error("failed to get user ID from context", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	if requesterID != req.GetUserId() {
		return nil, status.Error(
			codes.PermissionDenied,
			"user cannot list assignments for another user",
		)
	}

	assignments, err := s.assignmentRepo.GetAssignmentsByUserID(ctx, req.GetUserId())
	if err != nil {
		l.Error("failed to get assignments by user ID", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	l.Info(
		"successfully retrieved assignments for user",
		zap.Int("assignment_count", len(assignments)),
	)

	assignmentProtos := make([]*kitv1.Assignment, len(assignments))
	for i, assignment := range assignments {
		assignmentProtos[i] = s.mapper.ToAssignmentProto(assignment)
	}

	l.Info(
		"successfully retrieved assignments for user",
		zap.Int("assignment_count", len(assignments)),
	)

	return &kitv1.ListUserAssignmentsResponse{
		Assignments: assignmentProtos,
	}, nil
}
