FROM golang:1.19-bullseye

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
RUN go get golang.org/x/text/transform golang.org/x/text/unicode/norm github.com/swaggo/swag && \
    go install github.com/swaggo/swag/cmd/swag@latest

COPY . .
# Create .env aside main.go to prevent errors while running the code
COPY .env.dist .env

# generate the docs directory
RUN swag init --parseDependency --parseInternal

ENTRYPOINT ["/usr/bin/tini", "--"]

CMD ["go", "run", "main.go"]
