# Base Golang Image
FROM golang:latest AS builder

# Setup working directory
WORKDIR /usr/src/ocf-core

# Copy source code to
COPY . /usr/src/ocf-core

# Install Go Library & Swagger
RUN cd /usr/src/ocf-core && go get golang.org/x/text/transform \
    && go get golang.org/x/text/unicode/norm \
    && go install github.com/swaggo/swag/cmd/swag@v1.8.12

RUN go mod tidy

# Init Swagger
RUN cd /usr/src/ocf-core && swag init --parseDependency --parseInternal

RUN go mod tidy

RUN go build -o ocf /usr/src/ocf-core/main.go

FROM debian:latest

RUN apt-get update

# Install NodeJS
RUN apt-get install -y ca-certificates curl gnupg \
 && mkdir -p /etc/apt/keyrings \
 && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg

RUN NODE_MAJOR=21 \
 && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_$NODE_MAJOR.x nodistro main" | tee /etc/apt/sources.list.d/nodesource.list

RUN apt-get update \
 && apt-get install nodejs -y

# Install NPM dependencies
RUN npm install -g @marp-team/marp-core \
    && npm install -g markdown-it-include \
    && npm install -g markdown-it-container \
    && npm install -g markdown-it-attrs

RUN useradd -ms /bin/bash ocf

USER ocf
WORKDIR /home/ocf

COPY --from=builder /usr/src/ocf-core/ocf ocf
COPY --from=builder /usr/src/ocf-core/src/configuration/keymatch_model.conf src/configuration/keymatch_model.conf
COPY --from=builder /usr/src/ocf-core/token_jwt_key.pem token_jwt_key.pem


# Export ports
EXPOSE 8000/tcp
EXPOSE 443/tcp
EXPOSE 80/tcp

CMD ["./ocf"]
