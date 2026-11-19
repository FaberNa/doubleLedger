package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	ledgerv1 "doubleLedger/api/gen/ledger/v1"
	"doubleLedger/internal/application"
	ledgergrpc "doubleLedger/internal/grpc"
	"doubleLedger/internal/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	address := os.Getenv("GRPC_ADDR")
	if address == "" {
		address = "127.0.0.1:50051"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("configure PostgreSQL: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen for gRPC: %w", err)
	}
	defer listener.Close()

	server := grpc.NewServer()
	ledgerv1.RegisterLedgerServiceServer(server, ledgergrpc.NewServer(application.NewAccounts(postgres.NewAccounts(pool))))
	log.Printf("DoubleLedger listening on %s", listener.Addr())
	return server.Serve(listener)
}
