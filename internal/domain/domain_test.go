package domain_test

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"doubleLedger/internal/domain"
)

var now = time.Date(2026, 11, 9, 10, 0, 0, 0, time.FixedZone("local", 3600))

func TestAccountNormalization(t *testing.T) {
	a, err := domain.NewAccount("account", "bank_a-1", "  Banca città  ", domain.Asset, now)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID() != "account" || a.Code() != "BANK_A-1" || a.Name() != "Banca città" || a.Type() != domain.Asset || a.CreatedAt().Location() != time.UTC || !a.CreatedAt().Equal(now) {
		t.Fatalf("unexpected account: %+v", a)
	}
}

func TestAccountValidation(t *testing.T) {
	tests := []struct {
		name, id, code, label string
		kind                  domain.AccountType
		at                    time.Time
		valid                 bool
	}{
		{"maximum Unicode name", "a", strings.Repeat("A", 64), strings.Repeat("界", 255), domain.Asset, now, true},
		{"long name", "a", "A", strings.Repeat("界", 256), domain.Asset, now, false},
		{"blank name", "a", "A", " \t", domain.Asset, now, false},
		{"invalid UTF8", "a", "A", string([]byte{255}), domain.Asset, now, false},
		{"empty code", "a", "", "Bank", domain.Asset, now, false},
		{"long code", "a", strings.Repeat("A", 65), "Bank", domain.Asset, now, false},
		{"space in code", "a", "A B", "Bank", domain.Asset, now, false},
		{"Unicode code", "a", "é", "Bank", domain.Asset, now, false},
		{"unknown type", "a", "A", "Bank", "other", now, false},
		{"empty ID", " ", "A", "Bank", domain.Asset, now, false},
		{"missing timestamp", "a", "A", "Bank", domain.Asset, time.Time{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewAccount(tt.id, tt.code, tt.label, tt.kind, tt.at)
			if tt.valid {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("expected invalid argument, got %v", err)
			}
		})
	}
}

func TestAmountArithmetic(t *testing.T) {
	tests := []struct {
		name     string
		a, b     domain.Amount
		subtract bool
		want     domain.Amount
		overflow bool
	}{
		{"add boundary", math.MaxInt64 - 1, 1, false, math.MaxInt64, false},
		{"add positive overflow", math.MaxInt64, 1, false, 0, true},
		{"add negative overflow", math.MinInt64, -1, false, 0, true},
		{"add lower boundary", math.MinInt64 + 1, -1, false, math.MinInt64, false},
		{"subtract positive overflow", math.MinInt64, 1, true, 0, true},
		{"subtract negative overflow", math.MaxInt64, -1, true, 0, true},
		{"subtract minimum from itself", math.MinInt64, math.MinInt64, true, 0, false},
		{"subtract minimum overflow", 0, math.MinInt64, true, 0, true},
		{"negative balance", 30, 100, true, -70, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got domain.Amount
			var err error
			if tt.subtract {
				got, err = tt.a.Sub(tt.b)
			} else {
				got, err = tt.a.Add(tt.b)
			}
			if tt.overflow {
				if !errors.Is(err, domain.ErrOverflow) {
					t.Fatalf("expected overflow, got %v", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %d, %v; want %d", got, err, tt.want)
			}
		})
	}
}

func transfer() []domain.Posting {
	return []domain.Posting{{ID: "p1", AccountID: "A", Side: domain.Credit, Amount: 10000}, {ID: "p2", AccountID: "B", Side: domain.Debit, Amount: 10000}}
}

func TestJournalEntryProtectsHistory(t *testing.T) {
	postings := transfer()
	e, err := domain.NewJournalEntry("entry", "  Transfer  ", now, postings)
	if err != nil {
		t.Fatal(err)
	}
	postings[0].Amount = 1
	exposed := e.Postings()
	exposed[1].AccountID = "changed"
	if e.ID() != "entry" || e.Description() != "Transfer" || !e.PostedAt().Equal(now) || e.PostedAt().Location() != time.UTC || e.Postings()[0].Amount != 10000 || e.Postings()[1].AccountID != "B" {
		t.Fatalf("entry changed: %+v", e)
	}
}

func TestJournalValidation(t *testing.T) {
	tests := []struct {
		name   string
		change func([]domain.Posting) []domain.Posting
		want   error
	}{
		{"one posting", func(p []domain.Posting) []domain.Posting { return p[:1] }, domain.ErrInvalidArgument},
		{"same account", func(p []domain.Posting) []domain.Posting { p[1].AccountID = "A"; return p }, domain.ErrInvalidArgument},
		{"missing account", func(p []domain.Posting) []domain.Posting { p[0].AccountID = " "; return p }, domain.ErrInvalidArgument},
		{"missing posting ID", func(p []domain.Posting) []domain.Posting { p[0].ID = ""; return p }, domain.ErrInvalidArgument},
		{"duplicate posting ID", func(p []domain.Posting) []domain.Posting { p[1].ID = p[0].ID; return p }, domain.ErrInvalidArgument},
		{"invalid side", func(p []domain.Posting) []domain.Posting { p[0].Side = "other"; return p }, domain.ErrInvalidArgument},
		{"zero amount", func(p []domain.Posting) []domain.Posting { p[0].Amount = 0; return p }, domain.ErrInvalidArgument},
		{"negative amount", func(p []domain.Posting) []domain.Posting { p[0].Amount = -1; return p }, domain.ErrInvalidArgument},
		{"unbalanced", func(p []domain.Posting) []domain.Posting { p[0].Amount--; return p }, domain.ErrUnbalanced},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewJournalEntry("e", "Transfer", now, tt.change(transfer()))
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v; want %v", err, tt.want)
			}
		})
	}
	for _, side := range []domain.Side{domain.Debit, domain.Credit} {
		t.Run("overflow "+string(side), func(t *testing.T) {
			opposite := domain.Debit
			if side == domain.Debit {
				opposite = domain.Credit
			}
			p := []domain.Posting{{ID: "1", AccountID: "A", Side: side, Amount: math.MaxInt64}, {ID: "2", AccountID: "A", Side: side, Amount: 1}, {ID: "3", AccountID: "B", Side: opposite, Amount: math.MaxInt64}, {ID: "4", AccountID: "B", Side: opposite, Amount: 1}}
			_, err := domain.NewJournalEntry("e", "Overflow", now, p)
			if !errors.Is(err, domain.ErrOverflow) {
				t.Fatalf("expected overflow, got %v", err)
			}
		})
	}
}

