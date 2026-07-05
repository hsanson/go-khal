BINARY := go-khal
GO := go
GOFMT := gofmt
GOLANGCI_LINT := golangci-lint
PACKAGES := ./...

.PHONY: all build install test lint fmt fmt-check clean

all: fmt test lint build

build:
	$(GO) build -o $(BINARY) .

install:
	$(GO) install .

test:
	$(GO) test $(PACKAGES)

lint:
	$(GOLANGCI_LINT) run $(PACKAGES)

fmt:
	$(GOFMT) -w $$(find . -name '*.go' -not -path './.git/*')

fmt-check:
	@test -z "$$($(GOFMT) -l $$(find . -name '*.go' -not -path './.git/*'))" || \
		(echo "gofmt is required for:"; $(GOFMT) -l $$(find . -name '*.go' -not -path './.git/*'); exit 1)

clean:
	rm -f $(BINARY)
