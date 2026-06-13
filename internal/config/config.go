package config

import (
	"fmt"
	"os"
)

type Config struct {
	GRPCAddress             string
	DatabaseURL             string
	AuthorizationGRPCTarget string
	IdentityGRPCTarget      string
	NotificationsGRPCTarget string
	NATSURL                 string
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
	return cfg, nil
}
