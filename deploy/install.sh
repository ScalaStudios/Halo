#!/bin/sh
set -eu

version=0.1.1
raw=https://raw.githubusercontent.com/ScalaStudios/Halo
guide=https://halo.scala.gg/docs/self-hosting

if [ -t 1 ] && [ "${TERM:-dumb}" != dumb ]; then tty=1; else tty=; fi
if [ -n "$tty" ] && [ -z "${NO_COLOR:-}" ]; then
  esc=$(printf '\033')
  reset="${esc}[0m" bold="${esc}[1m" dim="${esc}[2m" red="${esc}[31m" green="${esc}[32m" yellow="${esc}[33m"
  amber="${esc}[38;5;214m" orange="${esc}[38;5;208m" ember="${esc}[38;5;202m"
else
  reset='' bold='' dim='' red='' green='' yellow='' amber='' orange='' ember=''
fi
cols=$( (stty size </dev/tty) 2>/dev/null | cut -d' ' -f2)
case $cols in '' | 0 | *[!0-9]*) cols=80 ;; esac
pid=
trap '[ -z "$tty" ] || printf "\033[?25h"' EXIT
trap '[ -z "$pid" ] || kill "$pid" 2>/dev/null || :; exit 130' INT TERM

fail() {
  printf '\n  %s✗%s %s\n\n' "$red" "$reset" "$1" >&2
  exit 1
}

banner() {
  printf '\n'
  printf '  %s   %s\n' \
    "    ${amber}▄▄${orange}██████▄▄${reset}    " "" \
    "  ${amber}▄█${orange}███████████▄${reset}  " "" \
    " ${amber}█${orange}███▀▀    ▀▀████${reset} " "${bold}Halo${reset}" \
    "${orange}████▀        ▀█${ember}███${reset}" "${dim}Open-source identity and access management${reset}" \
    "${orange}████          ${ember}████${reset}" "" \
    "${orange}████▄        ${ember}▄████${reset}" "$1" \
    " ${orange}████▄▄    ${ember}▄▄████${reset} " "${dim}$2${reset}" \
    "  ${orange}▀████${ember}█  █████▀${reset}  " "" \
    "    ${orange}▀${ember}▀██  ██▀▀${reset}    " ""
  printf '\n'
}

ask() {
  answer=$1
  if [ -n "$answer" ]; then
    printf '  %s?%s %s%s%s %s›%s %s\n' "$orange" "$reset" "$bold" "$2" "$reset" "$dim" "$reset" "$answer"
    return
  fi
  (: </dev/tty) 2>/dev/null || fail "$4"
  printf '  %s?%s %s%s%s%s%s ›%s ' "$orange" "$reset" "$bold" "$2" "$reset" "$dim" "$3" "$reset" >/dev/tty
  IFS= read -r answer </dev/tty || fail "$4"
}

escape() {
  printf '%s' "$1" | sed 's/[&|]/\\&/g'
}

duration() {
  if [ "$1" -lt 60 ]; then printf '%ss' "$1"; else printf '%sm %ss' $(($1 / 60)) $(($1 % 60)); fi
}

row() {
  printf '  %s%s/%s%s  %s%-18s%s  %s%-32.32s%s %6s ' "$dim" "$current" "$total" "$reset" "$bold" "$1" "$reset" "$dim" "$2" "$reset" "$3"
}

step() {
  title=$1 detail=$2
  shift 2
  current=$((current + 1))
  [ "$cols" -ge 70 ] || detail=
  started=$(date +%s)
  printf '\n[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$title" >>"$log"
  "$@" >>"$log" 2>&1 &
  pid=$!
  if [ -n "$tty" ]; then
    printf '\033[?25l'
    while kill -0 "$pid" 2>/dev/null; do
      for frame in ◜ ◠ ◝ ◞ ◡ ◟; do
        kill -0 "$pid" 2>/dev/null || break
        printf '\r%s%s%s%s' "$(row "$title" "$detail" "")" "$orange" "$frame" "$reset"
        sleep 0.1
      done
    done
    printf '\r'
  fi
  status=0
  wait "$pid" || status=$?
  pid=
  elapsed=$(duration $(($(date +%s) - started)))
  if [ "$status" -eq 0 ]; then
    printf '%s%s✓%s\n' "$(row "$title" "$detail" "$elapsed")" "$green" "$reset"
    return
  fi
  printf '%s%s✗%s\n' "$(row "$title" "$detail" "$elapsed")" "$red" "$reset"
  printf '\n  %s%s%s failed.%s Last lines of %s:\n\n' "$bold" "$red" "$task" "$reset" "$log"
  tail -n 15 "$log" | sed 's/^/    /'
  printf '\n'
  exit 1
}

