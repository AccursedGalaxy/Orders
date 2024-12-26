.PHONY: all setup test build lint clean pre-commit install-hooks help fmt coverage dev dev-down mock tools lint-fix

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
BINARY_NAME=streamer
VIEWER_NAME=redis-viewer
COVERAGE_FILE=coverage.txt
MIN_COVERAGE=70

# Tool versions
GOLANGCI_LINT_VERSION=v1.54.2
MOCKERY_VERSION=v2.32.4

# Build flags
LDFLAGS=-ldflags "-X main.Version=$$(cat version.txt)"

# Linter configuration
LINT_FLAGS=--timeout=5m --config=.golangci.yml
LINT_FIX_FLAGS=$(LINT_FLAGS) --fix

# Targets
all: help

help: ## Display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

setup: install-hooks tools ## Setup development environment
	$(GOCMD) mod download
	$(GOCMD) mod tidy

gitpush: ## Push changes to git - pass last commit message as 'm' argument
	git add .
	git commit -m "$(m)"
	git push

gitreset: ## Reset changes in git
	git reset --hard

tools: ## Install development tools
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)
	@echo "Installing mockery $(MOCKERY_VERSION)..."
	$(GOCMD) install github.com/vektra/mockery/v2@$(MOCKERY_VERSION)

install-hooks: ## Install git hooks
	cp scripts/pre-commit.sh .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit

test: ## Run tests
	$(GOTEST) -v -race ./...

test-coverage: ## Run tests with coverage
	$(GOTEST) -v -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	$(GOCMD) tool cover -func=$(COVERAGE_FILE) | grep total | awk '{if ($$3 < $(MIN_COVERAGE)) {exit 1}}'
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o coverage.html

test-short: ## Run tests in short mode
	$(GOTEST) -v -short ./...

build: ## Build binaries
	mkdir -p bin
	$(GOBUILD) $(LDFLAGS) -v -o bin/$(BINARY_NAME) cmd/streamer/main.go
	$(GOBUILD) $(LDFLAGS) -v -o bin/$(VIEWER_NAME) scripts/redis-viewer.go

build-all: ## Build for all platforms
	mkdir -p bin
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -v -o bin/$(BINARY_NAME)-linux-amd64 cmd/streamer/main.go
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -v -o bin/$(VIEWER_NAME)-linux-amd64 scripts/redis-viewer.go
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -v -o bin/$(BINARY_NAME)-darwin-amd64 cmd/streamer/main.go
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -v -o bin/$(VIEWER_NAME)-darwin-amd64 scripts/redis-viewer.go

lint: ## Run linter
	@echo "Running linter..."
	@golangci-lint run $(LINT_FLAGS) || (echo "Lint failed. Run 'make lint-fix' to attempt automatic fixes"; exit 1)

lint-fix: ## Run linter with auto-fix
	@echo "Running linter with auto-fix..."
	@golangci-lint run $(LINT_FIX_FLAGS)

lint-new: ## Run linter only on new changes
	@echo "Running linter on new changes..."
	@golangci-lint run $(LINT_FLAGS) --new-from-rev=HEAD~

fmt: ## Format code
	$(GOCMD) fmt ./...
	gofmt -s -w .

vet: ## Run go vet
	$(GOCMD) vet ./...

tidy: ## Tidy and verify go modules
	$(GOCMD) mod tidy
	$(GOCMD) mod verify

verify: fmt lint vet test ## Verify code quality

pre-commit: verify ## Run pre-commit checks

mock: ## Generate mocks
	mockery --all --keeptree

clean: ## Clean build artifacts
	rm -rf bin/
	rm -f $(COVERAGE_FILE)
	rm -f coverage.html

# Docker commands
dev: ## Start development environment
	docker-compose up -d

dev-down: ## Stop development environment
	docker-compose down -v

dev-logs: ## Show development logs
	docker-compose logs -f

# Release commands
release-patch: ## Create a patch release
	./scripts/bump-version.sh patch

release-minor: ## Create a minor release
	./scripts/bump-version.sh minor

release-major: ## Create a major release
	./scripts/bump-version.sh major

# Database commands
db-migrate: ## Run database migrations
	@echo "Running database migrations..."
	# Add your migration command here

db-rollback: ## Rollback database migrations
	@echo "Rolling back database migrations..."
	# Add your rollback command here

# Benchmarking
bench: ## Run benchmarks
	$(GOTEST) -v -run=^$$ -bench=. -benchmem ./...

bench-compare: ## Run benchmarks and compare with master
	@echo "Running benchmarks and comparing with master..."
	@git stash push -m "benchmark stash"
	@git checkout master
	@$(GOTEST) -v -run=^$$ -bench=. -benchmem ./... > bench-master.txt
	@git checkout -
	@git stash pop
	@$(GOTEST) -v -run=^$$ -bench=. -benchmem ./... > bench-current.txt
	@benchstat bench-master.txt bench-current.txt
	@rm bench-master.txt bench-current.txt
