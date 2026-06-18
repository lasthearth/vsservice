# Imperial Point & Progression Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build imperial point control management and talent tree progression system — 5 neutral-zone key locations with configurable БИ accrual rates, PoE-style talent trees (nodes + edges DAG), and cross-settlement БИ transfer.

**Architecture:** Three-domain addition — `internal/imperial-point/` for key point CRUD and control tracking, `internal/progression/` for talent tree templates (embedded nodes+edges) and purchase progress, extended `internal/settlement/` for БИ transfers. Cross-domain glue: `settlementuc.FavorOps` (public use case, pattern mirrors `donateuc`) injected into progression for БИ deduction; `ProgressionRollbacker` interface in imperial-point service satisfied by progression service for automatic last-node rollback on point capture loss.

**Tech Stack:** Go, gRPC + grpc-gateway, MongoDB v2 + mongox, Uber fx, goverter, protobuf/buf.

---

## File Structure

**New files:**
- `proto/imperialpoint/v1/imperialpoint.proto`
- `proto/progression/v1/progression.proto`
- `internal/settlement/settlementuc/favor.go`
- `internal/imperial-point/fx.go`
- `internal/imperial-point/internal/model/point.go`
- `internal/imperial-point/internal/dto/point.go`
- `internal/imperial-point/internal/repository/app.go`
- `internal/imperial-point/internal/repository/repository.go`
- `internal/imperial-point/internal/service/app.go`
- `internal/imperial-point/internal/service/interface.go`
- `internal/imperial-point/internal/service/scope.go`
- `internal/imperial-point/internal/service/service.go`
- `internal/progression/fx.go`
- `internal/progression/internal/model/tree.go`
- `internal/progression/internal/model/progress.go`
- `internal/progression/internal/dto/tree.go`
- `internal/progression/internal/dto/progress.go`
- `internal/progression/internal/repository/app.go`
- `internal/progression/internal/repository/repository.go`
- `internal/progression/internal/service/app.go`
- `internal/progression/internal/service/interface.go`
- `internal/progression/internal/service/scope.go`
- `internal/progression/internal/service/service.go`

**Modified files:**
- `proto/settlement/v1/settlement.proto` — add `TransferImperialFavor` RPC
- `internal/settlement/internal/service/interface.go` — no change (transfer uses existing methods)
- `internal/settlement/internal/service/scope.go` — add `TransferImperialFavor` to scope map
- `internal/settlement/fx.go` — expose `settlementuc.FavorOps` publicly
- `internal/server/app.go` — add two new service fields + register gateway handlers
- `main.go` — import and add `imperialpoint.App`, `progression.App`

---

## Task 1: Settlement — TransferImperialFavor + FavorOps use case

**Files:**
- Modify: `proto/settlement/v1/settlement.proto`
- Create: `internal/settlement/settlementuc/favor.go`
- Modify: `internal/settlement/fx.go`
- Modify: `internal/settlement/internal/service/scope.go`
- Create: `internal/settlement/internal/service/transfer.go`

- [ ] **Step 1: Add TransferImperialFavor to settlement proto**

Read `proto/settlement/v1/settlement.proto` first. Add to the `SettlementService`:

```proto
// Transfer Imperial Favor from one settlement to another.
// Caller must be the leader of from_settlement_id.
//
// Errors:
//   - INVALID_ARGUMENT (400): amount <= 0 or missing fields
//   - UNAUTHENTICATED (401): missing or invalid auth token
//   - PERMISSION_DENIED (403): caller is not the leader of from_settlement_id
//   - FAILED_PRECONDITION (412): insufficient imperial favor balance
//   - INTERNAL (500): database failure
rpc TransferImperialFavor(TransferImperialFavorRequest) returns (TransferImperialFavorResponse) {
  option (google.api.http) = {
    post: "/v1/settlements/{from_settlement_id}/imperial-favor:transfer"
    body: "*"
  };
}
```

Add message definitions (at the end of the proto file, near other ImperialFavor messages):

```proto
message TransferImperialFavorRequest {
  string from_settlement_id = 1 [(google.api.field_behavior) = REQUIRED];
  string to_settlement_id = 2 [(google.api.field_behavior) = REQUIRED];
  int64 amount = 3 [(google.api.field_behavior) = REQUIRED];
}

message TransferImperialFavorResponse {
  Settlement from_settlement = 1;
  Settlement to_settlement = 2;
}
```

- [ ] **Step 2: Regenerate proto stubs**

```bash
make proto
```

Expected: `gen/settlement/v1/` updated with `TransferImperialFavor` method on server/client interfaces. No compile errors.

- [ ] **Step 3: Create FavorOps use case**

Create `internal/settlement/settlementuc/favor.go`:

```go
package settlementuc

import (
	"context"

	"github.com/lasthearth/vsservice/internal/settlement/model"
)

type FavorRepository interface {
	UpdateSettlement(
		ctx context.Context,
		id string,
		updateFn func(ctx context.Context, s *model.Settlement) (*model.Settlement, error),
	) (*model.Settlement, error)
	IsLeaderOfSettlement(ctx context.Context, settlementID, userID string) error
	CreateFavorLog(ctx context.Context, log model.ImperialFavorLog) error
}

type FavorOps struct {
	repo FavorRepository
}

func NewFavorOps(repo FavorRepository) *FavorOps {
	return &FavorOps{repo: repo}
}

// Deduct removes amount from a settlement's imperial favor balance and records a log entry.
// Caller must pre-validate amount > 0.
func (f *FavorOps) Deduct(ctx context.Context, settlementID string, amount int64, reason, byPlayerID string) error {
	_, err := f.repo.UpdateSettlement(ctx, settlementID,
		func(_ context.Context, s *model.Settlement) (*model.Settlement, error) {
			return s, s.DeductFavor(amount)
		},
	)
	if err != nil {
		return err
	}
	_ = f.repo.CreateFavorLog(ctx, model.ImperialFavorLog{
		SettlementId: settlementID,
		AdminId:      byPlayerID,
		Amount:       -amount,
		Reason:       reason,
	})
	return nil
}

// Add increases amount for a settlement's imperial favor balance and records a log entry.
func (f *FavorOps) Add(ctx context.Context, settlementID string, amount int64, reason, byPlayerID string) error {
	_, err := f.repo.UpdateSettlement(ctx, settlementID,
		func(_ context.Context, s *model.Settlement) (*model.Settlement, error) {
			s.AddFavor(amount)
			return s, nil
		},
	)
	if err != nil {
		return err
	}
	_ = f.repo.CreateFavorLog(ctx, model.ImperialFavorLog{
		SettlementId: settlementID,
		AdminId:      byPlayerID,
		Amount:       amount,
		Reason:       reason,
	})
	return nil
}

// IsLeader checks that playerID is the leader of settlementID.
func (f *FavorOps) IsLeader(ctx context.Context, settlementID, playerID string) error {
	return f.repo.IsLeaderOfSettlement(ctx, settlementID, playerID)
}
```

- [ ] **Step 4: Expose FavorOps publicly in settlement/fx.go**

Read `internal/settlement/fx.go`. The settlement repo is provided privately as `service.SettlementRepository`. Add a public provider that wraps it:

```go
// In the public fx.Provide block (outside fx.Private):
fx.Provide(
    func(repo service.SettlementRepository) *settlementuc.FavorOps {
        return settlementuc.NewFavorOps(repo)
    },
),
```

Add import: `"github.com/lasthearth/vsservice/internal/settlement/settlementuc"`

- [ ] **Step 5: Implement TransferImperialFavor service method**

Create `internal/settlement/internal/service/transfer.go`:

