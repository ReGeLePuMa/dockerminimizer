#!/usr/bin/env bash

#Install Docker
install_docker() {
    if command -v docker >/dev/null 2>&1; then
        echo "Docker is already installed."
        return
    fi
    sudo apt update && \
    sudo apt install -y uidmap iptables && \
    curl -fsSL https://get.docker.com/rootless | sh
}

#Install Go
install_go() {
    if command -v go >/dev/null 2>&1; then
        echo "Go is already installed."
        return
    fi
    wget https://go.dev/dl/go1.24.1.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go1.24.1.linux-amd64.tar.gz
    echo "export PATH=$PATH:/usr/local/go/bin" >> ~/.bashrc
    source ~/.bashrc
    rm go1.24.1.linux-amd64.tar.gz
}

#Install strace
install_strace() {
    sudo apt update && \ 
    sudo apt install -y make gcc && \
    git clone https://github.com/strace/strace.git && \
    cd strace && \
    ./bootstrap && \
    ./configure LDFLAGS="-static -pthread" && \
    make -j$(nproc) && \
    sudo make install && \
    cd .. && \
    rm -rf strace
}

#Install dockerminimizer
install_dockerminimizer() {
    go get .
    go build -o dockerminimizer
    sudo install -o root -g root -m 0755 dockerminimizer /usr/local/bin/dockerminimizer
}

main() {
    CURRENT_DIR=$(pwd)
    install_docker
    install_go
    install_strace
    install_dockerminimizer
    cd .. && rm -rf $CURRENT_DIR
}

main
