package service_test

import (
	"context"
	"errors"
	"testing"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"google.golang.org/grpc/codes"
)

// The block the fixture has standing. World X and Z are the site's X and Y.
var fixturePlacement = model.NotifierPlacement{
	SettlementId: "s1",
	Position:     model.Vector3{X: 7, Y: 64, Z: -300},
}

// fixtureRepo is a settlement with two owners, a neighbour, and a notifier block
// standing. Every single-settlement handler below answers from it.
func fixtureRepo() *notifierRepo {
	set := memberSet(model.SettlementTypeVillage)
	set.ImperialFavor = 10
	set.RolesEnabled = true

	other := &model.Settlement{
		Id:            "s2",
		Name:          "Южный Форт",
		Type:          model.SettlementTypeCamp,
		ImperialFavor: 10,
		Leader:        model.Member{UserId: "owner9", RoleIds: []string{model.OwnerRoleId}},
		Members:       []model.Member{{UserId: "owner9", RoleIds: []string{model.OwnerRoleId}}},
	}
	return &notifierRepo{
		set:       set,
		others:    map[string]*model.Settlement{"s2": other},
		placement: &fixturePlacement,
	}
}

// singleSettlementCalls is one entry per rpc that answers with a single
// settlement. They all have to fill notifier_position, not only Get and
// GetByUserId: a site model refreshed from a PATCH or a TransferOwnership used
// to lose the field, so it rendered the upgrade form editable while Submit
// dropped the coordinates the player typed.
func singleSettlementCalls() []struct {
	name string
	call func(*service.Service) (*settlementv1.Settlement, error)
} {
	owner := asUser("owner1")
	attachments := []*settlementv1.UpdateSettlementRequest_UpdateAttachment{
		{Url: "https://cdn.test/a.png", Description: "d"},
	}

	return []struct {
		name string
		call func(*service.Service) (*settlementv1.Settlement, error)
	}{
		{"Get", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.Get(context.Background(), &settlementv1.GetRequest{Id: "s1"})
			return res.GetSettlement(), err
		}},
		{"GetByUserId", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.GetByUserId(context.Background(), &settlementv1.GetByUserIdRequest{UserId: "owner1"})
			return res.GetSettlement(), err
		}},
		{"AdminUpdateSettlement", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.AdminUpdateSettlement(owner, &settlementv1.AdminUpdateSettlementRequest{Id: "s1", Diplomacy: "neutral"})
			return res.GetSettlement(), err
		}},
		{"UpdateSettlement", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.UpdateSettlement(owner, &settlementv1.UpdateSettlementRequest{
				Id: "s1", Name: "Северный Оплот", Description: "d", Attachments: attachments,
			})
			return res.GetSettlement(), err
		}},
		{"CreateRole", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.CreateRole(owner, &settlementv1.CreateRoleRequest{SettlementId: "s1", Name: "разведчик"})
			return res.GetSettlement(), err
		}},
		{"AddOwner", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.AddOwner(owner, &settlementv1.AddOwnerRequest{SettlementId: "s1", UserId: "builder"})
			return res.GetSettlement(), err
		}},
		{"RemoveOwner", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.RemoveOwner(owner, &settlementv1.RemoveOwnerRequest{SettlementId: "s1", UserId: "owner2"})
			return res.GetSettlement(), err
		}},
		{"SetRolesEnabled", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.SetRolesEnabled(owner, &settlementv1.SetRolesEnabledRequest{SettlementId: "s1", Enabled: true})
			return res.GetSettlement(), err
		}},
		{"TransferOwnership", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.TransferOwnership(owner, &settlementv1.TransferOwnershipRequest{SettlementId: "s1", ToUserId: "builder"})
			return res.GetSettlement(), err
		}},
		{"UpdateContactInfo", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.UpdateContactInfo(owner, &settlementv1.UpdateContactInfoRequest{SettlementId: "s1", ContactInfo: "t.me/north"})
			return res.GetSettlement(), err
		}},
		{"AddTagToSettlement", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.AddTagToSettlement(owner, &settlementv1.AddTagToSettlementRequest{SettlementId: "s1", TagId: "t1"})
			return res.GetSettlement(), err
		}},
		{"RemoveTagFromSettlement", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.RemoveTagFromSettlement(owner, &settlementv1.RemoveTagFromSettlementRequest{SettlementId: "s1", TagId: "t1"})
			return res.GetSettlement(), err
		}},
		{"AddImperialFavor", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.AddImperialFavor(owner, &settlementv1.AddImperialFavorRequest{SettlementId: "s1", Amount: 5, Reason: "r"})
			return res.GetSettlement(), err
		}},
		{"DeductImperialFavor", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.DeductImperialFavor(owner, &settlementv1.DeductImperialFavorRequest{SettlementId: "s1", Amount: 5, Reason: "r"})
			return res.GetSettlement(), err
		}},
		{"TransferImperialFavor/from", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.TransferImperialFavor(owner, &settlementv1.TransferImperialFavorRequest{
				FromSettlementId: "s1", ToSettlementId: "s2", Amount: 5,
			})
			return res.GetFromSettlement(), err
		}},
		{"TransferImperialFavor/to", func(s *service.Service) (*settlementv1.Settlement, error) {
			res, err := s.TransferImperialFavor(owner, &settlementv1.TransferImperialFavorRequest{
				FromSettlementId: "s1", ToSettlementId: "s2", Amount: 5,
			})
			return res.GetToSettlement(), err
		}},
	}
}

