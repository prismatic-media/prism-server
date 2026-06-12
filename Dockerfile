# ---- frontend build stage ----
FROM node:22-alpine AS web-builder

WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- backend build stage ----
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Copy the compiled Angular output from the web-builder stage so it is
# available for //go:embed at compile time.
COPY --from=web-builder /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /prism ./cmd/server

# ---- runtime stage ----
FROM alpine:3.20

# ffmpeg for transcoding; ca-certificates for TMDB API calls; tzdata for time zones
RUN apk add --no-cache ffmpeg ca-certificates tzdata

COPY --from=builder /prism /usr/local/bin/prism

# Migrations are embedded in the binary — no external files needed.

EXPOSE 8080

ENTRYPOINT ["prism"]
