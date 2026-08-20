package service

import (
	"context"
	"errors"
	"testing"
	"time"

	imperialpointv1 "github.com/lasthearth/vsservice/gen/imperialpoint/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/progression/internal/model"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	testPointID = "652f1c1d1c1d1c1d1c1d1c1d"
	testTreeID  = "652f1c1d1c1d1c1d1c1d1c2e"
	sideEast    = "east"
	sideWest    = "west"
	settlA      = "652f1c1d1c1d1c1d1c1d1c3f"
	settlB      = "652f1c1d1c1d1c1d1c1d1c4a"
)

// fakeRepo implements the slice of ProgressionRepository that control
// transitions touch; the embedded interface panics on anything else, which keeps
// unexercised methods out of the fake.
type fakeRepo struct {
	ProgressionRepository

	point    *model.ImperialPoint
	progress map[string]*model.TalentProgress

	saveControlErr  error
	saveProgressErr error

	saveProgressCalls int
}

func progressKey(pointId, side, treeId string) string {
	return pointId + "|" + side + "|" + treeId
}

func (r *fakeRepo) GetPoint(_ context.Context, _ string) (*model.ImperialPoint, error) {
	return clonePoint(r.point), nil
}

func (r *fakeRepo) ListPoints(_ context.Context) ([]model.ImperialPoint, error) {
	return []model.ImperialPoint{*clonePoint(r.point)}, nil
}

func (r *fakeRepo) SaveControl(_ context.Context, _ string, control *model.PointControl) error {
	if r.saveControlErr != nil {
		return r.saveControlErr
	}
	r.point.RestoreControl(control)
	return nil
}

func (r *fakeRepo) GetOrCreateProgress(_ context.Context, ownerType, _, pointId, side, treeId string) (*model.TalentProgress, error) {
	key := progressKey(pointId, side, treeId)
	stored, ok := r.progress[key]
	if !ok {
		stored = model.ReconstituteTalentProgress("p-"+key, model.OwnerType(ownerType), "", pointId, side, treeId, nil)
		r.progress[key] = stored
	}
	return cloneProgress(stored), nil
}

func (r *fakeRepo) SaveProgress(_ context.Context, progress model.TalentProgress) error {
	r.saveProgressCalls++
	if r.saveProgressErr != nil {
		return r.saveProgressErr
	}
	r.progress[progressKey(progress.PointId, progress.Side, progress.TreeId)] = cloneProgress(&progress)
	return nil
}

func clonePoint(p *model.ImperialPoint) *model.ImperialPoint {
	out := &model.ImperialPoint{
		Id:            p.Id,
		Name:          p.Name,
		Description:   p.Description,
		BiRatePerHour: p.BiRatePerHour,
		TreeId:        p.TreeId,
	}
	if p.Control != nil {
		ctrl := *p.Control
		out.RestoreControl(&ctrl)
	}
	return out
}

func cloneProgress(p *model.TalentProgress) *model.TalentProgress {
	nodes := make([]model.PurchasedNode, len(p.PurchasedNodes))
	copy(nodes, p.PurchasedNodes)
	return model.ReconstituteTalentProgress(p.Id, p.OwnerType, p.SettlementId, p.PointId, p.Side, p.TreeId, nodes)
}

func newTestService(t *testing.T, repo ProgressionRepository) *Service {
	t.Helper()
	zc := zap.NewProductionConfig()
	zc.Level = zap.NewAtomicLevelAt(zapcore.FatalLevel)
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}
	return New(Opts{Log: l, Repo: repo})
}

// nodes builds a chronologically ordered purchase history.
func nodes(ids ...string) []model.PurchasedNode {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]model.PurchasedNode, len(ids))
	for i, id := range ids {
		out[i] = model.PurchasedNode{NodeId: id, PurchasedAt: base.Add(time.Duration(i) * time.Hour)}
	}
	return out
}

func nodeIDs(p *model.TalentProgress) []string {
	if p == nil {
		return nil
	}
	out := make([]string, len(p.PurchasedNodes))
	for i, n := range p.PurchasedNodes {
		out[i] = n.NodeId
	}
	return out
}

