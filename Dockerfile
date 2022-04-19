FROM golang:buster as builder

ENV GO111MODULE=on \
    CGO_ENABLED=1  \
    GOOS=linux     \
    GOARCH=amd64

WORKDIR /build

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .

RUN go build -o filsnap ./cmd/filsnap

FROM debian:buster

COPY --from=builder /build/filsnap /usr/local/bin

ENTRYPOINT ["/usr/local/bin/filsnap"]
CMD ["-help"]
