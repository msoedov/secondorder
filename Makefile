BINARY_NAME=secondorder
BUILD_DIR=.

.PHONY: build test run clean lint scan install i

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/secondorder

test:
	go test ./...

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
