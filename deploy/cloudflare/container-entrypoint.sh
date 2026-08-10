#!/bin/sh
set -eu

if [ "${TANTAN_CLOUDFLARE_CONTAINER:-false}" = "true" ]; then
  umask 077
  : "${TANTAN_REPLICA_BUCKET:?TANTAN_REPLICA_BUCKET is required}"
  : "${TANTAN_REPLICA_ENDPOINT:?TANTAN_REPLICA_ENDPOINT is required}"
  : "${TANTAN_REPLICA_PATH:?TANTAN_REPLICA_PATH is required}"
  : "${AWS_ACCESS_KEY_ID:?AWS_ACCESS_KEY_ID is required}"
  : "${AWS_SECRET_ACCESS_KEY:?AWS_SECRET_ACCESS_KEY is required}"
  /usr/local/bin/litestream restore \
    -config /etc/litestream.yml \
    -if-db-not-exists \
    -if-replica-exists \
    /var/lib/tantan/tantan.sqlite
  /app/tantan-api migrate --data-dir /var/lib/tantan
  exec /usr/local/bin/litestream replicate -config /etc/litestream.yml
fi

exec /app/tantan-api "$@"
