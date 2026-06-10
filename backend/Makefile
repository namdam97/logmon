.PHONY: build test test-race lint run tidy clean

GO        ?= go
SERVICE   ?= userservice
BIN_DIR   ?= bin

build: ## Build all binaries
	$(GO) build ./...

run: ## Run the user service
	$(GO) run ./cmd/$(SERVICE)

test: ## Run tests with coverage
	$(GO) test -cover ./...

test-race: ## Run tests with race detector + coverage
	$(GO) test -race -cover ./...

lint: ## Run golangci-lint
	golangci-lint run

tidy: ## Tidy go.mod/go.sum
	$(GO) mod tidy

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
	$(GO) clean
