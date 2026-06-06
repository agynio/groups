package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	authorizationv1 "github.com/agynio/groups/.gen/go/agynio/api/authorization/v1"
	groupsv1 "github.com/agynio/groups/.gen/go/agynio/api/groups/v1"
	identityv1 "github.com/agynio/groups/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/groups/internal/config"
	"github.com/agynio/groups/internal/db"
	"github.com/agynio/groups/internal/events"
	"github.com/agynio/groups/internal/server"
	"github.com/agynio/groups/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("groups-service: %v", err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("create connection pool: %w", err)
	}
	defer pool.Close()

	if err := db.ApplyMigrations(ctx, pool); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	authConn, err := grpc.NewClient(cfg.AuthorizationGRPCTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to authorization: %w", err)
	}
	defer authConn.Close()

	identityConn, err := grpc.NewClient(cfg.IdentityGRPCTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to identity: %w", err)
	}
	defer identityConn.Close()

	natsConn, err := events.Connect(cfg.NATSURL)
	if err != nil {
		return err
	}
	defer natsConn.Close()
	publisher, err := events.NewNATSJetStreamPublisher(natsConn)
	if err != nil {
		return err
	}

	grpcServer := grpc.NewServer()
	serverInstance := server.New(
		store.New(pool),
		authorizationv1.NewAuthorizationServiceClient(authConn),
		identityv1.NewIdentityServiceClient(identityConn),
		publisher,
	)
	groupsv1.RegisterGroupsServiceServer(grpcServer, serverInstance)

	lis, err := net.Listen("tcp", cfg.GRPCAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.GRPCAddress, err)
	}

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	log.Printf("GroupsService listening on %s", cfg.GRPCAddress)

	if err := grpcServer.Serve(lis); err != nil {
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
