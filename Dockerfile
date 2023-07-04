FROM golang:1.19-bullseye as builder

RUN apt update && \
    apt install tini nano npm -y

RUN npm install @marp-team/marp-core && \
    npm install markdown-it-include --save && \
    npm install markdown-it-container --save && \
    npm install markdown-it-attrs --save

ENV GOPATH="$HOME/go"
ENV PATH="$PATH:$GOPATH/bin"

WORKDIR /app

COPY go.sum go.mod ./

RUN go mod tidy && go mod download

# downloading the swaggo/swag package to fix the issue: unknown field 'LeftDelim'
# issue: https://github.com/swaggo/swag/issues/1568
RUN go get golang.org/x/text/transform golang.org/x/text/unicode/norm && \
    go install github.com/swaggo/swag/cmd/swag@latest
#    go get -u github.com/swaggo/swag && \

COPY src src
COPY main.go .

# track only the actual changed go code while creating the docs directory
RUN swag init --parseDependency --parseInternal

# copy remaining entire directory in case other directories are needed for the build
COPY . .
RUN go build -v -o /app/app-linux-amd64
# Create .env aside main.go to prevent errors while running the code
COPY .env.dist .env

ENTRYPOINT ["/usr/bin/tini", "--"]

CMD [ "/app/app-linux-amd64" ]

FROM debian:bookworm-slim as slim

ENV TINI_VERSION v0.19.0

ADD https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini /tini

RUN chmod +x /tini

WORKDIR /app

ARG UID=1000
ARG GID=1000

RUN groupadd -g "${GID}" app \
  && useradd --create-home --no-log-init -u "${UID}" -g "${GID}" app

COPY --from=builder /app /app
# RUN rm -rf node_modules/ # unused as listed in .dockerignore
RUN chown -R app:app /app

USER app
ENTRYPOINT ["/tini", "--"]

CMD [ "/app/app-linux-amd64" ]
