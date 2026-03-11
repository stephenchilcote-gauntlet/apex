package config

import (
	"strings"
	"testing"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func validEnv() map[string]string {
	return map[string]string{
		"APP_PORT":               "8080",
		"VENDOR_STUB_PORT":       "8081",
		"VENDOR_STUB_URL":        "http://localhost:8081",
		"DB_PATH":                "./data/mcd.db",
		"IMAGE_STORAGE_PATH":     "./data/images",
		"SETTLEMENT_OUTPUT_PATH": "./reports/settlement",
		"LOG_LEVEL":              "info",
		"TIMEZONE":               "America/Chicago",
		"EOD_CUTOFF_HOUR":        "18",
		"EOD_CUTOFF_MINUTE":      "30",
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	setEnv(t, validEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AppPort != "8080" {
		t.Errorf("AppPort = %q, want 8080", cfg.AppPort)
	}
	if cfg.EODCutoffHour != 18 {
		t.Errorf("EODCutoffHour = %d, want 18", cfg.EODCutoffHour)
	}
	if cfg.EODCutoffMinute != 30 {
		t.Errorf("EODCutoffMinute = %d, want 30", cfg.EODCutoffMinute)
	}
	if cfg.Timezone != "America/Chicago" {
		t.Errorf("Timezone = %q, want America/Chicago", cfg.Timezone)
	}
}

func TestLoad_MissingRequiredField(t *testing.T) {
	env := validEnv()
	delete(env, "APP_PORT")
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing APP_PORT, got nil")
	}
}

func TestLoad_InvalidEODCutoffHour(t *testing.T) {
	env := validEnv()
	env["EOD_CUTOFF_HOUR"] = "not-a-number"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for non-integer EOD_CUTOFF_HOUR, got nil")
	}
}

func TestLoad_OptionalFieldsDefaultWhenAbsent(t *testing.T) {
	setEnv(t, validEnv())
	// Ensure optional vars are unset
	t.Setenv("API_KEY", "")
	t.Setenv("UI_USERNAME", "")
	t.Setenv("RATE_LIMIT_RPM", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}
	if cfg.RateLimitRPM != 600 {
		t.Errorf("RateLimitRPM = %d, want 600 (default)", cfg.RateLimitRPM)
	}
}

func TestLoad_RateLimitRPMCustomValue(t *testing.T) {
	env := validEnv()
	env["RATE_LIMIT_RPM"] = "300"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RateLimitRPM != 300 {
		t.Errorf("RateLimitRPM = %d, want 300", cfg.RateLimitRPM)
	}
}

func TestLoad_RateLimitRPMInvalidFallsBackToDefault(t *testing.T) {
	env := validEnv()
	env["RATE_LIMIT_RPM"] = "bad"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RateLimitRPM != 600 {
		t.Errorf("RateLimitRPM = %d, want 600 (default fallback)", cfg.RateLimitRPM)
	}
}

func TestLoad_MultipleErrorsReported(t *testing.T) {
	// Leave out several required fields
	t.Setenv("APP_PORT", "")
	t.Setenv("VENDOR_STUB_PORT", "")
	t.Setenv("VENDOR_STUB_URL", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("IMAGE_STORAGE_PATH", "")
	t.Setenv("SETTLEMENT_OUTPUT_PATH", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("TIMEZONE", "")
	t.Setenv("EOD_CUTOFF_HOUR", "")
	t.Setenv("EOD_CUTOFF_MINUTE", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing required fields, got nil")
	}
	// Error message should mention .env.example
	if !strings.Contains(err.Error(), ".env.example") {
		t.Errorf("error message %q should mention .env.example", err.Error())
	}
}
