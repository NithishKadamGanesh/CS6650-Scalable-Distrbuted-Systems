FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod .
COPY leader-follower/ ./leader-follower/
RUN cd leader-follower && go build -o /app/node main.go

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/node .
EXPOSE 8080
CMD ["./node"]
