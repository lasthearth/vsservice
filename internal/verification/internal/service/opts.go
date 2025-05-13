package service

import "github.com/lasthearth/vsservice/internal/verification/model"

type VerifyOpts struct {
	UserID       string
	UserName     string
	UserGameName string
	Contacts     string
	Answers      []model.Answer
}
