package config

import (
	"os"
	"strconv"
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
}

func Load() Config {
	return Config{
		AppPort:              envOrDefault("APP_PORT", "8080"),
		VendorStubPort:       envOrDefault("VENDOR_STUB_PORT", "8081"),
		VendorStubURL:        envOrDefault("VENDOR_STUB_URL", "http://localhost:8081"),
		DBPath:               envOrDefault("DB_PATH", "./data/sqlite/mcd.db"),
		ImageStoragePath:     envOrDefault("IMAGE_STORAGE_PATH", "./data/images"),
		SettlementOutputPath: envOrDefault("SETTLEMENT_OUTPUT_PATH", "./reports/settlement"),
		LogLevel:             envOrDefault("LOG_LEVEL", "info"),
		Timezone:             envOrDefault("TIMEZONE", "America/Chicago"),
		EODCutoffHour:        envOrDefaultInt("EOD_CUTOFF_HOUR", 18),
		EODCutoffMinute:      envOrDefaultInt("EOD_CUTOFF_MINUTE", 30),
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envOrDefaultInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}
