package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LogLevel slog.Level `yaml:"log_level"`

	HTTPAddr         string  `yaml:"http_addr"`
	InboundRateLimit float64 `yaml:"inbound_rate_limit"`
	InboundBurst     int     `yaml:"inbound_burst"`

	PostgresDSN string `yaml:"postgres_dsn"`

	Workers      int           `yaml:"workers"`
	MaxAttempts  int           `yaml:"max_attempts"`
	RetryBackoff time.Duration `yaml:"retry_backoff"`

	BloomExpectedItems     uint    `yaml:"bloom_expected_items"`
	BloomFalsePositiveRate float64 `yaml:"bloom_false_positive_rate"`

	SweeperInterval time.Duration `yaml:"sweeper_interval"`
	Lease           time.Duration `yaml:"lease"`

	LLMLatency         time.Duration `yaml:"llm_latency"`
	LLMTokensPerSecond float64       `yaml:"llm_tokens_per_second"`
	LLMBurst           int           `yaml:"llm_burst"`

	BreakerFailureThreshold int           `yaml:"breaker_failure_threshold"`
	BreakerOpenTimeout      time.Duration `yaml:"breaker_open_timeout"`

	WebhookURL     string        `yaml:"webhook_url"`
	WebhookTimeout time.Duration `yaml:"webhook_timeout"`
}

func defaults() Config {
	return Config{
		LogLevel:                slog.LevelInfo,
		HTTPAddr:                ":8080",
		InboundRateLimit:        100,
		InboundBurst:            200,
		Workers:                 8,
		MaxAttempts:             5,
		RetryBackoff:            5 * time.Second,
		BloomExpectedItems:      1_000_000,
		BloomFalsePositiveRate:  0.01,
		SweeperInterval:         10 * time.Second,
		Lease:                   2 * time.Minute,
		LLMLatency:              300 * time.Millisecond,
		LLMTokensPerSecond:      5,
		LLMBurst:                10,
		BreakerFailureThreshold: 5,
		BreakerOpenTimeout:      30 * time.Second,
		WebhookTimeout:          5 * time.Second,
	}
}

func New(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if dsn := os.Getenv("POSTGRES_DSN"); dsn != "" {
		cfg.PostgresDSN = dsn
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	switch {
	case c.PostgresDSN == "":
		return errors.New("postgres_dsn is required")
	case c.Workers < 1:
		return errors.New("workers must be >= 1")
	case c.MaxAttempts < 1 || c.MaxAttempts > 20:
		return errors.New("max_attempts must be in [1, 20]")
	case c.SweeperInterval <= 0 || c.Lease <= 0 || c.RetryBackoff <= 0 || c.BreakerOpenTimeout <= 0 || c.WebhookTimeout <= 0:
		return errors.New("sweeper_interval, lease, retry_backoff, breaker_open_timeout and webhook_timeout must be > 0")
	case c.InboundRateLimit <= 0 || c.LLMTokensPerSecond <= 0:
		return errors.New("inbound_rate_limit and llm_tokens_per_second must be > 0")
	case c.InboundBurst < 1 || c.LLMBurst < 1:
		return errors.New("inbound_burst and llm_burst must be >= 1")
	case c.BreakerFailureThreshold < 1:
		return errors.New("breaker_failure_threshold must be >= 1")
	case c.BloomExpectedItems < 1 || c.BloomFalsePositiveRate <= 0 || c.BloomFalsePositiveRate >= 1:
		return errors.New("bloom_expected_items must be >= 1 and bloom_false_positive_rate in (0, 1)")
	}
	return nil
}