```go
package service

import (
	"context"
	"fmt"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TransferImperialFavor implements settlementv1.SettlementServiceServer.
func (s *Service) TransferImperialFavor(ctx context.Context, req *settlementv1.TransferImperialFavorRequest) (*settlementv1.TransferImperialFavorResponse, error) {
	l := s.log.WithMethod("TransferImperialFavor").
		With(zap.String("from", req.GetFromSettlementId()),
			zap.String("to", req.GetToSettlementId()),
			zap.Int64("amount", req.GetAmount()))

	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}
	if req.GetFromSettlementId() == req.GetToSettlementId() {
		return nil, status.Error(codes.InvalidArgument, "from and to must differ")
	}

	callerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.dbRepo.IsLeaderOfSettlement(ctx, req.GetFromSettlementId(), callerID); err != nil {
		return nil, status.Error(codes.PermissionDenied, "caller is not the leader of the source settlement")
	}

	from, err := s.dbRepo.UpdateSettlement(ctx, req.GetFromSettlementId(),
		func(_ context.Context, s *model.Settlement) (*model.Settlement, error) {
			return s, s.DeductFavor(req.GetAmount())
		},
	)
	if err != nil {
		l.Error("failed to deduct favor from source", zap.Error(err))
		return nil, err
	}

	to, err := s.dbRepo.UpdateSettlement(ctx, req.GetToSettlementId(),
		func(_ context.Context, s *model.Settlement) (*model.Settlement, error) {
			s.AddFavor(req.GetAmount())
			return s, nil
		},
	)
	if err != nil {
		l.Error("failed to add favor to target (deduction already applied)", zap.Error(err))
		return nil, err
	}

	reason := fmt.Sprintf("transfer to %s", req.GetToSettlementId())
	_ = s.dbRepo.CreateFavorLog(ctx, model.ImperialFavorLog{
		SettlementId: req.GetFromSettlementId(),
		AdminId:      callerID,
		Amount:       -req.GetAmount(),
		Reason:       reason,
	})
	_ = s.dbRepo.CreateFavorLog(ctx, model.ImperialFavorLog{
		SettlementId: req.GetToSettlementId(),
		AdminId:      callerID,
		Amount:       req.GetAmount(),
		Reason:       fmt.Sprintf("transfer from %s", req.GetFromSettlementId()),
	})

	return &settlementv1.TransferImperialFavorResponse{
		FromSettlement: s.mapper.ToSettlementProto(*from),
		ToSettlement:   s.mapper.ToSettlementProto(*to),
	}, nil
}
```

- [ ] **Step 6: Add TransferImperialFavor to scope**

Read `internal/settlement/internal/service/scope.go`. Add to the returned map:

```go
interceptor.Method(srvName + "TransferImperialFavor"): interceptor.Scope(""),
```

Empty scope = requires authentication only (any logged-in user, leader check done in handler).

- [ ] **Step 7: Build and verify**

```bash
make build
```

Expected: compiles cleanly. No unimplemented interface errors.

- [ ] **Step 8: Commit**

```bash
git add proto/settlement/v1/settlement.proto gen/settlement/ \
    internal/settlement/settlementuc/ \
    internal/settlement/fx.go \
    internal/settlement/internal/service/transfer.go \
    internal/settlement/internal/service/scope.go
git commit -m "feat(settlement): add TransferImperialFavor RPC and FavorOps use case"
```

---

## Task 2: Progression — proto + codegen

**Files:**
- Create: `proto/progression/v1/progression.proto`

- [ ] **Step 1: Create progression proto**

Create `proto/progression/v1/progression.proto`:

