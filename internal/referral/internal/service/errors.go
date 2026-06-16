package service

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrSelfReferral = ierror.FailedPrecondition("cannot use your own referral code")
)
