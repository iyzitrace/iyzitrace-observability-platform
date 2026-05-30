#!/usr/bin/env sh
set -eu

limit_bytes="${BINARY_ASSET_LIMIT_BYTES:-10485760}"
tmp_file="$(mktemp)"
trap 'rm -f "$tmp_file"' EXIT

find . -type f -size +"${limit_bytes}"c \
  ! -path './.git/*' \
  ! -path './docs/videos/*' \
  ! -path './docs/videos/*/*' \
  ! -path './node_modules/*' \
  ! -path './*/node_modules/*' \
  ! -path './.cache/*' \
  ! -path './dist/*' \
  ! -path './data/*' \
  > "$tmp_file"

if [ -s "$tmp_file" ]; then
  while IFS= read -r file; do
    echo "Unexpected large binary or artifact: ${file}" >&2
  done < "$tmp_file"
  exit 1
fi

exit 0
