# gRPC request examples

Follow the [local setup instructions](../README.md#run-and-test-createaccount)
to start the server. Run these examples from the repository root in a second
terminal. On macOS, install `grpcurl` with `brew install grpcurl` if needed.

## Contents

- [CreateAccount](#createaccount)

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
