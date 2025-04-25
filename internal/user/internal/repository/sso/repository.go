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

func (r *Repository) UpdateUserProfileNick(ctx context.Context, userID, nickname string) error {
	r.logger.Info("updating user profile nickname",
		zap.String("user_id", userID),
		zap.String("nickname", nickname))

	url := fmt.Sprintf("%s/api/users/%s", r.config.SsoUrl, userID)
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
