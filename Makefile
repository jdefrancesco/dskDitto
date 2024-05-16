GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=dskDitto

PREFIX=/usr/local/bin

all: test build

build:
	gosec -exclude=G104 ./...
	$(GOBUILD) -o $(BINARY_NAME) -v

test:
	$(GOTEST) -v ./...

.PHONY: gosec
gosec:
	gosec -exclude=G104 ./...

.PHONY: install
install:
	cp ./dskDitto $(PREFIX)/dskDitto

.PHONY: clean
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	# Clear log files...
	rm dskditto-*

