# gRPC contract

The `ledger.v1.LedgerService` contract is defined in
`api/proto/ledger/v1/ledger.proto`. Generated Go messages, client stubs, and server
interfaces are checked into `api/gen/ledger/v1`. Never edit generated files.
This phase defines the API; handlers and PostgreSQL are not implemented yet.

## Operations

| Method | Input | Result |
| --- | --- | --- |
| CreateAccount | Code, name, type | Created account |
| GetAccount | Account ID | Account |
| ListAccounts | Page size and cursor | Accounts and next cursor |
| PostJournalEntry | Idempotency key, description, ordered lines | Complete entry |
| GetJournalEntry | Entry ID | Complete entry |
| ListJournalEntries | Page size and cursor | Entry summaries and next cursor |
| ListAccountPostings | Account ID, page size and cursor | Lines with entry timestamps and next cursor |
| GetAccountBalance | Account ID | Debit total, credit total, normal balance |

IDs are server-generated UUIDs and timestamps are assigned by the server in UTC.
Requests do not accept caller-generated entity IDs or backdated posting times.
Every line receives a one-based line number preserving its submitted order.
Posting responses include both their own ID and their journal entry ID.

All amounts are signed `int64` values in smallest currency units, named
`amount_minor` or `*_minor`. For EUR, `10000` means 100 EUR. Posting amounts must
be positive; normal balances may be negative. Entry and balance responses expose
the service's configured currency code. Currency cannot be selected per request.
The currency and its precision must be fixed for the ledger when persistence is
configured; they are not inferred from the integer amount.

The accounting rules in `accounting.md` also apply to API requests. Proto3
comments describe constraints but do not enforce them: application validation
must reject unspecified or unknown enums and invalid/missing fields.

## Initial transport choices

These choices complete the initial contract and should be reviewed in this PR.

### Idempotency

- The key is required, case-sensitive, scoped to the single ledger, and contains
  1 to 128 visible ASCII characters (`0x21` through `0x7e`). It is not trimmed or
  normalized.
- The accounting payload consists of the trimmed description and the ordered
  sequence of account IDs, sides, and amounts. Account IDs are compared as UUID
  values rather than textual representations.
- Line order is significant. Reordering lines or splitting/merging lines changes
  the payload, even if accounting totals remain the same.
- Generated IDs and timestamps are excluded from payload comparison.
- Reusing the key with the same normalized payload returns the original complete
  entry, including its original ID and timestamp.
- Reusing it with a different payload returns `AlreadyExists`.
- The key and entry must be recorded atomically. Concurrent retries must be
  resolved by a database uniqueness constraint, not a preliminary lookup alone.
- A new key means a new operation. Retrying with a new key can duplicate a post.

### Pagination

- All list methods accept a page size and an opaque page token.
- A size of zero selects 50; explicit sizes must be between 1 and 200.
  Negative or excessive sizes return `InvalidArgument`.
- Journal entry lists return summaries and posting counts rather than all lines
  of every entry. Retrieve full lines with `GetJournalEntry` to avoid list
  responses growing by up to 1,000 postings per result.
- Ordering is ascending: accounts by `(created_at, id)`, entries by
  `(posted_at, id)`, and account lines by `(posted_at, journal_entry_id,
  line_number)`. UUID tie-breaking must match PostgreSQL UUID ordering.
- Each cursor resumes strictly after the last returned ordering tuple. Its
  concrete encoding is a persistence implementation detail.
- Cursors are bound to the list method and, for account lines, the account ID.
  Changing page size is allowed. Invalid or mismatched cursors are rejected.
- An empty next token means no further rows existed at query time. Pagination
  does not promise a shared snapshot across requests; concurrent commits may
  introduce rows before or after a cursor.
- A nonexistent account produces `NotFound` for account posting and balance
  queries. An existing account without activity has empty postings and zero
  totals.

### Errors

| gRPC status | Meaning |
| --- | --- |
| InvalidArgument | Invalid fields, malformed UUIDs, unbalanced entry, invalid pagination, or entry-total overflow |
| NotFound | Well-formed account or journal entry ID does not exist |
| AlreadyExists | Duplicate normalized account code or conflicting idempotency payload |
| OutOfRange | A balance query's persisted totals cannot be represented by the API's int64 fields |
| Canceled / DeadlineExceeded | Request canceled or deadline exceeded |
| Internal | Unexpected failure; database details must not be exposed |

Structured error details are deferred. `CreateAccount` is not idempotent: a retry
after a lost success response may return `AlreadyExists` for its unique code.

## Generation and verification

Requirements: Go 1.27+, Make, and network access for initial Go tool/module
downloads. Buf includes a Protobuf compiler, so a separate `protoc` installation
is not required. Generation uses local plugins, not a remote schema registry.

```sh
make proto-tools     # Install pinned tools into ignored .tools/bin/
make proto-generate  # Regenerate Go messages and gRPC stubs
make proto-check     # Format, lint, compile, and compare regenerated files
make check           # Go formatting, vet, race-enabled tests, and build
```

Tool versions are pinned in `Makefile`; application runtime dependencies are
pinned in `go.mod` and `go.sum`. `buf.yaml` selects lint and compatibility rules;
`buf.gen.yaml` selects generator paths and output directories.

CI runs these checks within the existing `check` job. Once main contains the Buf
configuration, CI also checks schema compatibility against main. Incompatible
field deletions or changes must be resolved before merge; removed field numbers
and names must be reserved rather than reused.

Tooling references: [Buf local plugin configuration](https://buf.build/docs/configuration/v2/buf-gen-yaml/)
and [gRPC Go generation](https://grpc.io/docs/languages/go/quickstart/).
