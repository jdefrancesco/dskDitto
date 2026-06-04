GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test

BINARY_NAME ?= dskDitto
GUI_PATH ?= .
BIN_DIR ?= ./bin
BIN_PATH := $(BIN_DIR)/$(BINARY_NAME)

# Keep Makefile fallback version aligned with source buildinfo.Version.
SOURCE_VERSION ?= $(shell awk -F'"' '/^[[:space:]]*var[[:space:]]+Version[[:space:]]*=[[:space:]]*"/ { print $$2; exit }' internal/buildinfo/version.go)
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || printf '%s' $(SOURCE_VERSION))
VERSION_LDFLAGS := -X github.com/jdefrancesco/dskDitto/internal/buildinfo.Version=$(VERSION)

INSTALL_PKG := github.com/jdefrancesco/dskDitto/cmd/$(BINARY_NAME)
REMOTE_NAME ?= origin
RELEASE_BRANCH ?= master
PREFIX ?= /usr/local/bin

BENCH_BIN := $(BIN_DIR)/bench.test
CPU_PROFILE ?= cpu.prof
MEM_PROFILE ?= mem.prof
PROFILE ?= $(CPU_PROFILE)
PPROF_ADDR ?= localhost:6060
DIR_SWEEP_VALUES ?= 16 24 32 48 64 96 128
DIR_SWEEP_PATH ?= .
NO_CACHE ?= 0

GOSEC_SCAN_EXCLUDES ?= G104,G108
GOSEC_STRICT_EXCLUDES ?= G104

.DEFAULT_GOAL := all

.PHONY: all security-scan check-gosec debug build build-gui run-gui build-darwin-arm64 \
	test bench bench-dir-sweep-build bench-dir-sweep bench-build bench-profile \
	pprof-web gosec install release-check release-install-check clean

all: test build

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

security-scan:
	@if command -v gosec >/dev/null 2>&1; then \
		gosec -exclude=$(GOSEC_SCAN_EXCLUDES) ./...; \
	else \
		echo "Skipping gosec scan: gosec not installed"; \
	fi

check-gosec:
	@command -v gosec >/dev/null 2>&1 || { \
		echo "gosec must be in PATH to run this target"; \
		exit 1; \
	}

debug: security-scan | $(BIN_DIR)
	$(GOBUILD) -ldflags "$(VERSION_LDFLAGS)" -o $(BIN_PATH) -v -gcflags "all=-N -l" ./cmd/$(BINARY_NAME)

build: security-scan | $(BIN_DIR)
	$(GOBUILD) -ldflags "$(VERSION_LDFLAGS)" -o $(BIN_PATH) ./cmd/$(BINARY_NAME)

build-gui: security-scan | $(BIN_DIR)
	CGO_ENABLED=1 $(GOBUILD) -ldflags "$(VERSION_LDFLAGS)" -o $(BIN_PATH) ./cmd/$(BINARY_NAME)
	@echo "GUI-capable binary built at $(BIN_PATH)"
	@echo "Run it with: $(BIN_PATH) --gui <path>"

run-gui: build-gui
	$(BIN_PATH) --gui $(GUI_PATH)

build-darwin-arm64: | $(BIN_DIR)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -ldflags "$(VERSION_LDFLAGS)" -o $(BIN_PATH) ./cmd/$(BINARY_NAME)

test:
	$(GOTEST) -v ./...

bench:
	$(GOTEST) -bench=. -benchmem ./internal/bench/

bench-dir-sweep-build: | $(BIN_DIR)
	$(GOBUILD) -ldflags "$(VERSION_LDFLAGS)" -o $(BIN_PATH) ./cmd/$(BINARY_NAME)

bench-dir-sweep: bench-dir-sweep-build
	@if [ ! -e "$(DIR_SWEEP_PATH)" ]; then \
		echo "DIR_SWEEP_PATH '$(DIR_SWEEP_PATH)' does not exist."; \
		exit 1; \
	fi
	@extra=""; \
	if [ "$(NO_CACHE)" = "1" ]; then extra="--no-cache"; fi; \
	for w in $(DIR_SWEEP_VALUES); do \
		echo "== dir-concurrency=$$w $$extra =="; \
		/usr/bin/time -p $(BIN_PATH) --no-banner --time-only $$extra --dir-concurrency "$$w" "$(DIR_SWEEP_PATH)"; \
	done

bench-build: | $(BIN_DIR)
	$(GOTEST) -c -o $(BENCH_BIN) ./internal/bench

bench-profile: bench-build
	$(BENCH_BIN) -test.run=^$$ -test.bench=. -test.benchmem -test.cpuprofile=$(CPU_PROFILE) -test.memprofile=$(MEM_PROFILE)
	@echo "CPU profile written to $(CPU_PROFILE)"
	@echo "Memory profile written to $(MEM_PROFILE)"
	@echo "Inspect profiles with: make pprof-web PROFILE=$(CPU_PROFILE)"

pprof-web: bench-build
	@if [ ! -f $(PROFILE) ]; then \
		echo "Profile '$(PROFILE)' not found. Run 'make bench-profile' or set PROFILE=<path>."; \
		exit 1; \
	fi
	$(GOCMD) tool pprof -http=$(PPROF_ADDR) $(BENCH_BIN) $(PROFILE)

gosec: check-gosec
	gosec -exclude=$(GOSEC_STRICT_EXCLUDES) ./...

install: build
	install -m 0755 $(BIN_PATH) $(PREFIX)/$(BINARY_NAME)

release-check:
	@echo "Release checklist for $(BINARY_NAME):"
	@echo "1. Ensure internal/buildinfo/version.go matches the tag you plan to publish."
	@echo "2. Push the release commit: git push $(REMOTE_NAME) $(RELEASE_BRANCH)"
	@echo "3. Create and push the tag: git tag -a vX.Y.Z -m 'vX.Y.Z' && git push $(REMOTE_NAME) vX.Y.Z"
	@echo "4. Verify the public install path after the tag is visible:"
	@echo '   tmpdir=$$(mktemp -d) && GOBIN="$$tmpdir" go install $(INSTALL_PKG)@latest && "$$tmpdir/$(BINARY_NAME)" --version && rm -rf "$$tmpdir"'
	@echo "Note: go install builds the tagged source directly and does not use Makefile ldflags."

release-install-check:
	@tmpdir=$$(mktemp -d); \
	trap 'rm -rf "$$tmpdir"' EXIT; \
	GOBIN="$$tmpdir" $(GOCMD) install $(INSTALL_PKG)@latest; \
	"$$tmpdir/$(BINARY_NAME)" --version

clean:
	$(GOCLEAN)
	@rm -f $(BIN_PATH) $(BENCH_BIN) *.log *.prof *.out 
