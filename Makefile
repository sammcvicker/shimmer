.PHONY: build test clean install

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-X github.com/sammcvicker/shimmer/internal/cmd.version=$(VERSION)" -o shimmer ./cmd/shimmer

test:
	go test ./... -v

clean:
	rm -f shimmer

install:
	go install -ldflags "-X github.com/sammcvicker/shimmer/internal/cmd.version=$(VERSION)" ./cmd/shimmer
