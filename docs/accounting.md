# Accounting domain decisions

This document records the agreed accounting domain rules. It supplements the
README and will evolve alongside the implementation. Account validation, journal
validation, checked amount arithmetic, and normal balance calculations are
implemented in `internal/domain`. Persistence guarantees will be implemented in
the database phase.

## Accounts

Each account has a service-generated ID, a unique code, a name, an accounting
type, and a creation timestamp.

### Account codes

- Codes are required and have a maximum length of 64 characters.
- Allowed characters are ASCII letters, digits, underscores (`_`), and hyphens
  (`-`). Spaces are not allowed.
- Codes are normalized to uppercase: `bank_a` becomes `BANK_A`.
- Uniqueness applies to the normalized code. `bank_a` and `BANK_A` therefore
  identify the same code and cannot be assigned to different accounts.
- PostgreSQL will enforce uniqueness when persistence is implemented.

### Account names

- Names are required and have a maximum length of 255 Unicode code points.
- Leading and trailing whitespace is removed before validation.
- A name that is empty after trimming is invalid.
- Duplicate names are allowed. IDs and normalized codes identify accounts
  uniquely.

### Account types and balances

| Type | Normal side | Balance calculation |
| --- | --- | --- |
| Asset | Debit | Total debits minus total credits |
| Expense | Debit | Total debits minus total credits |
| Liability | Credit | Total credits minus total debits |
| Equity | Credit | Total credits minus total debits |
| Revenue | Credit | Total credits minus total debits |

Balances may be positive, zero, or negative. The initial scope does not include
overdraft prevention or spending limits.

## Journal entries and postings

- A journal entry description is required and has a maximum length of 1,000
  Unicode code points after trimming leading and trailing whitespace.
- A description that is empty after trimming is invalid.
- Each journal entry contains between 2 and 1,000 postings, inclusive.
- The posting limit bounds request size and transaction work; it is a service
  constraint rather than a fundamental accounting rule.
- Each posting identifies an account, a debit or credit side, and a strictly
  positive amount.
- Total debits must equal total credits.
- Each journal entry must involve at least two distinct account IDs.
- The same account may appear in multiple postings, provided that the entry
  also involves another account.

The distinct-account requirement is an additional service rule. It rejects
entries such as a debit of 100 EUR and a credit of 100 EUR on the same sole
account, which can indicate that the caller accidentally selected the same
source and destination. It does not guarantee a nonzero net effect on accounts.

## Amounts and history

- Amounts use signed 64-bit integers in the configured currency's smallest unit;
  for EUR, amounts are in cents: `1` represents 0.01 EUR and `10000` represents
  100 EUR.
- No additional business amount cap below the capacity of `int64` is imposed.
  Posting amounts must be strictly positive and representable as `int64`.
- Floating-point arithmetic is not used for monetary amounts.
- Arithmetic and conversions must detect overflow, including totals and balance
  calculations.
- A journal entry is rejected in full if either its debit or credit total
  overflows `int64`, even if the mathematical totals would be equal.
- Balances are derived from persisted postings rather than independently mutable
  balance fields.
- A journal entry is persisted atomically, with all its postings or none.
- Posted entries are immutable. Corrections require new compensating entries.

## Corrections

- The first version has no dedicated reversal operation or structured reference
  to the original entry.
- To fully reverse an entry, the caller submits a new entry with the same
  accounts and amounts and the debit/credit sides exchanged.
- The correction description identifies the original journal entry ID. This is
  a caller convention, not a machine-validated link.
- The original entry remains in the accounting history.

## Domain implementation boundary

The application will generate IDs and server timestamps and supply them to domain
constructors. Domain constructors require nonblank IDs and nonzero timestamps,
normalize timestamps to UTC, and reject duplicate posting IDs within an entry.
They do not generate or parse UUIDs. Account existence, code uniqueness across
accounts, atomic persistence, and idempotency require the application and
PostgreSQL and are not implemented in this phase.

Constructors validate account and journal entry values. Accounts and journal
entries expose read accessors rather than mutation methods; journal entries copy
their posting slices on input and output to protect their history from changes
through shared slices. The application must use these constructors rather than
zero-value entities when creating or restoring domain objects.

Balance calculations accept nonnegative debit and credit totals. Persistence must
check that aggregated totals fit `int64` before converting database results; an
overflow must never be silently truncated.
