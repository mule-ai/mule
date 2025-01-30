.PHONY: all clean build

# Install golangci-lint if it doesn't exist
.PHONY: download-golangci-lint
download-golangci-lint:
ifeq (,$(wildcard ./bin/golangci-lint))
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v1.63.4 
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

# Run linting
.PHONY: lint
lint: download-golangci-lint tidy
	./bin/golangci-lint run

# Run full test
.PHONY: test
test: lint
	go test -v ./...

# Run air for test on save
.PHONY: air
air: download-golangci-lint download-air
	./bin/air

# Build everything
all: clean build

# Clean build artifacts
clean:
	rm -f dev-team

# Build backend
build:
	cd cmd/dev-team && CGO_ENABLED=0 GOOS=linux go build -o bin/dev-team

# Run the application
run: all
	./cmd/dev-team/bin/dev-team