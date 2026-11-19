package postgres

import (
	"context"
	"errors"
	"fmt"

	"doubleLedger/internal/application"
	"doubleLedger/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Accounts struct {
	pool *pgxpool.Pool
}

func NewAccounts(pool *pgxpool.Pool) *Accounts {
	return &Accounts{pool: pool}
}

func (a *Accounts) InsertAccount(ctx context.Context, account domain.Account) error {
	_, err := a.pool.Exec(ctx, `
		INSERT INTO ledger.accounts (id, code, name, account_type, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, account.ID(), account.Code(), account.Name(), string(account.Type()), account.CreatedAt())
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "accounts_code_key" {
		return application.ErrAccountCodeExists
	}
	return fmt.Errorf("insert account: %w", err)
}
