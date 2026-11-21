#!/bin/sh
set -eu
cd "$(dirname "$0")"
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-halo-conformance}"
export SUITE_TAG="${SUITE_TAG:-release-v5.3.4}"
mkdir -p out/bin out/certs out/config out/suite/certs-keys out/results

if [ ! -f out/certs/tls.crt ]; then
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 30 -subj "/CN=Halo conformance" \
    -addext "subjectAltName=DNS:halo.test,DNS:localhost.emobix.co.uk" -addext "basicConstraints=critical,CA:TRUE" \
    -addext "keyUsage=critical,digitalSignature,keyCertSign" -addext "extendedKeyUsage=serverAuth" \
    -keyout out/certs/tls.key -out out/certs/tls.crt 2>/dev/null
  chmod 644 out/certs/tls.key
fi
[ -f out/env ] || echo "HALO_SECRET_KEY=$(openssl rand -base64 32)" > out/env
set -a
. ./out/env
set +a

if [ -n "${HALO_BINARY:-}" ]; then
  cp "$HALO_BINARY" out/bin/halo
else
  (cd ../.. && CGO_ENABLED=0 go build -trimpath -o scripts/conformance/out/bin/halo ./cmd/halo)
fi
if [ -z "${HALO_WEB_IMAGE:-}" ]; then
  docker build -q -f ../../deploy/Dockerfile.web -t halo-web:conformance ../.. >/dev/null
fi
for f in run-test-plan.py conformance.py test_plan_parser.py; do
  curl -fsSL "https://gitlab.com/openid/conformance-suite/-/raw/$SUITE_TAG/scripts/$f" -o "out/suite/$f"
done

cleanup() {
  [ -z "${lifter:-}" ] || kill "$lifter" 2>/dev/null || true
  [ "${KEEP:-}" = 1 ] || docker compose --profile runner down -v --remove-orphans >/dev/null 2>&1
}
trap cleanup EXIT
docker compose up -d --wait
docker compose run --rm --no-deps runner python wait.py
docker compose exec -T halo-server /halo/halo bootstrap --email admin@halo.test --name "Conformance Admin" >/dev/null
cookie=$(docker compose exec -T -e HALO_DEV=1 halo-server /halo/halo dev-session --email admin@halo.test | tail -n 1)
docker compose run --rm --no-deps -e HALO_ADMIN_COOKIE="$cookie" runner python seed.py
while sleep 15; do
  docker compose exec -T postgres psql -U halo -d halo -qc "delete from rate_limit_windows where scope = 'registration'" >/dev/null 2>&1
done &
lifter=$!
docker compose run --rm --no-deps runner sh plans.sh "$@"
