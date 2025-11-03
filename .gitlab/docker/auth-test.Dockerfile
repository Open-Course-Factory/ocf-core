# Custom Docker image for auth tests
# Pre-installs Go, Docker Compose, and required utilities to speed up CI runs
FROM docker:24-dind

# Install required packages
RUN apk add --no-cache \
    docker-compose \
    postgresql-client \
    wget \
    bash

# Install Go 1.24.1
RUN wget https://go.dev/dl/go1.24.1.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.24.1.linux-amd64.tar.gz && \
    rm go1.24.1.linux-amd64.tar.gz

# Set up Go environment
ENV PATH=$PATH:/usr/local/go/bin:/go/bin
ENV GOPATH=/go

# Verify installation
RUN go version

WORKDIR /workspace
