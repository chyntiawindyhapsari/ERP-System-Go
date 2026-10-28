package config

import (
	"fmt"
	"net/url"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort       string
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	DBSSLMode     string
	AdminUsername string
	AdminPassword string
	AdminFullName string

	IntegrationAPIKey string // kunci untuk n8n/WA membaca data
	WebhookURL        string // tujuan event keluar (webhook n8n)
	WebhookSecret     string
}

func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		AppPort:       getEnv("APP_PORT", "8080"),
		DBHost:        getEnv("DB_HOST", "localhost"),
		DBPort:        getEnv("DB_PORT", "5432"),
		DBUser:        getEnv("DB_USER", "postgres"),
		DBPassword:    getEnv("DB_PASSWORD", "postgres"),
		DBName:        getEnv("DB_NAME", "first_project"),
		DBSSLMode:     getEnv("DB_SSLMODE", "disable"),
		AdminUsername: getEnv("ADMIN_USERNAME", ""),
		AdminPassword: getEnv("ADMIN_PASSWORD", ""),
		AdminFullName: getEnv("ADMIN_FULL_NAME", "Administrator"),

		IntegrationAPIKey: getEnv("INTEGRATION_API_KEY", ""),
		WebhookURL:        getEnv("SALE_WEBHOOK_URL", ""),
		WebhookSecret:     getEnv("SALE_WEBHOOK_SECRET", ""),
	}
}

func (c *Config) DatabaseURL() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.DBUser, c.DBPassword),
		Host:     fmt.Sprintf("%s:%s", c.DBHost, c.DBPort),
		Path:     c.DBName,
		RawQuery: "sslmode=" + c.DBSSLMode,
	}
	return u.String()
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
