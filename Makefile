GOCMD = go
GOBUILD= $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
GOGET = $(GOCMD) get
BINARY_NAME = dskDitto

BENCH_BIN = ./bin/bench.test
CPU_PROFILE ?= cpu.prof
MEM_PROFILE ?= mem.prof
PROFILE ?= $(CPU_PROFILE)
PPROF_ADDR ?= localhost:6060

PREFIX = /usr/local/bin

all: test build

debug:
	# Get rid of exposed profile webserver warning for now.
	gosec -exclude=G108,G104 ./...
	$(GOBUILD) -o ./bin/$(BINARY_NAME) -v -gcflags "all=-N -l" ./cmd/$(BINARY_NAME)

build:
	gosec -exclude=G104,G108 ./...
	go build -o ./bin/dskDitto ./cmd/$(BINARY_NAME)

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
gosec:
	gosec -exclude=G104 ./...

.PHONY: install
install:
	cp ./dskDitto $(PREFIX)/dskDitto



.PHONY: clean
clean:
	$(GOCLEAN)
	rm -f ./bin/$(BINARY_NAME)
	rm -f ./bin/bench.test
	# Clear log files...
	rm -f *.log
	# Clear profiling data
	rm *.prof
