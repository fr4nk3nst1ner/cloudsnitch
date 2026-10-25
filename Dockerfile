# Use the official Go image as a builder
FROM golang:1.24-alpine AS builder

# Set the working directory
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application from the main package
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/cloudsnitch .

# Use a minimal alpine image for the final container
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Set working directory in the final container
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/cloudsnitch .

# Ensure the binary has execute permissions
RUN chmod +x /app/cloudsnitch

# Set the entrypoint
ENTRYPOINT ["/app/cloudsnitch"]
