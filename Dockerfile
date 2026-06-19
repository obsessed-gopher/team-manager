FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app ./cmd/teammanager

FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app /app/teammanager
COPY config ./config
COPY migrations ./migrations

EXPOSE 8080

ENV TM_CONFIG_PATH=/app/config/docker.yml

ENTRYPOINT ["/app/teammanager"]
