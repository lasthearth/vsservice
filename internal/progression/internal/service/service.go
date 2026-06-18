package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	progressionv1 "github.com/lasthearth/vsservice/gen/progression/v1"
	"github.com/lasthearth/vsservice/internal/progression/internal/model"
	"github.com/lasthearth/vsservice/internal/server/interceptor"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time assertion that *Service satisfies the gRPC server interface.
var _ progressionv1.ProgressionServiceServer = (*Service)(nil)

// --- Trees ---

func (s *Service) CreateTree(ctx context.Context, req *progressionv1.CreateTreeRequest) (*progressionv1.TalentTree, error) {
	tree, err := s.repo.CreateTree(ctx, model.TalentTree{
		Name:        req.GetName(),
		Description: req.GetDescription(),
		Nodes:       protoNodesToModel(req.GetNodes()),
		Edges:       protoEdgesToModel(req.GetEdges()),
	})
	if err != nil {
		s.log.WithMethod("CreateTree").Error("failed", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}
	return treeToProto(tree), nil
}

func (s *Service) UpdateTree(ctx context.Context, req *progressionv1.UpdateTreeRequest) (*progressionv1.TalentTree, error) {
	tree, err := s.repo.UpdateTree(ctx, model.TalentTree{
		Id:          req.GetId(),
		Name:        req.GetName(),
		Description: req.GetDescription(),
		Nodes:       protoNodesToModel(req.GetNodes()),
		Edges:       protoEdgesToModel(req.GetEdges()),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "tree not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return treeToProto(tree), nil
}

func (s *Service) GetTree(ctx context.Context, req *progressionv1.GetTreeRequest) (*progressionv1.TalentTree, error) {
	tree, err := s.repo.GetTree(ctx, req.GetId())
	if err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "tree not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return treeToProto(tree), nil
}

func (s *Service) ListTrees(ctx context.Context, _ *progressionv1.ListTreesRequest) (*progressionv1.ListTreesResponse, error) {
	trees, err := s.repo.ListTrees(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	protos := make([]*progressionv1.TalentTree, len(trees))
	for i := range trees {
		protos[i] = treeToProto(&trees[i])
	}
	return &progressionv1.ListTreesResponse{Trees: protos}, nil
}

// --- Presets ---

func (s *Service) CreatePreset(ctx context.Context, req *progressionv1.CreatePresetRequest) (*progressionv1.TalentPreset, error) {
	preset, err := s.repo.CreatePreset(ctx, model.TalentPreset{Name: req.GetName(), TreeIds: req.GetTreeIds()})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return presetToProto(preset), nil
}

func (s *Service) UpdatePreset(ctx context.Context, req *progressionv1.UpdatePresetRequest) (*progressionv1.TalentPreset, error) {
	preset, err := s.repo.UpdatePreset(ctx, model.TalentPreset{Id: req.GetId(), Name: req.GetName(), TreeIds: req.GetTreeIds()})
	if err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "preset not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return presetToProto(preset), nil
}

func (s *Service) GetPreset(ctx context.Context, req *progressionv1.GetPresetRequest) (*progressionv1.TalentPreset, error) {
	preset, err := s.repo.GetPreset(ctx, req.GetId())
	if err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "preset not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return presetToProto(preset), nil
}

func (s *Service) ListPresets(ctx context.Context, _ *progressionv1.ListPresetsRequest) (*progressionv1.ListPresetsResponse, error) {
	presets, err := s.repo.ListPresets(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	protos := make([]*progressionv1.TalentPreset, len(presets))
	for i := range presets {
		protos[i] = presetToProto(&presets[i])
	}
	return &progressionv1.ListPresetsResponse{Presets: protos}, nil
}

// --- Progress ---

func (s *Service) GetSettlementProgress(ctx context.Context, req *progressionv1.GetSettlementProgressRequest) (*progressionv1.TalentProgress, error) {
	p, err := s.repo.GetOrCreateProgress(ctx, string(model.OwnerTypeSettlement), req.GetSettlementId(), "", "", req.GetTreeId())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return progressToProto(p), nil
}

func (s *Service) PurchaseSettlementNode(ctx context.Context, req *progressionv1.PurchaseSettlementNodeRequest) (*progressionv1.TalentProgress, error) {
	callerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.favor.IsLeader(ctx, req.GetSettlementId(), callerID); err != nil {
		return nil, status.Error(codes.PermissionDenied, "caller is not the leader of the settlement")
	}

	return s.purchaseNode(ctx, req.GetTreeId(), req.GetNodeId(), req.GetSettlementId(),
		string(model.OwnerTypeSettlement), req.GetSettlementId(), "", "", callerID)
}

func (s *Service) GetPointProgress(ctx context.Context, req *progressionv1.GetPointProgressRequest) (*progressionv1.TalentProgress, error) {
	p, err := s.repo.GetOrCreateProgress(ctx, string(model.OwnerTypePointSide), "", req.GetPointId(), req.GetSide(), req.GetTreeId())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return progressToProto(p), nil
}

func (s *Service) PurchasePointNode(ctx context.Context, req *progressionv1.PurchasePointNodeRequest) (*progressionv1.TalentProgress, error) {
	callerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	controllingSettlement, err := s.pointCtrl.GetControllingSettlement(ctx, req.GetPointId())
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch point control")
	}
	if controllingSettlement == "" {
		return nil, status.Error(codes.PermissionDenied, "point is not controlled by any settlement")
	}
	if req.GetSettlementId() != controllingSettlement {
		return nil, status.Error(codes.PermissionDenied, "settlement does not control this point")
	}
	if err := s.favor.IsLeader(ctx, req.GetSettlementId(), callerID); err != nil {
		return nil, status.Error(codes.PermissionDenied, "caller is not the leader of the controlling settlement")
	}

	return s.purchaseNode(ctx, req.GetTreeId(), req.GetNodeId(), req.GetSettlementId(),
		string(model.OwnerTypePointSide), "", req.GetPointId(), req.GetSide(), callerID)
}

func (s *Service) purchaseNode(
	ctx context.Context,
	treeId, nodeId, spendingSettlementId string,
	ownerType, settlementId, pointId, side string,
	callerID string,
) (*progressionv1.TalentProgress, error) {
	tree, err := s.repo.GetTree(ctx, treeId)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "tree not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	var targetNode *model.TalentNode
	for i := range tree.Nodes {
		if tree.Nodes[i].Id == nodeId {
			targetNode = &tree.Nodes[i]
			break
		}
	}
	if targetNode == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("node %q not found in tree", nodeId))
	}

	progress, err := s.repo.GetOrCreateProgress(ctx, ownerType, settlementId, pointId, side, treeId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if progress.HasNode(nodeId) {
		return nil, status.Error(codes.InvalidArgument, "node already purchased")
	}

	for _, edge := range tree.Edges {
		if edge.To == nodeId && !progress.HasNode(edge.From) {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("parent node %q must be purchased first", edge.From))
		}
	}

	reason := fmt.Sprintf("node purchase: %s in tree %s", nodeId, treeId)
	if err := s.favor.Deduct(ctx, spendingSettlementId, targetNode.CostBi, reason, callerID); err != nil {
		return nil, err
	}

	progress.AddNode(model.PurchasedNode{
		NodeId:                nodeId,
		PurchasedAt:           time.Now(),
		PurchasedBySettlement: spendingSettlementId,
	})
	if err := s.repo.SaveProgress(ctx, *progress); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return progressToProto(progress), nil
}

// RollbackLastPointNode removes the last purchased node for a point+side+tree.
// Called by imperial-point service when a point changes hands. BI is NOT refunded.
func (s *Service) RollbackLastPointNode(ctx context.Context, pointId, side, treeId string) error {
	progress, err := s.repo.GetOrCreateProgress(ctx, string(model.OwnerTypePointSide), "", pointId, side, treeId)
	if err != nil {
		return err
	}
	_, ok := progress.RollbackLast()
	if !ok {
		return nil
	}
	return s.repo.SaveProgress(ctx, *progress)
}

// --- proto converters ---

func treeToProto(t *model.TalentTree) *progressionv1.TalentTree {
	nodes := make([]*progressionv1.TalentNode, len(t.Nodes))
	for i, n := range t.Nodes {
		nodes[i] = &progressionv1.TalentNode{Id: n.Id, Name: n.Name, Description: n.Description, Effect: n.Effect, CostBi: n.CostBi}
	}
	edges := make([]*progressionv1.TalentEdge, len(t.Edges))
	for i, e := range t.Edges {
		edges[i] = &progressionv1.TalentEdge{From: e.From, To: e.To}
	}
	return &progressionv1.TalentTree{Id: t.Id, Name: t.Name, Description: t.Description, Nodes: nodes, Edges: edges}
}

func presetToProto(p *model.TalentPreset) *progressionv1.TalentPreset {
	return &progressionv1.TalentPreset{Id: p.Id, Name: p.Name, TreeIds: p.TreeIds}
}

func progressToProto(p *model.TalentProgress) *progressionv1.TalentProgress {
	nodes := make([]*progressionv1.PurchasedNode, len(p.PurchasedNodes))
	for i, n := range p.PurchasedNodes {
		nodes[i] = &progressionv1.PurchasedNode{
			NodeId:                  n.NodeId,
			PurchasedAt:             timestamppb.New(n.PurchasedAt),
			PurchasedBySettlementId: n.PurchasedBySettlement,
		}
	}
	return &progressionv1.TalentProgress{Id: p.Id, TreeId: p.TreeId, PurchasedNodes: nodes}
}

func protoNodesToModel(nodes []*progressionv1.TalentNode) []model.TalentNode {
	out := make([]model.TalentNode, len(nodes))
	for i, n := range nodes {
		out[i] = model.TalentNode{Id: n.GetId(), Name: n.GetName(), Description: n.GetDescription(), Effect: n.GetEffect(), CostBi: n.GetCostBi()}
	}
	return out
}

func protoEdgesToModel(edges []*progressionv1.TalentEdge) []model.TalentEdge {
	out := make([]model.TalentEdge, len(edges))
	for i, e := range edges {
		out[i] = model.TalentEdge{From: e.GetFrom(), To: e.GetTo()}
	}
	return out
}

func isNotFound(err error) bool {
	return errors.Is(err, mongo.ErrNoDocuments)
}
