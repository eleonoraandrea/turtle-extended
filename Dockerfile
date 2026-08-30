FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -o /atps ./cmd/atps
RUN go build -tags live -o /atps-live ./cmd/atps

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /atps /usr/local/bin/atps
COPY --from=builder /atps-live /usr/local/bin/atps-live
COPY configs/default.yaml /app/configs/default.yaml
WORKDIR /app
EXPOSE 8000
ENTRYPOINT ["atps"]
CMD ["--help"]
