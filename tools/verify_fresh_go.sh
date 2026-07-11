#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

"$ROOT/tools/build_libamk.sh"

cd "$ROOT"
if [ "$#" -eq 0 ]; then
    set -- ./...
fi

go test "$@"
