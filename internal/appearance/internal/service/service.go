package service

import (
	"context"
	"errors"

	appearancev1 "github.com/lasthearth/vsservice/gen/appearance/v1"
	"github.com/lasthearth/vsservice/internal/appearance/internal/ierror"
	"github.com/lasthearth/vsservice/internal/appearance/internal/model"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ListAppearances implements appearancev1.AppearanceServiceServer.
func (s *Service) ListAppearances(
	ctx context.Context,
	_ *appearancev1.ListAppearancesRequest,
) (*appearancev1.ListAppearancesResponse, error) {
	list, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	return &appearancev1.ListAppearancesResponse{Appearances: s.mapper.ToProtos(list)}, nil
}

// UpdateMyAppearance implements appearancev1.AppearanceServiceServer.
func (s *Service) UpdateMyAppearance(
	ctx context.Context,
	req *appearancev1.UpdateMyAppearanceRequest,
) (*appearancev1.Appearance, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}

	standing, err := s.standings.Standing(ctx, userID)
	if err != nil {
		return nil, err
	}

	look, err := model.NewAppearance(userID, s.mapper.RequestToChoice(req), standing, s.now())
	if err != nil {
		return nil, mapModelErr(err)
	}

	if err := s.repo.Save(ctx, look); err != nil {
		return nil, err
	}
	return s.mapper.ToProto(*look), nil
}

// ResetMyAppearance implements appearancev1.AppearanceServiceServer.
func (s *Service) ResetMyAppearance(
	ctx context.Context,
	_ *appearancev1.ResetMyAppearanceRequest,
) (*emptypb.Empty, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}
	if err := s.repo.Delete(ctx, userID); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// mapModelErr turns model errors into gRPC statuses, keeping the message that
// names the item.
func mapModelErr(err error) error {
	switch {
	case errors.Is(err, model.ErrUnknownItem):
		return ierror.Unknown(err.Error())
	case errors.Is(err, model.ErrLocked), errors.Is(err, model.ErrNotForSale):
		return ierror.Locked(err.Error())
	case errors.Is(err, model.ErrAlreadyOwned):
		return ierror.Owned(err.Error())
	default:
		return err
	}
}
