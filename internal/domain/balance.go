package domain

import "fmt"

// Balance exposes both totals and the balance relative to an account's normal side.
type Balance struct {
	Debits  Amount
	Credits Amount
	Normal  Amount
}

// NewBalance accepts totals from a single consistent persistence query.
func NewBalance(accountType AccountType, debits, credits Amount) (Balance, error) {
	side, err := accountType.NormalSide()
	if err != nil {
		return Balance{}, err
	}
	if debits < 0 || credits < 0 {
		return Balance{}, fmt.Errorf("%w: totals cannot be negative", ErrInvalidArgument)
	}
	var normal Amount
	if side == Debit {
		normal, err = debits.Sub(credits)
	} else {
		normal, err = credits.Sub(debits)
	}
	if err != nil {
		return Balance{}, err
	}
	return Balance{Debits: debits, Credits: credits, Normal: normal}, nil
}
