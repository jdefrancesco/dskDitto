GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=dskDitto

PREFIX=/usr/local/bin

all: test build

build:
	$(GOBUILD) -o $(BINARY_NAME) -v

test:
	$(GOTEST) -v ./...

.PHONY: install
install:
	cp ./dskDitto $(PREFIX)/dskDitto

.PHONY: clean
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	# Clear log files...
	rm dskditto-*

