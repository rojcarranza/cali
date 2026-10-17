.PHONY: install uninstall build test lint

# Build cali for this machine and put it on PATH (macOS, Linux, WSL, Git Bash).
install:
	sh scripts/install.sh

uninstall:
	sh scripts/install.sh --uninstall

build:
	go build -o bin/cali ./cmd/cali

test:
	go test -v ./...

lint:
	go vet ./...
	golangci-lint run
