#!/usr/bin/env bash
set -euo pipefail

TEMPLATE="${TEMPLATE:-startup}"
MODEL="${MODEL:-claude}"
VERBOSITY="${VERBOSITY:-}"
PORT="${PORT:-3001}"

args=(-t "$TEMPLATE" -m "$MODEL")

if [[ -n "$VERBOSITY" ]]; then
    args+=("$VERBOSITY")
fi

args+=("$PORT")

exec secondorder "${args[@]}"
