#!/usr/bin/env bash
# Build and verify a release on this machine. It never publishes anything.
#
#   scripts/release-local.sh            # checks, snapshot build, verification
#   scripts/release-local.sh --fast     # skip `go test -race` (use after a full run)
#   scripts/release-local.sh --tag      # real build of the tag at HEAD (clean tree required)
#   scripts/release-local.sh --rpm-smoke# also install the rpm in a Fedora container (podman/docker)
#
# Artifacts land in dist/ with checksums.txt. Publishing is a separate, manual
# step printed at the end.
set -euo pipefail
cd "$(dirname "$0")/.."

fast=0 tag=0 rpm=0
for arg in "$@"; do
  case "$arg" in
    --fast) fast=1 ;;
    --tag) tag=1 ;;
    --rpm-smoke) rpm=1 ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

step() { printf '\n== %s\n' "$*"; }
need() { command -v "$1" >/dev/null 2>&1 || { echo "missing tool: $1 ($2)" >&2; exit 1; }; }

need go "install Go, see go.mod for the version"
GORELEASER="$(command -v goreleaser || true)"
[ -z "$GORELEASER" ] && [ -x "$HOME/go/bin/goreleaser" ] && GORELEASER="$HOME/go/bin/goreleaser"
[ -n "$GORELEASER" ] || { echo "missing tool: goreleaser (go install github.com/goreleaser/goreleaser/v2@latest)" >&2; exit 1; }

step "format and vet"
unformatted="$(gofmt -l cmd internal)"
[ -z "$unformatted" ] || { echo "gofmt needed:"; echo "$unformatted"; exit 1; }
go vet ./...

if [ "$fast" -eq 0 ]; then
  step "tests (race detector)"
  go test -race -count=1 ./...
fi

step "docs"
python3 scripts/check-docs-links.py .
python3 -c "import json; json.load(open('docs/contracts/openapi.json'))"
git diff --check

args=(release --clean --skip=publish)
if [ "$tag" -eq 1 ]; then
  git describe --exact-match --tags HEAD >/dev/null 2>&1 || { echo "HEAD is not at a tag" >&2; exit 1; }
  [ -z "$(git status --porcelain --untracked-files=no)" ] || { echo "the working tree is not clean" >&2; exit 1; }
else
  args+=(--snapshot)
fi

step "build ($([ "$tag" -eq 1 ] && echo tag || echo snapshot))"
"$GORELEASER" "${args[@]}"

step "verify checksums"
( cd dist && sha256sum -c checksums.txt )

step "smoke the linux/amd64 binary"
bin="$(find dist -type f -name 502drive -path '*linux_amd64*' | head -n1)"
[ -n "$bin" ] || { echo "no linux/amd64 binary in dist/" >&2; exit 1; }
"$bin" version

if [ "$rpm" -eq 1 ]; then
  step "rpm smoke (Fedora container)"
  engine="$(command -v podman || command -v docker || true)"
  [ -n "$engine" ] || { echo "need podman or docker for --rpm-smoke" >&2; exit 1; }
  rpmfile="$(ls dist/*linux_amd64*.rpm dist/*.x86_64.rpm 2>/dev/null | head -n1)"
  [ -n "$rpmfile" ] || { echo "no rpm in dist/" >&2; exit 1; }
  "$engine" run --rm -v "$PWD/dist:/dist:ro" fedora:latest bash -c \
    "dnf install -y /dist/$(basename "$rpmfile") >/dev/null && 502drive version && rpm -qi 502drive | head -n 5"
fi

step "done"
ls -1 dist | sed 's/^/  /'
cat <<'MSG'

Nothing was published. To release this build by hand (after tagging and pushing the tag):

  gh release create vX.Y.Z --title "502Drive vX.Y.Z" --notes-file RELEASE_NOTES.md \
    dist/*.tar.gz dist/*.zip dist/*.rpm dist/*.deb dist/checksums.txt

The installer for Windows (packaging/windows/502drive.iss) needs Inno Setup on Windows.
MSG
