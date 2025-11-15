#! /bin/bash

# Install basic debian tools
apt update && apt install -y git wget build-essential libolm-dev

# Install golang

GO_FILE=go1.25.4.linux-amd64.tar.gz

wget https://go.dev/dl/${GO_FILE}

tar -C /usr/local -xzf go1.25.4.linux-amd64.tar.gz

export PATH=$PATH:/usr/local/go/bin

rm ${GO_FILE}

# Install golangci-lint

go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
