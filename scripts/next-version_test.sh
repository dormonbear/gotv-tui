#!/bin/sh
# Self-check for next-version.sh; run in CI.
set -eu
cd "$(dirname "$0")"
check() { got=$(printf '%b' "$2" | sh next-version.sh "$1"); [ "$got" = "$3" ] || { echo "FAIL: $1 + '$2' -> '$got', want '$3'"; exit 1; }; }
check 0.0.0 'feat: radar\nfix: x\n' 0.1.0
check 0.1.0 'fix: x\ndocs: y\n' 0.1.1
check 0.1.1 'perf(radar): faster\n' 0.1.2
check 0.1.1 'docs: readme\nchore: ci\nMerge pull request #3\n' ''
check 0.3.2 'feat!: new config\n' 0.4.0
check 1.3.2 'refactor!: drop flag\n' 2.0.0
check 1.3.2 'feat(ui): zoom\nfix: y\n' 1.4.0
echo ok
