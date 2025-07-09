GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=dskDitto

PREFIX=/usr/local/bin

all: test build

debug:
	$(GOBUILD) -o $(BINARY_NAME) -v -gcflags "all=-N -l"

build:
	gosec -exclude=G104 ./...
	go build -o ./bin/dskDitto ./cmd/$(BINARY_NAME)

test:
	$(GOTEST) -v ./...

bench:
	$(GOTEST) -bench=. -benchmem ./internal/bench/


.PHONY: gosec
gosec:
	gosec -exclude=G104 ./...

.PHONY: install
install:
	cp ./dskDitto $(PREFIX)/dskDitto

# flamegraph: profile
# 	pprof -raw ./internal/bench/benchmarks.test cpu.prof | inferno-flamegraph > flamegraph.svg
# 	@echo "Generated flamegraph.svg"

# fuzz:
# 	$(GOTEST) -fuzz=Fuzz -fuzztime=10s ./internal/...

.PHONY: clean
clean:
	$(GOCLEAN)
	rm -f ./bin/$(BINARY_NAME)
	# Clear log files...
	rm -rf app.log
