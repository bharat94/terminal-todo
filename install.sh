#!/bin/sh
# Install the terminal-todo `todo` binary from a GitHub release.
#
# The archive is verified against the release's checksums.txt before anything
# is installed. A download that does not match its published checksum is
# discarded and the script fails; it never installs unverified bytes.
#
# Usage:
#   ./install.sh                          # latest release, into ~/.local/bin
#   VERSION=v0.1.0-beta.1 ./install.sh    # a specific release
#   PREFIX=/usr/local ./install.sh        # install into $PREFIX/bin
#
# This route trades the release archive's SBOM and provenance attestation for
# convenience. Verify those manually when they matter; see docs/installation.md.

set -eu

REPO="bharat94/terminal-todo"
PROJECT="terminal-todo"
BINARY="todo"
PREFIX="${PREFIX:-$HOME/.local}"
BINDIR="${BINDIR:-$PREFIX/bin}"
VERSION="${VERSION:-}"

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required but not installed"
}

need uname
need mktemp
need tar

# One of curl or wget is enough; prefer curl.
if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL "$1" -o "$2"; }
	fetch_stdout() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -qO "$2" "$1"; }
	fetch_stdout() { wget -qO- "$1"; }
else
	fail "curl or wget is required"
fi

# Checksum verification is not optional, so a missing tool is a hard failure
# rather than a reason to skip the check.
if command -v sha256sum >/dev/null 2>&1; then
	checksum() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
	checksum() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
	fail "sha256sum or shasum is required to verify the download"
fi

# Archive names use the Go names for platform and architecture, which is what
# the release pipeline templates them from.
detect_os() {
	os=$(uname -s)
	case "$os" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	*) fail "unsupported operating system: $os (Windows archives must be installed manually)" ;;
	esac
}

detect_arch() {
	arch=$(uname -m)
	case "$arch" in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*) fail "unsupported architecture: $arch (use a release archive)" ;;
	esac
}

resolve_latest() {
	# Read the tag from the releases API without requiring a JSON parser.
	fetch_stdout "https://api.github.com/repos/$REPO/releases/latest" |
		sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
		head -n 1
}

OS=$(detect_os)
ARCH=$(detect_arch)

if [ -z "$VERSION" ]; then
	VERSION=$(resolve_latest)
	[ -n "$VERSION" ] || fail "could not determine the latest release; set VERSION explicitly"
fi

# Archive names carry the version without its leading v.
NUMERIC_VERSION=${VERSION#v}
ARCHIVE="${PROJECT}_${NUMERIC_VERSION}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/$REPO/releases/download/$VERSION"

WORKDIR=$(mktemp -d)
cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT INT TERM

echo "Downloading $ARCHIVE ($VERSION)"
fetch "$BASE/$ARCHIVE" "$WORKDIR/$ARCHIVE" ||
	fail "could not download $ARCHIVE; check that $VERSION publishes an archive for $OS/$ARCH"
fetch "$BASE/checksums.txt" "$WORKDIR/checksums.txt" ||
	fail "could not download checksums.txt for $VERSION"

expected=$(grep " $ARCHIVE\$" "$WORKDIR/checksums.txt" | cut -d' ' -f1)
[ -n "$expected" ] || fail "checksums.txt does not list $ARCHIVE"

actual=$(checksum "$WORKDIR/$ARCHIVE")
if [ "$expected" != "$actual" ]; then
	fail "checksum mismatch for $ARCHIVE
  expected $expected
  actual   $actual
The download was discarded. Do not install this file."
fi
echo "Checksum verified"

tar -xzf "$WORKDIR/$ARCHIVE" -C "$WORKDIR"
[ -f "$WORKDIR/$BINARY" ] || fail "$ARCHIVE did not contain a $BINARY binary"

mkdir -p "$BINDIR"
# Install through a temporary name in the destination directory so an
# interrupted install cannot leave a partial binary at the final path.
install -m 755 "$WORKDIR/$BINARY" "$BINDIR/.$BINARY.incoming"
mv "$BINDIR/.$BINARY.incoming" "$BINDIR/$BINARY"

echo "Installed $BINDIR/$BINARY"
"$BINDIR/$BINARY" --version || true

case ":$PATH:" in
*":$BINDIR:"*) ;;
*) echo "Note: $BINDIR is not on your PATH. Add it to use 'todo' directly." ;;
esac
