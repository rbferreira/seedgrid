#!/usr/bin/env sh
# Tests for check-docs-parity.sh with disposable folders.
# Usage: sh scripts/test-check-docs-parity.sh   (runs in CI)
set -eu

SRC="$(cd "$(dirname "$0")" && pwd)/check-docs-parity.sh"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
FAILS=0

fresh() {
  rm -rf "$T/docs"; mkdir -p "$T/docs/pt-BR"
  printf '# **1. Goal**\nSee ADR-012 and #34.\n| a | b |\n|---|---|\n```sh\n# a comment, not a heading\nls\n```\n' > "$T/docs/A.md"
  printf '# **1. Objetivo**\nVer ADR-012 e #34.\n| a | b |\n|---|---|\n```sh\n# um comentário, não título\nls\n```\n' > "$T/docs/pt-BR/A.md"
}
expect() { # expect <description> <expected output>
  status=0; sh "$SRC" "$T/docs" > "$T/out" 2>&1 || status=$?
  if [ "$status" -eq "$2" ]; then echo "ok: $1"; else echo "FAILED: $1 (exit $status, expected $2)"; sed 's/^/    /' "$T/out"; FAILS=$((FAILS + 1)); fi
}

fresh; expect "pairs with the same structure pass (code block comment is not a title)" 0
fresh; printf '# **2. More**\n' >> "$T/docs/A.md"; expect "extra title in one language fails" 1
grep -q 'titles: English 2, Portuguese 1' "$T/out" && echo "ok: says what diverges and how much" || { echo "FAILED: did not say what diverges"; FAILS=$((FAILS + 1)); }
fresh; printf 'Also ADR-099.\n' >> "$T/docs/pt-BR/A.md"; expect "ADR reference in only one language fails" 1
fresh; printf '# **X**\n' > "$T/docs/B.md"; expect "file only in English fails" 1
fresh; printf '# **X**\n' > "$T/docs/pt-BR/C.md"; expect "file only in Portuguese fails" 1
fresh; printf 'Color &#34; and x#12 are not issues.\n' >> "$T/docs/A.md"; expect "HTML entity and #12 attached to word do not count as issue" 0
rm -rf "$T/docs/pt-BR"; expect "missing docs/pt-BR is a usage error" 2

[ "$FAILS" -eq 0 ] && echo "== check-docs-parity: all tests passed" || { echo "== check-docs-parity: $FAILS failure(s)"; exit 1; }
