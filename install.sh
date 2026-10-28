#!/bin/sh
# Install yardo from its GitHub release: the build for this machine's
# architecture, checked against the release's checksums.txt.
#
#   curl -fsSL https://github.com/eugenetaranov/yardo/releases/latest/download/install.sh | sudo sh
#   ... | sudo sh -s -- --version v0.1.0      a given release
#   ... | sudo sh -s -- --prefix /opt/bin     another directory
#
# YARDO_VERSION does the same as --version.
set -eu

repo=eugenetaranov/yardo
version=${YARDO_VERSION:-}
prefix=/usr/local/bin

while [ $# -gt 0 ]; do
  case $1 in
    --version) version=${2:?--version needs a tag, like v0.1.0}; shift ;;
    --prefix) prefix=${2:?--prefix needs a directory}; shift ;;
    -h|--help)
      echo "Install yardo from its GitHub release, checked against its checksums.txt."
      echo "  ... | sudo sh                          the latest release"
      echo "  ... | sudo sh -s -- --version v0.1.0   a given release (or YARDO_VERSION)"
      echo "  ... | sudo sh -s -- --prefix /opt/bin  another directory"
      exit 0 ;;
    *) echo "install.sh: unknown argument $1" >&2; exit 2 ;;
  esac
  shift
done

die() { echo "install.sh: $*" >&2; exit 1; }

[ "$(uname -s)" = Linux ] || die "yardo sets up Linux machines; this is $(uname -s)"
case $(uname -m) in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "no yardo build for $(uname -m)" ;;
esac
# yardo sets up Ubuntu and Debian (and their derivatives) only.
os=$( (. /etc/os-release && echo " ${ID:-} ${ID_LIKE:-} ") 2>/dev/null || true)
case $os in
  *" ubuntu "*|*" debian "*) ;;
  *) die "yardo sets up Ubuntu (22.04 or later) and Debian (12 or later); this is $( (. /etc/os-release && echo "${PRETTY_NAME:-$ID}") 2>/dev/null || echo "a system without /etc/os-release")" ;;
esac
[ -w "$prefix" ] || die "can't write to $prefix: run it with sudo (curl ... | sudo sh)"

if command -v curl >/dev/null 2>&1; then
  get() { curl -fsSL --retry 3 -o "$2" "$1"; }
  latest() { curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest"; }
elif command -v wget >/dev/null 2>&1; then
  get() { wget -q -O "$2" "$1"; }
  latest() { wget -q -S --spider --max-redirect=5 "https://github.com/$repo/releases/latest" 2>&1 | sed -n 's/^ *[Ll]ocation: //p' | tail -1; }
else
  die "needs curl or wget"
fi

if [ -z "$version" ]; then
  version=$(latest | sed 's#.*/tag/##' | tr -d '\r')
  [ -n "$version" ] || die "couldn't find the latest release"
fi
case $version in v*) ;; *) version=v$version ;; esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
tarball="yardo_${version#v}_linux_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/$version"
get "$base/$tarball" "$tmp/$tarball" || die "download $base/$tarball failed"
get "$base/checksums.txt" "$tmp/checksums.txt" || die "download checksums.txt failed"

want=$(awk -v f="$tarball" '$2 == f {print $1}' "$tmp/checksums.txt")
[ -n "$want" ] || die "$tarball isn't in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$tarball" | cut -d' ' -f1)
else
  got=$(shasum -a 256 "$tmp/$tarball" | cut -d' ' -f1)
fi
[ "$got" = "$want" ] || die "checksum mismatch for $tarball"

tar -xzf "$tmp/$tarball" -C "$tmp" yardo
install -m 0755 "$tmp/yardo" "$prefix/yardo.new"
mv -f "$prefix/yardo.new" "$prefix/yardo"
echo "yardo $version installed to $prefix/yardo; run: sudo yardo"
