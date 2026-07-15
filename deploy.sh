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
#
# Rollback: the binary being replaced is kept as ${BIN_NAME}.prev on the
# server, so undoing a bad deploy is
#   ssh myserver 'cd /home/pdlife/web/pdlife.app/public_html && \
#     mv pdlife.prev pdlife && systemctl restart pdlife'

set -euo pipefail

DEPLOY_HOST="${DEPLOY_HOST:-myserver}"
REMOTE_DIR="/home/pdlife/web/pdlife.app/public_html"
SERVICE_NAME="pdlife"
BIN_NAME="pdlife"

cd "$(dirname "$0")"

BUILD_PATH="$(mktemp -t pdlife-build.XXXXXX)"
trap 'rm -f "$BUILD_PATH"' EXIT

# Gate the deploy on the tests rather than trusting whatever was last run
# by hand — this is a patient-facing app and deploys are one command.
echo "==> Running tests"
go test ./...

echo "==> Building linux/amd64 binary"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$BUILD_PATH" .

# The migrations are not applied here — AutoMigrate is off and they are run
# by hand (migrations/README.md). They are shipped so that the file is on
# the server when someone needs to run it; before this, applying one meant
# first working out how to get it there, which is a bad thing to be
# improvising against a live database.
# tar over ssh rather than scp: the server has no sftp subsystem.
echo "==> Syncing migrations/ (not running them)"
tar czf - migrations | ssh "$DEPLOY_HOST" "cd ${REMOTE_DIR} && tar xzf -"

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
# cp -p (not mv) keeps the outgoing binary as .prev while leaving the
# running one in place, so a failed restart can be rolled back to a file
# that is known to have served traffic.
ssh "$DEPLOY_HOST" "chmod +x ${REMOTE_DIR}/${BIN_NAME}.new && \
  if [ -f ${REMOTE_DIR}/${BIN_NAME} ]; then \
    cp -p ${REMOTE_DIR}/${BIN_NAME} ${REMOTE_DIR}/${BIN_NAME}.prev; \
  fi && \
  mv ${REMOTE_DIR}/${BIN_NAME}.new ${REMOTE_DIR}/${BIN_NAME} && \
  chown pdlife:pdlife ${REMOTE_DIR}/${BIN_NAME} && \
  systemctl restart ${SERVICE_NAME} && \
  sleep 2 && \
  systemctl is-active ${SERVICE_NAME}"

rollback() {
  echo "!!! Smoke test failed — rolling back to the previous binary" >&2
  ssh "$DEPLOY_HOST" "cd ${REMOTE_DIR} && \
    if [ -f ${BIN_NAME}.prev ]; then \
      mv ${BIN_NAME}.prev ${BIN_NAME} && systemctl restart ${SERVICE_NAME}; \
    else \
      echo 'no .prev binary to roll back to — service left as is' >&2; exit 1; \
    fi"
  echo "!!! Rolled back. The bad build was NOT kept; fix and re-deploy." >&2
  exit 1
}

echo "==> Smoke test"
for path in / /healthz /register /login; do
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "https://pdlife.app${path}")" || code="000"
  echo "  ${path} -> ${code}"
  case "$code" in
    2*|3*) ;;
    *) rollback ;;
  esac
done

echo "==> Deploy complete"
echo
echo "    Migrations are in ${REMOTE_DIR}/migrations/ but were NOT run."
echo "    If this deploy needed one:"
echo "      ssh ${DEPLOY_HOST} 'cd ${REMOTE_DIR} && mysql -u DB_USER -p DB_NAME < migrations/FILE.sql'"