func TestSingleSettlementResponsesFillTheNotifierPosition(t *testing.T) {
	for _, tc := range singleSettlementCalls() {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := newNotifierService(t, fixtureRepo())

			got, err := tc.call(svc)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			p := got.GetNotifierPosition()
			if p.GetX() != 7 || p.GetY() != 64 || p.GetZ() != -300 {
				t.Errorf("%s: notifier_position = %v, want (7,64,-300)", tc.name, p)
			}
		})
	}
}

// "No document" is a normal answer and leaves the position unset. Only a decode
// or connection failure fails closed, so the two must not be conflated.
func TestSingleSettlementResponseWithoutBlockServesUnsetPosition(t *testing.T) {
	repo := fixtureRepo()
	repo.placement = nil
	svc, _, _ := newNotifierService(t, repo)

	res, err := svc.AdminUpdateSettlement(asUser("owner1"),
		&settlementv1.AdminUpdateSettlementRequest{Id: "s1", Diplomacy: "neutral"})
	if err != nil {
		t.Fatal(err)
	}
	if p := res.GetSettlement().GetNotifierPosition(); p != nil {
		t.Errorf("notifier_position = %v, want unset while no block stands", p)
	}
}

// A failing read must not be served as an unset position: the client cannot tell
// the two apart, and an unset position tells it the coordinates are editable.
func TestSingleSettlementResponsesFailClosedOnNotifierRead(t *testing.T) {
	for _, tc := range singleSettlementCalls() {
		t.Run(tc.name, func(t *testing.T) {
			repo := fixtureRepo()
			repo.notifierErr = errors.New("decode failed")
			svc, _, _ := newNotifierService(t, repo)

			_, err := tc.call(svc)
			if !errors.Is(err, ierror.ErrNotifierRead) {
				t.Fatalf("%s: want ErrNotifierRead, got %v", tc.name, err)
			}
			var de *pkgerr.DomainError
			if !errors.As(err, &de) || de.Code != codes.Unavailable {
				t.Errorf("%s: error = %v, want a typed Unavailable", tc.name, err)
			}
		})
	}
}

// List deliberately leaves the position unset: filling it would cost one extra
// read per row over every settlement the server has.
func TestListLeavesTheNotifierPositionUnset(t *testing.T) {
	svc, _, _ := newNotifierService(t, fixtureRepo())

	res, err := svc.List(context.Background(), &settlementv1.ListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetSettlements()) == 0 {
		t.Fatal("no settlements listed")
	}
	for _, s := range res.GetSettlements() {
		if p := s.GetNotifierPosition(); p != nil {
			t.Errorf("settlement %s: notifier_position = %v, want unset in List", s.GetId(), p)
		}
	}
}