func TestJournalBoundaries(t *testing.T) {
	p := make([]domain.Posting, 1000)
	for i := range p {
		side := domain.Debit
		account := "A"
		if i%2 == 1 {
			side = domain.Credit
			account = "B"
		}
		p[i] = domain.Posting{ID: fmt.Sprint(i), AccountID: account, Side: side, Amount: 1}
	}
	if _, err := domain.NewJournalEntry("e", strings.Repeat("界", 1000), now, p); err != nil {
		t.Fatal(err)
	}
	p = append(p, domain.Posting{ID: "extra", AccountID: "A", Side: domain.Debit, Amount: 1})
	if _, err := domain.NewJournalEntry("e", "Too many", now, p); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected posting count rejection: %v", err)
	}
	for _, desc := range []string{" \n", strings.Repeat("界", 1001), string([]byte{255})} {
		if _, err := domain.NewJournalEntry("e", desc, now, transfer()); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("expected description rejection: %v", err)
		}
	}
	if _, err := domain.NewJournalEntry(" ", "Transfer", now, transfer()); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ID rejection: %v", err)
	}
	if _, err := domain.NewJournalEntry("e", "Transfer", time.Time{}, transfer()); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected timestamp rejection: %v", err)
	}
	p = transfer()
	p[0].Amount = math.MaxInt64
	p[1].Amount = math.MaxInt64
	if _, err := domain.NewJournalEntry("e", "Maximum", now, p); err != nil {
		t.Fatal(err)
	}
}

func TestBalances(t *testing.T) {
	for _, kind := range []domain.AccountType{domain.Asset, domain.Expense, domain.Liability, domain.Equity, domain.Revenue} {
		for _, totals := range [][2]domain.Amount{{10000, 3000}, {3000, 10000}, {0, 0}, {math.MaxInt64, 0}, {0, math.MaxInt64}} {
			b, err := domain.NewBalance(kind, totals[0], totals[1])
			if err != nil {
				t.Fatal(err)
			}
			want := totals[0] - totals[1]
			if kind != domain.Asset && kind != domain.Expense {
				want = -want
			}
			if b.Debits != totals[0] || b.Credits != totals[1] || b.Normal != want {
				t.Fatalf("%s: got %+v; want %d", kind, b, want)
			}
		}
	}
	for _, totals := range [][2]domain.Amount{{-1, 0}, {0, -1}} {
		if _, err := domain.NewBalance(domain.Asset, totals[0], totals[1]); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("expected invalid totals: %v", err)
		}
	}
	if _, err := domain.NewBalance("unknown", 0, 0); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected invalid type: %v", err)
	}
}
