.PHONY: build test run clean install lint fmt vet install-service uninstall-service service-status service-logs service-trigger ci install-hooks

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

# Deploy paths
DEPLOY_DIR=$(CURDIR)/deploy
WRAPPER_SRC=$(DEPLOY_DIR)/run-wrapper.sh
PLIST_SRC=$(DEPLOY_DIR)/launchd/com.jflowers.gcal-organizer.plist
PLIST_DEST=$(HOME)/Library/LaunchAgents/com.jflowers.gcal-organizer.plist
LOG_DIR=$(HOME)/Library/Logs
SYSTEMD_DIR=$(HOME)/.config/systemd/user
WRAPPER_DEST=$(HOME)/.local/bin/gcal-organizer-wrapper.sh

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

# Install the binary to ~/.local/bin, /usr/local/bin, or GOPATH/bin and man page
install:
	@if [ -d $(HOME)/.local/bin ] && [ -w $(HOME)/.local/bin ]; then \
		$(GOBUILD) -ldflags="$(LDFLAGS)" -o $(HOME)/.local/bin/$(BINARY_NAME) $(BINARY_PATH); \
		echo "Binary installed to $(HOME)/.local/bin/$(BINARY_NAME)"; \
	elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then \
		$(GOBUILD) -ldflags="$(LDFLAGS)" -o /usr/local/bin/$(BINARY_NAME) $(BINARY_PATH); \
		echo "Binary installed to /usr/local/bin/$(BINARY_NAME)"; \
	else \
		$(GOCMD) install -ldflags="$(LDFLAGS)" $(BINARY_PATH); \
		echo "Binary installed to $$(go env GOPATH)/bin/$(BINARY_NAME)"; \
		echo "Note: Make sure $$(go env GOPATH)/bin is in your PATH"; \
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

# ─── Service Management ───────────────────────────────────────

# Install as an hourly service (auto-detects macOS vs Linux)
install-service: install
ifeq ($(shell uname),Darwin)
	@echo "🍎 Installing macOS LaunchAgent..."
	@mkdir -p $(LOG_DIR)
	@sed -e 's|WRAPPER_PATH_PLACEHOLDER|$(WRAPPER_DEST)|g' \
	     -e 's|LOG_PATH_PLACEHOLDER|$(LOG_DIR)/gcal-organizer.log|g' \
	     -e 's|HOME_PATH_PLACEHOLDER|$(HOME)|g' \
	     -e "s|BINARY_PATH_PLACEHOLDER|$$(go env GOPATH)/bin/gcal-organizer|g" \
	     $(PLIST_SRC) > $(PLIST_DEST)
	@mkdir -p $(HOME)/.local/bin
	@cp $(WRAPPER_SRC) $(WRAPPER_DEST)
	@chmod +x $(WRAPPER_DEST)
	@launchctl bootout gui/$$(id -u) $(PLIST_DEST) 2>/dev/null || true
	@launchctl bootstrap gui/$$(id -u) $(PLIST_DEST)
	@echo "✅ Installed! Will run every hour."
	@echo "   Logs: $(LOG_DIR)/gcal-organizer.log"
	@echo "   Trigger now: make service-trigger"
else
	@echo "🐧 Installing systemd user service..."
	@mkdir -p $(SYSTEMD_DIR)
	@mkdir -p $(HOME)/.local/bin
	@cp $(WRAPPER_SRC) $(WRAPPER_DEST)
	@chmod +x $(WRAPPER_DEST)
	@cp $(DEPLOY_DIR)/systemd/gcal-organizer.service $(SYSTEMD_DIR)/
	@cp $(DEPLOY_DIR)/systemd/gcal-organizer.timer $(SYSTEMD_DIR)/
	@systemctl --user daemon-reload
	@systemctl --user enable --now gcal-organizer.timer
	@echo "✅ Installed! Timer active."
	@echo "   Logs: journalctl --user -u gcal-organizer.service"
	@echo "   Trigger now: make service-trigger"
endif

# Uninstall the service
uninstall-service:
ifeq ($(shell uname),Darwin)
	@echo "🍎 Removing macOS LaunchAgent..."
	@launchctl bootout gui/$$(id -u) $(PLIST_DEST) 2>/dev/null || true
	@rm -f $(PLIST_DEST)
	@rm -f $(WRAPPER_DEST)
	@echo "✅ Service removed."
else
	@echo "🐧 Removing systemd user service..."
	@systemctl --user disable --now gcal-organizer.timer 2>/dev/null || true
	@rm -f $(SYSTEMD_DIR)/gcal-organizer.service
	@rm -f $(SYSTEMD_DIR)/gcal-organizer.timer
	@rm -f $(WRAPPER_DEST)
	@systemctl --user daemon-reload
	@echo "✅ Service removed."
endif

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
	@echo "  install           - Install to GOPATH/bin"
	@echo "  clean             - Remove build artifacts"
	@echo "  vet               - Run go vet"
	@echo "  fmt               - Format code"
	@echo "  lint              - Run golangci-lint"
	@echo "  check             - Run fmt, vet, and test"
	@echo "  dev               - Build and run"
	@echo ""
	@echo "Service management:"
	@echo "  install-service   - Install as hourly service (macOS/Fedora)"
	@echo "  uninstall-service - Remove the service"
	@echo "  service-status    - Show service state"
	@echo "  service-logs      - Show recent logs"
	@echo "  service-trigger   - Trigger an immediate run"
	@echo "  help              - Show this help"
