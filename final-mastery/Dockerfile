FROM golang:1.24 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/final-mastery .

FROM debian:bookworm-slim

WORKDIR /app
ENV DATA_DIR=/app/data
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/final-mastery /app/final-mastery

EXPOSE 8000

CMD ["/app/final-mastery"]
