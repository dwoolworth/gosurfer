# Stage 1: Build the Go binary
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git upx
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /gosurfer-example ./examples/search/ \
    && upx --best --lzma /gosurfer-example

# Stage 2: Runtime (~945MB with Chromium + shared libs)
FROM alpine:3.20
RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    font-noto-emoji \
    && rm -rf /var/cache/apk/*

# Chromium flags for containerized usage
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV CHROME_FLAGS="--no-sandbox --disable-dev-shm-usage --disable-gpu --headless"

# Create non-root user
RUN adduser -D -u 1000 gosurfer
USER gosurfer
WORKDIR /home/gosurfer

COPY --from=builder /gosurfer-example .

ENTRYPOINT ["./gosurfer-example"]
