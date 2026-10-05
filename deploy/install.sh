#!/bin/sh
set -eu

repo=${HALO_REPO:-https://github.com/ScalaStudios/Halo.git}
ref=${HALO_REF:-main}
guide=https://github.com/ScalaStudios/Halo/blob/main/deploy/README.md
if [ "$(id -u)" -eq 0 ]; then dir=${HALO_DIR:-/opt/halo}; else dir=${HALO_DIR:-$HOME/halo}; fi

fail() {
  printf 'halo: %s\n' "$1" >&2
  exit 1
}

ask() {
  (: </dev/tty) 2>/dev/null || fail "$2"
  printf '%s' "$1" >/dev/tty
  IFS= read -r answer </dev/tty || fail "$2"
  printf '%s' "$answer"
}

escape() {
  printf '%s' "$1" | sed 's/[&|]/\\&/g'
}

for tool in git openssl docker; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is not installed. Install it and run this script again."
done
docker compose version >/dev/null 2>&1 || fail "the Docker Compose plugin is not installed. See https://docs.docker.com/compose/install/linux/"
docker info >/dev/null 2>&1 || fail "Docker is not running, or this user cannot use it. Start Docker, or run this script as root."
[ -e "$dir" ] && fail "$dir already exists. To upgrade, follow $guide#upgrade. To install somewhere else, set HALO_DIR."

domain=${HALO_DOMAIN:-}
[ -n "$domain" ] || domain=$(ask "Domain for Halo, such as auth.example.com: " "set HALO_DOMAIN to the domain Halo will answer on, such as HALO_DOMAIN=auth.example.com.")
case $domain in
  '' | .* | *. | *[!A-Za-z0-9.-]*) fail "$domain is not a domain name. Enter only the host name, such as auth.example.com." ;;
esac

organization=${HALO_ORGANIZATION:-}
[ -n "$organization" ] || organization=$(ask "Organization name, shown on sign-in pages: " "set HALO_ORGANIZATION to your organization's name.")
case $organization in
  '' | *'"'* | *'\'* | *'$'* | *'`'*) fail "the organization name cannot contain quotes, backslashes, dollar signs or backticks." ;;
esac

printf 'Downloading Halo into %s\n' "$dir"
git clone --quiet --depth 1 --branch "$ref" "$repo" "$dir"
cd "$dir/deploy"

(
  umask 077
  sed -e "s|^HALO_DOMAIN=.*|HALO_DOMAIN=$domain|" \
    -e "s|^HALO_ORGANIZATION=.*|HALO_ORGANIZATION=\"$(escape "$organization")\"|" \
    -e "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$(openssl rand -hex 32)|" \
    -e "s|^HALO_SECRET_KEY=.*|HALO_SECRET_KEY=$(openssl rand -base64 32)|" \
    .env.example >.env
)

printf 'Downloading and starting Halo.\n'
docker compose pull --quiet
docker compose up --wait --wait-timeout 600

cat <<EOF

Halo is running from $dir.

1. Copy HALO_SECRET_KEY from $dir/deploy/.env into your password manager.
   It encrypts Halo's signing keys, and backups cannot be restored without it.

2. Create the first administrator:

   cd $dir/deploy && docker compose exec halo-server halo bootstrap --email you@example.com --name "Your Name"

3. Open the link it prints and create a passkey. The console is at https://$domain/admin.

If https://$domain does not load, check that $domain points at this host and that
ports 80 and 443 are open, then read: docker compose logs caddy

Backups, upgrades and email: $guide
EOF
