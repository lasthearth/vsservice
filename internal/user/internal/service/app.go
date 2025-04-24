package service

import (
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
)

var _ userv1.UserServiceServer = (*Service)(nil)

type Service struct {
	repo Repository
}
