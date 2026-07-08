FROM --platform=$BUILDPLATFORM golang:1.26 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o app ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /app/app /app/app
COPY --from=builder /app/sql/schema /app/sql/schema
COPY --from=builder /app/config /app/config

ENV MIGRATE_PATH=/app/sql/schema
ENV CONFIG_PATH=/app/config/env.yaml

ENTRYPOINT ["/app/app"]
