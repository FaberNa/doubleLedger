# gRPC request examples

Follow the [local setup instructions](../README.md#run-and-test-createaccount)
to start the server. Run these examples from the repository root in a second
terminal. On macOS, install `grpcurl` with `brew install grpcurl` if needed.

## Contents

- [CreateAccount](#createaccount)
- [GetAccount](#getaccount)

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
