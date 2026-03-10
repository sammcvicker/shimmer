.PHONY: build test clean install

build:
	go build -o shimmer ./cmd/shimmer

test:
	go test ./... -v

clean:
	rm -f shimmer

install:
	go install ./cmd/shimmer
