FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/core ./cmd/core \
 && CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 go build -o /out/importer ./cmd/importer

# psql нужен для начального наполнения (sql/seed/seed.sql) в контейнере core-seed
FROM alpine:3.20
RUN apk add --no-cache postgresql16-client tzdata ca-certificates \
 && adduser -D -H core
COPY --from=builder /out/ /usr/local/bin/
COPY sql /opt/core/sql
COPY data /opt/core/data
USER core
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/core"]
