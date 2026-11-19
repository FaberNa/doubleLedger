-- +goose Up
CREATE SCHEMA ledger;

CREATE TABLE ledger.accounts (
    id uuid PRIMARY KEY,
    code text COLLATE "C" NOT NULL UNIQUE CHECK (code ~ '^[A-Z0-9_-]{1,64}$'),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 255 AND btrim(name) <> ''),
    account_type text NOT NULL CHECK (account_type IN ('asset', 'liability', 'equity', 'expense', 'revenue')),
    created_at timestamptz NOT NULL
);

CREATE TABLE ledger.journal_entries (
    id uuid PRIMARY KEY,
    idempotency_key text COLLATE "C" NOT NULL UNIQUE CHECK (idempotency_key ~ '^[!-~]{1,128}$'),
    description text NOT NULL CHECK (char_length(description) BETWEEN 1 AND 1000 AND btrim(description) <> ''),
    posted_at timestamptz NOT NULL
);

CREATE TABLE ledger.postings (
    id uuid PRIMARY KEY,
    journal_entry_id uuid NOT NULL REFERENCES ledger.journal_entries (id),
    account_id uuid NOT NULL REFERENCES ledger.accounts (id),
    line_number integer NOT NULL CHECK (line_number BETWEEN 1 AND 1000),
    side text NOT NULL CHECK (side IN ('debit', 'credit')),
    amount_minor bigint NOT NULL CHECK (amount_minor > 0),
    UNIQUE (journal_entry_id, line_number)
);

CREATE INDEX accounts_created_at_id_idx ON ledger.accounts (created_at, id);
CREATE INDEX journal_entries_posted_at_id_idx ON ledger.journal_entries (posted_at, id);
CREATE INDEX postings_account_id_idx ON ledger.postings (account_id);

-- +goose Down
DROP TABLE ledger.postings;
DROP TABLE ledger.journal_entries;
DROP TABLE ledger.accounts;
DROP SCHEMA ledger;
