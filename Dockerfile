# ---- build ----
FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/lampa-sync-server .

# ---- runtime ----
FROM alpine:3.21 AS runner
WORKDIR /app

ENV PORT=3000 \
    DATA_DIR=/app/data

RUN apk add --no-cache ca-certificates su-exec \
 && addgroup -S apps && adduser -S apps -G apps

COPY --from=build /out/lampa-sync-server /usr/local/bin/lampa-sync-server
COPY public ./public
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod +x /usr/local/bin/docker-entrypoint.sh \
 && mkdir -p /app/data \
 && chown -R apps:apps /app

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:3000/health >/dev/null || exit 1

ENTRYPOINT ["docker-entrypoint.sh"]
