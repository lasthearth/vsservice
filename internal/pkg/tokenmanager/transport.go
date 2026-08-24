package tokenmanager

import (
	"net/http"
)

func (m *Manager) RoundTrip(req *http.Request) (*http.Response, error) {
	accessToken, err := m.accessToken()
	if err != nil {
		return nil, err
	}

	// Set, not Add: retryablehttp replays the same *http.Request, so Add would
	// append a second Authorization header carrying the previous token.
	req.Header.Set("Authorization", "Bearer "+accessToken)

	if m.client.Transport != nil {
		return m.client.Transport.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}
