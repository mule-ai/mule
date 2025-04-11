all:
	go mod tidy
	go fmt ./...
	./bin/golangci-lint run

build:
	go build -o bin/mule cmd/mule/main.go

agent:
	go build -o bin/agent cmd/agent/main.go

install:
	go install ./...

lint:
	./bin/golangci-lint run

test:
	go test ./...

clean:
	go clean
