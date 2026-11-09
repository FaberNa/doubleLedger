package domain

import (
	"errors"
	"math"
)

// Amount is denominated in the configured currency's smallest unit (EUR cents).
// Balances can be negative; posting validation requires positive amounts.
type Amount int64

var (
	ErrInvalidArgument = errors.New("invalid domain argument")
	ErrOverflow        = errors.New("amount overflow")
	ErrUnbalanced      = errors.New("journal entry is not balanced")
)

func (a Amount) Add(b Amount) (Amount, error) {
	if (b > 0 && a > Amount(math.MaxInt64)-b) || (b < 0 && a < Amount(math.MinInt64)-b) {
		return 0, ErrOverflow
	}
	return a + b, nil
}

func (a Amount) Sub(b Amount) (Amount, error) {
	if (b > 0 && a < Amount(math.MinInt64)+b) || (b < 0 && a > Amount(math.MaxInt64)+b) {
		return 0, ErrOverflow
	}
	return a - b, nil
}
