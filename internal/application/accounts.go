package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"doubleLedger/internal/domain"

	"github.com/google/uuid"
)

var ErrAccountCodeExists = errors.New("account code already exists")

type AccountWriter interface {
	InsertAccount(context.Context, domain.Account) error
}

type Accounts struct {
	writer AccountWriter
}

func NewAccounts(writer AccountWriter) *Accounts {
	return &Accounts{writer: writer}
}

func (a *Accounts) Create(ctx context.Context, code, name string, accountType domain.AccountType) (domain.Account, error) {
	// PostgreSQL text cannot store a NUL character.
	if strings.ContainsRune(name, 0) {
		return domain.Account{}, fmt.Errorf("%w: account name contains a NUL character", domain.ErrInvalidArgument)
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return domain.Account{}, fmt.Errorf("generate account ID: %w", err)
	}
	// Match PostgreSQL timestamp precision so the API returns the stored value.
	account, err := domain.NewAccount(id.String(), code, name, accountType, time.Now().Truncate(time.Microsecond))
	if err != nil {
		return domain.Account{}, err
	}
	if err := a.writer.InsertAccount(ctx, account); err != nil {
		return domain.Account{}, err
	}
	return account, nil
}
