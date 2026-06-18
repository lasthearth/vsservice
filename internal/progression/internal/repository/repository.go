package repository

import (
	"context"
	"errors"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/progression/internal/dto"
	"github.com/lasthearth/vsservice/internal/progression/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// --- Trees ---

func (r *Repository) CreateTree(ctx context.Context, tree model.TalentTree) (*model.TalentTree, error) {
	d := dto.TalentTree{
		Model:       mongox.NewModel(),
		Name:        tree.Name,
		Description: tree.Description,
		Nodes:       toNodeDTOs(tree.Nodes),
		Edges:       toEdgeDTOs(tree.Edges),
	}
	if _, err := r.treesColl.InsertOne(ctx, d); err != nil {
		return nil, err
	}
	return model.ReconstituteTalentTree(d.Id.Hex(), tree.Name, tree.Description, tree.Nodes, tree.Edges), nil
}

func (r *Repository) UpdateTree(ctx context.Context, tree model.TalentTree) (*model.TalentTree, error) {
	oid, err := mongox.ParseObjectID(tree.Id)
	if err != nil {
		return nil, err
	}
	update := bson.M{"$set": bson.M{
		"name":        tree.Name,
		"description": tree.Description,
		"nodes":       toNodeDTOs(tree.Nodes),
		"edges":       toEdgeDTOs(tree.Edges),
	}}
	res, err := r.treesColl.UpdateByID(ctx, oid, update)
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, mongo.ErrNoDocuments
	}
	return &tree, nil
}

func (r *Repository) GetTree(ctx context.Context, id string) (*model.TalentTree, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, err
	}
	var d dto.TalentTree
	if err := r.treesColl.FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		return nil, err
	}
	return fromTreeDTO(d), nil
}

func (r *Repository) ListTrees(ctx context.Context) ([]model.TalentTree, error) {
	cur, err := r.treesColl.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var docs []dto.TalentTree
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]model.TalentTree, len(docs))
	for i, d := range docs {
		out[i] = *fromTreeDTO(d)
	}
	return out, nil
}

// --- Presets ---

func (r *Repository) CreatePreset(ctx context.Context, preset model.TalentPreset) (*model.TalentPreset, error) {
	d := dto.TalentPreset{
		Model:   mongox.NewModel(),
		Name:    preset.Name,
		TreeIds: preset.TreeIds,
	}
	if _, err := r.presetsColl.InsertOne(ctx, d); err != nil {
		return nil, err
	}
	return model.ReconstituteTalentPreset(d.Id.Hex(), preset.Name, preset.TreeIds), nil
}

func (r *Repository) UpdatePreset(ctx context.Context, preset model.TalentPreset) (*model.TalentPreset, error) {
	oid, err := mongox.ParseObjectID(preset.Id)
	if err != nil {
		return nil, err
	}
	update := bson.M{"$set": bson.M{"name": preset.Name, "tree_ids": preset.TreeIds}}
	res, err := r.presetsColl.UpdateByID(ctx, oid, update)
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, mongo.ErrNoDocuments
	}
	return &preset, nil
}

func (r *Repository) GetPreset(ctx context.Context, id string) (*model.TalentPreset, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, err
	}
	var d dto.TalentPreset
	if err := r.presetsColl.FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		return nil, err
	}
	return model.ReconstituteTalentPreset(d.Id.Hex(), d.Name, d.TreeIds), nil
}

func (r *Repository) ListPresets(ctx context.Context) ([]model.TalentPreset, error) {
	cur, err := r.presetsColl.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var docs []dto.TalentPreset
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]model.TalentPreset, len(docs))
	for i, d := range docs {
		out[i] = *model.ReconstituteTalentPreset(d.Id.Hex(), d.Name, d.TreeIds)
	}
	return out, nil
}

// --- Progress ---

