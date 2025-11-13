package service

import (
	"context"
	"errors"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/settlement-tag/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement-tag/internal/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GetTag implements settlementv1.SettlementTagServiceServer.
func (s *Service) GetTag(ctx context.Context, req *settlementv1.GetTagRequest) (*settlementv1.SettlementTag, error) {
	tag, err := s.repo.GetTagById(ctx, req.TagId)
	if err != nil {
		if errors.Is(err, ierror.ErrNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		if errors.Is(err, ierror.ErrInvalidArgument) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to get tag: %v", err)
	}
	resp := s.mapper.TagToProto(*tag)
	return resp, nil
}

// GetTagsByIds implements settlementv1.SettlementTagServiceServer.
func (s *Service) GetTagsByIds(ctx context.Context, req *settlementv1.GetTagsByIdsRequest) (*settlementv1.GetTagsByIdsResponse, error) {
	tags, err := s.repo.GetTagsByIds(ctx, req.TagIds)
	if err != nil {
		if errors.Is(err, ierror.ErrNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		if errors.Is(err, ierror.ErrInvalidArgument) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to get tags: %v", err)
	}
	resp := s.mapper.TagsToProto(tags)
	return &settlementv1.GetTagsByIdsResponse{
		Tags: resp,
	}, nil
}

// CreateTag implements settlementv1.SettlementTagServiceServer.
func (s *Service) CreateTag(ctx context.Context, req *settlementv1.SettlementTag) (*settlementv1.SettlementTag, error) {
	color := model.Color{
		Red:   req.Color.Red,
		Green: req.Color.Green,
		Blue:  req.Color.Blue,
		Alpha: req.Color.Alpha.Value,
	}
	m, err := model.NewTag(
		req.Name,
		req.Description,
		color,
	)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	tag, err := s.repo.CreateTag(ctx, m)
	if err != nil {
		if errors.Is(err, ierror.ErrInvalidArgument) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to create tag: %v", err)
	}
	resp := s.mapper.TagToProto(*tag)
	return resp, nil
}

// DeleteTag implements settlementv1.SettlementTagServiceServer.
func (s *Service) DeleteTag(ctx context.Context, req *settlementv1.DeleteTagRequest) (*emptypb.Empty, error) {
	err := s.repo.SoftDeleteTag(ctx, req.TagId)
	if err != nil {
		if errors.Is(err, ierror.ErrNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		if errors.Is(err, ierror.ErrInvalidArgument) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to delete tag: %v", err)
	}
	return &emptypb.Empty{}, nil
}
