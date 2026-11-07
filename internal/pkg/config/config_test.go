package config

import (
	"os"
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// unsetEnv removes a variable for the duration of a test. t.Setenv cannot
// express "absent", and absent is exactly the case under test: envconfig only
// applies a `default` tag when the key is not in the environment at all, so an
// inherited value from the developer's shell would silently pass the test.
func unsetEnv(t *testing.T, key string) {
	t.Helper()

	prev, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if had {
			if err := os.Setenv(key, prev); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	})
}

// setRequiredEnv satisfies the variables Process rejects when they are absent.
// None of them is under test here; they only have to be present for Process to
// reach the sweeper knobs at all.
func setRequiredEnv(t *testing.T) {
	t.Helper()

	t.Setenv("ISSUER", "https://issuer.test")
	t.Setenv("AUDIENCE", "audience.test")
	t.Setenv("MONGO_URL_FILE", os.DevNull)
	t.Setenv("DISCORD_BOT_TOKEN", "token")
	t.Setenv("DISCORD_ALLOWED_CHANNELS", "1")
}

// processedConfig is envconfig.Process over a zero Config. Process rather than
// New: New also loads .env from the working directory, which would put the
// developer's own deployment values back over the defaults under test.
func processedConfig(t *testing.T) Config {
	t.Helper()

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		t.Fatal(err)
	}

	return cfg
}

// TestSweeperDefaults pins the three knobs the presence and serverinfo sweepers
// run on when the environment says nothing.
//
// The values are the rollout ones. 60 s sits well above the game's 15 s
// heartbeat, so a live player is never taken offline between two beats, and
// 30 s sits above the game's 5 s serverinfo write, so a live server's online
// count is never zeroed underneath it.
func TestSweeperDefaults(t *testing.T) {
	for _, key := range []string{
		"PRESENCE_SWEEP_INTERVAL",
		"PRESENCE_STALE_AFTER",
		"SERVERINFO_STALE_AFTER",
	} {
		unsetEnv(t, key)
	}
	setRequiredEnv(t)

	cfg := processedConfig(t)

	cases := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"PRESENCE_SWEEP_INTERVAL", cfg.PresenceSweepInterval, time.Minute},
		{"PRESENCE_STALE_AFTER", cfg.PresenceStaleAfter, time.Minute},
		{"SERVERINFO_STALE_AFTER", cfg.ServerInfoStaleAfter, 30 * time.Second},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestSweeperKnobsParseADuration pins the type: an interval expressed as a bare
// number of seconds in the environment would be read as nanoseconds, and the
// sweepers would then run thousands of times a second.
func TestSweeperKnobsParseADuration(t *testing.T) {
	t.Setenv("PRESENCE_SWEEP_INTERVAL", "45s")
	t.Setenv("PRESENCE_STALE_AFTER", "2m")
	t.Setenv("SERVERINFO_STALE_AFTER", "10s")
	setRequiredEnv(t)

	cfg := processedConfig(t)

	if cfg.PresenceSweepInterval != 45*time.Second {
		t.Errorf("PresenceSweepInterval = %v, want 45s", cfg.PresenceSweepInterval)
	}
	if cfg.PresenceStaleAfter != 2*time.Minute {
		t.Errorf("PresenceStaleAfter = %v, want 2m", cfg.PresenceStaleAfter)
	}
	if cfg.ServerInfoStaleAfter != 10*time.Second {
		t.Errorf("ServerInfoStaleAfter = %v, want 10s", cfg.ServerInfoStaleAfter)
	}
}