```proto
syntax = "proto3";

package progression.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/empty.proto";
import "google/protobuf/timestamp.proto";

option go_package = "github.com/lasthearth/vsservice/gen/progression/v1";

// ProgressionService manages talent tree templates, presets, and settlement/point progression.
service ProgressionService {

  // --- Talent Trees (admin) ---

  // Create a new talent tree template. Requires progression:write scope.
  //
  // Errors:
  //   - INVALID_ARGUMENT (400): missing required fields
  //   - UNAUTHENTICATED (401): missing or invalid auth token
  //   - PERMISSION_DENIED (403): requires progression:write scope
  //   - INTERNAL (500): database failure
  rpc CreateTree(CreateTreeRequest) returns (TalentTree) {
    option (google.api.http) = {
      post: "/v1/progression/trees"
      body: "*"
    };
  }

  // Update an existing talent tree template. Requires progression:write scope.
  //
  // Errors:
  //   - NOT_FOUND (404): tree not found
  //   - UNAUTHENTICATED (401): missing or invalid auth token
  //   - PERMISSION_DENIED (403): requires progression:write scope
  //   - INTERNAL (500): database failure
  rpc UpdateTree(UpdateTreeRequest) returns (TalentTree) {
    option (google.api.http) = {
      patch: "/v1/progression/trees/{id}"
      body: "*"
    };
  }

  // Get a talent tree by ID.
  //
  // Errors:
  //   - NOT_FOUND (404): tree not found
  //   - INVALID_ARGUMENT (400): invalid id format
  //   - INTERNAL (500): database failure
  rpc GetTree(GetTreeRequest) returns (TalentTree) {
    option (google.api.http) = {get: "/v1/progression/trees/{id}"};
  }

  // List all talent trees.
  //
  // Errors:
  //   - INTERNAL (500): database failure
  rpc ListTrees(ListTreesRequest) returns (ListTreesResponse) {
    option (google.api.http) = {get: "/v1/progression/trees"};
  }

  // --- Presets (admin) ---

  // Create a settlement preset (a named collection of tree IDs available to settlements).
  // Requires progression:write scope.
  //
  // Errors:
  //   - INVALID_ARGUMENT (400): missing required fields
  //   - UNAUTHENTICATED (401): missing or invalid auth token
  //   - PERMISSION_DENIED (403): requires progression:write scope
  //   - INTERNAL (500): database failure
  rpc CreatePreset(CreatePresetRequest) returns (TalentPreset) {
    option (google.api.http) = {
      post: "/v1/progression/presets"
      body: "*"
    };
  }

  // Update a settlement preset. Requires progression:write scope.
  //
  // Errors:
  //   - NOT_FOUND (404): preset not found
  //   - UNAUTHENTICATED (401): missing or invalid auth token
  //   - PERMISSION_DENIED (403): requires progression:write scope
  //   - INTERNAL (500): database failure
  rpc UpdatePreset(UpdatePresetRequest) returns (TalentPreset) {
    option (google.api.http) = {
      patch: "/v1/progression/presets/{id}"
      body: "*"
    };
  }

  // Get a preset by ID.
  //
  // Errors:
  //   - NOT_FOUND (404): preset not found
  //   - INTERNAL (500): database failure
  rpc GetPreset(GetPresetRequest) returns (TalentPreset) {
    option (google.api.http) = {get: "/v1/progression/presets/{id}"};
  }

  // List all presets.
  //
  // Errors:
  //   - INTERNAL (500): database failure
  rpc ListPresets(ListPresetsRequest) returns (ListPresetsResponse) {
    option (google.api.http) = {get: "/v1/progression/presets"};
  }

  // --- Progress ---

  // Get a settlement's progress on a specific tree.
  //
  // Errors:
  //   - INTERNAL (500): database failure
  rpc GetSettlementProgress(GetSettlementProgressRequest) returns (TalentProgress) {
    option (google.api.http) = {
      get: "/v1/progression/settlements/{settlement_id}/trees/{tree_id}"
    };
  }

  // Purchase a node in a settlement's talent tree.
  // Caller must be the leader of settlement_id.
  //
  // Errors:
  //   - NOT_FOUND (404): tree or node not found
  //   - INVALID_ARGUMENT (400): node already purchased or parent not purchased
  //   - UNAUTHENTICATED (401): missing or invalid auth token
  //   - PERMISSION_DENIED (403): caller is not the leader of the settlement
  //   - FAILED_PRECONDITION (412): insufficient imperial favor
  //   - INTERNAL (500): database failure
  rpc PurchaseSettlementNode(PurchaseSettlementNodeRequest) returns (TalentProgress) {
    option (google.api.http) = {
      post: "/v1/progression/settlements/{settlement_id}/trees/{tree_id}/nodes/{node_id}:purchase"
      body: "*"
    };
  }

  // Get a point's progression for a specific side (east/west).
  //
  // Errors:
  //   - INTERNAL (500): database failure
  rpc GetPointProgress(GetPointProgressRequest) returns (TalentProgress) {
    option (google.api.http) = {
      get: "/v1/progression/points/{point_id}/sides/{side}/trees/{tree_id}"
    };
  }

  // Purchase a node in a key point's talent tree.
  // Caller must be the leader of the settlement that controls the point.
  //
  // Errors:
  //   - NOT_FOUND (404): point, tree, or node not found
  //   - INVALID_ARGUMENT (400): node already purchased or parent not purchased
  //   - UNAUTHENTICATED (401): missing or invalid auth token
  //   - PERMISSION_DENIED (403): caller's settlement does not control this point
  //   - FAILED_PRECONDITION (412): insufficient imperial favor
  //   - INTERNAL (500): database failure
  rpc PurchasePointNode(PurchasePointNodeRequest) returns (TalentProgress) {
    option (google.api.http) = {
      post: "/v1/progression/points/{point_id}/sides/{side}/trees/{tree_id}/nodes/{node_id}:purchase"
      body: "*"
    };
  }
}

message TalentNode {
  string id = 1;
  string name = 2;
  string description = 3;
  string effect = 4;
  int64 cost_bi = 5;
}

message TalentEdge {
  string from = 1;
  string to = 2;
}

message TalentTree {
  string id = 1;
  string name = 2;
  string description = 3;
  repeated TalentNode nodes = 4;
  repeated TalentEdge edges = 5;
}

message TalentPreset {
  string id = 1;
  string name = 2;
  repeated string tree_ids = 3;
}

message PurchasedNode {
  string node_id = 1;
  google.protobuf.Timestamp purchased_at = 2;
  string purchased_by_settlement_id = 3;
}

message TalentProgress {
  string id = 1;
  string tree_id = 2;
  repeated PurchasedNode purchased_nodes = 3;
}

// Tree requests
message CreateTreeRequest {
  string name = 1 [(google.api.field_behavior) = REQUIRED];
  string description = 2;
  repeated TalentNode nodes = 3;
  repeated TalentEdge edges = 4;
}

message UpdateTreeRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string name = 2;
  string description = 3;
  repeated TalentNode nodes = 4;
  repeated TalentEdge edges = 5;
}

message GetTreeRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListTreesRequest {}

message ListTreesResponse {
  repeated TalentTree trees = 1;
}

// Preset requests
message CreatePresetRequest {
  string name = 1 [(google.api.field_behavior) = REQUIRED];
  repeated string tree_ids = 2;
}

message UpdatePresetRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string name = 2;
  repeated string tree_ids = 3;
}

message GetPresetRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListPresetsRequest {}

message ListPresetsResponse {
  repeated TalentPreset presets = 1;
}

// Progress requests
message GetSettlementProgressRequest {
  string settlement_id = 1 [(google.api.field_behavior) = REQUIRED];
  string tree_id = 2 [(google.api.field_behavior) = REQUIRED];
}

message PurchaseSettlementNodeRequest {
  string settlement_id = 1 [(google.api.field_behavior) = REQUIRED];
  string tree_id = 2 [(google.api.field_behavior) = REQUIRED];
  string node_id = 3 [(google.api.field_behavior) = REQUIRED];
}

message GetPointProgressRequest {
  string point_id = 1 [(google.api.field_behavior) = REQUIRED];
  string side = 2 [(google.api.field_behavior) = REQUIRED];
  string tree_id = 3 [(google.api.field_behavior) = REQUIRED];
}

message PurchasePointNodeRequest {
  string point_id = 1 [(google.api.field_behavior) = REQUIRED];
  string side = 2 [(google.api.field_behavior) = REQUIRED];
  string tree_id = 3 [(google.api.field_behavior) = REQUIRED];
  string node_id = 4 [(google.api.field_behavior) = REQUIRED];
  string settlement_id = 5 [(google.api.field_behavior) = REQUIRED];
}
```

- [ ] **Step 2: Run codegen**

```bash
make proto
```

Expected: `gen/progression/v1/` created with `.pb.go`, `_grpc.pb.go`, `*.pb.gw.go` files.

- [ ] **Step 3: Commit**

```bash
git add proto/progression/ gen/progression/
git commit -m "feat(progression): add progression proto and generated stubs"
```

---

## Task 3: Progression — data layer

**Files:**
- Create: `internal/progression/internal/model/tree.go`
- Create: `internal/progression/internal/model/progress.go`
- Create: `internal/progression/internal/dto/tree.go`
- Create: `internal/progression/internal/dto/progress.go`
- Create: `internal/progression/internal/repository/app.go`
- Create: `internal/progression/internal/repository/repository.go`

- [ ] **Step 1: Create domain models**

Create `internal/progression/internal/model/tree.go`:

```go
package model

type TalentNode struct {
	Id          string
	Name        string
	Description string
	Effect      string
	CostBi      int64
}

type TalentEdge struct {
	From string
	To   string
}

type TalentTree struct {
	Id          string
	Name        string
	Description string
	Nodes       []TalentNode
	Edges       []TalentEdge
}

type TalentPreset struct {
	Id      string
	Name    string
	TreeIds []string
}
```

Create `internal/progression/internal/model/progress.go`:

```go
package model

import "time"

type OwnerType string

const (
	OwnerTypeSettlement OwnerType = "settlement"
	OwnerTypePointSide  OwnerType = "point_side"
)

type PurchasedNode struct {
	NodeId                string
	PurchasedAt           time.Time
	PurchasedBySettlement string
}

type TalentProgress struct {
	Id             string
	OwnerType      OwnerType
	SettlementId   string // set when OwnerType == OwnerTypeSettlement
	PointId        string // set when OwnerType == OwnerTypePointSide
	Side           string // "east" | "west" — set when OwnerType == OwnerTypePointSide
	TreeId         string
	PurchasedNodes []PurchasedNode
}

// RollbackLast removes the last purchased node and returns it.
// Returns false if no nodes are purchased.
func (p *TalentProgress) RollbackLast() (PurchasedNode, bool) {
	if len(p.PurchasedNodes) == 0 {
		return PurchasedNode{}, false
	}
	last := p.PurchasedNodes[len(p.PurchasedNodes)-1]
	p.PurchasedNodes = p.PurchasedNodes[:len(p.PurchasedNodes)-1]
	return last, true
}

// HasNode reports whether nodeId is already purchased.
func (p *TalentProgress) HasNode(nodeId string) bool {
	for _, n := range p.PurchasedNodes {
		if n.NodeId == nodeId {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Create BSON DTOs**

Create `internal/progression/internal/dto/tree.go`:

```go
package dto

import "github.com/lasthearth/vsservice/internal/pkg/mongox"

type TalentNode struct {
	Id          string `bson:"id"`
	Name        string `bson:"name"`
	Description string `bson:"description"`
	Effect      string `bson:"effect"`
	CostBi      int64  `bson:"cost_bi"`
}

type TalentEdge struct {
	From string `bson:"from"`
	To   string `bson:"to"`
}

type TalentTree struct {
	mongox.Model `bson:",inline"`
	Name         string       `bson:"name"`
	Description  string       `bson:"description"`
	Nodes        []TalentNode `bson:"nodes"`
	Edges        []TalentEdge `bson:"edges"`
}

