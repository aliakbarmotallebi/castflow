FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /castflow ./cmd/castflow
RUN CGO_ENABLED=0 go build -o /worker ./cmd/worker

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates
WORKDIR /app
COPY --from=builder /castflow /app/castflow
COPY --from=builder /worker /app/worker
COPY web/player /app/web/player
COPY migrations /app/migrations
ENV CASTFLOW_HTTP_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/app/castflow"]
