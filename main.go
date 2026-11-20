package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	ledgerv1 "doubleLedger/api/gen/ledger/v1"
	"doubleLedger/internal/application"
	ledgergrpc "doubleLedger/internal/grpc"
	"doubleLedger/internal/postgres"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	metricsAddress := os.Getenv("METRICS_ADDR")
	if metricsAddress == "" {
		metricsAddress = "127.0.0.1:9091"
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

	metricsListener, err := net.Listen("tcp", metricsAddress)
	if err != nil {
		return fmt.Errorf("listen for metrics: %w", err)
	}
	defer metricsListener.Close()

	rpcMetrics := grpcprom.NewServerMetrics(grpcprom.WithServerHandlingTimeHistogram())
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector(), rpcMetrics)

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{EnableOpenMetrics: true}))
	metricsServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer metricsServer.Close()

	server := grpc.NewServer(grpc.UnaryInterceptor(rpcMetrics.UnaryServerInterceptor()))
	defer server.Stop()
	ledgerv1.RegisterLedgerServiceServer(server, ledgergrpc.NewServer(
		application.NewAccounts(postgres.NewAccounts(pool)),
		application.NewJournal(postgres.NewJournal(pool)),
	))
	log.Printf("DoubleLedger listening on %s", listener.Addr())
	log.Printf("Metrics listening on http://%s/metrics", metricsListener.Addr())

	metricsErrors := make(chan error, 1)
	go func() { metricsErrors <- metricsServer.Serve(metricsListener) }()
	grpcErrors := make(chan error, 1)
	go func() { grpcErrors <- server.Serve(listener) }()
	select {
	case err := <-metricsErrors:
		return fmt.Errorf("serve metrics: %w", err)
	case err := <-grpcErrors:
		return err
	}
}
