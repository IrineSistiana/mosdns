FROM golang:latest as builder
ARG CGO_ENABLED=0

COPY ./ /root/src/
WORKDIR /root/src/
RUN go build -ldflags "-s -w -X main.version=$(git describe --tags --long --always)" -trimpath -o mosdns

FROM alpine:latest

COPY --from=builder /root/src/mosdns /usr/bin/

RUN apk add --no-cache ca-certificates