package grpc_test

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	ledgerv1 "doubleLedger/api/gen/ledger/v1"
	"doubleLedger/internal/application"
	ledgergrpc "doubleLedger/internal/grpc"
	"doubleLedger/internal/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

func postgresTestClient(t *testing.T) (context.Context, *pgxpool.Pool, ledgerv1.LedgerServiceClient) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a migrated development database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	listener := bufconn.Listen(64 * 1024)
	server := grpc.NewServer()
	ledgerv1.RegisterLedgerServiceServer(server, ledgergrpc.NewServer(
		application.NewAccounts(postgres.NewAccounts(pool)),
		application.NewJournal(postgres.NewJournal(pool)),
	))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///ledger-test",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := ledgerv1.NewLedgerServiceClient(conn)
	return ctx, pool, client
}

func TestAccountsPostgres(t *testing.T) {
	ctx, pool, client := postgresTestClient(t)
	const asset = ledgerv1.AccountType_ACCOUNT_TYPE_ASSET

	t.Run("reads an existing account by UUID", func(t *testing.T) {
		created, err := client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{
			Code: testAccountCode(t, pool),
			Name: "Sales",
			Type: ledgerv1.AccountType_ACCOUNT_TYPE_REVENUE,
		})
		if err != nil {
			t.Fatal(err)
		}
		account := created.GetAccount()
		response, err := client.GetAccount(ctx, &ledgerv1.GetAccountRequest{AccountId: strings.ToUpper(account.GetId())})
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(response.GetAccount(), account) {
			t.Fatalf("GetAccount returned %v, want %v", response.GetAccount(), account)
		}
	})

	t.Run("rejects invalid and missing account IDs", func(t *testing.T) {
		for _, tc := range []struct {
			id   string
			want codes.Code
		}{
			{"", codes.InvalidArgument},
			{"invalid", codes.InvalidArgument},
			{strings.ReplaceAll(uuid.NewString(), "-", ""), codes.InvalidArgument},
			{uuid.NewString(), codes.NotFound},
		} {
			_, err := client.GetAccount(ctx, &ledgerv1.GetAccountRequest{AccountId: tc.id})
			if status.Code(err) != tc.want {
				t.Errorf("GetAccount(%q) returned %v, want %v", tc.id, err, tc.want)
			}
		}
	})

	t.Run("creates and persists the normalized account", func(t *testing.T) {
		code := testAccountCode(t, pool)
		response, err := client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{Code: code, Name: " Cash ", Type: asset})
		if err != nil {
			t.Fatal(err)
		}
		account := response.GetAccount()
		if _, err := uuid.Parse(account.GetId()); err != nil {
			t.Fatalf("server ID is not a UUID: %v", err)
		}
		if account.GetCode() != strings.ToUpper(code) || account.GetName() != "Cash" || account.GetType() != asset {
			t.Fatalf("unexpected account: %v", account)
		}
		if account.GetCreatedAt() == nil || !account.GetCreatedAt().IsValid() {
			t.Fatalf("invalid creation timestamp: %v", account.GetCreatedAt())
		}
		var storedCode, storedName, storedType string
		var storedAt time.Time
		err = pool.QueryRow(ctx, "SELECT code, name, account_type, created_at FROM ledger.accounts WHERE id = $1", account.GetId()).
			Scan(&storedCode, &storedName, &storedType, &storedAt)
		if err != nil {
			t.Fatal(err)
		}
		if storedCode != account.GetCode() || storedName != account.GetName() || storedType != "asset" || !storedAt.Equal(account.GetCreatedAt().AsTime()) {
			t.Fatalf("stored account differs from response: %s %s %s %s", storedCode, storedName, storedType, storedAt)
		}
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		code := testAccountCode(t, pool)
		for _, request := range []*ledgerv1.CreateAccountRequest{
			{Code: code + " ", Name: "Cash", Type: asset},
			{Code: code, Name: "  ", Type: asset},
			{Code: code, Name: "Cash\x00", Type: asset},
			{Code: code, Name: "Cash"},
			{Code: code, Name: "Cash", Type: ledgerv1.AccountType(99)},
		} {
			if _, err := client.CreateAccount(ctx, request); status.Code(err) != codes.InvalidArgument {
				t.Errorf("CreateAccount(%v) returned %v, want InvalidArgument", request, err)
			}
		}
	})

	t.Run("concurrent equivalent codes produce one account", func(t *testing.T) {
		code := testAccountCode(t, pool)
		results := make(chan codes.Code, 2)
		for _, variant := range []string{code, strings.ToUpper(code)} {
			go func() {
				_, err := client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{Code: variant, Name: "Cash", Type: asset})
				results <- status.Code(err)
			}()
		}
		first, second := <-results, <-results
		if !((first == codes.OK && second == codes.AlreadyExists) || (second == codes.OK && first == codes.AlreadyExists)) {
			t.Fatalf("RPC statuses = %v, %v, want one OK and one AlreadyExists", first, second)
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM ledger.accounts WHERE code = $1", strings.ToUpper(code)).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("stored account count = %d, want 1", count)
		}
	})
}

func testAccountCode(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	code := "test-" + uuid.NewString()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DELETE FROM ledger.accounts WHERE code = $1", strings.ToUpper(code)); err != nil {
			t.Errorf("delete test account: %v", err)
		}
	})
	return code
}
