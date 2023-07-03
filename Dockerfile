FROM golang:1.19-bullseye

WORKDIR /app

RUN export GOPATH=$HOME/go
RUN export PATH=$PATH:$GOPATH/bin

COPY go.sum go.mod ./

RUN go mod tidy && go mod download
RUN apt update && apt install tini nano npm -y

RUN go get golang.org/x/text/transform && \
    go get golang.org/x/text/unicode/norm && \
    go get -u github.com/swaggo/swag && \
    go install github.com/swaggo/swag/cmd/swag@latest

RUN npm install @marp-team/marp-core && \
    npm install markdown-it-include --save && \
    npm install markdown-it-container --save && \
    npm install markdown-it-attrs --save

COPY . .
COPY .env.dist .env

RUN swag init --parseDependency --parseInternal

ENTRYPOINT ["/usr/bin/tini", "--"]

CMD ["go", "run", "main.go"]
