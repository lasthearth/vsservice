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
)

type Config struct {
	ClientID     string
	ClientSecret string
	TokenUrl     string
	Resource     string
	Scopes       []string
}

type Manager struct {
	client *http.Client
	config Config
	token  *token

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

func (m *Manager) getToken() error {
	m.rw.Lock()
	defer m.rw.Unlock()

	v := url.Values{
		"grant_type": {"client_credentials"},
		"resource":   {m.config.Resource},
		"scope":      {strings.Join(m.config.Scopes, " ")},
	}

	req, err := http.NewRequest(http.MethodPost, m.config.TokenUrl, strings.NewReader(v.Encode()))
	if err != nil {
		return err
	}

	auth := fmt.Sprintf("%s:%s", m.config.ClientID, m.config.ClientSecret)
	encoded := base64.StdEncoding.EncodeToString([]byte(auth))

	req.Header.Add("Authorization", fmt.Sprintf("Basic %s", encoded))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

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

	return nil
}
