package sso

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-faster/errors"
	httpdto "github.com/lasthearth/vsservice/internal/verification/internal/dto/http"
	"go.uber.org/zap"
)

func (r *Repository) GetUserRoles(ctx context.Context, userId string) ([]httpdto.Role, error) {
	r.logger.Info("getting user roles", zap.String("user_id", userId))

	existingRolesUrl := fmt.Sprintf("%s/api/users/%s/roles", r.cfg.SsoUrl, userId)
	r.logger.Debug("prepared SSO API URL", zap.String("url", existingRolesUrl))

	req, err := http.NewRequest(http.MethodGet, existingRolesUrl, nil)
	if err != nil {
		r.logger.Error("failed to create request", zap.Error(err), zap.String("url", existingRolesUrl))
		return nil, err
	}

	r.logger.Debug("sending HTTP request",
		zap.String("method", http.MethodGet),
		zap.String("url", existingRolesUrl))

	resp, err := r.client.Do(req)
	if err != nil {
		r.logger.Error("HTTP request failed", zap.Error(err), zap.String("url", existingRolesUrl))
		return nil, err
	}
	defer resp.Body.Close()

	r.logger.Debug("received HTTP response", zap.Int("status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		r.logger.Error("failed to read response body", zap.Error(err))
		return nil, ErrReadResponseBody
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		r.logger.Error("HTTP status not OK",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)))
		return nil, errors.Wrap(ErrHTTPStatusNotOK, string(body))
	}
	var roles []httpdto.Role
	err = json.Unmarshal(body, &roles)
	if err != nil {
		r.logger.Error("failed to unmarshal JSON response", zap.Error(err), zap.String("response_body", string(body)))
		return nil, ErrUnmarshalJSON
	}

	r.logger.Info("successfully retrieved user roles",
		zap.String("user_id", userId),
		zap.Int("roles_count", len(roles)))
	return roles, nil
}

func (r *Repository) GetRoles(ctx context.Context) ([]httpdto.Role, error) {
	r.logger.Info("getting all roles")

	getRolesUrl := fmt.Sprintf("%s/api/roles", r.cfg.SsoUrl)
	r.logger.Debug("prepared SSO API URL", zap.String("url", getRolesUrl))

	req, err := http.NewRequest(http.MethodGet, getRolesUrl, nil)
	if err != nil {
		r.logger.Error("failed to create request", zap.Error(err), zap.String("url", getRolesUrl))
		return nil, err
	}

	r.logger.Debug("sending HTTP request",
		zap.String("method", http.MethodGet),
		zap.String("url", getRolesUrl))

	resp, err := r.client.Do(req)
	if err != nil {
		r.logger.Error("HTTP request failed", zap.Error(err), zap.String("url", getRolesUrl))
		return nil, err
	}
	defer resp.Body.Close()

	r.logger.Debug("received HTTP response", zap.Int("status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		r.logger.Error("failed to read response body", zap.Error(err))
		return nil, ErrReadResponseBody
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		r.logger.Error("HTTP status not OK",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)))
		return nil, errors.Wrap(ErrHTTPStatusNotOK, string(body))
	}
	var roles []httpdto.Role
	err = json.Unmarshal(body, &roles)
	if err != nil {
		r.logger.Error("failed to unmarshal JSON response", zap.Error(err), zap.String("response_body", string(body)))
		return nil, ErrUnmarshalJSON
	}

	r.logger.Info("successfully retrieved all roles", zap.Int("roles_count", len(roles)))
	return roles, nil
}

func (r *Repository) UpdateUserRoles(ctx context.Context, userId string, roleIds []string) error {
	r.logger.Info("updating user roles",
		zap.String("user_id", userId),
		zap.Strings("role_ids", roleIds))

	updateRolesUrl := fmt.Sprintf("%s/api/users/%s/roles", r.cfg.SsoUrl, userId)
	r.logger.Debug("prepared SSO API URL", zap.String("url", updateRolesUrl))

	rBody := struct {
		RoleIds []string `json:"roleIds"`
	}{
		RoleIds: roleIds,
	}

	encoded, err := json.Marshal(rBody)
	if err != nil {
		r.logger.Error("failed to marshal JSON payload", zap.Error(err))
		return ErrMarshalJSON
	}
	r.logger.Debug("encoded request payload", zap.String("payload", string(encoded)))

	req, err := http.NewRequest(http.MethodPut, updateRolesUrl, bytes.NewBuffer(encoded))
	if err != nil {
		r.logger.Error("failed to create request", zap.Error(err), zap.String("url", updateRolesUrl))
		return ErrFailedCreateReq
	}

	req.Header.Set("Content-Type", "application/json")
	r.logger.Debug("sending HTTP request",
		zap.String("method", http.MethodPut),
		zap.String("url", updateRolesUrl))

	resp, err := r.client.Do(req)
	if err != nil {
		r.logger.Error("HTTP request failed", zap.Error(err), zap.String("url", updateRolesUrl))
		return ErrHTTPRequestFailed
	}
	defer resp.Body.Close()

	r.logger.Debug("received HTTP response", zap.Int("status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		r.logger.Error("failed to read response body", zap.Error(err))
		return ErrReadResponseBody
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		r.logger.Error("HTTP status not OK",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)))
		return errors.Wrap(ErrHTTPStatusNotOK, string(body))
	}

	r.logger.Info("successfully updated user roles",
		zap.String("user_id", userId),
		zap.Strings("role_ids", roleIds))
	return nil
}

func (r *Repository) UpdateUserProfileNick(ctx context.Context, userID, nickname string) error {
	r.logger.Info("updating user profile nickname",
		zap.String("user_id", userID),
		zap.String("nickname", nickname))

	url := fmt.Sprintf("%s/api/users/%s", r.cfg.SsoUrl, userID)
	r.logger.Debug("prepared SSO API URL", zap.String("url", url))

	v := struct {
		Profile struct {
			Nickname string `json:"nickname"`
		} `json:"profile"`
	}{
		Profile: struct {
			Nickname string `json:"nickname"`
		}{
			Nickname: nickname,
		},
	}

	encoded, err := json.Marshal(v)
	if err != nil {
		r.logger.Error("failed to marshal JSON payload", zap.Error(err))
		return ErrMarshalJSON
	}
	r.logger.Debug("encoded request payload", zap.String("payload", string(encoded)))

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(encoded))
	if err != nil {
		r.logger.Error("failed to create request", zap.Error(err), zap.String("url", url))
		return ErrFailedCreateReq
	}

	req.Header.Set("Content-Type", "application/json")
	r.logger.Debug("sending HTTP request",
		zap.String("method", http.MethodPatch),
		zap.String("url", url))

	resp, err := r.client.Do(req)
	if err != nil {
		r.logger.Error("HTTP request failed", zap.Error(err), zap.String("url", url))
		return ErrHTTPRequestFailed
	}
	defer resp.Body.Close()

	r.logger.Debug("received HTTP response", zap.Int("status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		r.logger.Error("failed to read response body", zap.Error(err))
		return ErrReadResponseBody
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		r.logger.Error("HTTP status not OK",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)))
		return errors.Wrap(ErrHTTPStatusNotOK, string(body))
	}

	r.logger.Info("successfully updated user profile nickname",
		zap.String("user_id", userID),
		zap.String("nickname", nickname))
	return nil
}
