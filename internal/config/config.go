package config

import (
	"fmt"
	"os"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config is the single source of truth for all runtime configuration.
// Secrets come from Doppler (env vars); everything else from config.yaml.
type Config struct {
	Server   ServerConfig
	Postgres PostgresConfig
	Webhook  WebhookConfig
}

type ServerConfig struct {
	Port                    int
	ReadTimeout             time.Duration
	WriteTimeout            time.Duration
	GracefulShutdownTimeout time.Duration
}

type PostgresConfig struct {
	DSN               string // POSTGRES_DSN — injected by Doppler
	MaxConns          int32
	MinConns          int32
	MaxConnIdle       time.Duration
	HealthCheckPeriod time.Duration
}

type WebhookConfig struct {
	MaxPayloadBytes  int64
	DeliveryTimeout  time.Duration
	MaxRetryAttempts int
	SigningSecret    string // WEBHOOK_SIGNING_SECRET — injected by Doppler
}

// raw mirrors config.yaml exactly. All durations are plain integers so we
type raw struct {
	Server struct {
		Port                    int `yaml:"port"`
		ReadTimeoutSeconds      int `yaml:"read_timeout_seconds"`
		WriteTimeoutSeconds     int `yaml:"write_timeout_seconds"`
		GracefulShutdownSeconds int `yaml:"graceful_shutdown_seconds"`
	} `yaml:"server"`

	Postgres struct {
		MaxConns                 int32 `yaml:"max_conns"`
		MinConns                 int32 `yaml:"min_conns"`
		MaxConnIdleMinutes       int   `yaml:"max_conn_idle_minutes"`
		HealthCheckPeriodSeconds int   `yaml:"health_check_period_seconds"`
	} `yaml:"postgres"`

	Webhook struct {
		MaxPayloadBytes        int64 `yaml:"max_payload_bytes"`
		DeliveryTimeoutSeconds int   `yaml:"delivery_timeout_seconds"`
		MaxRetryAttempts       int   `yaml:"max_retry_attempts"`
	} `yaml:"webhook"`
}

// Load reads config.yaml from path, merges Doppler-injected env vars,
// converts all durations, validates, and returns an immutable *Config.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer f.Close()

	var r raw
	if err := yaml.NewDecoder(f).Decode(&r); err != nil {
		return nil, fmt.Errorf("config: decode yaml: %w", err)
	}

	cfg := &Config{
		Server: ServerConfig{
			Port:                    r.Server.Port,
			ReadTimeout:             time.Duration(r.Server.ReadTimeoutSeconds) * time.Second,
			WriteTimeout:            time.Duration(r.Server.WriteTimeoutSeconds) * time.Second,
			GracefulShutdownTimeout: time.Duration(r.Server.GracefulShutdownSeconds) * time.Second,
		},
		Postgres: PostgresConfig{
			DSN:               mustEnv("POSTGRES_DSN"),
			MaxConns:          r.Postgres.MaxConns,
			MinConns:          r.Postgres.MinConns,
			MaxConnIdle:       time.Duration(r.Postgres.MaxConnIdleMinutes) * time.Minute,
			HealthCheckPeriod: time.Duration(r.Postgres.HealthCheckPeriodSeconds) * time.Second,
		},
		Webhook: WebhookConfig{
			MaxPayloadBytes:  r.Webhook.MaxPayloadBytes,
			DeliveryTimeout:  time.Duration(r.Webhook.DeliveryTimeoutSeconds) * time.Second,
			MaxRetryAttempts: r.Webhook.MaxRetryAttempts,
			SigningSecret:    mustEnv("WEBHOOK_SIGNING_SECRET"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// mustEnv panics if a required secret is missing. A missing secret at startup
// is a deployment bug — crash loudly rather than surface a nil pointer later.
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("config: required env var %q is not set", key))
	}
	return v
}

func (c *Config) validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("config: invalid server port: %d", c.Server.Port)
	}

	if c.Postgres.MaxConns <= 0 {
		return fmt.Errorf("config: postgres max_conns must be > 0: %d", c.Postgres.MaxConns)
	}

	if c.Postgres.MinConns < 0 {
		return fmt.Errorf("config: postgres min_conns cannot be negative: %d", c.Postgres.MinConns)
	}
	if c.Postgres.MaxConns < c.Postgres.MinConns {
		return fmt.Errorf("config: postgres max_conns (%d) < min_conns (%d)",
			c.Postgres.MaxConns, c.Postgres.MinConns)
	}
	if c.Webhook.MaxRetryAttempts < 0 {
		return fmt.Errorf("config: max_retry_attempts cannot be negative: %d", c.Webhook.MaxRetryAttempts)
	}
	return nil
}
