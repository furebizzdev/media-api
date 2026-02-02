# Stage 1: Build the Go binary
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN go build -o media-api ./cmd/server/main.go

# Stage 2: Create the final lightweight image
FROM alpine:latest

WORKDIR /app

# Install dependencies (ffmpeg is crucial, python3 for yt-dlp)
RUN apk add --no-cache ffmpeg python3 curl ca-certificates

# Download yt-dlp
RUN curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp && \
    chmod +x /usr/local/bin/yt-dlp

# Copy binary from builder
COPY --from=builder /app/media-api .

# Copy web assets
COPY --from=builder /app/web ./web

# Create downloads directory
RUN mkdir -p downloads

# Expose port (Render uses 10000 by default sometimes, but we bind 3000. 
# Render sets PORT env var. We should update main.go to respect PORT env var technically, 
# but for now we expose 3000 and tell Render to listen on 3000.)
EXPOSE 3000

# Command to run
CMD ["./media-api"]
