#!/bin/sh
# Usage: next-version.sh <current version, e.g. 0.2.1> < commit subjects
# Prints the next version from Conventional Commit subjects, or nothing if no
# commit warrants a release. Before 1.0, a breaking change bumps the minor.
set -eu
cur=${1:-0.0.0}
major=${cur%%.*}; rest=${cur#*.}; minor=${rest%%.*}; patch=${rest#*.}
bump=
while IFS= read -r s; do
  case "$s" in
    *'!:'* | *'BREAKING CHANGE'*) bump=breaking ;;
    feat:* | feat\(*) [ "$bump" = breaking ] || bump=minor ;;
    fix:* | fix\(* | perf:* | perf\(*) [ -n "$bump" ] || bump=patch ;;
  esac
done
case $bump in
  breaking) if [ "$major" = 0 ]; then echo "0.$((minor + 1)).0"; else echo "$((major + 1)).0.0"; fi ;;
  minor) echo "$major.$((minor + 1)).0" ;;
  patch) echo "$major.$minor.$((patch + 1))" ;;
esac
