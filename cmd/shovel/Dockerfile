FROM golang:1.23-alpine AS builder
RUN apk add git
WORKDIR /app
COPY go.mod go.sum .
RUN go mod download
ADD . /app
RUN go build -o /app/shovel ./cmd/shovel/

FROM alpine:latest
COPY --from=builder /app/shovel /usr/local/bin
CMD ["shovel"]
