.PHONY: all clean build

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