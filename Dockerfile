# Base Golang Image
FROM golang:1.27-trixie AS builder

# Setup working directory
WORKDIR /usr/src/ocf-core

# Dep layer — cached unless go.mod/go.sum change
COPY go.mod go.sum ./
RUN go mod download

# Tooling layer — cached unless version bumps
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

# Source layer — invalidates on every commit, but deps above are cached
COPY . .

# Init Swagger
RUN swag init --parseDependency --parseInternal

RUN go build -o ocf main.go

# Same Debian release as the builder: the binary links the builder's glibc (cgo).
FROM debian:13-slim

# CA bundle for outbound TLS (Stripe, SMTP, Casdoor); zone data for TZ handling.
# hadolint ignore=DL3008
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates tzdata \
 && rm -rf /var/lib/apt/lists/*

RUN useradd -ms /bin/bash ocf

USER ocf
WORKDIR /home/ocf

COPY --from=builder /usr/src/ocf-core/ocf ocf
COPY --from=builder /usr/src/ocf-core/src/configuration/keymatch_model.conf src/configuration/keymatch_model.conf
COPY --from=builder /usr/src/ocf-core/token_jwt_key.pem token_jwt_key.pem
COPY --from=builder /usr/src/ocf-core/VERSION VERSION


# Export ports
EXPOSE 8000/tcp
EXPOSE 443/tcp
EXPOSE 80/tcp

CMD ["./ocf"]
