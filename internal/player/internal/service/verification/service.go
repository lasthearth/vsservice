package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	verificationv1 "github.com/lasthearth/vsservice/gen/verification/v1"
	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	httpdto "github.com/lasthearth/vsservice/internal/player/internal/dto/http"
	playerierror "github.com/lasthearth/vsservice/internal/player/internal/ierror"
	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/verification/repository/repoerr"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

type DbRepository interface {
	GetVerification(ctx context.Context, userId string) (*verification.Verification, error)
	GetVerificationRequests(ctx context.Context) ([]verification.Verification, error)
	GetVerificationStatusByUserGameName(ctx context.Context, userGameName string) (verification.Status, error)
	Create(ctx context.Context, userId string, v verification.Verification) error
	Update(ctx context.Context, userId string, v verification.Verification) error
}

type SsoRepository interface {
	GetUserRoles(ctx context.Context, userId string) ([]httpdto.Role, error)
	GetRoles(ctx context.Context) ([]httpdto.Role, error)
	UpdateUserRoles(ctx context.Context, userId string, roleIds []string) error
	UpdateUserProfileNick(ctx context.Context, userId, nickname string) error
}

// Approve implements verificationv1.VerificationServiceServer.
func (s *Service) Approve(ctx context.Context, req *verificationv1.ApproveRequest) (*verificationv1.ApproveResponse, error) {
	err := s.checkUserRoles(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	roles, err := s.ssoRepo.GetRoles(ctx)
	if err != nil {
		return nil, err
	}

	playerRoleId := ""
	for i := range roles {
		if roles[i].Name == "player" {
			playerRoleId = roles[i].Id
		}
	}

	v, err := s.dbRepo.GetVerification(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	if err := v.Approve(); err != nil {
		return nil, ierror.FailedPrecondition(err.Error())
	}

	err = s.dbRepo.Update(ctx, req.GetUserId(), *v)
	if err != nil {
		return nil, err
	}

	err = s.ssoRepo.UpdateUserRoles(ctx, req.GetUserId(), []string{playerRoleId})
	if err != nil {
		return nil, err
	}

	return &verificationv1.ApproveResponse{}, nil
}

// Reject implements verificationv1.VerificationServiceServer.
func (s *Service) Reject(ctx context.Context, req *verificationv1.RejectRequest) (*verificationv1.RejectResponse, error) {
	v, err := s.dbRepo.GetVerification(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	if err := v.Reject(req.GetRejectionReason().GetRejectionReason()); err != nil {
		return nil, ierror.FailedPrecondition(err.Error())
	}

	err = s.dbRepo.Update(ctx, req.GetUserId(), *v)
	if err != nil {
		return nil, err
	}

	return &verificationv1.RejectResponse{}, nil
}

// Submit implements verificationv1.VerificationServiceServer.
func (s *Service) Submit(ctx context.Context, req *verificationv1.SubmitRequest) (*verificationv1.SubmitResponse, error) {
	// TODO: переделать на маппер
	answers := lo.Map(req.GetAnswers(), func(v *verificationv1.Answer, _ int) verification.Answer {
		return verification.Answer{
			Question: v.GetQuestion(),
			Answer:   v.GetAnswer(),
		}
	})

	userId, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.ssoRepo.UpdateUserProfileNick(ctx, userId, req.GetUserGameName()); err != nil {
		return nil, err
	}

	v := verification.New(
		userId,
		req.GetUserName(),
		req.GetUserGameName(),
		answers,
		req.GetContacts(),
	)

	tgText := fmt.Sprintf("У игрока %s появилась новая анкета.", req.GetUserGameName())
	existVerification, err := s.dbRepo.GetVerification(ctx, userId)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			if err := s.dbRepo.Create(ctx, userId, *v); err != nil {
				if errors.Is(err, repoerr.ErrNickAlreadyExists) {
					return nil, playerierror.ErrNickAlreadyExists
				}
				return nil, err
			}

			_ = s.SendToTelegram(ctx, tgText)
			return &verificationv1.SubmitResponse{}, nil
		}

		return nil, err
	}

	if err := existVerification.CanSubmit(); err != nil {
		return nil, ierror.FailedPrecondition(err.Error())
	}

	if err := s.dbRepo.Update(ctx, userId, *v); err != nil {
		if errors.Is(err, repoerr.ErrNickAlreadyExists) {
			return nil, playerierror.ErrNickAlreadyExists
		}
		return nil, err
	}
	_ = s.SendToTelegram(ctx, tgText)
	return &verificationv1.SubmitResponse{}, nil
}

func (s *Service) SendToTelegram(ctx context.Context, text string) error {
	v := struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{
		ChatID: s.cfg.GroupId,
		Text:   text,
	}

	encoded, err := json.Marshal(v)
	if err != nil {
		s.log.Error("failed to marshal JSON payload", zap.Error(err))
		return err
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.cfg.TelegramToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(encoded))
	if err != nil {
		s.log.Error("failed to create telegram request", zap.Error(err))
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		s.log.Error("failed to send telegram message", zap.Error(err))
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}

// Details implements verificationv1.VerificationServiceServer.
func (s *Service) Details(ctx context.Context, req *verificationv1.DetailsRequest) (*verificationv1.DetailsResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	verif, err := s.dbRepo.GetVerification(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &verificationv1.DetailsResponse{
		Id:              verif.Id,
		Status:          string(verif.Status),
		RejectionReason: verif.RejectionReason,
	}, nil
}

// List implements verificationv1.VerificationServiceServer.
func (s *Service) List(ctx context.Context, req *verificationv1.ListRequest) (*verificationv1.ListResponse, error) {
	reqs, err := s.dbRepo.GetVerificationRequests(ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]*verificationv1.ListResponse_VerifyUserRequest, len(reqs))
	for i, req := range reqs {
		answers := lo.Map(
			req.Answers,
			func(a verification.Answer, _ int) *verificationv1.Answer {
				return &verificationv1.Answer{
					Question: a.Question,
					Answer:   a.Answer,
				}
			},
		)

		resp[i] = &verificationv1.ListResponse_VerifyUserRequest{
			UserId:       req.UserId,
			UserName:     req.UserName,
			UserGameName: req.UserGameName,
			Contacts:     req.Contacts,
			Answers:      answers,
		}
	}

	return &verificationv1.ListResponse{
		Requests: resp,
	}, nil
}

func (s *Service) checkUserRoles(ctx context.Context, userId string) error {
	ssoRoleName := "player"

	roles, err := s.ssoRepo.GetUserRoles(ctx, userId)
	if err != nil {
		return err
	}

	isVerified := slices.ContainsFunc(roles, func(role httpdto.Role) bool {
		return role.Name == ssoRoleName
	})
	if isVerified {
		return verification.ErrAlreadyVerified
	}

	return nil
}

// VerificationStatus implements verificationv1.VerificationServiceServer.
func (s *Service) VerificationStatus(
	ctx context.Context,
	req *verificationv1.VerifyStatusRequest,
) (*verificationv1.VerifyStatusResponse, error) {
	v, err := s.dbRepo.GetVerification(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	return &verificationv1.VerifyStatusResponse{
		Status: string(v.Status),
	}, nil
}

// VerifyStatusByName implements verificationv1.VerificationServiceServer.
func (s *Service) VerifyStatusByName(ctx context.Context, req *verificationv1.VerifyStatusByNameRequest) (*verificationv1.VerifyStatusResponse, error) {
	status, err := s.dbRepo.GetVerificationStatusByUserGameName(ctx, req.GetUserGameName())
	if err != nil {
		return nil, err
	}

	return &verificationv1.VerifyStatusResponse{
		Status: string(status),
	}, nil
}
