#!/usr/bin/env sh
# Checks that docs/ (English) and docs/pt-BR/ (Portuguese) contain the same content: the same set
# of files and, in each pair, the same structure — headings, code fences, table rows,
# references to ADRs and issues. It doesn't compare the text (different languages); it compares the
# skeleton, which is what diverges when someone updates one side and forgets the other.
#
# Usage: sh scripts/check-docs-parity.sh [docs-root-dir]   (default: docs; runs in CI)
set -eu

DOCS="${1:-docs}"
PT="$DOCS/pt-BR"
FAIL=0

# Structural signature of a file: one "label=count" line per item.
signature() {
  awk '
    /^[[:space:]]*```/ { fences++; infence = !infence; next }
    infence { next }
    /^#+ / { headings++ }
    /^[[:space:]]*\|/ { tables++ }
    {
      line = $0
      while (match(line, /ADR-[0-9]+/)) { adrs++; line = substr(line, RSTART + RLENGTH) }
      line = $0
      while (match(line, /#[0-9][0-9][0-9]?[0-9]?/)) {
        prev = (RSTART > 1) ? substr(line, RSTART - 1, 1) : " "
        if (prev !~ /[A-Za-z0-9&]/) issues++
        line = substr(line, RSTART + RLENGTH)
      }
    }
    END {
      printf "titles=%d\ncode fences=%d\ntable rows=%d\nADR references=%d\nissue references=%d\n",
        headings, fences, tables, adrs, issues
    }' "$1"
}

list() { (cd "$1" && for f in *.md; do [ -f "$f" ] && echo "$f"; done) | sort; }

if [ ! -d "$PT" ]; then
  echo "check-docs-parity: $PT does not exist" >&2
  exit 2
fi

only_en=$(list "$DOCS" | while IFS= read -r f; do [ -f "$PT/$f" ] || echo "$f"; done)
only_pt=$(list "$PT" | while IFS= read -r f; do [ -f "$DOCS/$f" ] || echo "$f"; done)
if [ -n "$only_en" ]; then
  echo "only in English (missing in $PT/):"; printf '  %s\n' $only_en | sed 's/^/ /'; FAIL=1
fi
if [ -n "$only_pt" ]; then
  echo "only in Portuguese (missing in $DOCS/):"; printf '  %s\n' $only_pt | sed 's/^/ /'; FAIL=1
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
list "$DOCS" | while IFS= read -r f; do
  [ -f "$PT/$f" ] || continue
  signature "$DOCS/$f" > "$TMP/en"
  signature "$PT/$f" > "$TMP/pt"
  if ! cmp -s "$TMP/en" "$TMP/pt"; then
    echo "$f: structure differs between English and Portuguese"
    paste -d'|' "$TMP/en" "$TMP/pt" | awk -F'|' '$1 != $2 { split($1, a, "="); split($2, b, "="); printf "    %s: English %s, Portuguese %s\n", a[1], a[2], b[2] }'
    touch "$TMP/fail"
  fi
done
[ -f "$TMP/fail" ] && FAIL=1

if [ "$FAIL" -ne 0 ]; then
  echo "check-docs-parity: the two documentation versions diverge. Update the other language in the same pull request" >&2
  echo "(ver CONTRIBUTING.md, \"Documentation in two languages\")." >&2
  exit 1
fi
echo "check-docs-parity: ok ($(list "$DOCS" | wc -l | tr -d ' ') pairs)"
