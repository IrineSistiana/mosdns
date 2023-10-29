# syntax=docker/dockerfile:1

FROM golang:1.21 as build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -v -o /mosdns

FROM alpine as main

COPY --from=build /mosdns /
RUN mkdir -p /config
CMD ["/mosdns", "-d", "/config", "start", "-c", "config.yaml"]
