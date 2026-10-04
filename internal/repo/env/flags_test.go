package env

import (
	"strings"
	"testing"
)

// Every setting has a flag, named from its environment variable, so the two
// can never drift apart: INDRI_LISTEN_PORT is -listen-port.
func TestSettings_EverySettingHasAFlagNamedFromItsEnvVar(t *testing.T) {
	settings := Settings()

	if len(settings) == 0 {
		t.Fatal("no settings found")
	}

	seen := map[string]bool{}

	for _, s := range settings {
		if want := strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s.EnvVar, "INDRI_")), "_", "-"); s.Flag != want {
			t.Errorf("setting %s has flag %q, want %q", s.EnvVar, s.Flag, want)
		}

		if seen[s.Flag] {
			t.Errorf("two settings share the flag %q", s.Flag)
		}

		seen[s.Flag] = true
	}

	for _, want := range []string{"listen-port", "allowed-origins", "db-backend", "rtc-udp-port"} {
		if !seen[want] {
			t.Errorf("no setting has the flag -%s", want)
		}
	}
}

// Defaults, then the environment, then the command line.
func TestLoad_FlagsOverrideTheEnvironmentWhichOverridesDefaults(t *testing.T) {
	t.Setenv("INDRI_LISTEN_PORT", "6001")
	t.Setenv("INDRI_LISTEN_ADDRESS", "0.0.0.0")

	v, err := Load(map[string]string{"listen-port": "7002", "db-backend": "sqlite"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v.ListenPort != 7002 {
		t.Errorf("ListenPort = %d, want the flag's 7002", v.ListenPort)
	}

	if v.ListenAddress != "0.0.0.0" {
		t.Errorf("ListenAddress = %q, want the environment's 0.0.0.0", v.ListenAddress)
	}

	if v.DBBackend != "sqlite" {
		t.Errorf("DBBackend = %q, want the flag's sqlite", v.DBBackend)
	}

	if v.RedisPort != 6379 {
		t.Errorf("RedisPort = %d, want the default 6379", v.RedisPort)
	}

	if GetEnv() != v {
		t.Error("GetEnv does not return what Load loaded")
	}
}

func TestLoad_RefusesABadOverride(t *testing.T) {
	for name, overrides := range map[string]map[string]string{
		"an unknown flag":          {"no-such-setting": "1"},
		"a number that is not one": {"listen-port": "lots"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(overrides); err == nil {
				t.Fatalf("Load(%v) succeeded", overrides)
			}
		})
	}
}