type TalentPreset struct {
	mongox.Model `bson:",inline"`
	Name         string   `bson:"name"`
	TreeIds      []string `bson:"tree_ids"`
}
```

Create `internal/progression/internal/dto/progress.go`:

```go
package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type PurchasedNode struct {
	NodeId                string    `bson:"node_id"`
	PurchasedAt           time.Time `bson:"purchased_at"`
	PurchasedBySettlement string    `bson:"purchased_by_settlement"`
}

type TalentProgress struct {
	mongox.Model   `bson:",inline"`
	OwnerType      string          `bson:"owner_type"`
	SettlementId   bson.ObjectID   `bson:"settlement_id,omitempty"`
	PointId        bson.ObjectID   `bson:"point_id,omitempty"`
	Side           string          `bson:"side,omitempty"`
	TreeId         bson.ObjectID   `bson:"tree_id"`
	PurchasedNodes []PurchasedNode `bson:"purchased_nodes"`
}
```

- [ ] **Step 3: Create repository interface (in service package)**

Create `internal/progression/internal/service/interface.go`:

```go
package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/progression/internal/model"
)

// ProgressionRepository is the data access interface consumed by Service.
// The concrete implementation lives in internal/progression/internal/repository/.
type ProgressionRepository interface {
	// Trees
	CreateTree(ctx context.Context, tree model.TalentTree) (*model.TalentTree, error)
	UpdateTree(ctx context.Context, tree model.TalentTree) (*model.TalentTree, error)
	GetTree(ctx context.Context, id string) (*model.TalentTree, error)
	ListTrees(ctx context.Context) ([]model.TalentTree, error)

	// Presets
	CreatePreset(ctx context.Context, preset model.TalentPreset) (*model.TalentPreset, error)
	UpdatePreset(ctx context.Context, preset model.TalentPreset) (*model.TalentPreset, error)
	GetPreset(ctx context.Context, id string) (*model.TalentPreset, error)
	ListPresets(ctx context.Context) ([]model.TalentPreset, error)

	// Progress
	GetOrCreateProgress(ctx context.Context, ownerType, settlementId, pointId, side, treeId string) (*model.TalentProgress, error)
	SaveProgress(ctx context.Context, progress model.TalentProgress) error
}

// FavorDeductor deducts imperial favor from a settlement.
// Implemented by settlementuc.FavorOps, injected via fx.
type FavorDeductor interface {
	Deduct(ctx context.Context, settlementID string, amount int64, reason, byPlayerID string) error
	IsLeader(ctx context.Context, settlementID, playerID string) error
}

// PointControlReader fetches the current controlling settlement for a point.
// Implemented by imperial-point service, injected via fx.
type PointControlReader interface {
	GetControllingSettlement(ctx context.Context, pointID string) (settlementID string, err error)
}
```

- [ ] **Step 4: Create repository implementation**

Create `internal/progression/internal/repository/app.go`:

```go
package repository

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In

	Log      logger.Logger
	Database *mongo.Database
}

type Repository struct {
	log         logger.Logger
	treesColl   *mongo.Collection
	presetsColl *mongo.Collection
	progressColl *mongo.Collection
}

func New(opts Opts) *Repository {
	return &Repository{
		log:          opts.Log,
		treesColl:    opts.Database.Collection("talent_trees"),
		presetsColl:  opts.Database.Collection("talent_presets"),
		progressColl: opts.Database.Collection("talent_progress"),
	}
}

// ensure indexes on startup
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	_, err := r.progressColl.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "settlement_id", Value: 1}, {Key: "tree_id", Value: 1}}},
		{Keys: bson.D{{Key: "point_id", Value: 1}, {Key: "side", Value: 1}, {Key: "tree_id", Value: 1}}},
	})
	return err
}
```

Add missing imports: `"context"`, `"go.mongodb.org/mongo-driver/v2/bson"`.

Create `internal/progression/internal/repository/repository.go`:

```go
package repository

import (
	"context"
	"errors"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/progression/internal/dto"
	"github.com/lasthearth/vsservice/internal/progression/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
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
	tree.Id = d.Model.Id.Hex()
	return &tree, nil
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
	preset.Id = d.Model.Id.Hex()
	return &preset, nil
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
	return &model.TalentPreset{Id: d.Model.Id.Hex(), Name: d.Name, TreeIds: d.TreeIds}, nil
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
		out[i] = model.TalentPreset{Id: d.Model.Id.Hex(), Name: d.Name, TreeIds: d.TreeIds}
	}
	return out, nil
}

// --- Progress ---

func (r *Repository) GetOrCreateProgress(ctx context.Context, ownerType, settlementId, pointId, side, treeId string) (*model.TalentProgress, error) {
	filter, err := buildProgressFilter(ownerType, settlementId, pointId, side, treeId)
	if err != nil {
		return nil, err
	}
	var d dto.TalentProgress
	err = r.progressColl.FindOne(ctx, filter).Decode(&d)
	if err == nil {
		return fromProgressDTO(d), nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	// Create empty progress document
	treeOid, err := mongox.ParseObjectID(treeId)
	if err != nil {
		return nil, err
	}
	d = dto.TalentProgress{
		Model:          mongox.NewModel(),
		OwnerType:      ownerType,
		TreeId:         treeOid,
		PurchasedNodes: []dto.PurchasedNode{},
	}
	if settlementId != "" {
		soid, err := mongox.ParseObjectID(settlementId)
		if err != nil {
			return nil, err
		}
		d.SettlementId = soid
	}
	if pointId != "" {
		poid, err := mongox.ParseObjectID(pointId)
		if err != nil {
			return nil, err
		}
		d.PointId = poid
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

func buildProgressFilter(ownerType, settlementId, pointId, side, treeId string) (bson.M, error) {
	treeOid, err := mongox.ParseObjectID(treeId)
	if err != nil {
		return nil, err
	}
	f := bson.M{"owner_type": ownerType, "tree_id": treeOid}
	if settlementId != "" {
		soid, err := mongox.ParseObjectID(settlementId)
		if err != nil {
			return nil, err
		}
		f["settlement_id"] = soid
	}
	if pointId != "" {
		poid, err := mongox.ParseObjectID(pointId)
		if err != nil {
			return nil, err
		}
		f["point_id"] = poid
		f["side"] = side
	}
	return f, nil
}

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
	return &model.TalentTree{Id: d.Model.Id.Hex(), Name: d.Name, Description: d.Description, Nodes: nodes, Edges: edges}
}

func fromProgressDTO(d dto.TalentProgress) *model.TalentProgress {
	nodes := make([]model.PurchasedNode, len(d.PurchasedNodes))
	for i, n := range d.PurchasedNodes {
		nodes[i] = model.PurchasedNode{NodeId: n.NodeId, PurchasedAt: n.PurchasedAt, PurchasedBySettlement: n.PurchasedBySettlement}
	}
	return &model.TalentProgress{
		Id:             d.Model.Id.Hex(),
		OwnerType:      model.OwnerType(d.OwnerType),
		SettlementId:   d.SettlementId.Hex(),
		PointId:        d.PointId.Hex(),
		Side:           d.Side,
		TreeId:         d.TreeId.Hex(),
		PurchasedNodes: nodes,
	}
}
```

- [ ] **Step 5: Build**

```bash
make build
```

Expected: compiles cleanly.

- [ ] **Step 6: Commit**

```bash
git add internal/progression/internal/model/ \
    internal/progression/internal/dto/ \
    internal/progression/internal/repository/ \
    internal/progression/internal/service/interface.go
git commit -m "feat(progression): data layer — models, DTOs, repository"
```

---

## Task 4: Progression — service + fx + server registration

**Files:**
- Create: `internal/progression/internal/service/app.go`
- Create: `internal/progression/internal/service/scope.go`
- Create: `internal/progression/internal/service/service.go`
- Create: `internal/progression/fx.go`
- Modify: `internal/server/app.go`
- Modify: `main.go`

- [ ] **Step 1: Create service Opts and struct**

Create `internal/progression/internal/service/app.go`:

```go
package service

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/settlement/settlementuc"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In

	Log        logger.Logger
	Repo       ProgressionRepository
	Favor      *settlementuc.FavorOps
	PointCtrl  PointControlReader
}

type Service struct {
	log       logger.Logger
	repo      ProgressionRepository
	favor     *settlementuc.FavorOps
	pointCtrl PointControlReader
}

func New(opts Opts) *Service {
	return &Service{
		log:       opts.Log,
		repo:      opts.Repo,
		favor:     opts.Favor,
		pointCtrl: opts.PointCtrl,
	}
}
```

- [ ] **Step 2: Create scope**

Create `internal/progression/internal/service/scope.go`:

```go
package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srv := "/progression.v1.ProgressionService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srv + "CreateTree"):   interceptor.Scope("progression:write"),
		interceptor.Method(srv + "UpdateTree"):   interceptor.Scope("progression:write"),
		interceptor.Method(srv + "CreatePreset"): interceptor.Scope("progression:write"),
		interceptor.Method(srv + "UpdatePreset"): interceptor.Scope("progression:write"),
		// purchases require auth, leader check done in handler
		interceptor.Method(srv + "PurchaseSettlementNode"): interceptor.Scope(""),
		interceptor.Method(srv + "PurchasePointNode"):       interceptor.Scope(""),
	}
}
```

- [ ] **Step 3: Implement service methods**

Create `internal/progression/internal/service/service.go`:

```go
package service

