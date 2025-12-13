#! /bin/bash

# set apt proxy
echo 'Acquire::http { Proxy "http://apt.botnet:3142"; };' >> /etc/apt/apt.conf.d/02proxy

# setup docker repo
apt install ca-certificates curl
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
echo \
"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian \
 $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
tee /etc/apt/sources.list.d/docker.list > /dev/null

# Install basic debian tools and docker
apt update && apt install -y git wget build-essential libolm-dev docker-ce-cli

# set docker to use host socket
export DOCKER_HOST="tcp://10.10.199.96:2375"

# Install golang
GO_FILE=go1.25.5.linux-amd64.tar.gz

wget https://supabaseapi.butler.ooo/storage/v1/object/public/butler/static/${GO_FILE}

tar -C /usr/local -xzf ${GO_FILE}

export PATH=$PATH:/usr/local/go/bin

rm ${GO_FILE}

# Install golangci-lint
mkdir -p bin
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin
