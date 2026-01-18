# Build stage
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# Copy source code
COPY . .

# Download dependencies (allow toolchain upgrade)
ENV GOTOOLCHAIN=auto
RUN go mod tidy && go mod download

# Build the binary
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags '-linkmode external -extldflags "-static"' -o slimdeploy ./cmd/slimdeploy

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache \
    docker-cli \
    docker-cli-compose \
    git \
    openssh-client \
    ca-certificates \
    tzdata

WORKDIR /app

# Create directories
RUN mkdir -p /app/data /app/deployments /app/.ssh

# Copy binary from builder
COPY --from=builder /app/slimdeploy /app/slimdeploy

# Set permissions
RUN chmod +x /app/slimdeploy

# Environment variables
ENV DATA_DIR=/app/data
ENV DEPLOYMENTS_DIR=/app/deployments
ENV LISTEN_ADDR=:8080

EXPOSE 8080

CMD ["/app/slimdeploy"]
