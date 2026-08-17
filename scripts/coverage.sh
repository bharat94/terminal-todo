#!/usr/bin/env bash
# Enforce per-package statement-coverage floors.
#
# The floors ratchet: raise one when coverage rises, never lower one to turn a
# red build green. They exist to stop a regression, not to certify a number — a
# package can be well tested below its floor and badly tested above it.
#
# A package with no floor is not exempt; it simply has no regression baseline
# yet. Adding one is the point at which it gains a guarantee.
#
# Written for POSIX-ish bash rather than bash 4 associative arrays, so it runs
# the same on a maintainer's macOS shell as it does in CI.
set -euo pipefail

# One "<import path> <floor percent>" per line. Floors are integers.
FLOORS="
github.com/bharat94/terminal-todo/internal/cli 60
github.com/bharat94/terminal-todo/store 66
github.com/bharat94/terminal-todo/dag 80
github.com/bharat94/terminal-todo/lock 90
github.com/bharat94/terminal-todo/fsutil 75
github.com/bharat94/terminal-todo/conformance 78
github.com/bharat94/terminal-todo/internal/projectclock 75
"

echo "Measuring coverage..."
output=$(go test ./... -cover -count=1 -timeout 600s)
echo "$output"
echo

failed=0
while read -r package floor; do
	[ -n "$package" ] || continue

	line=$(printf '%s\n' "$output" | grep -E "^ok[[:space:]]+${package}[[:space:]]" || true)
	if [ -z "$line" ]; then
		echo "FAIL ${package}: no coverage reported" >&2
		failed=1
		continue
	fi

	percent=$(printf '%s\n' "$line" | sed -n 's/.*coverage: \([0-9.]*\)% of statements.*/\1/p')
	if [ -z "$percent" ]; then
		echo "FAIL ${package}: could not parse coverage from: ${line}" >&2
		failed=1
		continue
	fi

	# Compare in tenths of a percent using integer arithmetic, so the check
	# does not depend on bc or on locale-sensitive float parsing.
	actual_tenths=$(printf '%s\n' "$percent" | awk '{printf "%d", ($1 * 10) + 0.5}')
	floor_tenths=$((floor * 10))

	if [ "$actual_tenths" -lt "$floor_tenths" ]; then
		echo "FAIL ${package}: ${percent}% is below the ${floor}% floor" >&2
		failed=1
	else
		echo "ok   ${package}: ${percent}% (floor ${floor}%)"
	fi
done <<EOF
$(printf '%s\n' "$FLOORS")
EOF

if [ "$failed" -ne 0 ]; then
	echo >&2
	echo "Coverage fell below a floor. Add tests rather than lowering the floor." >&2
	exit 1
fi

echo
echo "All coverage floors met."
