package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppPort              string
	VendorStubPort       string
	VendorStubURL        string
	DBPath               string
	ImageStoragePath     string
	SettlementOutputPath string
	LogLevel             string
	Timezone             string
	EODCutoffHour        int
	EODCutoffMinute      int

	// Security (optional — omitting disables auth enforcement, safe for dev)
	APIKey        string
	UIUsername    string
	UIPassword    string
	SessionSecret string
}

func Load() (Config, error) {
	var errs []string

	eodHour, err := requiredInt("EOD_CUTOFF_HOUR")
	if err != nil {
		errs = append(errs, err.Error())
	}

	eodMinute, err := requiredInt("EOD_CUTOFF_MINUTE")
	if err != nil {
		errs = append(errs, err.Error())
	}

	cfg := Config{
		AppPort:              required("APP_PORT", &errs),
		VendorStubPort:       required("VENDOR_STUB_PORT", &errs),
		VendorStubURL:        required("VENDOR_STUB_URL", &errs),
		DBPath:               required("DB_PATH", &errs),
		ImageStoragePath:     required("IMAGE_STORAGE_PATH", &errs),
		SettlementOutputPath: required("SETTLEMENT_OUTPUT_PATH", &errs),
		LogLevel:             required("LOG_LEVEL", &errs),
		Timezone:             required("TIMEZONE", &errs),
		EODCutoffHour:        eodHour,
		EODCutoffMinute:      eodMinute,
		APIKey:               os.Getenv("API_KEY"),
		UIUsername:           os.Getenv("UI_USERNAME"),
		UIPassword:           os.Getenv("UI_PASSWORD"),
		SessionSecret:        os.Getenv("SESSION_SECRET"),
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("configuration errors:\n  %s\nSee .env.example for required variables", strings.Join(errs, "\n  "))
	}

	return cfg, nil
}

func required(key string, errs *[]string) string {
	v := os.Getenv(key)
	if v == "" {
		*errs = append(*errs, fmt.Sprintf("%s: required but not set", key))
	}
	return v
}

func requiredInt(key string) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return 0, fmt.Errorf("%s: required but not set", key)
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a valid integer", key, v)
	}
	return i, nil
}
