FROM golang:buster as builder

RUN apt-get update && apt-get install -y ca-certificates

ENV GO111MODULE=on \
    CGO_ENABLED=1  \
    GOOS=linux     \
    GOARCH=amd64

WORKDIR /build

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .

RUN go build -o filecoin-chain-archiver ./cmd/filecoin-chain-archiver

FROM debian:buster

COPY --from=builder /etc/ssl/certs                 /etc/ssl/certs
COPY --from=builder /build/filecoin-chain-archiver /usr/local/bin

ENTRYPOINT ["/usr/local/bin/filecoin-chain-archiver"]
CMD ["-help"]
