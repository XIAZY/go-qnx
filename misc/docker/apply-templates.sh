#!/usr/bin/env bash
# Generates misc/docker/<major.minor>/<variant>/Dockerfile from Dockerfile-linux.template
# and versions.json, as docker-library/golang does (with plain jq and awk
# instead of bashbrew's template engine).
set -Eeuo pipefail

cd "$(dirname "$(readlink -f "$BASH_SOURCE")")"

# bashbrew architecture name -> dpkg architecture name
declare -A dpkgArches=(
	[amd64]='amd64'
	[arm64v8]='arm64'
)

for version in $(jq -r 'keys[]' versions.json); do
	export version
	full="$(jq -r '.[env.version].version' versions.json)"

	srcSha="$(jq -r '.[env.version].src.sha256' versions.json)"
	if [ "$srcSha" = 'null' ]; then
		echo >&2 "error: $version: checksums not filled in yet (run update.sh)"
		exit 1
	fi

	cases=
	for arch in $(jq -r '.[env.version].arches | to_entries[] | select(.value.supported) | .key' versions.json); do
		export arch
		dpkgArch="${dpkgArches[$arch]:?unknown arch $arch}"
		url="$(jq -r '.[env.version].arches[env.arch].url' versions.json)"
		sha256="$(jq -r '.[env.version].arches[env.arch].sha256' versions.json)"
		file="$(basename "$url")"
		cases+="		'$dpkgArch') \\"$'\n'
		cases+="			url=\"\$GO_DOWNLOAD_BASE/$file\"; \\"$'\n'
		cases+="			sha256='$sha256'; \\"$'\n'
		cases+="			;; \\"$'\n'
	done
	cases="${cases%$'\n'}"

	base="$(jq -r '.[env.version].arches.amd64.url' versions.json)"
	base="${base%/*}"

	for variant in $(jq -r '.[env.version].variants[]' versions.json); do
		dir="$version/$variant"
		mkdir -p "$dir"
		echo "processing $dir ..."
		CASES="$cases" awk \
			-v variant="$variant" -v ver="$full" -v base="$base" '
			{
				if ($0 == "{{ARCH_CASES}}") { print ENVIRON["CASES"]; next }
				gsub(/\{\{VARIANT\}\}/, variant)
				gsub(/\{\{VERSION\}\}/, ver)
				gsub(/\{\{DOWNLOAD_BASE\}\}/, base)
				print
			}' Dockerfile-linux.template > "$dir/Dockerfile"
	done
done
