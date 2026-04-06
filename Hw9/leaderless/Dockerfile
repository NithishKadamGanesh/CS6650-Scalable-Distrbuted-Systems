FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod .
COPY leaderless/ ./leaderless/
RUN cd leaderless && go build -o /app/node main.go

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/node .
EXPOSE 8080
CMD ["./node"]
