.DEFAULT_GOAL := check

GO ?= go
GOFMT ?= gofmt
TOOLS_BIN := $(CURDIR)/.tools/bin
BUF := $(TOOLS_BIN)/buf
BUF_VERSION := v1.50.0
PROTOBUF_GO_VERSION := v1.36.6
GRPC_GO_PLUGIN_VERSION := v1.5.1

.PHONY: proto-tools proto-generate proto-lint proto-check check

# Install pinned tools locally without changing the application's dependencies.
proto-tools:
	GOBIN="$(TOOLS_BIN)" $(GO) install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	GOBIN="$(TOOLS_BIN)" $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOBUF_GO_VERSION)
	GOBIN="$(TOOLS_BIN)" $(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(GRPC_GO_PLUGIN_VERSION)

proto-generate:
	$(BUF) generate

proto-lint:
	$(BUF) format --diff --exit-code
	$(BUF) lint
	$(BUF) build -o /dev/null

# Generate into a temporary directory and compare, including untracked files.
proto-check: proto-lint
	@set -eu; generated_dir=$$(mktemp -d); \
	trap 'rm -rf "$$generated_dir"' EXIT; \
	$(BUF) generate --output "$$generated_dir"; \
	diff -ru api/gen "$$generated_dir/api/gen"

check:
	@test -z "$$($(GOFMT) -l .)"
	$(GO) vet ./...
	$(GO) test -race ./...
	$(GO) build ./...
