package tokenmanager

import (
	"net/http"
	"time"
)

func (m *Manager) RoundTrip(req *http.Request) (*http.Response, error) {
	m.rw.RLock()
	if m.token == nil {
		m.rw.RUnlock()
		err := m.getToken()
		if err != nil {
			return nil, err
		}
	} else {
		expire := time.Now().Add(-1 * time.Duration(m.token.ExpiresIn) * time.Second)
		m.rw.RUnlock()

		if expire.After(time.Now().Add(-30 * time.Second)) {
			err := m.getToken()
			if err != nil {
				return nil, err
			}
		}
	}
	req.Header.Add("Authorization", "Bearer "+m.token.AccessToken)

	if m.client.Transport != nil {
		return m.client.Transport.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}
