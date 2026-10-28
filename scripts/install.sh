#!/bin/sh
# Install 502Drive from a GitHub release. The archive is verified against the
# release's checksums.txt before anything is installed.
#
#   curl -fsSL https://raw.githubusercontent.com/itsgiahung/502Drive/main/scripts/install.sh | sh
#
# Environment:
#   VERSION=v1.2.3   install this release instead of the latest
#   PREFIX=DIR       install into DIR/bin (default: $HOME/.local)
#   REQUIRE_SIGNATURE=1  refuse to install unless the Sigstore signature of
#                    checksums.txt verifies (needs `cosign`; by default it is
#                    checked when cosign is installed and the release has one)
#
# Installs `502drive` and the short alias `502d`. It never uses sudo; for a
# system-wide install use the .rpm/.deb package instead.
set -eu

repo="itsgiahung/502Drive"
prefix="${PREFIX:-$HOME/.local}"

say() { printf '%s\n' "$*"; }
die() { printf 'install: %s\n' "$*" >&2; exit 1; }

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported OS $(uname -s); Windows users: see scripts/install.ps1" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported CPU $(uname -m)" ;;
esac
[ "$os-$arch" != "darwin-amd64" ] || die "no macOS Intel build is published"

if [ -n "${DRIVE502_RELEASE_BASE:-}" ]; then
  base="$DRIVE502_RELEASE_BASE" # tests only
elif [ -n "${VERSION:-}" ]; then
  base="https://github.com/$repo/releases/download/$VERSION"
else
  base="https://github.com/$repo/releases/latest/download"
fi

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -O "$2" "$1"; }
else
  die "need curl or wget"
fi
if command -v sha256sum >/dev/null 2>&1; then
  sum() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
  sum() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
  die "need sha256sum or shasum to verify the download"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

say "Fetching checksums..."
fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "cannot download $base/checksums.txt (is there a published release?)"
# The checksums only protect against a corrupted download. Their Sigstore
# signature shows they were produced by this project's release workflow.
identity='^https://github\.com/itsgiahung/502Drive/\.github/workflows/go-release\.yml@refs/tags/v.+$'
if command -v cosign >/dev/null 2>&1; then
  if fetch "$base/checksums.txt.sigstore.json" "$tmp/checksums.txt.sigstore.json" 2>/dev/null; then
    cosign verify-blob --bundle "$tmp/checksums.txt.sigstore.json" \
      --certificate-identity-regexp "$identity" \
      --certificate-oidc-issuer https://token.actions.githubusercontent.com \
      "$tmp/checksums.txt" >/dev/null 2>&1 \
      || die "the signature of checksums.txt does not verify; nothing was installed"
    say "Signature verified (Sigstore)."
  elif [ "${REQUIRE_SIGNATURE:-}" = 1 ]; then
    die "this release has no signature and REQUIRE_SIGNATURE=1; nothing was installed"
  else
    say "No signature published for this release; checking checksums only."
  fi
elif [ "${REQUIRE_SIGNATURE:-}" = 1 ]; then
  die "REQUIRE_SIGNATURE=1 needs cosign (https://docs.sigstore.dev/cosign/installation/)"
fi

line="$(grep "_${os}_${arch}\.tar\.gz\$" "$tmp/checksums.txt" | head -n1 || true)"
[ -n "$line" ] || die "no $os/$arch archive in this release"
want="${line%% *}"
name="${line##* }"

say "Downloading $name..."
fetch "$base/$name" "$tmp/$name" || die "download failed"
got="$(sum "$tmp/$name")"
[ "$got" = "$want" ] || die "checksum mismatch for $name (expected $want, got $got); nothing was installed"

tar -xzf "$tmp/$name" -C "$tmp" 502drive || die "the archive does not contain 502drive"
mkdir -p "$prefix/bin"
install -m 0755 "$tmp/502drive" "$prefix/bin/502drive"
ln -sf 502drive "$prefix/bin/502d"

say "Installed $prefix/bin/502drive (and the short alias 502d)."
case ":$PATH:" in
  *":$prefix/bin:"*) ;;
  *) say "Add it to your PATH:  export PATH=\"$prefix/bin:\$PATH\"" ;;
esac
say "Start with:  502drive      (or just: 502d)"
