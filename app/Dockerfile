# --- Build Stage ---
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Download dependencies first to take advantage of Docker caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build a statically linked binary stripped of debugging information
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/main main.go

# --- Runtime Stage ---
FROM alpine:3.21

# Upgrade packages to get security fixes
RUN apk update && apk upgrade --no-cache

# Create a non-root user and group
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /home/appuser

# Copy the statically compiled binary from the builder stage
COPY --from=builder /app/main .

# Change ownership to the non-root user
RUN chown appuser:appgroup main

# Run as the non-root user
USER appuser

# Set runtime configurations
ENV PORT=8080
ENV CONTAINERIZED=true
ENV ENVIRONMENT=production

# Expose port
EXPOSE 8080

# Command to execute
ENTRYPOINT ["./main"]
