package service

import (
	"context"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	mongomodel "github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/model"
)

func permsFromProto(ps []settlementv1.Permission) []model.Permission {
	out := make([]model.Permission, 0, len(ps))
	for _, p := range ps {
		if m := PermissionFromProto(p); m != "" {
			out = append(out, m)
		}
	}
	return out
}

// CreateRole implements settlementv1.SettlementServiceServer.
func (s *Service) CreateRole(ctx context.Context, req *settlementv1.CreateRoleRequest) (*settlementv1.CreateRoleResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireOwner(ctx, req.GetSettlementId(), uid); err != nil {
		return nil, err
	}

	roleID := mongomodel.NewModel().Id.Hex()
	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, set *model.Settlement) (*model.Settlement, error) {
			if err := set.CreateRole(roleID, req.GetName(), permsFromProto(req.GetPermissions())); err != nil {
				return nil, mapModelErr(err)
			}
			return set, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &settlementv1.CreateRoleResponse{Settlement: s.mapper.ToSettlementProto(*updated)}, nil
}

// UpdateRole implements settlementv1.SettlementServiceServer.
func (s *Service) UpdateRole(ctx context.Context, req *settlementv1.UpdateRoleRequest) (*settlementv1.UpdateRoleResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireOwner(ctx, req.GetSettlementId(), uid); err != nil {
		return nil, err
	}

	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, set *model.Settlement) (*model.Settlement, error) {
			if err := set.UpdateRole(req.GetRoleId(), req.GetName(), permsFromProto(req.GetPermissions())); err != nil {
				return nil, mapModelErr(err)
			}
			return set, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &settlementv1.UpdateRoleResponse{Settlement: s.mapper.ToSettlementProto(*updated)}, nil
}

// DeleteRole implements settlementv1.SettlementServiceServer.
func (s *Service) DeleteRole(ctx context.Context, req *settlementv1.DeleteRoleRequest) (*settlementv1.DeleteRoleResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireOwner(ctx, req.GetSettlementId(), uid); err != nil {
		return nil, err
	}

	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, set *model.Settlement) (*model.Settlement, error) {
			if err := set.DeleteRole(req.GetRoleId()); err != nil {
				return nil, mapModelErr(err)
			}
			return set, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &settlementv1.DeleteRoleResponse{Settlement: s.mapper.ToSettlementProto(*updated)}, nil
}

// AssignRole implements settlementv1.SettlementServiceServer.
func (s *Service) AssignRole(ctx context.Context, req *settlementv1.AssignRoleRequest) (*settlementv1.AssignRoleResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireOwner(ctx, req.GetSettlementId(), uid); err != nil {
		return nil, err
	}

	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, set *model.Settlement) (*model.Settlement, error) {
			if err := set.AssignRole(req.GetUserId(), req.GetRoleId()); err != nil {
				return nil, mapModelErr(err)
			}
			return set, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &settlementv1.AssignRoleResponse{Settlement: s.mapper.ToSettlementProto(*updated)}, nil
}

// UnassignRole implements settlementv1.SettlementServiceServer.
func (s *Service) UnassignRole(ctx context.Context, req *settlementv1.UnassignRoleRequest) (*settlementv1.UnassignRoleResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireOwner(ctx, req.GetSettlementId(), uid); err != nil {
		return nil, err
	}

	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, set *model.Settlement) (*model.Settlement, error) {
			if err := set.UnassignRole(req.GetUserId(), req.GetRoleId()); err != nil {
				return nil, mapModelErr(err)
			}
			return set, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &settlementv1.UnassignRoleResponse{Settlement: s.mapper.ToSettlementProto(*updated)}, nil
}

// AddOwner implements settlementv1.SettlementServiceServer (admin).
func (s *Service) AddOwner(ctx context.Context, req *settlementv1.AddOwnerRequest) (*settlementv1.AddOwnerResponse, error) {
	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, set *model.Settlement) (*model.Settlement, error) {
			if err := set.GrantOwner(req.GetUserId()); err != nil {
				return nil, mapModelErr(err)
			}
			return set, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &settlementv1.AddOwnerResponse{Settlement: s.mapper.ToSettlementProto(*updated)}, nil
}

// RemoveOwner implements settlementv1.SettlementServiceServer (admin).
func (s *Service) RemoveOwner(ctx context.Context, req *settlementv1.RemoveOwnerRequest) (*settlementv1.RemoveOwnerResponse, error) {
	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, set *model.Settlement) (*model.Settlement, error) {
			if err := set.RevokeOwner(req.GetUserId()); err != nil {
				return nil, mapModelErr(err)
			}
			return set, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &settlementv1.RemoveOwnerResponse{Settlement: s.mapper.ToSettlementProto(*updated)}, nil
}

// SetRolesEnabled implements settlementv1.SettlementServiceServer (admin).
func (s *Service) SetRolesEnabled(ctx context.Context, req *settlementv1.SetRolesEnabledRequest) (*settlementv1.SetRolesEnabledResponse, error) {
	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, set *model.Settlement) (*model.Settlement, error) {
			set.SetRolesEnabled(req.GetEnabled())
			return set, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &settlementv1.SetRolesEnabledResponse{Settlement: s.mapper.ToSettlementProto(*updated)}, nil
}

// DeleteSettlement implements settlementv1.SettlementServiceServer (admin).
func (s *Service) DeleteSettlement(ctx context.Context, req *settlementv1.DeleteSettlementRequest) (*settlementv1.DeleteSettlementResponse, error) {
	if err := s.dbRepo.DeleteSettlement(ctx, req.GetSettlementId()); err != nil {
		return nil, err
	}
	return &settlementv1.DeleteSettlementResponse{}, nil
}
