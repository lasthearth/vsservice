package sso

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-faster/errors"
	httpdto "github.com/lasthearth/vsservice/internal/player/internal/dto/http"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

func (r *Repository) UpdateUserAvatar(ctx context.Context, userID, avatar string) error {
	l := r.logger.
		WithMethod("update_user_avatar").
		With(
			zap.String("user_id", userID),
			zap.String("avatar", avatar),
		)
	l.Info("updating user profile avatar")

	url := fmt.Sprintf("%s/api/users/%s", r.cfg.SsoUrl, userID)
	l.Debug("prepared SSO API URL", zap.String("url", url))

	v := struct {
		Avatar string `json:"avatar"`
	}{
		Avatar: avatar,
	}

	encoded, err := json.Marshal(v)
	if err != nil {
		l.Error("failed to marshal JSON payload", zap.Error(err))
		return ErrMarshalJSON
	}
	l.Debug("encoded request payload", zap.String("payload", string(encoded)))

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(encoded))
	if err != nil {
		l.Error("failed to create request", zap.Error(err), zap.String("url", url))
		return ErrFailedCreateReq
	}

	req.Header.Set("Content-Type", "application/json")
	l.Debug("sending HTTP request",
		zap.String("method", http.MethodPatch),
		zap.String("url", url))

	resp, err := r.client.Do(req)
	if err != nil {
		l.Error("HTTP request failed", zap.Error(err), zap.String("url", url))
		return ErrHTTPRequestFailed
	}
	defer resp.Body.Close()

	l.Debug("received HTTP response", zap.Int("status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		l.Error("failed to read response body", zap.Error(err))
		return ErrReadResponseBody
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		l.Error("HTTP status not OK",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)))
		return errors.Wrap(ErrHTTPStatusNotOK, string(body))
	}

	l.Info("successfully updated user profile nickname")
	return nil
}

func (r *Repository) getRoles(ctx context.Context) ([]httpdto.Role, error) {
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

// GetAdminUsers retrieves user IDs of users with admin roles from Logto
func (r *Repository) GetAdminUsers(ctx context.Context) ([]string, error) {
	l := r.logger.
		WithMethod("get_logto_admin_users")
	l.Info("fetching admin users from logto")

	roles, err := r.getRoles(ctx)
	if err != nil {
		l.Error("failed to retrieve roles", zap.Error(err))
		return nil, err
	}

	role, finded := lo.Find(roles, func(item httpdto.Role) bool {
		return item.Name == "admin"
	})
	if !finded {
		l.Error("failed to find admin role")
		return nil, err
	}

	url := fmt.Sprintf("%s/api/roles/%s/users", r.cfg.SsoUrl, role.Id)
	l.Debug("prepared Logto API URL", zap.String("url", url))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		l.Error("failed to create request", zap.Error(err), zap.String("url", url))
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	l.Debug("sending HTTP request",
		zap.String("method", http.MethodGet),
		zap.String("url", url))

	resp, err := r.client.Do(req)
	if err != nil {
		l.Error("HTTP request failed", zap.Error(err), zap.String("url", url))
		return nil, err
	}
	defer resp.Body.Close()

	l.Debug("received HTTP response", zap.Int("status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		l.Error("failed to read response body", zap.Error(err))
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		l.Error("HTTP status not OK",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)))
		return nil, errors.Wrap(ErrHTTPStatusNotOK, string(body))
	}

	var users []httpdto.User
	if err := json.Unmarshal(body, &users); err != nil {
		l.Error("failed to unmarshal response", zap.Error(err))
		return nil, err
	}

	ids := lo.Map(users, func(item httpdto.User, index int) string {
		return item.Id
	})

	l.Info("successfully retrieved admin users", zap.Int("count", len(ids)))
	return ids, nil
}
