# gRPC request examples

Follow the [local setup instructions](../README.md#run-and-test-createaccount)
to start the server. Run these examples from the repository root in a second
terminal. On macOS, install `grpcurl` with `brew install grpcurl` if needed.

## Contents

- [CreateAccount](#createaccount)
- [GetAccount](#getaccount)
- [PostJournalEntry](#postjournalentry)
- [GetJournalEntry](#getjournalentry)

## CreateAccount

Create an asset account with code `cash` and name `Cash`.

```sh
grpcurl -plaintext \
  -import-path api/proto \
  -proto ledger/v1/ledger.proto \
  -d '{"code":"cash","name":"Cash","type":"ACCOUNT_TYPE_ASSET"}' \
  127.0.0.1:50051 ledger.v1.LedgerService/CreateAccount
```

The response includes the normalized code `CASH`, the account UUID, and its
creation timestamp. The account remains in the local database. Repeating the
call with the same code returns `AlreadyExists`.

## GetAccount

Set `ACCOUNT_ID` to the `account.id` returned by `CreateAccount`.

```sh
ACCOUNT_ID='<account UUID>'
grpcurl -plaintext \
  -import-path api/proto \
  -proto ledger/v1/ledger.proto \
  -d "{\"accountId\":\"3d84d2be-4552-42ce-99bb-4e0f8d5d11ca\"}" \
  127.0.0.1:50051 ledger.v1.LedgerService/GetAccount
```

An invalid UUID returns `InvalidArgument`. A valid UUID with no matching
account returns `NotFound`.

## PostJournalEntry

Use `CreateAccount` to set up a cash account and two revenue accounts. Set
these variables to the returned account IDs and generate one idempotency key
for this operation.

```sh
CASH_ID='<cash account UUID>'
SERVICES_ID='<service revenue account UUID>'
GOODS_ID='<goods revenue account UUID>'
IDEMPOTENCY_KEY="$(uuidgen)"
```

Register a €100 cash receipt split evenly between services and goods. Amounts
are in EUR cents and the order of the posting lines is significant.

```sh
grpcurl -plaintext \
  -import-path api/proto \
  -proto ledger/v1/ledger.proto \
  -d @ 127.0.0.1:50051 ledger.v1.LedgerService/PostJournalEntry <<JSON
{
  "idempotencyKey": "$IDEMPOTENCY_KEY",
  "description": "Cash receipt for goods and services",
  "postings": [
    {"accountId": "$CASH_ID", "side": "SIDE_DEBIT", "amountMinor": "10000"},
    {"accountId": "$SERVICES_ID", "side": "SIDE_CREDIT", "amountMinor": "5000"},
    {"accountId": "$GOODS_ID", "side": "SIDE_CREDIT", "amountMinor": "5000"}
  ]
}
JSON
```

The response includes the journal entry ID, timestamp, currency, and ordered
posting lines. To retry, rerun only the `grpcurl` block with the original key.
The same payload returns the original entry. Changing the payload while
keeping the key returns `AlreadyExists`.

## GetJournalEntry

Set `JOURNAL_ENTRY_ID` to the `journalEntry.id` returned by `PostJournalEntry`.

```sh
JOURNAL_ENTRY_ID='<journal entry UUID>'
grpcurl -plaintext \
  -import-path api/proto \
  -proto ledger/v1/ledger.proto \
  -d "{\"journalEntryId\":\"$JOURNAL_ENTRY_ID\"}" \
  127.0.0.1:50051 ledger.v1.LedgerService/GetJournalEntry
```

The response includes all postings in their original order. A malformed UUID
returns `InvalidArgument`. An unknown UUID returns `NotFound`.
