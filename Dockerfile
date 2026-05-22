# ---- build stage ----
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /galactic ./cmd/server

# ---- runtime stage ----
FROM alpine:3.20

# ffmpeg for transcoding; ca-certificates for TMDB API calls; tzdata for time zones
RUN apk add --no-cache ffmpeg ca-certificates tzdata

COPY --from=builder /galactic /usr/local/bin/galactic

# Migrations are embedded in the binary — no external files needed.

EXPOSE 8080

ENTRYPOINT ["galactic"]
