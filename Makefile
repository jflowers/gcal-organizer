.PHONY: build test run clean install lint fmt vet service-status service-logs service-trigger ci install-hooks

# Binary name
BINARY_NAME=gcal-organizer
BINARY_PATH=./cmd/gcal-organizer

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOVET=$(GOCMD) vet
GOFMT=gofmt

# Version from git tags (injected at build time)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.Version=$(VERSION)

# Service paths (used by convenience targets only)
PLIST_DEST=$(HOME)/Library/LaunchAgents/com.jflowers.gcal-organizer.plist
LOG_DIR=$(HOME)/Library/Logs

# Build the binary
build:
	$(GOBUILD) -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) $(BINARY_PATH)

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	$(GOTEST) -v -cover -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Run the application
run:
	$(GOCMD) run $(BINARY_PATH) $(ARGS)

# Run with dry-run flag
dry-run:
	$(GOCMD) run $(BINARY_PATH) run --dry-run --verbose

# Install the binary to GOPATH/bin (symlinks if GOPATH/bin not in PATH)
install:
	$(GOCMD) install -ldflags="$(LDFLAGS)" $(BINARY_PATH)
	@GOBIN=$$(go env GOPATH)/bin; \
	if echo "$$PATH" | tr ':' '\n' | grep -qx "$$GOBIN"; then \
		echo "Binary installed to $$GOBIN/$(BINARY_NAME)"; \
	elif [ -d $(HOME)/.local/bin ] && [ -w $(HOME)/.local/bin ]; then \
		ln -sf "$$GOBIN/$(BINARY_NAME)" $(HOME)/.local/bin/$(BINARY_NAME); \
		echo "Binary installed to $$GOBIN/$(BINARY_NAME)"; \
		echo "Symlinked to $(HOME)/.local/bin/$(BINARY_NAME)"; \
	elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then \
		ln -sf "$$GOBIN/$(BINARY_NAME)" /usr/local/bin/$(BINARY_NAME); \
		echo "Binary installed to $$GOBIN/$(BINARY_NAME)"; \
		echo "Symlinked to /usr/local/bin/$(BINARY_NAME)"; \
	else \
		echo "Binary installed to $$GOBIN/$(BINARY_NAME)"; \
		echo "Note: Make sure $$GOBIN is in your PATH"; \
	fi
	@if [ -d /usr/local/share/man/man1 ] && [ -w /usr/local/share/man/man1 ]; then \
		cp man/gcal-organizer.1 /usr/local/share/man/man1/; \
		echo "Man page installed to /usr/local/share/man/man1/"; \
	elif [ -d $(HOME)/.local/share/man/man1 ] || mkdir -p $(HOME)/.local/share/man/man1 2>/dev/null; then \
		cp man/gcal-organizer.1 $(HOME)/.local/share/man/man1/; \
		echo "Man page installed to $(HOME)/.local/share/man/man1/"; \
	else \
		echo "Skipping man page install (no writable man directory found)"; \
	fi

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html

# Run go vet
vet:
	$(GOVET) ./...

# Format code
fmt:
	$(GOFMT) -w .

# Lint (requires golangci-lint)
lint:
	golangci-lint run

# Check all (format, vet, test)
check: fmt vet test

# CI check (mirrors GitHub Actions exactly — check-only, fails on violations)
ci:
	@echo "🔍 Running CI checks..."
	@echo "  Checking go.mod/go.sum..."
	@$(GOCMD) mod tidy
	@git diff --exit-code go.mod go.sum || (echo "❌ go.mod/go.sum not tidy" && exit 1)
	@echo "  Checking gofmt..."
	@unformatted=$$($(GOFMT) -l .); if [ -n "$$unformatted" ]; then echo "❌ Unformatted: $$unformatted"; exit 1; fi
	@echo "  Running go vet..."
	@$(GOVET) ./...
	@echo "  Building..."
	@$(GOBUILD) -o /dev/null $(BINARY_PATH)
	@echo "  Running tests..."
	@$(GOTEST) -race ./...
	@echo "✅ All CI checks passed!"

# Install git hooks
install-hooks:
	git config core.hooksPath .githooks
	@echo "✅ Git hooks installed (using .githooks/)"


# Development: build and run
dev: build
	./$(BINARY_NAME) $(ARGS)

# ─── Service Convenience Shortcuts ────────────────────────────
# Service install/uninstall is handled by:
#   gcal-organizer install
#   gcal-organizer uninstall
# These targets are quick shortcuts for status, logs, and triggering.

# Show service status
service-status:
ifeq ($(shell uname),Darwin)
	@echo "🍎 macOS LaunchAgent status:"
	@launchctl print gui/$$(id -u)/com.jflowers.gcal-organizer 2>/dev/null || echo "   Not installed"
else
	@echo "🐧 systemd timer status:"
	@systemctl --user status gcal-organizer.timer 2>/dev/null || echo "   Not installed"
endif

# Show recent logs
service-logs:
ifeq ($(shell uname),Darwin)
	@echo "🍎 Recent logs (last 50 lines):"
	@tail -50 $(LOG_DIR)/gcal-organizer.log 2>/dev/null || echo "   No logs yet"
else
	@echo "🐧 Recent logs:"
	@journalctl --user -u gcal-organizer.service --since "24 hours ago" --no-pager 2>/dev/null || echo "   No logs yet"
endif

# Trigger an immediate run
service-trigger:
ifeq ($(shell uname),Darwin)
	@echo "🍎 Triggering immediate run..."
	@launchctl kickstart gui/$$(id -u)/com.jflowers.gcal-organizer
else
	@echo "🐧 Triggering immediate run..."
	@systemctl --user start gcal-organizer.service
endif

# Help
help:
	@echo "Available targets:"
	@echo "  build             - Build the binary"
	@echo "  test              - Run tests"
	@echo "  test-coverage     - Run tests with coverage report"
	@echo "  run               - Run the application (use ARGS=... for arguments)"
	@echo "  dry-run           - Run with --dry-run --verbose flags"
	@echo "  install           - Install to GOPATH/bin (symlinks if not in PATH)"
	@echo "  clean             - Remove build artifacts"
	@echo "  vet               - Run go vet"
	@echo "  fmt               - Format code"
	@echo "  lint              - Run golangci-lint"
	@echo "  check             - Run fmt, vet, and test"
	@echo "  dev               - Build and run"
	@echo ""
	@echo "Service management (install/uninstall via 'gcal-organizer install/uninstall'):"
	@echo "  service-status    - Show service state"
	@echo "  service-logs      - Show recent logs"
	@echo "  service-trigger   - Trigger an immediate run"
	@echo "  help              - Show this help"
