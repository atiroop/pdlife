#!/usr/bin/env bash
# Deploy pdlife.app to the production VPS.
#
# Cross-compiles the Go binary for linux/amd64 locally. Templates
# (web/templates/*.html) are embedded into the binary via go:embed at
# build time, so nothing but the binary needs to reach the server.
# Does NOT touch nginx config or .env — those are managed separately.
#
# Usage: ./deploy.sh
# Override the SSH target with DEPLOY_HOST (defaults to the "myserver"
# entry in ~/.ssh/config).

set -euo pipefail

DEPLOY_HOST="${DEPLOY_HOST:-myserver}"
REMOTE_DIR="/home/pdlife/web/pdlife.app/public_html"
SERVICE_NAME="pdlife"
BIN_NAME="pdlife"

cd "$(dirname "$0")"

BUILD_PATH="$(mktemp -t pdlife-build.XXXXXX)"
trap 'rm -f "$BUILD_PATH"' EXIT

echo "==> Building linux/amd64 binary"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$BUILD_PATH" .

echo "==> Uploading binary"
ssh "$DEPLOY_HOST" "cat > ${REMOTE_DIR}/${BIN_NAME}.new" < "$BUILD_PATH"

echo "==> Verifying transfer"
LOCAL_SUM="$(sha256sum "$BUILD_PATH" | cut -d' ' -f1)"
REMOTE_SUM="$(ssh "$DEPLOY_HOST" "sha256sum ${REMOTE_DIR}/${BIN_NAME}.new" | cut -d' ' -f1)"
if [ "$LOCAL_SUM" != "$REMOTE_SUM" ]; then
  echo "Checksum mismatch after upload — aborting without touching the running service" >&2
  ssh "$DEPLOY_HOST" "rm -f ${REMOTE_DIR}/${BIN_NAME}.new"
  exit 1
fi

echo "==> Installing binary and restarting service"
ssh "$DEPLOY_HOST" "chmod +x ${REMOTE_DIR}/${BIN_NAME}.new && \
  mv ${REMOTE_DIR}/${BIN_NAME}.new ${REMOTE_DIR}/${BIN_NAME} && \
  chown pdlife:pdlife ${REMOTE_DIR}/${BIN_NAME} && \
  systemctl restart ${SERVICE_NAME} && \
  sleep 2 && \
  systemctl is-active ${SERVICE_NAME}"

echo "==> Smoke test"
curl -sf -o /dev/null -w '  / -> %{http_code}\n' https://pdlife.app/
curl -sf -o /dev/null -w '  /healthz -> %{http_code}\n' https://pdlife.app/healthz
curl -sf -o /dev/null -w '  /register -> %{http_code}\n' https://pdlife.app/register
curl -sf -o /dev/null -w '  /login -> %{http_code}\n' https://pdlife.app/login

echo "==> Deploy complete"
