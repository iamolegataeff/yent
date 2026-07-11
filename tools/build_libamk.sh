#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
SRC="$ROOT/yent/c/ariannamethod.c"
OBJ="$ROOT/yent/c/ariannamethod.o"
LIB="$ROOT/yent/c/libamk.a"
TMP="$LIB.tmp.$$"

CC_BIN=${CC:-cc}
AR_BIN=${AR:-ar}
AMK_CFLAGS=${AMK_CFLAGS:-"-O2 -DAM_BLOOD_DISABLED -DAM_ASYNC_DISABLED"}

cleanup() {
    if [ -f "$TMP" ]; then
        rm -f "$TMP"
    fi
}
trap cleanup EXIT INT HUP TERM

if [ ! -f "$SRC" ]; then
    printf 'amk: missing source: %s\n' "$SRC" >&2
    exit 1
fi

# AMK_CFLAGS is intentionally word-split so callers can pass normal compiler flags.
"$CC_BIN" $AMK_CFLAGS -I "$ROOT/yent/c" -c "$SRC" -o "$OBJ"
"$AR_BIN" rcs "$TMP" "$OBJ"
mv "$TMP" "$LIB"

printf 'amk: built %s\n' "$LIB"
