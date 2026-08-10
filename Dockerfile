# syntax=docker/dockerfile:1.7

ARG NODE_VERSION=22
ARG GO_VERSION=1.26.2

FROM node:${NODE_VERSION}-bookworm-slim AS web-builder
WORKDIR /workspace
RUN apt-get update \
  && apt-get install --yes --no-install-recommends git python3 make g++ \
  && rm -rf /var/lib/apt/lists/* \
  && corepack enable \
  && corepack prepare pnpm@10.17.0 --activate
COPY . .
RUN pnpm install --frozen-lockfile
RUN pnpm build:web

FROM golang:${GO_VERSION}-bookworm AS api-builder
WORKDIR /workspace/services/tantan-api
COPY services/tantan-api/go.mod services/tantan-api/go.sum ./
RUN go mod download
COPY services/tantan-api ./
ARG TANTAN_VERSION=dev
RUN CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags="-s -w -X main.buildVersion=${TANTAN_VERSION}" \
  -o /out/tantan-api \
  ./cmd/tantan-api

FROM litestream/litestream:0.5.14 AS litestream

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
  && apt-get install --yes --no-install-recommends ca-certificates curl tzdata \
  && rm -rf /var/lib/apt/lists/* \
  && groupadd --gid 10001 tantan \
  && useradd --uid 10001 --gid 10001 --no-create-home --home-dir /var/lib/tantan tantan \
  && mkdir -p /app/static /var/lib/tantan \
  && chown -R 10001:10001 /app /var/lib/tantan

COPY --from=api-builder --chown=10001:10001 /out/tantan-api /app/tantan-api
COPY --from=web-builder --chown=10001:10001 /workspace/apps/desktop/out/web /app/static
COPY --from=litestream /usr/local/bin/litestream /usr/local/bin/litestream
COPY --chown=10001:10001 deploy/cloudflare/litestream.yml /etc/litestream.yml
COPY --chown=10001:10001 deploy/cloudflare/container-entrypoint.sh /app/container-entrypoint.sh

RUN chmod 0555 /app/container-entrypoint.sh /app/tantan-api /usr/local/bin/litestream \
  && chmod 0444 /etc/litestream.yml

ENV HOME=/var/lib/tantan \
  TANTAN_DATA_DIR=/var/lib/tantan \
  TANTAN_STATIC_DIR=/app/static \
  TANTAN_LISTEN_ADDR=127.0.0.1:3000

USER 10001:10001
VOLUME ["/var/lib/tantan"]
EXPOSE 8080
ENTRYPOINT ["/app/container-entrypoint.sh"]
CMD ["serve"]
