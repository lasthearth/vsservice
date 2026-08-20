package service

import (
	"context"

	imperialpointv1 "github.com/lasthearth/vsservice/gen/imperialpoint/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/progression/internal/model"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time assertion that *Service satisfies the gRPC server interface.
var _ imperialpointv1.ImperialPointServiceServer = (*Service)(nil)

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
	return pointToProto(point), nil
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
	return pointToProto(point), nil
}

func (s *Service) GetPoint(ctx context.Context, req *imperialpointv1.GetPointRequest) (*imperialpointv1.ImperialPoint, error) {
	point, err := s.repo.GetPoint(ctx, req.GetId())
	if err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "point not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return pointToProto(point), nil
}

func (s *Service) ListPoints(ctx context.Context, _ *imperialpointv1.ListPointsRequest) (*imperialpointv1.ListPointsResponse, error) {
	points, err := s.repo.ListPoints(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	protos := make([]*imperialpointv1.ImperialPoint, len(points))
	for i := range points {
		protos[i] = pointToProto(&points[i])
	}
	return &imperialpointv1.ListPointsResponse{Points: protos}, nil
}

func (s *Service) SetControl(ctx context.Context, req *imperialpointv1.SetControlRequest) (*imperialpointv1.ImperialPoint, error) {
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

	// The losing side forfeits its last node only when the point actually
	// changes hands.
	rollbackSide := ""
	if prevSide != "" && prevSide != req.GetSide() && point.TreeId != "" {
		rollbackSide = prevSide
	}

	if err := s.applyControl(ctx, l, req.GetPointId(), point.Control, rollbackSide, point.TreeId); err != nil {
		return nil, err
	}
	return pointToProto(point), nil
}

func (s *Service) ReleaseControl(ctx context.Context, req *imperialpointv1.ReleaseControlRequest) (*imperialpointv1.ImperialPoint, error) {
	l := s.log.WithMethod("ReleaseControl").With(zap.String("point_id", req.GetPointId()))

	point, err := s.repo.GetPoint(ctx, req.GetPointId())
	if err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "point not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	releasedSide := point.ReleaseControl()
	rollbackSide := ""
	if releasedSide != "" && point.TreeId != "" {
		rollbackSide = releasedSide
	}

	if err := s.applyControl(ctx, l, req.GetPointId(), nil, rollbackSide, point.TreeId); err != nil {
		return nil, err
	}
	return pointToProto(point), nil
}

// applyControl persists a control change together with the progression rollback
// of the side that lost the point.
//
// Order matters and it is deliberate: the rollback is written FIRST, the control
// change second. The deployed MongoDB is a standalone (no replica set), so
// multi-document transactions are not available and the two writes — one in
// talent_progress, one in imperial_points — cannot commit together. Of the two
// possible orders, rollback-first is the only one where a rollback failure
// leaves nothing persisted: the caller gets an error and the point still belongs
// to the previous side, so the operation can simply be retried. The reverse
// order (which the old cross-module code used, with the rollback silently
// swallowed) committed the control change and then destroyed a node under an
// error return.
//
// The remaining window — rollback committed, control save failed — is closed by
// re-adding the removed node. If that compensation also fails there is nothing
// left to do but log it: the node is gone while the point did not change hands.
func (s *Service) applyControl(
	ctx context.Context,
	l logger.Logger,
	pointId string,
	control *model.PointControl,
	rollbackSide, treeId string,
) error {
	var err error
	restore := noRestore
	if rollbackSide != "" {
		restore, err = s.rollbackLastPointNode(ctx, pointId, rollbackSide, treeId)
		if err != nil {
			l.Error("progression rollback failed, control change aborted", zap.Error(err))
			return status.Error(codes.Internal, "failed to roll back progression for the losing side")
		}
	}

	if err := s.repo.SaveControl(ctx, pointId, control); err != nil {
		l.Error("failed to save control", zap.Error(err))
		if rerr := restore(ctx); rerr != nil {
			l.Error("failed to restore rolled-back node after control save failure",
				zap.Error(rerr), zap.String("side", rollbackSide), zap.String("tree_id", treeId))
		}
		return status.Error(codes.Internal, err.Error())
	}
	return nil
}

// noRestore is the compensation for "nothing was rolled back".
func noRestore(context.Context) error { return nil }

// rollbackLastPointNode removes the last purchased node for a point+side+tree.
// BI is NOT refunded. It returns a compensating function that re-adds the node.
func (s *Service) rollbackLastPointNode(ctx context.Context, pointId, side, treeId string) (func(context.Context) error, error) {
	progress, err := s.repo.GetOrCreateProgress(ctx, string(model.OwnerTypePointSide), "", pointId, side, treeId)
	if err != nil {
		return nil, err
	}
	node, ok := progress.RollbackLast()
	if !ok {
		return noRestore, nil
	}
	if err := s.repo.SaveProgress(ctx, *progress); err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		// The node was the most recent purchase, so re-appending it keeps the
		// chronological invariant AddNode enforces.
		if err := progress.AddNode(node); err != nil {
			return err
		}
		return s.repo.SaveProgress(ctx, *progress)
	}, nil
}

// controllingSettlement returns the settlement currently controlling the point,
// or "" when the point is unclaimed.
func (s *Service) controllingSettlement(ctx context.Context, pointId string) (string, error) {
	point, err := s.repo.GetPoint(ctx, pointId)
	if err != nil {
		return "", err
	}
	if point.Control == nil {
		return "", nil
	}
	return point.Control.SettlementId, nil
}

func pointToProto(p *model.ImperialPoint) *imperialpointv1.ImperialPoint {
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
