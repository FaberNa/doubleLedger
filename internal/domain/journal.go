package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxPostings          = 1000
	MaxDescriptionLength = 1000
)

type Side string

const (
	Debit  Side = "debit"
	Credit Side = "credit"
)

type Posting struct {
	ID        string
	AccountID string
	Side      Side
	Amount    Amount
}

func (p Posting) Validate() error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.AccountID) == "" {
		return fmt.Errorf("%w: posting ID and account ID are required", ErrInvalidArgument)
	}
	if p.Side != Debit && p.Side != Credit {
		return fmt.Errorf("%w: unknown posting side", ErrInvalidArgument)
	}
	if p.Amount <= 0 {
		return fmt.Errorf("%w: posting amount must be positive", ErrInvalidArgument)
	}
	return nil
}

// JournalEntry is an immutable posted entry. Accessors never expose its stored slice.
// The application supplies IDs and the server-assigned posting timestamp.
type JournalEntry struct {
	id          string
	description string
	postedAt    time.Time
	postings    []Posting
}

func NewJournalEntry(id, description string, postedAt time.Time, postings []Posting) (JournalEntry, error) {
	if strings.TrimSpace(id) == "" || postedAt.IsZero() {
		return JournalEntry{}, fmt.Errorf("%w: journal ID and posting timestamp are required", ErrInvalidArgument)
	}
	description = strings.TrimSpace(description)
	if !utf8.ValidString(description) || description == "" || utf8.RuneCountInString(description) > MaxDescriptionLength {
		return JournalEntry{}, fmt.Errorf("%w: description must contain 1 to %d Unicode characters", ErrInvalidArgument, MaxDescriptionLength)
	}
	if len(postings) < 2 || len(postings) > MaxPostings {
		return JournalEntry{}, fmt.Errorf("%w: expected 2 to %d postings", ErrInvalidArgument, MaxPostings)
	}
	accounts := make(map[string]struct{})
	ids := make(map[string]struct{})
	var debits, credits Amount
	for i, p := range postings {
		if err := p.Validate(); err != nil {
			return JournalEntry{}, fmt.Errorf("posting %d: %w", i, err)
		}
		if _, exists := ids[p.ID]; exists {
			return JournalEntry{}, fmt.Errorf("%w: duplicate posting ID", ErrInvalidArgument)
		}
		ids[p.ID] = struct{}{}
		accounts[p.AccountID] = struct{}{}
		var err error
		if p.Side == Debit {
			debits, err = debits.Add(p.Amount)
		} else {
			credits, err = credits.Add(p.Amount)
		}
		if err != nil {
			return JournalEntry{}, fmt.Errorf("posting %d: %w", i, err)
		}
	}
	if len(accounts) < 2 {
		return JournalEntry{}, fmt.Errorf("%w: at least two distinct accounts are required", ErrInvalidArgument)
	}
	if debits != credits {
		return JournalEntry{}, ErrUnbalanced
	}
	return JournalEntry{id: id, description: description, postedAt: postedAt.UTC(), postings: slices.Clone(postings)}, nil
}

func (e JournalEntry) ID() string          { return e.id }
func (e JournalEntry) Description() string { return e.description }
func (e JournalEntry) PostedAt() time.Time { return e.postedAt }
func (e JournalEntry) Postings() []Posting { return slices.Clone(e.postings) }
