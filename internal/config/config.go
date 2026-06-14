package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const defaultReconciliationInterval = time.Minute

type Config struct {
	GRPCAddress             string
	DatabaseURL             string
	AuthorizationGRPCTarget string
	IdentityGRPCTarget      string
	NotificationsGRPCTarget string
	NATSURL                 string
	ReconciliationInterval  time.Duration
}

func FromEnv() (Config, error) {
	cfg := Config{}
	cfg.GRPCAddress = os.Getenv("GRPC_ADDRESS")
	if cfg.GRPCAddress == "" {
		cfg.GRPCAddress = ":50051"
	}
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL must be set")
	}
	cfg.AuthorizationGRPCTarget = os.Getenv("AUTHORIZATION_GRPC_TARGET")
	if cfg.AuthorizationGRPCTarget == "" {
		cfg.AuthorizationGRPCTarget = "authorization:50051"
	}
	cfg.IdentityGRPCTarget = os.Getenv("IDENTITY_GRPC_TARGET")
	if cfg.IdentityGRPCTarget == "" {
		cfg.IdentityGRPCTarget = "identity:50051"
	}
	cfg.NotificationsGRPCTarget = os.Getenv("NOTIFICATIONS_GRPC_TARGET")
	if cfg.NotificationsGRPCTarget == "" {
		cfg.NotificationsGRPCTarget = "notifications:50051"
	}
	cfg.NATSURL = os.Getenv("NATS_URL")
	if cfg.NATSURL == "" {
		cfg.NATSURL = "nats://nats:4222"
	}
	reconciliationInterval, err := durationFromEnv("RECONCILIATION_INTERVAL", defaultReconciliationInterval)
	if err != nil {
		return Config{}, err
	}
	cfg.ReconciliationInterval = reconciliationInterval
	return cfg, nil
}

func durationFromEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err == nil {
		return duration, nil
	}
	seconds, secondsErr := strconv.Atoi(value)
	if secondsErr != nil {
		return 0, fmt.Errorf("%s must be a duration or seconds value", key)
	}
	return time.Duration(seconds) * time.Second, nil
}
