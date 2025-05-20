FROM debian:12-slim

SHELL ["/bin/bash", "-c"]

# Install deps
RUN apt-get update && apt-get install -y wget

# Install go
ENV GO_VERSION 1.24.2
RUN wget -P /tmp "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" && \
    tar -C /usr/local -xzf "/tmp/go${GO_VERSION}.linux-amd64.tar.gz" && \
    rm "/tmp/go${GO_VERSION}.linux-amd64.tar.gz"
ENV PATH /go/bin:/usr/local/go/bin:$PATH

COPY . .

# Build client
RUN go build .

ENTRYPOINT ["./bittorrent-cli"]
