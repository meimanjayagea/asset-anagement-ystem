#!/usr/bin/env sh
set -eu
mkdir -p backups
umask 077
file="backups/assetflow-$(date -u +%Y%m%dT%H%M%SZ).dump"
docker compose exec -T db pg_dump -U assetflow -d assetflow -Fc > "$file"
[ -s "$file" ] || { rm -f "$file"; exit 1; }
printf 'Backup created: %s\n' "$file"
