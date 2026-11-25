package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"doubleLedger/internal/domain"

	"github.com/google/uuid"
)

var ErrIdempotencyConflict = errors.New("idempotency key already used with different journal data")
var ErrJournalEntryNotFound = errors.New("journal entry not found")

type PostingInput struct {
	AccountID string
	Side      domain.Side
	Amount    domain.Amount
}

type JournalStore interface {
	// Save atomically inserts the whole entry or returns the entry already stored for the key.
	Save(context.Context, string, domain.JournalEntry) (domain.JournalEntry, error)
	FindJournalEntry(context.Context, string) (domain.JournalEntry, error)
}

type Journal struct {
	store JournalStore
}

func NewJournal(store JournalStore) *Journal {
	return &Journal{store: store}
}

func (j *Journal) Get(ctx context.Context, entryID string) (domain.JournalEntry, error) {
	id, err := uuid.Parse(entryID)
	if err != nil || !strings.EqualFold(entryID, id.String()) {
		return domain.JournalEntry{}, fmt.Errorf("%w: journal entry ID must be a UUID", domain.ErrInvalidArgument)
	}
	return j.store.FindJournalEntry(ctx, id.String())
}

func (j *Journal) Post(ctx context.Context, key, description string, inputs []PostingInput) (domain.JournalEntry, error) {
	if len(key) == 0 || len(key) > 128 || strings.ContainsFunc(key, func(r rune) bool { return r < '!' || r > '~' }) {
		return domain.JournalEntry{}, fmt.Errorf("%w: idempotency key must contain 1 to 128 visible ASCII characters", domain.ErrInvalidArgument)
	}
	if strings.ContainsRune(description, 0) {
		return domain.JournalEntry{}, fmt.Errorf("%w: description contains a NUL character", domain.ErrInvalidArgument)
	}
	if len(inputs) < 2 || len(inputs) > domain.MaxPostings {
		return domain.JournalEntry{}, fmt.Errorf("%w: expected 2 to %d postings", domain.ErrInvalidArgument, domain.MaxPostings)
	}

	postings := make([]domain.Posting, len(inputs))
	for i, input := range inputs {
		accountID, err := uuid.Parse(input.AccountID)
		if err != nil || !strings.EqualFold(input.AccountID, accountID.String()) {
			return domain.JournalEntry{}, fmt.Errorf("%w: posting %d account ID must be a UUID", domain.ErrInvalidArgument, i+1)
		}
		postingID, err := uuid.NewRandom()
		if err != nil {
			return domain.JournalEntry{}, fmt.Errorf("generate posting ID: %w", err)
		}
		postings[i] = domain.Posting{ID: postingID.String(), AccountID: accountID.String(), Side: input.Side, Amount: input.Amount}
	}
	entryID, err := uuid.NewRandom()
	if err != nil {
		return domain.JournalEntry{}, fmt.Errorf("generate journal entry ID: %w", err)
	}
	entry, err := domain.NewJournalEntry(entryID.String(), description, time.Now().Truncate(time.Microsecond), postings)
	if err != nil {
		return domain.JournalEntry{}, err
	}
	stored, err := j.store.Save(ctx, key, entry)
	if err != nil {
		return domain.JournalEntry{}, err
	}
	if stored.ID() == entry.ID() {
		return stored, nil
	}
	if stored.Description() != entry.Description() || !slices.EqualFunc(stored.Postings(), postings, samePostingInput) {
		return domain.JournalEntry{}, ErrIdempotencyConflict
	}
	return stored, nil
}

func samePostingInput(a, b domain.Posting) bool {
	return a.AccountID == b.AccountID && a.Side == b.Side && a.Amount == b.Amount
}