func (r *Repository) GetOrCreateProgress(ctx context.Context, ownerType, settlementId, pointId, side, treeId string) (*model.TalentProgress, error) {
	// Parse all ObjectIDs once; reused for both the lookup filter and the create path.
	treeOid, err := mongox.ParseObjectID(treeId)
	if err != nil {
		return nil, err
	}
	var settlementOid, pointOid bson.ObjectID
	if settlementId != "" {
		if settlementOid, err = mongox.ParseObjectID(settlementId); err != nil {
			return nil, err
		}
	}
	if pointId != "" {
		if pointOid, err = mongox.ParseObjectID(pointId); err != nil {
			return nil, err
		}
	}

	filter := bson.M{"owner_type": ownerType, "tree_id": treeOid}
	if settlementId != "" {
		filter["settlement_id"] = settlementOid
	}
	if pointId != "" {
		filter["point_id"] = pointOid
		filter["side"] = side
	}

	var d dto.TalentProgress
	err = r.progressColl.FindOne(ctx, filter).Decode(&d)
	if err == nil {
		return fromProgressDTO(d), nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}

	// Create empty progress document.
	d = dto.TalentProgress{
		Model:          mongox.NewModel(),
		OwnerType:      ownerType,
		TreeId:         treeOid,
		PurchasedNodes: []dto.PurchasedNode{},
	}
	if settlementId != "" {
		d.SettlementId = settlementOid
	}
	if pointId != "" {
		d.PointId = pointOid
		d.Side = side
	}
	if _, err := r.progressColl.InsertOne(ctx, d); err != nil {
		return nil, err
	}
	return fromProgressDTO(d), nil
}

func (r *Repository) SaveProgress(ctx context.Context, progress model.TalentProgress) error {
	oid, err := mongox.ParseObjectID(progress.Id)
	if err != nil {
		return err
	}
	nodes := make([]dto.PurchasedNode, len(progress.PurchasedNodes))
	for i, n := range progress.PurchasedNodes {
		nodes[i] = dto.PurchasedNode{
			NodeId:                n.NodeId,
			PurchasedAt:           n.PurchasedAt,
			PurchasedBySettlement: n.PurchasedBySettlement,
		}
	}
	_, err = r.progressColl.UpdateByID(ctx, oid, bson.M{"$set": bson.M{"purchased_nodes": nodes}})
	return err
}

// --- helpers ---

func toNodeDTOs(nodes []model.TalentNode) []dto.TalentNode {
	out := make([]dto.TalentNode, len(nodes))
	for i, n := range nodes {
		out[i] = dto.TalentNode{Id: n.Id, Name: n.Name, Description: n.Description, Effect: n.Effect, CostBi: n.CostBi}
	}
	return out
}

func toEdgeDTOs(edges []model.TalentEdge) []dto.TalentEdge {
	out := make([]dto.TalentEdge, len(edges))
	for i, e := range edges {
		out[i] = dto.TalentEdge{From: e.From, To: e.To}
	}
	return out
}

func fromTreeDTO(d dto.TalentTree) *model.TalentTree {
	nodes := make([]model.TalentNode, len(d.Nodes))
	for i, n := range d.Nodes {
		nodes[i] = model.TalentNode{Id: n.Id, Name: n.Name, Description: n.Description, Effect: n.Effect, CostBi: n.CostBi}
	}
	edges := make([]model.TalentEdge, len(d.Edges))
	for i, e := range d.Edges {
		edges[i] = model.TalentEdge{From: e.From, To: e.To}
	}
	return model.ReconstituteTalentTree(d.Id.Hex(), d.Name, d.Description, nodes, edges)
}

func fromProgressDTO(d dto.TalentProgress) *model.TalentProgress {
	nodes := make([]model.PurchasedNode, len(d.PurchasedNodes))
	for i, n := range d.PurchasedNodes {
		nodes[i] = model.PurchasedNode{NodeId: n.NodeId, PurchasedAt: n.PurchasedAt, PurchasedBySettlement: n.PurchasedBySettlement}
	}
	settlementId := ""
	if !d.SettlementId.IsZero() {
		settlementId = d.SettlementId.Hex()
	}
	pointId := ""
	if !d.PointId.IsZero() {
		pointId = d.PointId.Hex()
	}
	return model.ReconstituteTalentProgress(
		d.Id.Hex(),
		model.OwnerType(d.OwnerType),
		settlementId,
		pointId,
		d.Side,
		d.TreeId.Hex(),
		nodes,
	)
}
