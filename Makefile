GOCMD = go
GOBUILD= $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
GOGET = $(GOCMD) get
BINARY_NAME = dskDitto
GUI_PATH ?= .
DEFAULT_VERSION = 0.4.1
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || printf '%s' $(DEFAULT_VERSION))
VERSION_LDFLAGS = -X github.com/jdefrancesco/dskDitto/internal/buildinfo.Version=$(VERSION)
INSTALL_PKG = github.com/jdefrancesco/dskDitto/cmd/$(BINARY_NAME)
REMOTE_NAME ?= origin
RELEASE_BRANCH ?= master

BENCH_BIN = ./bin/bench.test
CPU_PROFILE ?= cpu.prof
MEM_PROFILE ?= mem.prof
PROFILE ?= $(CPU_PROFILE)
PPROF_ADDR ?= localhost:6060

PREFIX = /usr/local/bin

all: test build

.PHONY: security-scan
security-scan:
	@if command -v gosec >/dev/null 2>&1; then \
		gosec -exclude=G104,G108 ./...; \
	else \
		echo "Skipping gosec scan: gosec not installed"; \
	fi

.PHONY: check-gosec
check-gosec:
	@command -v gosec >/dev/null 2>&1 || \
		(echo '\n`gosec` must be in $$PATH to build this!\n'; exit 1)

debug: security-scan
	# Get rid of exposed profile webserver warning for now.
	$(GOBUILD) -ldflags "$(VERSION_LDFLAGS)" -o ./bin/$(BINARY_NAME) -v -gcflags "all=-N -l" ./cmd/$(BINARY_NAME)

build: security-scan
	$(GOBUILD) -ldflags "$(VERSION_LDFLAGS)" -o ./bin/dskDitto ./cmd/$(BINARY_NAME)

.PHONY: build-gui
build-gui: security-scan
	CGO_ENABLED=1 $(GOBUILD) -ldflags "$(VERSION_LDFLAGS)" -o ./bin/$(BINARY_NAME) ./cmd/$(BINARY_NAME)
	@echo "GUI-capable binary built at ./bin/$(BINARY_NAME)"
	@echo "Run it with: ./bin/$(BINARY_NAME) --gui <path>"

.PHONY: run-gui
run-gui: build-gui
	./bin/$(BINARY_NAME) --gui $(GUI_PATH)

.PHONY: build-darwin-arm64
build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -ldflags "$(VERSION_LDFLAGS)" -o ./bin/dskDitto ./cmd/$(BINARY_NAME)

.PHONY: test
test:
	$(GOTEST) -v ./...

.PHONY: bench
bench:
	$(GOTEST) -bench=. -benchmem ./internal/bench/


.PHONY: bench-build
bench-build:
	mkdir -p $(dir $(BENCH_BIN))
	$(GOTEST) -c -o $(BENCH_BIN) ./internal/bench

.PHONY: bench-profile
bench-profile: bench-build
	$(BENCH_BIN) -test.run=^$$ -test.bench=. -test.benchmem -test.cpuprofile=$(CPU_PROFILE) -test.memprofile=$(MEM_PROFILE)
	@echo "CPU profile written to $(CPU_PROFILE)"
	@echo "Memory profile written to $(MEM_PROFILE)"
	@echo "Inspect profiles with: make pprof-web PROFILE=$(CPU_PROFILE)"

.PHONY: pprof-web
pprof-web: bench-build
	@if [ ! -f $(PROFILE) ]; then \
		echo "Profile '$(PROFILE)' not found. Run 'make bench-profile' or set PROFILE=<path>."; \
		exit 1; \
	fi
	go tool pprof -http=$(PPROF_ADDR) $(BENCH_BIN) $(PROFILE)


.PHONY: gosec
gosec: check-gosec
	gosec -exclude=G104 ./...

.PHONY: install
install: build
	install -m 0755 ./bin/$(BINARY_NAME) $(PREFIX)/$(BINARY_NAME)


.PHONY: release-check
release-check:
	@echo "Release checklist for $(BINARY_NAME):"
	@echo "1. Ensure internal/buildinfo/version.go matches the tag you plan to publish."
	@echo "2. Push the release commit: git push $(REMOTE_NAME) $(RELEASE_BRANCH)"
	@echo "3. Create and push the tag: git tag -a vX.Y.Z -m 'vX.Y.Z' && git push $(REMOTE_NAME) vX.Y.Z"
	@echo "4. Verify the public install path after the tag is visible:"
	@echo '   tmpdir=$$(mktemp -d) && GOBIN="$$tmpdir" go install $(INSTALL_PKG)@latest && "$$tmpdir/$(BINARY_NAME)" --version && rm -rf "$$tmpdir"'
	@echo "Note: go install builds the tagged source directly and does not use Makefile ldflags."

.PHONY: release-install-check
release-install-check:
	@tmpdir=$$(mktemp -d); \
	trap 'rm -rf "$$tmpdir"' EXIT; \
	GOBIN="$$tmpdir" $(GOCMD) install $(INSTALL_PKG)@latest; \
	"$$tmpdir/$(BINARY_NAME)" --version



.PHONY: clean
clean:
	$(GOCLEAN)
	@if [ -e ./bin/$(BINARY_NAME) ]; then rm ./bin/$(BINARY_NAME); fi
	@if [ -e ./bin/bench.test ]; then rm ./bin/bench.test; fi
	@if ls *.log >/dev/null 2>&1; then rm *.log; fi
	@if ls *.prof >/dev/null 2>&1; then rm *.prof; fi