func TestControlTransitionRollsBackLosingSide(t *testing.T) {
	saveErr := errors.New("boom")

	cases := []struct {
		name string

		heldBy      string // side controlling the point before the call, "" = unclaimed
		release     bool   // ReleaseControl instead of SetControl
		capturedBy  string // side passed to SetControl
		treeId      string
		eastNodes   []string
		controlErr  error
		progressErr error

		wantErr       bool
		wantControl   string // side persisted after the call, "" = unclaimed
		wantEastNodes []string
	}{
		{
			name:          "claim of an unclaimed point rolls back nothing",
			heldBy:        "",
			capturedBy:    sideEast,
			treeId:        testTreeID,
			eastNodes:     []string{"n1", "n2"},
			wantControl:   sideEast,
			wantEastNodes: []string{"n1", "n2"},
		},
		{
			name:          "capture by the other side drops the loser's last node",
			heldBy:        sideEast,
			capturedBy:    sideWest,
			treeId:        testTreeID,
			eastNodes:     []string{"n1", "n2"},
			wantControl:   sideWest,
			wantEastNodes: []string{"n1"},
		},
		{
			name:          "re-claim by the same side keeps its progress",
			heldBy:        sideEast,
			capturedBy:    sideEast,
			treeId:        testTreeID,
			eastNodes:     []string{"n1", "n2"},
			wantControl:   sideEast,
			wantEastNodes: []string{"n1", "n2"},
		},
		{
			name:          "capture of a point without a tree rolls back nothing",
			heldBy:        sideEast,
			capturedBy:    sideWest,
			treeId:        "",
			eastNodes:     []string{"n1", "n2"},
			wantControl:   sideWest,
			wantEastNodes: []string{"n1", "n2"},
		},
		{
			name:          "capture with no purchases succeeds",
			heldBy:        sideEast,
			capturedBy:    sideWest,
			treeId:        testTreeID,
			wantControl:   sideWest,
			wantEastNodes: nil,
		},
		{
			name:          "release drops the holder's last node",
			heldBy:        sideEast,
			release:       true,
			treeId:        testTreeID,
			eastNodes:     []string{"n1", "n2"},
			wantControl:   "",
			wantEastNodes: []string{"n1"},
		},
		{
			// The bug this merge fixes: a failing control write must not leave
			// the rollback committed.
			name:          "control write failure restores the rolled-back node and keeps the old holder",
			heldBy:        sideEast,
			capturedBy:    sideWest,
			treeId:        testTreeID,
			eastNodes:     []string{"n1", "n2"},
			controlErr:    saveErr,
			wantErr:       true,
			wantControl:   sideEast,
			wantEastNodes: []string{"n1", "n2"},
		},
		{
			// The other half: a failing rollback must abort the capture.
			name:          "rollback failure aborts the capture",
			heldBy:        sideEast,
			capturedBy:    sideWest,
			treeId:        testTreeID,
			eastNodes:     []string{"n1", "n2"},
			progressErr:   saveErr,
			wantErr:       true,
			wantControl:   sideEast,
			wantEastNodes: []string{"n1", "n2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			point := &model.ImperialPoint{Id: testPointID, Name: "point", TreeId: tc.treeId}
			if tc.heldBy != "" {
				point.SetControl(tc.heldBy, settlA)
			}

			repo := &fakeRepo{point: point, progress: map[string]*model.TalentProgress{}}
			if len(tc.eastNodes) > 0 {
				key := progressKey(testPointID, sideEast, testTreeID)
				repo.progress[key] = model.ReconstituteTalentProgress(
					"p-"+key, model.OwnerTypePointSide, "", testPointID, sideEast, testTreeID, nodes(tc.eastNodes...))
			}
			repo.saveControlErr = tc.controlErr
			repo.saveProgressErr = tc.progressErr

			svc := newTestService(t, repo)

			var err error
			if tc.release {
				_, err = svc.ReleaseControl(context.Background(), &imperialpointv1.ReleaseControlRequest{PointId: testPointID})
			} else {
				_, err = svc.SetControl(context.Background(), &imperialpointv1.SetControlRequest{
					PointId:      testPointID,
					Side:         tc.capturedBy,
					SettlementId: settlB,
				})
			}

			if tc.wantErr != (err != nil) {
				t.Fatalf("error = %v, wantErr = %v", err, tc.wantErr)
			}

			gotControl := ""
			if repo.point.Control != nil {
				gotControl = repo.point.Control.Side
			}
			if gotControl != tc.wantControl {
				t.Errorf("persisted control side = %q, want %q", gotControl, tc.wantControl)
			}

			got := nodeIDs(repo.progress[progressKey(testPointID, sideEast, testTreeID)])
			if len(got) != len(tc.wantEastNodes) {
				t.Fatalf("east nodes = %v, want %v", got, tc.wantEastNodes)
			}
			for i := range got {
				if got[i] != tc.wantEastNodes[i] {
					t.Fatalf("east nodes = %v, want %v", got, tc.wantEastNodes)
				}
			}
		})
	}
}
