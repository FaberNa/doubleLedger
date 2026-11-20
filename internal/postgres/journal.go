package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"doubleLedger/internal/application"
	"doubleLedger/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Journal struct {
	pool *pgxpool.Pool
}

func NewJournal(pool *pgxpool.Pool) *Journal {
	return &Journal{pool: pool}
}

func (j *Journal) Save(ctx context.Context, key string, entry domain.JournalEntry) (domain.JournalEntry, error) {
	var stored domain.JournalEntry
	err := pgx.BeginTxFunc(ctx, j.pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `
			INSERT INTO ledger.journal_entries (id, idempotency_key, description, posted_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (idempotency_key) DO NOTHING
		`, entry.ID(), key, entry.Description(), entry.PostedAt())
		if err != nil {
			return fmt.Errorf("insert journal entry: %w", err)
		}
		if result.RowsAffected() == 0 {
			// At READ COMMITTED this new statement can see the winning transaction.
			stored, err = readJournalByKey(ctx, tx, key)
			return err
		}

		var batch pgx.Batch
		for i, p := range entry.Postings() {
			batch.Queue(`
				INSERT INTO ledger.postings (id, journal_entry_id, account_id, line_number, side, amount_minor)
				VALUES ($1, $2, $3, $4, $5, $6)
			`, p.ID, entry.ID(), p.AccountID, i+1, string(p.Side), int64(p.Amount))
		}
		if err := tx.SendBatch(ctx, &batch).Close(); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "postings_account_id_fkey" {
				return application.ErrAccountNotFound
			}
			return fmt.Errorf("insert journal postings: %w", err)
		}
		stored = entry
		return nil
	})
	if err != nil {
		return domain.JournalEntry{}, err
	}
	return stored, nil
}

const journalWithPostings = `
		SELECT e.id, e.description, e.posted_at, p.id, p.account_id, p.side, p.amount_minor
		FROM ledger.journal_entries AS e
		JOIN ledger.postings AS p ON p.journal_entry_id = e.id
`

func (j *Journal) FindJournalEntry(ctx context.Context, id string) (domain.JournalEntry, error) {
	rows, err := j.pool.Query(ctx, journalWithPostings+`WHERE e.id = $1 ORDER BY p.line_number`, id)
	return readJournal(rows, err)
}

func readJournalByKey(ctx context.Context, tx pgx.Tx, key string) (domain.JournalEntry, error) {
	rows, err := tx.Query(ctx, journalWithPostings+`WHERE e.idempotency_key = $1 ORDER BY p.line_number`, key)
	return readJournal(rows, err)
}

func readJournal(rows pgx.Rows, err error) (domain.JournalEntry, error) {
	if err != nil {
		return domain.JournalEntry{}, fmt.Errorf("read existing journal entry: %w", err)
	}
	defer rows.Close()
	var id, description string
	var postedAt time.Time
	var postings []domain.Posting
	for rows.Next() {
		var posting domain.Posting
		var side string
		var amount int64
		if err := rows.Scan(&id, &description, &postedAt, &posting.ID, &posting.AccountID, &side, &amount); err != nil {
			return domain.JournalEntry{}, fmt.Errorf("scan journal posting: %w", err)
		}
		posting.Side, posting.Amount = domain.Side(side), domain.Amount(amount)
		postings = append(postings, posting)
	}
	if err := rows.Err(); err != nil {
		return domain.JournalEntry{}, fmt.Errorf("read journal postings: %w", err)
	}
	if len(postings) == 0 {
		return domain.JournalEntry{}, application.ErrJournalEntryNotFound
	}
	entry, err := domain.NewJournalEntry(id, description, postedAt, postings)
	if err != nil {
		return domain.JournalEntry{}, fmt.Errorf("invalid stored journal entry: %v", err)
	}
	return entry, nil
}
