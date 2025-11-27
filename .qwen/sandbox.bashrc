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
mkdir -p bin
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin

# Install docker cli
apt install ca-certificates curl
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
echo \
"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian \
 $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
tee /etc/apt/sources.list.d/docker.list > /dev/null

apt update
apt install -y docker-ce-cli
export DOCKER_HOST="tcp://10.10.199.96:2375"
