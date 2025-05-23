.PHONY: build clean install test coverage

BINARY_NAME=gogitup
GO=go
COVERAGE_FILE=coverage.out

build:
	$(GO) build -o $(BINARY_NAME) ./cmd/gogitup

clean:
	rm -f $(BINARY_NAME)
	rm -f $(COVERAGE_FILE)
	$(GO) clean

install: build
	mv $(BINARY_NAME) $(GOPATH)/bin/

test:
	$(GO) test -v -race -coverprofile=$(COVERAGE_FILE) ./...

coverage: test
	$(GO) tool cover -html=$(COVERAGE_FILE)

lint:
	$(GO) vet ./...

all: clean build 
