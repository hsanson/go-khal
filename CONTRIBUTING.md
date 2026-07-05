# Contributing

## Prerequisites

- Go 1.24.2 or newer
- `make`
- `golangci-lint`
- A local vdir calendar/addressbook setup is useful for manual testing

Install `golangci-lint` using the upstream instructions for your platform, or use your package manager if it provides a recent version.

## Clone And Build

```bash
git clone https://github.com/hsanson/go-khal.git
cd go-khal
make build
```

This creates a `go-khal` binary in the project root.

## Common Commands

```bash
make fmt        # format Go files with gofmt
make test       # run go test ./...
make lint       # run golangci-lint run ./...
make build      # build ./go-khal
make install    # install go-khal with go install .
make all        # fmt, test, lint, then build
make clean      # remove the local binary
```

## Code Style

Use standard Go formatting. Run `make fmt` before committing. The project does not use a custom formatter or style layer beyond `gofmt`.

Keep changes focused and prefer existing package patterns over new abstractions. Add tests when changing calendar persistence, recurrence behavior, TUI navigation, or editor workflows.

## Linting

Run `make lint` before sending changes. The default linter command is:

```bash
golangci-lint run ./...
```

There is no project-specific golangci-lint configuration yet, so the active rules are the default rules from the installed `golangci-lint` version.
