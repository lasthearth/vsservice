package service

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// validateImageURL checks that the URL is non-empty and its host is allowed
// (our CDN or an allowlisted external host).
func (s *Service) validateImageURL(u string) error {
	if u == "" {
		return status.Error(codes.InvalidArgument, "image_url is required")
	}
	if err := s.mediaUrl.Validate(u); err != nil {
		return status.Error(codes.InvalidArgument, "image_url host is not allowed")
	}
	return nil
}