check_docker() {
  docker compose version >/dev/null 2>&1 || fail "The Docker Compose plugin is not installed: https://docs.docker.com/compose/install/linux/"
  docker info >/dev/null 2>&1 || fail "Docker is not running, or this user can't use it. Start Docker, or run this script as root."
}

fetch() {
  curl -fsSL "$raw/$1" -o "$2.new" && mv "$2.new" "$2"
}

compose() {
  [ -f "$dir/.env" ] || fail "Halo is not installed in $dir. Install it with: curl -fsSL https://halo.scala.gg/install.sh | sh"
  docker compose --project-directory "$dir" -f "$dir/compose.yml" "$@"
}

install_docker() {
  script=$(curl -fsSL https://get.docker.com)
  sh -c "$script"
}

download() {
  fetch "v$version/deploy/install.sh" "$dir/halo.sh"
  fetch "v$version/deploy/compose.yml" "$dir/compose.yml"
  fetch "v$version/deploy/Caddyfile" "$dir/Caddyfile"
  chmod 755 "$dir/halo.sh"
}

configure() {
  template=$(curl -fsSL "$raw/v$version/deploy/.env.example")
  password=$(openssl rand -hex 32)
  key=$(openssl rand -base64 32)
  umask 077
  printf '%s\n' "$template" | sed \
    -e "s|^HALO_DOMAIN=.*|HALO_DOMAIN=$domain|" \
    -e "s|^HALO_ORGANIZATION=.*|HALO_ORGANIZATION=\"$(escape "$organization")\"|" \
    -e "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$password|" \
    -e "s|^HALO_SECRET_KEY=.*|HALO_SECRET_KEY=$key|" >"$dir/.env.new"
  mv "$dir/.env.new" "$dir/.env"
}

install() {
  task=Installation log=$dir/halo.sh.log current=0 total=4 began=$(date +%s)
  banner "Installing Halo $version into $dir" "Log: $log"
  for tool in curl openssl; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool is not installed. Install it and run this script again."
  done
  [ ! -e "$dir/.env" ] || fail "Halo is already installed in $dir. To update it, run: $dir/halo.sh update"
  if command -v docker >/dev/null 2>&1; then
    check_docker
  else
    [ "$(id -u)" -eq 0 ] || fail "Docker is not installed. Run this script as root to install it, or install Docker first: https://docs.docker.com/engine/install/"
    total=5
  fi

  ask "${HALO_DOMAIN:-}" Domain ", such as auth.example.com" "Set HALO_DOMAIN to the domain Halo will answer on, such as HALO_DOMAIN=auth.example.com."
  domain=$answer
  case $domain in
    '' | .* | *. | *[!A-Za-z0-9.-]*) fail "$domain is not a domain name. Enter only the host name, such as auth.example.com." ;;
  esac
  ask "${HALO_ORGANIZATION:-}" "Organization name" ", shown on sign-in pages" "Set HALO_ORGANIZATION to your organization's name."
  organization=$answer
  case $organization in
    '' | *'"'* | *'\'* | *'$'* | *'`'*) fail "The organization name can't contain quotes, backslashes, dollar signs or backticks." ;;
  esac
  printf '\n'

  mkdir -p "$dir" 2>/dev/null || fail "Can't create $dir. Run this script as root, or set HALO_DIR to a directory you can write to."
  if [ "$total" -eq 5 ]; then
    step "Installing Docker" "get.docker.com" install_docker
  fi
  step "Downloading Halo" "halo.sh, compose.yml, Caddyfile" download
  step "Generating secrets" "database password, secret key" configure
  step "Pulling images" "postgres, halo, caddy" compose pull --quiet
  step "Starting Halo" "$domain" compose up -d --wait --wait-timeout 600

  printf '\n  %s%sHalo is ready.%s %sInstalled in %s.%s\n\n' "$bold" "$orange" "$reset" "$dim" "$(duration $(($(date +%s) - began)))" "$reset"
  printf '  %-9s %s%s%s\n' Console "$bold" "https://$domain/admin" "$reset"
  printf '  %-9s %s\n\n' Manage "$dir/halo.sh"
  printf '  %sNext:%s create the first administrator, then open the link it prints to add a passkey.\n\n' "$bold" "$reset"
  printf '    %s/halo.sh admin you@example.com "Your Name"\n\n' "$dir"
  printf '  %s!%s Copy %sHALO_SECRET_KEY%s from %s/.env into your password manager.\n' "$yellow" "$reset" "$bold" "$reset" "$dir"
  printf "    Backups can't be restored without it.\n\n"
  printf '  %sNot loading? Point %s at this server and open ports 80 and 443.%s\n\n' "$dim" "$domain" "$reset"
}

update() {
  command -v curl >/dev/null 2>&1 || fail "curl is not installed. Install it and run this script again."
  [ -f "$dir/compose.yml" ] || fail "Halo is not installed in $dir."
  fetch main/deploy/install.sh "$dir/halo.sh" || fail "Couldn't download the latest halo.sh."
  chmod 755 "$dir/halo.sh"
  HALO_DIR=$dir exec "$dir/halo.sh" update-files
}

update_files() {
  task=Update log=$dir/halo.sh.log current=0 total=3 began=$(date +%s)
  banner "Updating Halo to $version" "Log: $log"
  check_docker
  printf '  %s!%s Migrations only run forward. Back up first: %s#back-up\n\n' "$yellow" "$reset" "$guide"
  step "Downloading Halo" "halo.sh, compose.yml, Caddyfile" download
  step "Pulling images" "postgres, halo, caddy" compose pull --quiet
  step "Restarting Halo" "" compose up -d --wait --wait-timeout 600
  printf '\n  %s%sHalo %s is running.%s %sUpdated in %s.%s\n\n' "$bold" "$orange" "$version" "$reset" "$dim" "$(duration $(($(date +%s) - began)))" "$reset"
}

usage() {
  banner "Version $version" "$dir"
  cat <<EOF
  Usage: halo.sh <command>

  install                 Install Halo into $dir
  update                  Update Halo to the latest release
  start, stop, restart    Start, stop or restart Halo
  logs [service]          Follow the logs of every service, or of one
  admin <email> <name>    Create the first global administrator
  version                 Print the version of Halo this script installs

  Set HALO_DIR to manage Halo in another directory.

EOF
}

script_dir=$(cd "$(dirname "$0")" 2>/dev/null && pwd)
if [ -n "${HALO_DIR:-}" ]; then dir=$HALO_DIR
elif [ -f "$script_dir/compose.yml" ] && [ -f "$script_dir/halo.sh" ]; then dir=$script_dir
elif [ "$(id -u)" -eq 0 ]; then dir=/opt/halo
else dir=$HOME/halo
fi

command=${1:-install}
[ $# -eq 0 ] || shift
case $command in
  install) install ;;
  update) update ;;
  update-files) update_files ;;
  start) compose up -d --wait --wait-timeout 600 ;;
  stop) compose stop ;;
  restart) compose up -d --force-recreate --wait --wait-timeout 600 ;;
  logs) compose logs -f --tail 200 "$@" ;;
  admin)
    [ $# -eq 2 ] || fail 'Usage: halo.sh admin you@example.com "Your Name"'
    compose exec halo-server halo bootstrap --email "$1" --name "$2"
    ;;
  version) printf '%s\n' "$version" ;;
  help | -h | --help) usage ;;
  *) usage >&2; exit 1 ;;
esac