import (
	"context"
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
		if err == mongo.ErrNoDocuments {
			return nil, status.Error(codes.NotFound, "tree not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return treeToProto(tree), nil
}

func (s *Service) GetTree(ctx context.Context, req *progressionv1.GetTreeRequest) (*progressionv1.TalentTree, error) {
	tree, err := s.repo.GetTree(ctx, req.GetId())
	if err != nil {
		if err == mongo.ErrNoDocuments {
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
		if err == mongo.ErrNoDocuments {
			return nil, status.Error(codes.NotFound, "preset not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return presetToProto(preset), nil
}

func (s *Service) GetPreset(ctx context.Context, req *progressionv1.GetPresetRequest) (*progressionv1.TalentPreset, error) {
	preset, err := s.repo.GetPreset(ctx, req.GetId())
	if err != nil {
		if err == mongo.ErrNoDocuments {
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
	l := s.log.WithMethod("PurchaseSettlementNode")

	callerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.favor.IsLeader(ctx, req.GetSettlementId(), callerID); err != nil {
		return nil, status.Error(codes.PermissionDenied, "caller is not the leader of the settlement")
	}

	return s.purchaseNode(ctx, req.GetTreeId(), req.GetNodeId(), req.GetSettlementId(),
		string(model.OwnerTypeSettlement), req.GetSettlementId(), "", "", callerID, l)
}

func (s *Service) GetPointProgress(ctx context.Context, req *progressionv1.GetPointProgressRequest) (*progressionv1.TalentProgress, error) {
	p, err := s.repo.GetOrCreateProgress(ctx, string(model.OwnerTypePointSide), "", req.GetPointId(), req.GetSide(), req.GetTreeId())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return progressToProto(p), nil
}

func (s *Service) PurchasePointNode(ctx context.Context, req *progressionv1.PurchasePointNodeRequest) (*progressionv1.TalentProgress, error) {
	l := s.log.WithMethod("PurchasePointNode")

	callerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Verify caller is leader of the settlement that controls this point
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
		string(model.OwnerTypePointSide), "", req.GetPointId(), req.GetSide(), callerID, l)
}

// purchaseNode contains shared node purchase logic.
func (s *Service) purchaseNode(
	ctx context.Context,
	treeId, nodeId, spendingSettlementId string,
	ownerType, settlementId, pointId, side string,
	callerID string,
	l interface{ Error(string, ...zap.Field) },
) (*progressionv1.TalentProgress, error) {
	tree, err := s.repo.GetTree(ctx, treeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "tree not found")
	}

	// Find node in tree
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

	// Check all parent nodes are purchased
	for _, edge := range tree.Edges {
		if edge.To == nodeId && !progress.HasNode(edge.From) {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("parent node %q must be purchased first", edge.From))
		}
	}

	// Deduct БИ
	reason := fmt.Sprintf("node purchase: %s in tree %s", nodeId, treeId)
	if err := s.favor.Deduct(ctx, spendingSettlementId, targetNode.CostBi, reason, callerID); err != nil {
		return nil, err
	}

	// Record purchase
	progress.PurchasedNodes = append(progress.PurchasedNodes, model.PurchasedNode{
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
// Called by imperial-point service when a point changes hands.
// БИ is NOT refunded (by design).
func (s *Service) RollbackLastPointNode(ctx context.Context, pointId, side, treeId string) error {
	progress, err := s.repo.GetOrCreateProgress(ctx, string(model.OwnerTypePointSide), "", pointId, side, treeId)
	if err != nil {
		return err
	}
	_, ok := progress.RollbackLast()
	if !ok {
		return nil // nothing to roll back
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
			NodeId:                   n.NodeId,
			PurchasedAt:              timestamppb.New(n.PurchasedAt),
			PurchasedBySettlementId:  n.PurchasedBySettlement,
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
```

- [ ] **Step 4: Create progression fx.go**

Create `internal/progression/fx.go`:

```go
package progression

import (
	progressionv1 "github.com/lasthearth/vsservice/gen/progression/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	repository "github.com/lasthearth/vsservice/internal/progression/internal/repository"
	"github.com/lasthearth/vsservice/internal/progression/internal/service"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "progression"

var App = fx.Options(
	fx.Module(
		module,
		fx.Decorate(
			func(l logger.Logger) logger.Logger {
				return l.WithScope(module)
			},
		),

		fx.Provide(
			fx.Private,
			fx.Annotate(
				repository.New,
				fx.As(new(service.ProgressionRepository)),
			),
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(progressionv1.ProgressionServiceServer)),
			),
			fx.Annotate(service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),
	),
)
```

Note: `service.PointControlReader` will be satisfied by the imperial-point service — that's wired in Task 7.

- [ ] **Step 5: Register in server/app.go**

Read `internal/server/app.go`. Add `ProgressionV1 progressionv1.ProgressionServiceServer` field to `Opts` and `Server` struct, set it in `New()`, register in `RunInProcessGateway`:

```go
// In Opts struct:
ProgressionV1 progressionv1.ProgressionServiceServer

// In Server struct:
progressionV1 progressionv1.ProgressionServiceServer

// In New():
progressionV1: opts.ProgressionV1,

// In RegisterGrpcServer():
progressionv1.RegisterProgressionServiceServer(s.grpcSrv, s.progressionV1)

// In RunInProcessGateway():
if err := progressionv1.RegisterProgressionServiceHandlerFromEndpoint(ctx, mux, grpcaddr, dopts); err != nil {
    return errors.Wrap(err, "register progression service handler")
}
```

Add import: `progressionv1 "github.com/lasthearth/vsservice/gen/progression/v1"`

- [ ] **Step 6: Add to main.go**

Read `main.go`. Add `progression.App` to `fx.New(...)` and import `"github.com/lasthearth/vsservice/internal/progression"`.

- [ ] **Step 7: Build**

```bash
make build
```

Expected: compiles. Note: will fail if `PointControlReader` interface is not yet satisfied — that's resolved in Task 7. To unblock, temporarily comment out `PointCtrl PointControlReader` field in service `Opts` and `pointCtrl` usage in `PurchasePointNode`.

- [ ] **Step 8: Commit**

```bash
git add internal/progression/ main.go internal/server/app.go
git commit -m "feat(progression): talent trees, presets, node purchase service"
```

---

## Task 5: Imperial-point — proto + codegen

**Files:**
- Create: `proto/imperialpoint/v1/imperialpoint.proto`

- [ ] **Step 1: Create imperialpoint proto**

Create `proto/imperialpoint/v1/imperialpoint.proto`:

```proto
syntax = "proto3";

package imperialpoint.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/empty.proto";
import "google/protobuf/timestamp.proto";

option go_package = "github.com/lasthearth/vsservice/gen/imperialpoint/v1";

// ImperialPointService manages neutral-zone key locations and their control state.
service ImperialPointService {

  // Create a new imperial point. Requires imperialpoint:write scope.
  //
  // Errors:
  //   - INVALID_ARGUMENT (400): missing name
  //   - UNAUTHENTICATED (401): missing or invalid auth token
  //   - PERMISSION_DENIED (403): requires imperialpoint:write scope
  //   - INTERNAL (500): database failure
  rpc CreatePoint(CreatePointRequest) returns (ImperialPoint) {
    option (google.api.http) = {
      post: "/v1/imperial-points"
      body: "*"
    };
  }

  // Update an imperial point config (name, description, bi_rate, tree_id).
  // Requires imperialpoint:write scope.
  //
  // Errors:
  //   - NOT_FOUND (404): point not found
  //   - UNAUTHENTICATED (401): missing or invalid auth token
  //   - PERMISSION_DENIED (403): requires imperialpoint:write scope
  //   - INTERNAL (500): database failure
  rpc UpdatePoint(UpdatePointRequest) returns (ImperialPoint) {
    option (google.api.http) = {
      patch: "/v1/imperial-points/{id}"
      body: "*"
    };
  }

  // Get an imperial point by ID.
  //
  // Errors:
  //   - NOT_FOUND (404): point not found
  //   - INTERNAL (500): database failure
  rpc GetPoint(GetPointRequest) returns (ImperialPoint) {
    option (google.api.http) = {get: "/v1/imperial-points/{id}"};
  }

  // List all imperial points.
  //
  // Errors:
  //   - INTERNAL (500): database failure
  rpc ListPoints(ListPointsRequest) returns (ListPointsResponse) {
    option (google.api.http) = {get: "/v1/imperial-points"};
  }

  // Set which settlement controls an imperial point.
  // If the point was previously controlled by the opposite side, the last
  // purchased node in the point's tree is rolled back (no БИ refund).
  // Requires imperialpoint:write scope.
  //
  // Errors:
  //   - NOT_FOUND (404): point or settlement not found
  //   - UNAUTHENTICATED (401): missing or invalid auth token
  //   - PERMISSION_DENIED (403): requires imperialpoint:write scope
  //   - INTERNAL (500): database failure
  rpc SetControl(SetControlRequest) returns (ImperialPoint) {
    option (google.api.http) = {
      post: "/v1/imperial-points/{point_id}:set-control"
      body: "*"
    };
  }

  // Release control of an imperial point. Rolls back the last purchased node.
  // Requires imperialpoint:write scope.
  //
  // Errors:
  //   - NOT_FOUND (404): point not found
  //   - UNAUTHENTICATED (401): missing or invalid auth token
  //   - PERMISSION_DENIED (403): requires imperialpoint:write scope
  //   - INTERNAL (500): database failure
  rpc ReleaseControl(ReleaseControlRequest) returns (ImperialPoint) {
    option (google.api.http) = {
      post: "/v1/imperial-points/{point_id}:release-control"
      body: "*"
    };
  }
}

message PointControl {
  string side = 1;
  string settlement_id = 2;
  google.protobuf.Timestamp controlled_since = 3;
}

message ImperialPoint {
  string id = 1;
  string name = 2;
  string description = 3;
  int64 bi_rate_per_hour = 4;
  string tree_id = 5;
  PointControl control = 6;
}

message CreatePointRequest {
  string name = 1 [(google.api.field_behavior) = REQUIRED];
  string description = 2;
  int64 bi_rate_per_hour = 3;
  string tree_id = 4;
}

message UpdatePointRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string name = 2;
  string description = 3;
  int64 bi_rate_per_hour = 4;
  string tree_id = 5;
}

message GetPointRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListPointsRequest {}

message ListPointsResponse {
  repeated ImperialPoint points = 1;
}

message SetControlRequest {
  string point_id = 1 [(google.api.field_behavior) = REQUIRED];
  string settlement_id = 2 [(google.api.field_behavior) = REQUIRED];
  string side = 3 [(google.api.field_behavior) = REQUIRED];
}

message ReleaseControlRequest {
  string point_id = 1 [(google.api.field_behavior) = REQUIRED];
}
```

- [ ] **Step 2: Run codegen**

```bash
make proto
```

Expected: `gen/imperialpoint/v1/` created.

- [ ] **Step 3: Commit**

```bash
git add proto/imperialpoint/ gen/imperialpoint/
git commit -m "feat(imperial-point): add imperialpoint proto and generated stubs"
```

---

## Task 6: Imperial-point — data layer

**Files:**
- Create: `internal/imperial-point/internal/model/point.go`
- Create: `internal/imperial-point/internal/dto/point.go`
- Create: `internal/imperial-point/internal/repository/app.go`
- Create: `internal/imperial-point/internal/repository/repository.go`
- Create: `internal/imperial-point/internal/service/interface.go`

- [ ] **Step 1: Create model**

Create `internal/imperial-point/internal/model/point.go`:

```go
package model

import "time"

type PointControl struct {
	Side            string
	SettlementId    string
	ControlledSince time.Time
}

type ImperialPoint struct {
	Id            string
	Name          string
	Description   string
	BiRatePerHour int64
	TreeId        string
	Control       *PointControl // nil = unclaimed
}

// SetControl updates the controlling settlement. Returns the previous side (empty if unclaimed).
func (p *ImperialPoint) SetControl(side, settlementId string) string {
	prev := ""
	if p.Control != nil {
		prev = p.Control.Side
	}
	p.Control = &PointControl{
		Side:            side,
		SettlementId:    settlementId,
		ControlledSince: time.Now(),
	}
	return prev
}

// ReleaseControl clears the controlling settlement. Returns the side that was released.
func (p *ImperialPoint) ReleaseControl() string {
	if p.Control == nil {
		return ""
	}
	side := p.Control.Side
	p.Control = nil
	return side
}
```

- [ ] **Step 2: Create BSON DTO**

Create `internal/imperial-point/internal/dto/point.go`:

```go
package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type PointControl struct {
	Side            string        `bson:"side"`
	SettlementId    bson.ObjectID `bson:"settlement_id"`
	ControlledSince time.Time     `bson:"controlled_since"`
}

type ImperialPoint struct {
	mongox.Model  `bson:",inline"`
	Name          string        `bson:"name"`
	Description   string        `bson:"description"`
	BiRatePerHour int64         `bson:"bi_rate_per_hour"`
	TreeId        bson.ObjectID `bson:"tree_id,omitempty"`
	Control       *PointControl `bson:"control,omitempty"`
}
```

- [ ] **Step 3: Create service interface**

Create `internal/imperial-point/internal/service/interface.go`:

```go
package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/imperial-point/internal/model"
)

// ImperialPointRepository is the data access interface consumed by Service.
type ImperialPointRepository interface {
	CreatePoint(ctx context.Context, point model.ImperialPoint) (*model.ImperialPoint, error)
	UpdatePoint(ctx context.Context, point model.ImperialPoint) (*model.ImperialPoint, error)
	GetPoint(ctx context.Context, id string) (*model.ImperialPoint, error)
	ListPoints(ctx context.Context) ([]model.ImperialPoint, error)
	SaveControl(ctx context.Context, pointId string, control *model.PointControl) error
}

// ProgressionRollbacker rolls back the last purchased node for a point+side+tree.
// Implemented by internal/progression Service, injected via fx.
type ProgressionRollbacker interface {
	RollbackLastPointNode(ctx context.Context, pointId, side, treeId string) error
}
```

Note on import path: Go packages cannot have hyphens in their import paths. The directory is `internal/imperial-point/` but the Go package is named `imperialpoint`. Adjust the import path in actual code to use underscores or a quoted import. Verify the actual Go module path resolves correctly — buf-generated code uses `gen/imperialpoint/v1`.

- [ ] **Step 4: Create repository**

Create `internal/imperial-point/internal/repository/app.go`:

```go
package repository

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In

	Log      logger.Logger
	Database *mongo.Database
}

type Repository struct {
	log   logger.Logger
	coll  *mongo.Collection
}

func New(opts Opts) *Repository {
	return &Repository{
		log:  opts.Log,
		coll: opts.Database.Collection("imperial_points"),
	}
}
```

Create `internal/imperial-point/internal/repository/repository.go`:

```go
package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/imperial-point/internal/dto"
	"github.com/lasthearth/vsservice/internal/imperial-point/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (r *Repository) CreatePoint(ctx context.Context, point model.ImperialPoint) (*model.ImperialPoint, error) {
	treeOid := bson.NilObjectID
	if point.TreeId != "" {
		var err error
		treeOid, err = mongox.ParseObjectID(point.TreeId)
		if err != nil {
			return nil, err
		}
	}
	d := dto.ImperialPoint{
		Model:         mongox.NewModel(),
		Name:          point.Name,
		Description:   point.Description,
		BiRatePerHour: point.BiRatePerHour,
		TreeId:        treeOid,
	}
	if _, err := r.coll.InsertOne(ctx, d); err != nil {
		return nil, err
	}
	point.Id = d.Model.Id.Hex()
	return &point, nil
}

func (r *Repository) UpdatePoint(ctx context.Context, point model.ImperialPoint) (*model.ImperialPoint, error) {
	oid, err := mongox.ParseObjectID(point.Id)
	if err != nil {
		return nil, err
	}
	update := bson.M{"$set": bson.M{
		"name":             point.Name,
		"description":      point.Description,
		"bi_rate_per_hour": point.BiRatePerHour,
		"tree_id":          point.TreeId,
	}}
	res, err := r.coll.UpdateByID(ctx, oid, update)
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, mongo.ErrNoDocuments
	}
	return &point, nil
}

func (r *Repository) GetPoint(ctx context.Context, id string) (*model.ImperialPoint, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, err
	}
	var d dto.ImperialPoint
	if err := r.coll.FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		return nil, err
	}
	return fromDTO(d), nil
}

func (r *Repository) ListPoints(ctx context.Context) ([]model.ImperialPoint, error) {
	cur, err := r.coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var docs []dto.ImperialPoint
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]model.ImperialPoint, len(docs))
	for i, d := range docs {
		out[i] = *fromDTO(d)
	}
	return out, nil
}

func (r *Repository) SaveControl(ctx context.Context, pointId string, control *model.PointControl) error {
	oid, err := mongox.ParseObjectID(pointId)
	if err != nil {
		return err
	}
	var update bson.M
	if control == nil {
		update = bson.M{"$unset": bson.M{"control": ""}}
	} else {
		soid, err := mongox.ParseObjectID(control.SettlementId)
		if err != nil {
			return err
		}
		update = bson.M{"$set": bson.M{"control": dto.PointControl{
			Side:            control.Side,
			SettlementId:    soid,
			ControlledSince: control.ControlledSince,
		}}}
	}
	_, err = r.coll.UpdateByID(ctx, oid, update)
	return err
}

func fromDTO(d dto.ImperialPoint) *model.ImperialPoint {
	p := &model.ImperialPoint{
		Id:            d.Model.Id.Hex(),
		Name:          d.Name,
		Description:   d.Description,
		BiRatePerHour: d.BiRatePerHour,
		TreeId:        d.TreeId.Hex(),
	}
	if d.Control != nil {
		p.Control = &model.PointControl{
			Side:            d.Control.Side,
			SettlementId:    d.Control.SettlementId.Hex(),
			ControlledSince: d.Control.ControlledSince,
		}
	}
	return p
}
```

- [ ] **Step 5: Build**

```bash
make build
```

Expected: compiles cleanly.

- [ ] **Step 6: Commit**

```bash
git add internal/imperial-point/internal/model/ \
    internal/imperial-point/internal/dto/ \
    internal/imperial-point/internal/repository/ \
    internal/imperial-point/internal/service/interface.go
git commit -m "feat(imperial-point): data layer — models, DTOs, repository"
```

---

## Task 7: Imperial-point — service + fx + full wiring

**Files:**
- Create: `internal/imperial-point/internal/service/app.go`
- Create: `internal/imperial-point/internal/service/scope.go`
- Create: `internal/imperial-point/internal/service/service.go`
- Create: `internal/imperial-point/fx.go`
- Modify: `internal/progression/fx.go` — wire PointControlReader
- Modify: `internal/server/app.go` — add imperialpoint service
- Modify: `main.go` — add imperialpoint.App

- [ ] **Step 1: Create service Opts**

Create `internal/imperial-point/internal/service/app.go`:

```go
package service

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In

	Log         logger.Logger
	Repo        ImperialPointRepository
	Progression ProgressionRollbacker
}

