package service

import (
	"context"

	"github.com/go-faster/errors"
	imperialpointv1 "github.com/lasthearth/vsservice/gen/imperialpoint/v1"
	"github.com/lasthearth/vsservice/internal/imperial-point/internal/model"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) CreatePoint(ctx context.Context, req *imperialpointv1.CreatePointRequest) (*imperialpointv1.ImperialPoint, error) {
	point, err := s.repo.CreatePoint(ctx, model.ImperialPoint{
		Name:          req.GetName(),
		Description:   req.GetDescription(),
		BiRatePerHour: req.GetBiRatePerHour(),
		TreeId:        req.GetTreeId(),
	})
	if err != nil {
		s.log.WithMethod("CreatePoint").Error("failed", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}
	return toProto(point), nil
}

func (s *Service) UpdatePoint(ctx context.Context, req *imperialpointv1.UpdatePointRequest) (*imperialpointv1.ImperialPoint, error) {
	point, err := s.repo.UpdatePoint(ctx, model.ImperialPoint{
		Id:            req.GetId(),
		Name:          req.GetName(),
		Description:   req.GetDescription(),
		BiRatePerHour: req.GetBiRatePerHour(),
		TreeId:        req.GetTreeId(),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "point not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return toProto(point), nil
}

func (s *Service) GetPoint(ctx context.Context, req *imperialpointv1.GetPointRequest) (*imperialpointv1.ImperialPoint, error) {
	point, err := s.repo.GetPoint(ctx, req.GetId())
	if err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "point not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return toProto(point), nil
}

func (s *Service) ListPoints(ctx context.Context, _ *imperialpointv1.ListPointsRequest) (*imperialpointv1.ListPointsResponse, error) {
	points, err := s.repo.ListPoints(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	protos := make([]*imperialpointv1.ImperialPoint, len(points))
	for i := range points {
		protos[i] = toProto(&points[i])
	}
	return &imperialpointv1.ListPointsResponse{Points: protos}, nil
}

func (s *Service) SetControl(ctx context.Context, req *imperialpointv1.SetControlRequest) (*imperialpointv1.ImperialPoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	l := s.log.WithMethod("SetControl").With(zap.String("point_id", req.GetPointId()))

	point, err := s.repo.GetPoint(ctx, req.GetPointId())
	if err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "point not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Enforce max 2 points per side
	allPoints, err := s.repo.ListPoints(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	count := 0
	for _, p := range allPoints {
		if p.Control != nil && p.Control.Side == req.GetSide() && p.Id != req.GetPointId() {
			count++
		}
	}
	if count >= 2 {
		return nil, status.Error(codes.FailedPrecondition, "side already controls 2 points")
	}

	prevSide := point.SetControl(req.GetSide(), req.GetSettlementId())

	// Roll back last node if capturing side differs from the previous one
	if prevSide != "" && prevSide != req.GetSide() && point.TreeId != "" {
		if err := s.progression.RollbackLastPointNode(ctx, req.GetPointId(), prevSide, point.TreeId); err != nil {
			l.Error("rollback failed (non-fatal)", zap.Error(err))
		}
	}

	if err := s.repo.SaveControl(ctx, req.GetPointId(), point.Control); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return toProto(point), nil
}

func (s *Service) ReleaseControl(ctx context.Context, req *imperialpointv1.ReleaseControlRequest) (*imperialpointv1.ImperialPoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	l := s.log.WithMethod("ReleaseControl").With(zap.String("point_id", req.GetPointId()))

	point, err := s.repo.GetPoint(ctx, req.GetPointId())
	if err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "point not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	releasedSide := point.ReleaseControl()

	if releasedSide != "" && point.TreeId != "" {
		if err := s.progression.RollbackLastPointNode(ctx, req.GetPointId(), releasedSide, point.TreeId); err != nil {
			l.Error("rollback failed (non-fatal)", zap.Error(err))
		}
	}

	if err := s.repo.SaveControl(ctx, req.GetPointId(), nil); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return toProto(point), nil
}

// GetControllingSettlement implements pointcontrol.Reader.
func (s *Service) GetControllingSettlement(ctx context.Context, pointId string) (string, error) {
	point, err := s.repo.GetPoint(ctx, pointId)
	if err != nil {
		return "", err
	}
	if point.Control == nil {
		return "", nil
	}
	return point.Control.SettlementId, nil
}

func toProto(p *model.ImperialPoint) *imperialpointv1.ImperialPoint {
	proto := &imperialpointv1.ImperialPoint{
		Id:            p.Id,
		Name:          p.Name,
		Description:   p.Description,
		BiRatePerHour: p.BiRatePerHour,
		TreeId:        p.TreeId,
	}
	if p.Control != nil {
		proto.Control = &imperialpointv1.PointControl{
			Side:            p.Control.Side,
			SettlementId:    p.Control.SettlementId,
			ControlledSince: timestamppb.New(p.Control.ControlledSince),
		}
	}
	return proto
}

func isNotFound(err error) bool {
	return errors.Is(err, mongo.ErrNoDocuments)
}