// submitRepo is a settlement whose request was already approved once, so Submit
// takes the upgrade path: the only one where a block can override coordinates.
func submitRepo(placement *model.NotifierPlacement) *notifierRepo {
	return &notifierRepo{
		request: &model.SettlementVerification{
			Id: "s1", Status: model.SettlementStatusApproved,
			Type: model.SettlementTypeCamp, UpdatedAt: submittedAt,
		},
		set:       memberSet(model.SettlementTypeCamp),
		placement: placement,
	}
}

func submitFor(t *testing.T, svc *service.Service, x, y int32) *settlementv1.SubmitResponse {
	t.Helper()
	res, err := svc.Submit(asUser("owner1"), &settlementv1.SubmitRequest{
		Type:        settlementv1.SubmitRequest_CAMP,
		Name:        "Северный Оплот",
		Coordinates: &settlementv1.Vector2{X: x, Y: y},
		Attachments: []*settlementv1.SubmitRequest_SubmitAttachment{{Url: "https://cdn.test/a.png", Description: "d"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// The override used to be silent: an empty SubmitResponse and a 200, while the
// stored coordinates were the block's and not the ones the player typed.
func TestSubmitReportsThatTheNotifierLockedTheCoordinates(t *testing.T) {
	repo := submitRepo(&model.NotifierPlacement{
		SettlementId: "s1", Position: model.Vector3{X: 100, Y: 64, Z: -300},
	})
	svc, _, _ := newNotifierService(t, repo)

	res := submitFor(t, svc, 10, 20)

	if !res.GetCoordinatesLocked() {
		t.Error("coordinates_locked = false, want true: the block overrode the typed coordinates")
	}
	if got := repo.updatedReq.Coordinates; got != (model.Vector2{X: 100, Y: -300}) {
		t.Errorf("stored coordinates = %+v, want the block's (100,-300)", got)
	}
}

func TestSubmitReportsUnlockedCoordinatesWithoutNotifier(t *testing.T) {
	repo := submitRepo(nil)
	svc, _, _ := newNotifierService(t, repo)

	res := submitFor(t, svc, 10, 20)

	if res.GetCoordinatesLocked() {
		t.Error("coordinates_locked = true, want false with no block placed")
	}
	if got := repo.updatedReq.Coordinates; got != (model.Vector2{X: 10, Y: 20}) {
		t.Errorf("stored coordinates = %+v, want the typed (10,20)", got)
	}
}

// A first submission has no settlement, so it has no block and nothing can be
// locked — even when a stale placement for that id is still on file.
func TestFirstSubmissionIsNeverLocked(t *testing.T) {
	repo := &notifierRepo{placement: &fixturePlacement}
	svc, _, _ := newNotifierService(t, repo)

	res := submitFor(t, svc, 10, 20)

	if res.GetCoordinatesLocked() {
		t.Error("coordinates_locked = true on a first submission, want false")
	}
	if repo.createdReq == nil {
		t.Fatal("no request was created")
	}
	if got := repo.createdReq.Coordinates; got != (model.Vector2{X: 10, Y: 20}) {
		t.Errorf("stored coordinates = %+v, want the typed (10,20)", got)
	}
}

// Submit used to return the raw driver error, which the gateway turned into a
// bare INTERNAL. It now fails closed with the same typed error the readers use.
func TestSubmitFailsClosedOnNotifierRead(t *testing.T) {
	repo := submitRepo(nil)
	repo.notifierErr = errors.New("connection reset")
	svc, _, _ := newNotifierService(t, repo)

	_, err := svc.Submit(asUser("owner1"), &settlementv1.SubmitRequest{
		Type:        settlementv1.SubmitRequest_CAMP,
		Name:        "Северный Оплот",
		Coordinates: &settlementv1.Vector2{X: 10, Y: 20},
		Attachments: []*settlementv1.SubmitRequest_SubmitAttachment{{Url: "https://cdn.test/a.png", Description: "d"}},
	})
	if !errors.Is(err, ierror.ErrNotifierRead) {
		t.Fatalf("want ErrNotifierRead, got %v", err)
	}
	var de *pkgerr.DomainError
	if !errors.As(err, &de) || de.Code != codes.Unavailable {
		t.Errorf("error = %v, want a typed Unavailable", err)
	}
	if repo.updatedReq != nil {
		t.Error("the request was written although the notifier read failed")
	}
}
