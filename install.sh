#!/usr/bin/env bash

#Install Docker
install_docker() {
    if command -v docker >/dev/null 2>&1; then
        echo "Docker is already installed."
        return
    fi
    sudo apt update && \
    sudo apt install -y uidmap iptables && \
    curl -fsSL https://get.docker.com/| sudo sh && \
    dockerd-rootless-setuptool.sh install
}

#Install Go
install_go() {
    if command -v go >/dev/null 2>&1; then
        echo "Go is already installed."
        return
    fi
    wget https://go.dev/dl/go1.24.1.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go1.24.1.linux-amd64.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo "export PATH=$PATH:/usr/local/go/bin" | sudo tee -a /etc/profile
    source /etc/profile
    rm go1.24.1.linux-amd64.tar.gz
}

#Install strace
install_strace() {
    read -r -p "Compile a static strace binary? [y/N] " response
    case "$response" in
        [yY][eE][sS]|[yY])
            echo "Compiling a static strace binary..."
            ;;
        *)
            echo "Skipping strace compilation."
            return
            ;;
    esac
    sudo apt update && \ 
    sudo apt install -y make gcc gawk autoconf gcc-multilib g++-multilib libc6-dev-i386 && \
    git clone https://github.com/strace/strace.git tmp && \
    cd tmp && \
    ./bootstrap && \
    ./configure LDFLAGS="-static -pthread" && \
    make -j$(nproc) && \
    sudo make install && \
    cd .. && \
    rm -rf tmp
}

#Install dockerminimizer
install_dockerminimizer() {
    go get .
    go build -o dockerminimizer ./cmd
    sudo install -o root -g root -m 0755 dockerminimizer /usr/local/bin/dockerminimizer
}

main() {
    install_docker
    install_go
    install_strace
    install_dockerminimizer
}

main
