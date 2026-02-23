FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY . .

RUN go mod tidy
RUN go build -o app

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/app .

EXPOSE 8080
CMD ["./app"]