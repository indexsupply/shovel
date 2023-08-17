FROM golang:1.21-alpine as builder
RUN apk add git
WORKDIR /app
COPY go.mod go.sum .
RUN go mod download
ADD . /app
RUN go build -o /app/e2pg ./cmd/e2pg/

FROM alpine:latest
COPY --from=builder /app/e2pg /usr/local/bin
CMD ["e2pg"]
