#!/usr/bin/env bash
# Fills the checksums in versions.json from a directory of tarballs built
# independently of CI (make.bash -distpack from the pinned commit), then
# regenerates the Dockerfiles. CI then has to reproduce those bytes exactly.
#
# usage: ./update.sh <major.minor> <dir-with-tarballs>
set -Eeuo pipefail

cd "$(dirname "$(readlink -f "$BASH_SOURCE")")"

export major="${1:?usage: $0 <major.minor> <dir>}"
dir="${2:?usage: $0 <major.minor> <dir>}"

sum() { sha256sum "$dir/$1" | awk '{ print $1 }'; }

v="$(jq -r '.[env.major].version' versions.json)"
src="$(sum "go$v.src.tar.gz")"
json="$(jq --arg s "$src" '.[env.major].src.sha256 = $s' versions.json)"
for arch in $(jq -r '.[env.major].arches | to_entries[] | select(.value.supported) | .key' versions.json); do
	export arch
	file="$(basename "$(jq -r '.[env.major].arches[env.arch].url' versions.json)")"
	json="$(jq --arg s "$(sum "$file")" '.[env.major].arches[env.arch].sha256 = $s' <<<"$json")"
done
printf '%s\n' "$json" > versions.json

./apply-templates.sh
