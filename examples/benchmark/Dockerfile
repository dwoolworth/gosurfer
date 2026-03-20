# Benchmark container: measures gosurfer memory in a real Docker environment
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN apk add --no-cache upx \
    && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /benchmark ./examples/benchmark/ \
    && upx --best --lzma /benchmark

FROM alpine:3.20
RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    font-noto-emoji \
    procps \
    && rm -rf /var/cache/apk/*

ENV CHROME_BIN=/usr/bin/chromium-browser

# Non-root user
RUN adduser -D -u 1000 gosurfer
USER gosurfer
WORKDIR /home/gosurfer

COPY --from=builder /benchmark .

ENTRYPOINT ["./benchmark"]
