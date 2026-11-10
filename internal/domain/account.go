package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxAccountCodeLength = 64
	MaxAccountNameLength = 255
)

type AccountType string

const (
	Asset     AccountType = "asset"
	Liability AccountType = "liability"
	Equity    AccountType = "equity"
	Expense   AccountType = "expense"
	Revenue   AccountType = "revenue"
)

func (t AccountType) NormalSide() (Side, error) {
	switch t {
	case Asset, Expense:
		return Debit, nil
	case Liability, Equity, Revenue:
		return Credit, nil
	default:
		return "", fmt.Errorf("%w: unknown account type %q", ErrInvalidArgument, t)
	}
}

// Account has no mutation methods. IDs and timestamps are supplied by the caller;
// uniqueness and account existence are enforced by persistence.
type Account struct {
	id          string
	code        string
	name        string
	accountType AccountType
	createdAt   time.Time
}

func NewAccount(id, code, name string, accountType AccountType, createdAt time.Time) (Account, error) {
	if strings.TrimSpace(id) == "" || createdAt.IsZero() {
		return Account{}, fmt.Errorf("%w: account ID and creation timestamp are required", ErrInvalidArgument)
	}
	if len(code) == 0 || len(code) > MaxAccountCodeLength {
		return Account{}, fmt.Errorf("%w: account code must contain 1 to %d ASCII characters", ErrInvalidArgument, MaxAccountCodeLength)
	}
	for _, c := range code {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return Account{}, fmt.Errorf("%w: invalid account code character", ErrInvalidArgument)
		}
	}
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || name == "" || utf8.RuneCountInString(name) > MaxAccountNameLength {
		return Account{}, fmt.Errorf("%w: account name must contain 1 to %d Unicode characters", ErrInvalidArgument, MaxAccountNameLength)
	}
	if _, err := accountType.NormalSide(); err != nil {
		return Account{}, err
	}
	return Account{id: id, code: strings.ToUpper(code), name: name, accountType: accountType, createdAt: createdAt.UTC()}, nil
}

func (a Account) ID() string           { return a.id }
func (a Account) Code() string         { return a.code }
func (a Account) Name() string         { return a.name }
func (a Account) Type() AccountType    { return a.accountType }
func (a Account) CreatedAt() time.Time { return a.createdAt }
