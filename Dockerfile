# Base Golang Image
FROM golang:latest

# Setup working directory
WORKDIR /usr/src/osf-core

# Install Git and NodeJS
RUN curl -sL https://deb.nodesource.com/setup_16.x | bash -
RUN apt-get install -y nodejs npm

# Install NPM dependencies
RUN npm install -g @marp-team/marp-core \
    && npm install -g markdown-it-include \
    && npm install -g markdown-it-container \
    && npm install -g markdown-it-attrs

# Install Go Library & Swagger
RUN cd /usr/src/osf-core && go get golang.org/x/text/transform \
    && go get golang.org/x/text/unicode/norm \
    && go install github.com/swaggo/swag/cmd/swag@v1.8.12

# Copy source code to
COPY . /usr/src/osf-core

# Init Swagger
RUN cd /usr/src/osf-core && swag init --parseDependency --parseInternal

RUN go mod tidy

# Export ports
EXPOSE 8000/tcp
EXPOSE 443/tcp
EXPOSE 80/tcp

# Launch the API
CMD ["go", "run", "/usr/src/osf-core/main.go"]

