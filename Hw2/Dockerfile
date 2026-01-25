# Start from the official Go image
FROM golang:1.25-alpine

# Set working directory inside container
WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the app
COPY . .

# Build the binary
RUN go build -o server

# Expose port 8080
EXPOSE 8080

# Start the server
CMD ["./server"]
