FROM golang:alpine3.19 AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy all files
COPY . .

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Build the Go app
RUN go build -o diffy-go .

# Start a new stage from scratch
FROM alpine:latest

# Set the Current Working Directory inside the container
WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/diffy-go .

# Command to run the executable
CMD ["./diffy-go"]