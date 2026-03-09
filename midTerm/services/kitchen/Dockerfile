FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /service main.go

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /service /service
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1
EXPOSE 8080
ENTRYPOINT ["/service"]
