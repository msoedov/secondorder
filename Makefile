BINARY_NAME=mesa
BUILD_DIR=.
BUILD_DATE=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-X github.com/msoedov/mesa/internal/models.BuildDate=$(BUILD_DATE)

.PHONY: build test run clean lint scan install i

build:
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/mesa

test:
	go test ./...
t: test

run: build
	./$(BINARY_NAME)

install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

i: install

clean:
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	go clean

lint:
	golangci-lint run ./...

gl:
	gitleaks detect --source . -v