type Service struct {
	log         logger.Logger
	repo        ImperialPointRepository
	progression ProgressionRollbacker
}

func New(opts Opts) *Service {
	return &Service{
		log:         opts.Log,
		repo:        opts.Repo,
		progression: opts.Progression,
	}
}
```

- [ ] **Step 2: Create scope**

Create `internal/imperial-point/internal/service/scope.go`:

```go
package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srv := "/imperialpoint.v1.ImperialPointService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srv + "CreatePoint"):     interceptor.Scope("imperialpoint:write"),
		interceptor.Method(srv + "UpdatePoint"):     interceptor.Scope("imperialpoint:write"),
		interceptor.Method(srv + "SetControl"):      interceptor.Scope("imperialpoint:write"),
		interceptor.Method(srv + "ReleaseControl"):  interceptor.Scope("imperialpoint:write"),
	}
}
```

- [ ] **Step 3: Implement service methods**

Create `internal/imperial-point/internal/service/service.go`:

```go
package service

import (
	"context"

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
		if err == mongo.ErrNoDocuments {
			return nil, status.Error(codes.NotFound, "point not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return toProto(point), nil
}

func (s *Service) GetPoint(ctx context.Context, req *imperialpointv1.GetPointRequest) (*imperialpointv1.ImperialPoint, error) {
	point, err := s.repo.GetPoint(ctx, req.GetId())
	if err != nil {
		if err == mongo.ErrNoDocuments {
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
	l := s.log.WithMethod("SetControl").With(zap.String("point_id", req.GetPointId()))

	point, err := s.repo.GetPoint(ctx, req.GetPointId())
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, status.Error(codes.NotFound, "point not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	prevSide := point.SetControl(req.GetSide(), req.GetSettlementId())

	// Roll back last node if the capturing side differs from the previous one
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
	l := s.log.WithMethod("ReleaseControl").With(zap.String("point_id", req.GetPointId()))

	point, err := s.repo.GetPoint(ctx, req.GetPointId())
	if err != nil {
		if err == mongo.ErrNoDocuments {
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

// GetControllingSettlement implements progression.PointControlReader.
func (s *Service) GetControllingSettlement(_ context.Context, pointId string) (string, error) {
	point, err := s.repo.GetPoint(context.Background(), pointId)
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
```

- [ ] **Step 4: Create imperial-point fx.go**

Create `internal/imperial-point/fx.go`:

```go
package imperialpoint

import (
	imperialpointv1 "github.com/lasthearth/vsservice/gen/imperialpoint/v1"
	"github.com/lasthearth/vsservice/internal/imperial-point/internal/repository"
	"github.com/lasthearth/vsservice/internal/imperial-point/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/progression/internal/service" progressionsvc
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)
```

Wait — this creates a circular import: imperial-point imports progression service, progression service imports imperial-point service (for PointControlReader). This is a circular dependency.

**Resolution:** The `PointControlReader` interface is defined in the progression service package. The imperial-point Service implements it. To avoid the cycle:

- Remove `PointControlReader` from `internal/progression/internal/service/interface.go`
- Instead define it as a **shared interface** in a neutral package, e.g. `internal/pkg/gameinterface/` or inline it in progression's fx.go without importing imperial-point's types

**Simplest fix:** Remove `PointControlReader` from progression service package. In `progression/fx.go`, declare a local interface:

```go
// In internal/progression/fx.go — local interface matching imperialpoint.Service
type pointControlReader interface {
    GetControllingSettlement(ctx context.Context, pointId string) (string, error)
}
```

Then wire it:
```go
fx.Provide(
    fx.Annotate(
        imperialpoint_service.New,  // ← still a circular import problem
    ),
)
```

This still creates the cycle. The correct architectural fix:

**Use fx's cross-module injection without import**. Both modules declare their own interface types that happen to have matching method sets. fx matches by type at runtime if the concrete type satisfies the interface. But fx matches by interface type identity, not structural typing at the DI level.

**Actual solution:** Move `PointControlReader` to a shared package `internal/pkg/pointcontrol/`:

```go
// internal/pkg/pointcontrol/interface.go
package pointcontrol

import "context"

type Reader interface {
    GetControllingSettlement(ctx context.Context, pointId string) (string, error)
}
```

Both progression and imperial-point import this package. No cycle.

Apply this fix:
1. Create `internal/pkg/pointcontrol/interface.go` with the `Reader` interface.
2. Replace `PointControlReader` in `internal/progression/internal/service/interface.go` with `pointcontrol.Reader`.
3. In `internal/imperial-point/internal/service/service.go`, add `var _ pointcontrol.Reader = (*Service)(nil)` compile guard.
4. In `internal/progression/fx.go`, annotate imperialpoint.Service.New as `fx.As(new(pointcontrol.Reader))`.

- [ ] **Step 5: Create shared pointcontrol package**

Create `internal/pkg/pointcontrol/interface.go`:

```go
package pointcontrol

import "context"

// Reader retrieves the settlement currently controlling an imperial point.
type Reader interface {
	GetControllingSettlement(ctx context.Context, pointId string) (string, error)
}
```

Update `internal/progression/internal/service/interface.go` — replace `PointControlReader` with `pointcontrol.Reader`:

```go
import "github.com/lasthearth/vsservice/internal/pkg/pointcontrol"

// In Opts:
PointCtrl pointcontrol.Reader
```

Update `internal/progression/internal/service/app.go` — field type becomes `pointcontrol.Reader`.

- [ ] **Step 6: Create imperial-point fx.go (corrected)**

Create `internal/imperial-point/fx.go`:

```go
package imperialpoint

import (
	imperialpointv1 "github.com/lasthearth/vsservice/gen/imperialpoint/v1"
	"github.com/lasthearth/vsservice/internal/imperial-point/internal/repository"
	"github.com/lasthearth/vsservice/internal/imperial-point/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/pointcontrol"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "imperial-point"

var App = fx.Options(
	fx.Module(
		module,
		fx.Decorate(
			func(l logger.Logger) logger.Logger {
				return l.WithScope(module)
			},
		),

		fx.Provide(
			fx.Private,
			fx.Annotate(
				repository.New,
				fx.As(new(service.ImperialPointRepository)),
			),
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(imperialpointv1.ImperialPointServiceServer)),
			),
			fx.Annotate(service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
			// Expose GetControllingSettlement to progression domain
			fx.Annotate(service.New,
				fx.As(new(pointcontrol.Reader)),
			),
		),
	),
)
```

Note: `service.New` is annotated three times — fx handles this via multiple `fx.Annotate` calls providing the same singleton under different interface types.

- [ ] **Step 7: Register in server/app.go**

Read `internal/server/app.go`. Add `ImperialPointV1 imperialpointv1.ImperialPointServiceServer` to `Opts` and `Server`, set in `New()`, register in gateway and gRPC:

```go
// Import:
imperialpointv1 "github.com/lasthearth/vsservice/gen/imperialpoint/v1"

// Opts field:
ImperialPointV1 imperialpointv1.ImperialPointServiceServer

// Server field:
imperialPointV1 imperialpointv1.ImperialPointServiceServer

// New():
imperialPointV1: opts.ImperialPointV1,

// RegisterGrpcServer():
imperialpointv1.RegisterImperialPointServiceServer(s.grpcSrv, s.imperialPointV1)

// RunInProcessGateway():
if err := imperialpointv1.RegisterImperialPointServiceHandlerFromEndpoint(ctx, mux, grpcaddr, dopts); err != nil {
    return errors.Wrap(err, "register imperialpoint service handler")
}
```

- [ ] **Step 8: Update main.go**

Read `main.go`. Add `imperialpoint.App` to `fx.New(...)`. Import:
```go
imperialpoint "github.com/lasthearth/vsservice/internal/imperial-point"
```

Also restore the previously-commented `PointCtrl` wiring in progression service (from Task 4 Step 7).

- [ ] **Step 9: Final build**

```bash
make build
```

Expected: compiles cleanly. All interface implementations satisfied. No circular imports.

- [ ] **Step 10: Commit**

```bash
git add internal/imperial-point/ \
    internal/progression/fx.go \
    internal/pkg/pointcontrol/ \
    internal/server/app.go \
    main.go
git commit -m "feat(imperial-point): service, fx wiring, PointControlReader integration"
```

---

## Self-Review

**Spec coverage:**
- 5 configurable key points with per-point БИ rate ✓ (imperialpoint CRUD)
- Capture-and-hold control tracking ✓ (SetControl / ReleaseControl)
- БИ accrual is manual ✓ (existing AddImperialFavor, no automation added)
- Transfer БИ freely cross-settlement ✓ (Task 1)
- PoE-style DAG talent tree with nodes+edges ✓ (talent_trees collection, embedded nodes/edges)
- Settlement presets (named tree collections) ✓ (Task 2-4)
- Per-settlement progress ✓ (GetSettlementProgress / PurchaseSettlementNode)
- Per-point+side progress ✓ (GetPointProgress / PurchasePointNode)
- Rollback last node on point loss, no БИ refund ✓ (RollbackLastPointNode in SetControl/ReleaseControl)
- Only suzerain (controlling settlement's leader) can upgrade point trees ✓ (GetControllingSettlement check)
- All configurable from frontend (no hardcoded tree structures) ✓ (full CRUD on trees/presets/points)
- Max 2 points per side ✓ — **MISSING**: this rule is not enforced in SetControl. Add validation in SetControl: query all points, count those controlled by the requesting side, return FAILED_PRECONDITION if already 2.

**Gap — add to Task 7 Step 3 `SetControl`:**

```go
// Before calling point.SetControl(), count existing controlled points for the side:
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
```

**Placeholder scan:** No TBD or TODO markers found.

**Type consistency:** `RollbackLastPointNode(ctx, pointId, side, treeId)` — signature consistent across service.go, interface.go, and imperial-point service call sites. ✓
