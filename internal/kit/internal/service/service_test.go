package service

import (
	"context"
	"errors"
	"testing"

	kitv1 "github.com/lasthearth/vsservice/gen/kit/v1"
	"github.com/lasthearth/vsservice/internal/kit/internal/model"
	"github.com/lasthearth/vsservice/internal/kit/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeAssignments stands in for Mongo. It records what the service asked it to
// store, which is the whole point of these tests: the assignment itself is the
// deliverable, and the event that used to be published next to it had no
// subscriber on either side.
type fakeAssignments struct {
	created []*model.KitAssignment
	err     error
}

func (r *fakeAssignments) CreateAssignment(
	_ context.Context,
	assignment *model.KitAssignment,
) (*model.KitAssignment, error) {
	if r.err != nil {
		return nil, r.err
	}

	// Through the model's own method: the id is persisted identity, and setting
	// the field from here is what the modelguard linter exists to stop.
	assignment.AssignID("assignment-1")
	r.created = append(r.created, assignment)

	return assignment, nil
}

func (r *fakeAssignments) GetAssignment(context.Context, string) (*model.KitAssignment, error) {
	return nil, errors.New("not reached")
}

func (r *fakeAssignments) UpdateAssignment(
	context.Context,
	string,
	func(context.Context, *model.KitAssignment) (*model.KitAssignment, error),
) error {
	return errors.New("not reached")
}

func (r *fakeAssignments) GetAssignmentsByUserID(context.Context, string) ([]*model.KitAssignment, error) {
	return nil, errors.New("not reached")
}

var _ AssignmentRepository = (*fakeAssignments)(nil)

func newService(t *testing.T, repo AssignmentRepository) *Service {
	t.Helper()

	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	// The kit repository is nil exactly as fx builds it: AssignKitToUser never
	// reads the kit list.
	return New(nil, repo, l, &sermapper.MapperImpl{})
}

// TestAssignKitToUserCreatesTheAssignment pins that removing the bus left the
// write in place. The publish used to sit between the create and the response,
// so a removal that took the create with it would still answer OK.
func TestAssignKitToUserCreatesTheAssignment(t *testing.T) {
	repo := &fakeAssignments{}
	svc := newService(t, repo)

	ctx := interceptor.ContextWithUserID(context.Background(), "admin-1")
	if _, err := svc.AssignKitToUser(ctx, &kitv1.AssignKitToUserRequest{
		UserId:       "user-1",
		UserGameName: "Ветер",
		KitName:      "starter",
	}); err != nil {
		t.Fatalf("AssignKitToUser: %v", err)
	}

	if len(repo.created) != 1 {
		t.Fatalf("AssignKitToUser stored %d assignments, want 1", len(repo.created))
	}

	got := repo.created[0]
	if got.UserId != "user-1" {
		t.Errorf("UserId = %q, want user-1", got.UserId)
	}
	if got.UserGameName != "Ветер" {
		t.Errorf("UserGameName = %q, want Ветер", got.UserGameName)
	}
	if got.KitName != "starter" {
		t.Errorf("KitName = %q, want starter", got.KitName)
	}
	// The requester, not the recipient: an assignment must say which admin made
	// it, and that is the value the interceptor put in ctx.
	if got.AssignedBy != "admin-1" {
		t.Errorf("AssignedBy = %q, want admin-1", got.AssignedBy)
	}
	if got.Status != model.AssignmentStatusPending {
		t.Errorf("Status = %v, want pending: nothing is delivered until the game picks it up", got.Status)
	}
}

// TestAssignKitToUserReportsAFailedCreate pins the error path. A create the
// repository refuses must reach the caller as Internal rather than be logged and
// answered as a success, which is what a fire-and-forget publish would have done.
func TestAssignKitToUserReportsAFailedCreate(t *testing.T) {
	repo := &fakeAssignments{err: errors.New("mongo is unreachable")}
	svc := newService(t, repo)

	ctx := interceptor.ContextWithUserID(context.Background(), "admin-1")
	_, err := svc.AssignKitToUser(ctx, &kitv1.AssignKitToUserRequest{
		UserId:       "user-1",
		UserGameName: "Ветер",
		KitName:      "starter",
	})
	if err == nil {
		t.Fatal("AssignKitToUser answered OK for a create the repository refused")
	}

	if status.Code(err) != codes.Internal {
		t.Errorf("code = %v, want Internal", status.Code(err))
	}
	if len(repo.created) != 0 {
		t.Errorf("the fake stored %d assignments after a refused create", len(repo.created))
	}
}

// TestAssignKitToUserNeedsACaller pins that the requester is read from ctx: with
// no user id there is nobody to record as the assigner, and Validate would
// reject an assignment with an empty AssignedBy anyway.
func TestAssignKitToUserNeedsACaller(t *testing.T) {
	svc := newService(t, &fakeAssignments{})

	_, err := svc.AssignKitToUser(context.Background(), &kitv1.AssignKitToUserRequest{
		UserId:       "user-1",
		UserGameName: "Ветер",
		KitName:      "starter",
	})
	if err == nil {
		t.Fatal("AssignKitToUser answered OK with no caller in the context")
	}

	if status.Code(err) != codes.Internal {
		t.Errorf("code = %v, want Internal", status.Code(err))
	}
}
