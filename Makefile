.PHONY: all clean build

# Install golangci-lint if it doesn't exist
.PHONY: download-golangci-lint
download-golangci-lint:
ifeq (,$(wildcard ./bin/golangci-lint))
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s  
endif

# Install air if it doesn't exist
.PHONY: download-air
download-air:
ifeq (,$(wildcard ./bin/air))
	curl -sSfL https://raw.githubusercontent.com/air-verse/air/master/install.sh | sh -s
endif

# Download missing modules
.PHONY: tidy
tidy:
	go mod tidy

# Run go fmt
.PHONY: fmt
fmt:
	go fmt ./...

# Run linting
.PHONY: lint
lint: download-golangci-lint tidy fmt
	./bin/golangci-lint run ./pkg/... ./internal/... ./cmd/...

# Run full test
.PHONY: test
test: lint
	go test -v ./...

# Run air for test on save
.PHONY: air
air: download-golangci-lint download-air
	./bin/air

# Build everything
all: clean fmt test build

# Clean build artifacts
clean:
	rm -f ./cmd/mule/bin/mule

# Build backend
build:
	cd cmd/mule && CGO_ENABLED=1 GOOS=linux go build -o bin/mule

# Run the application
run: all
	./cmd/mule/bin/mule
