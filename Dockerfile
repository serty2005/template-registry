FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/templates-registry ./cmd/server

FROM alpine:3.22

WORKDIR /app

RUN addgroup -S app && adduser -S app -G app && apk add --no-cache ca-certificates

COPY --from=builder /out/templates-registry /app/templates-registry
COPY migrations /app/migrations

USER app

EXPOSE 8000

ENTRYPOINT ["/app/templates-registry"]
