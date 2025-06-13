package sso

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-faster/errors"
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
