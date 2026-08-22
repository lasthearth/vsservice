package tokenmanager

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct {
	ClientID     string
	ClientSecret string
	TokenUrl     string
	Resource     string
	Scopes       []string
}

type Manager struct {
	client   *http.Client
	config   Config
	token    *token
	tokenIat time.Time

	rw sync.RWMutex
}

func NewManager(client *http.Client, config Config) *Manager {
	return &Manager{
		client: client,
		config: config,
	}
}

func (m *Manager) Client(ctx context.Context) *http.Client {
	return &http.Client{
		Transport:     m,
		CheckRedirect: m.client.CheckRedirect,
		Jar:           m.client.Jar,
		Timeout:       m.client.Timeout,
	}
}

// accessToken returns a currently-valid access token, fetching a new one when
// the cached one is missing or expired.
//
// Every read of m.token and m.tokenIat happens under a lock. The fast path
// takes the read lock and copies the token string out, so the caller never
// dereferences m.token while getTokenLocked is replacing it.
func (m *Manager) accessToken() (string, error) {
	m.rw.RLock()
	if tok, ok := m.validTokenLocked(); ok {
		m.rw.RUnlock()
		return tok, nil
	}
	m.rw.RUnlock()

	m.rw.Lock()
	defer m.rw.Unlock()

	// Re-check under the write lock: while this goroutine waited for it, a peer
	// may already have refreshed. Without this, every goroutine that observed
	// the same expiry performs its own client_credentials grant.
	if tok, ok := m.validTokenLocked(); ok {
		return tok, nil
	}

	if err := m.getTokenLocked(); err != nil {
		return "", err
	}
	return m.token.AccessToken, nil
}

// validTokenLocked reports whether the cached token is usable. The caller must
// hold either the read or the write lock.
func (m *Manager) validTokenLocked() (string, bool) {
	if m.token == nil {
		return "", false
	}
	expires := m.tokenIat.Add(time.Duration(m.token.ExpiresIn) * time.Second)
	if !time.Now().Before(expires) {
		return "", false
	}
	return m.token.AccessToken, true
}

// getTokenLocked fetches a fresh token. The caller must hold the write lock.
func (m *Manager) getTokenLocked() error {
	v := url.Values{
		"grant_type": {"client_credentials"},
		"resource":   {m.config.Resource},
		"scope":      {strings.Join(m.config.Scopes, " ")},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, m.config.TokenUrl, strings.NewReader(v.Encode()))
	if err != nil {
		return err
	}

	auth := fmt.Sprintf("%s:%s", m.config.ClientID, m.config.ClientSecret)
	encoded := base64.StdEncoding.EncodeToString([]byte(auth))

	req.Header.Add("Authorization", "Basic "+encoded)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("token generation failed: %s, %s", resp.Status, string(body))
	}

	var token token
	err = json.Unmarshal(body, &token)
	if err != nil {
		return err
	}

	m.token = &token
	m.tokenIat = time.Now()

	return nil
}
