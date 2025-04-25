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
		m.rw.RUnlock()
		expires := m.tokenIat.Add(time.Duration(m.token.ExpiresIn) * time.Second)
		if time.Now().After(expires) {
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
