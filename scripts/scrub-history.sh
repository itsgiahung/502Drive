#!/usr/bin/env bash
# Rewrite the author/committer e-mail of every commit (and tag) in a COPY of this
# repository. File contents are not touched: the tree of every commit stays
# byte-identical, only commit hashes change. Nothing is pushed.
#
#   scripts/scrub-history.sh OLD_EMAIL NEW_EMAIL [DEST_DIR]
#
# Commit what you want to publish first (the copy is made from committed
# history only). DEST_DIR defaults to ../502Drive-scrubbed and must not exist.
# Requires git-filter-repo.
set -euo pipefail

old="${1:?usage: $0 OLD_EMAIL NEW_EMAIL [DEST_DIR]}"
new="${2:?usage: $0 OLD_EMAIL NEW_EMAIL [DEST_DIR]}"
src="$(git rev-parse --show-toplevel)"
dest="${3:-$src/../502Drive-scrubbed}"

command -v git-filter-repo >/dev/null || { echo "install git-filter-repo first" >&2; exit 1; }
[ ! -e "$dest" ] || { echo "$dest already exists" >&2; exit 1; }

git clone --no-local --quiet "$src" "$dest"
mailmap="$(mktemp)"
trap 'rm -f "$mailmap"' EXIT
# "<proper> <commit>" maps only the e-mail; author names stay as they were.
printf '<%s> <%s>\n' "$new" "$old" >"$mailmap"
( cd "$dest" && git filter-repo --quiet --mailmap "$mailmap" )

echo "== verify"
before="$(git -C "$src" rev-list --count --branches --tags)"
after="$(git -C "$dest" rev-list --count --branches --tags)"
[ "$before" = "$after" ] || { echo "commit count changed: $before -> $after" >&2; exit 1; }
[ "$(git -C "$src" rev-parse 'HEAD^{tree}')" = "$(git -C "$dest" rev-parse 'HEAD^{tree}')" ] \
  || { echo "the tree of HEAD changed" >&2; exit 1; }
if git -C "$dest" log --all --format='%ae%n%ce' | grep -qiF "$old"; then
  echo "the old e-mail is still in the history" >&2; exit 1
fi
echo "commits: $after, HEAD tree identical, old e-mail gone"
git -C "$dest" log --all --format='%an <%ae>' | sort | uniq -c | sort -rn

cat <<MSG

Done. $dest has the rewritten history. Nothing was pushed.
To publish it over the existing GitHub repository (this REPLACES the remote
history; anyone who cloned it must re-clone):

  cd $dest
  git remote add origin <repo-url>
  git push --force --all origin
  git push --force --tags origin

Then check the GitHub web UI, and remember that GitHub keeps old commits
reachable by hash for a while: contact GitHub support to purge cached views if
the old e-mail must disappear completely.
MSG
