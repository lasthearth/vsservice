package tokenmanager

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// TestAccessTokenSingleFlight pins two properties that were both broken when
// RoundTrip read m.token outside the lock: the concurrent reads must be
// race-free (run with -race), and N goroutines observing a missing token must
// produce exactly one grant, not N.
func TestAccessTokenSingleFlight(t *testing.T) {
	var grants atomic.Int64

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		grants.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok",
			"expires_in":   3600,
		})
	}))
	defer tokenSrv.Close()

	m := NewManager(tokenSrv.Client(), Config{TokenUrl: tokenSrv.URL})

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			tok, err := m.accessToken()
			if err != nil {
				t.Errorf("accessToken: %v", err)
				return
			}
			if tok != "tok" {
				t.Errorf("want token %q, got %q", "tok", tok)
			}
		}()
	}
	wg.Wait()

	if got := grants.Load(); got != 1 {
		t.Errorf("want exactly 1 token grant, got %d", got)
	}
}

// TestRoundTripSetsSingleAuthHeader covers the retry case: Add appended a second
// Authorization header on every replay of the same request.
func TestRoundTripSetsSingleAuthHeader(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := len(r.Header.Values("Authorization")); got != 1 {
			t.Errorf("want 1 Authorization header, got %d: %v", got, r.Header.Values("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	m := NewManager(tokenSrv.Client(), Config{TokenUrl: tokenSrv.URL})

	req, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	// Two round trips on the same *http.Request, as retryablehttp does.
	for i := range 2 {
		resp, err := m.RoundTrip(req)
		if err != nil {
			t.Fatalf("round trip %d: %v", i, err)
		}
		_ = resp.Body.Close()
	}
}
