FROM golang:1.24.1-alpine as builder1

WORKDIR /app
COPY . .
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64
RUN go get . && \
    go build -o dockerminimizer ./cmd

FROM ubuntu:22.04 as builder2

RUN apt update && \
    apt install -y git gawk make gcc autoconf gcc-multilib g++-multilib libc6-dev-i386 && \
    git clone  https://github.com/strace/strace.git tmp && \
    cd tmp && \
    ./bootstrap && \
    ./configure LDFLAGS="-static -pthread" && \
    make -j$(nproc) && \
    make install

FROM alpine:3.20

RUN apk update && \
    apk add --no-cache docker

COPY --from=builder1 /app/dockerminimizer /usr/local/bin/dockerminimizer
COPY --from=builder2 /usr/local/bin/strace /usr/local/bin/strace
WORKDIR /app
ENTRYPOINT [ "/usr/local/bin/dockerminimizer" ]
CMD [ "--help" ]