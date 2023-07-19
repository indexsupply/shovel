FROM golang:1.20-alpine as builder
WORKDIR /app
COPY go.mod go.sum .
RUN go mod download
ADD . /app
RUN go build -o /app/e2pg ./cmd/e2pg/

FROM alpine:latest
COPY --from=builder /app/e2pg /usr/local/bin
CMD ["e2pg"]
