# --- Go build stage ---
FROM golang:1.25-alpine AS go-builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=1
RUN go build -ldflags="-s -w" -o /beacon ./cmd/server

# --- Runtime ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=go-builder /beacon /app/beacon
COPY web/templates /app/web/templates
RUN mkdir -p /app/data /app/web/static
EXPOSE 8080
ENTRYPOINT ["/app/beacon"]
