# syntax=docker/dockerfile:1

FROM golang:1.27.3-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/the-bebot ./cmd/bot

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/the-bebot /app/the-bebot

USER nonroot:nonroot

ENTRYPOINT ["/app/the-bebot"]
