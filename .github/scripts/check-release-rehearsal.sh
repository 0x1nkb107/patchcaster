#!/usr/bin/env bash
# Checks the outcome of the CI release rehearsal (ADR-023) against the release
# contract: what the registry serves for the tag, what the archives contain,
# what the binary embeds and reports, and that the web interface builds the
# same bytes twice. Run it after `just release-publish` in the rehearsal job,
# with PATCHCASTER_IMAGE_REPO pointing at the throwaway registry and the
# rehearsal tag checked out.
set -euo pipefail

repo="${PATCHCASTER_IMAGE_REPO:?PATCHCASTER_IMAGE_REPO must name the rehearsal registry}"
tag="$(git describe --tags --exact-match)"
version="${tag#v}"
revision="$(git rev-parse HEAD)"
commit_date="$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ)"
platforms=$'linux/amd64\nlinux/arm64'
licence_files=(LICENSE LICENSES/LicenseRef-PatchCaster-Accepted-AGPL.txt)
web_sbom="patchcaster_${version}_web.cdx.json"
failures=0

# fail records a failed check and keeps going, so one run reports them all.
fail() {
	echo "FAIL: $*" >&2
	failures=$((failures + 1))
}

pass() {
	echo "ok:   $*"
}

# expect_equal compares a value with what the release contract requires.
expect_equal() {
	local what="$1" want="$2" got="$3"
	if [[ "${got}" == "${want}" ]]; then
		pass "${what}"
	else
		fail "${what}: want '${want}', got '${got}'"
	fi
}

raw_manifest() {
	docker buildx imagetools inspect --raw "$1"
}

# image_platforms lists the platforms of the image manifests in an index,
# leaving out the attestation manifests that sit next to them.
image_platforms() {
	jq -r '.manifests[]
		| select(.annotations["vnd.docker.reference.type"] != "attestation-manifest")
		| "\(.platform.os)/\(.platform.architecture)"' | sort
}

check_index() {
	local index
	index="$(raw_manifest "${repo}:${tag}")"
	expect_equal "index platforms" "${platforms}" "$(image_platforms <<<"${index}")"

	local key want
	declare -A annotations=(
		[org.opencontainers.image.version]="${version}"
		[org.opencontainers.image.revision]="${revision}"
		[org.opencontainers.image.created]="${commit_date}"
		[org.opencontainers.image.source]="https://github.com/patchcaster-app/patchcaster"
		[org.opencontainers.image.licenses]="AGPL-3.0-only OR LicenseRef-PatchCaster-Accepted-AGPL"
	)
	for key in "${!annotations[@]}"; do
		want="${annotations[${key}]}"
		expect_equal "index annotation ${key}" "${want}" \
			"$(jq -r --arg key "${key}" '.annotations[$key] // ""' <<<"${index}")"
	done
}

check_latest() {
	local tagged latest
	tagged="$(raw_manifest "${repo}:${tag}" | sha256sum | cut -d' ' -f1)"
	latest="$(raw_manifest "${repo}:latest" | sha256sum | cut -d' ' -f1)"
	expect_equal "latest serves the tagged index" "${tagged}" "${latest}"
}

# check_sbom_attestations requires one SBOM attestation per image manifest.
check_sbom_attestations() {
	local index digest attestation
	index="$(raw_manifest "${repo}:${tag}")"
	for digest in $(jq -r '.manifests[]
		| select(.annotations["vnd.docker.reference.type"] != "attestation-manifest")
		| .digest' <<<"${index}"); do
		attestation="$(jq -r --arg digest "${digest}" '.manifests[]
			| select(.annotations["vnd.docker.reference.digest"] == $digest)
			| .digest' <<<"${index}")"
		if [[ -z "${attestation}" ]]; then
			fail "no attestation manifest for ${digest}"
			continue
		fi
		if raw_manifest "${repo}@${attestation}" | jq -e '[.layers[].annotations["in-toto.io/predicate-type"] // ""
			| select(startswith("https://spdx.dev/Document"))] | length > 0' >/dev/null; then
			pass "SBOM attestation for ${digest}"
		else
			fail "attestation ${attestation} for ${digest} carries no SPDX document"
		fi
	done
}

# check_sbom_content proves the scanner saw the binary's Go modules, not only
# the packages of the base image.
check_sbom_content() {
	local sbom platform
	sbom="$(docker buildx imagetools inspect "${repo}:${tag}" --format '{{ json .SBOM }}')"
	for platform in ${platforms}; do
		if jq -e --arg platform "${platform}" \
			'[.[$platform].SPDX.packages[].name | select(. == "github.com/jackc/pgx/v5")] | length > 0' \
			<<<"${sbom}" >/dev/null; then
			pass "SBOM for ${platform} lists the binary's Go modules"
		else
			fail "SBOM for ${platform} does not list github.com/jackc/pgx/v5"
		fi
	done
}

check_labels() {
	local images platform
	images="$(docker buildx imagetools inspect "${repo}:${tag}" --format '{{ json .Image }}')"
	for platform in ${platforms}; do
		expect_equal "${platform} label org.opencontainers.image.version" "${version}" \
			"$(jq -r --arg platform "${platform}" \
				'.[$platform].config.Labels["org.opencontainers.image.version"] // ""' <<<"${images}")"
	done
}

check_archives() {
	local arch archive
	for arch in amd64 arm64; do
		archive="dist/patchcaster_${version}_linux_${arch}.tar.gz"
		if [[ ! -f "${archive}" ]]; then
			fail "missing ${archive}"
			continue
		fi
		expect_equal "contents of ${archive}" \
			"$(printf '%s\n' "${licence_files[@]}" patchcaster | LC_ALL=C sort)" \
			"$(tar -tzf "${archive}" | LC_ALL=C sort)"
	done
	if (cd dist && sha256sum --strict --quiet -c checksums.txt); then
		pass "checksums.txt matches the archives"
	else
		fail "checksums.txt does not match the archives"
	fi
}

# check_image_licence_files requires the image to carry the licence texts
# exactly as the repository holds them (ADR-024).
check_image_licence_files() {
	local container file shipped
	docker pull --quiet --platform linux/amd64 "${repo}:${tag}" >/dev/null
	container="$(docker create --platform linux/amd64 "${repo}:${tag}")"
	for file in "${licence_files[@]}"; do
		shipped="$(docker cp "${container}:/usr/share/doc/patchcaster/$(basename "${file}")" - | tar -xO | sha256sum)"
		expect_equal "image carries ${file}" "$(sha256sum <"${file}")" "${shipped}"
	done
	docker rm "${container}" >/dev/null
}

# check_web_sbom requires the SBOM of the web interface's packages next to
# the archives, listed in the checksums and describing this build (ADR-044).
check_web_sbom() {
	if [[ ! -f "dist/${web_sbom}" ]]; then
		fail "missing dist/${web_sbom}"
		return
	fi
	if grep -q -F "  ${web_sbom}" dist/checksums.txt; then
		pass "checksums.txt lists ${web_sbom}"
	else
		fail "checksums.txt does not list ${web_sbom}"
	fi
	expect_equal "${web_sbom} names the commit" "${revision}" \
		"$(jq -r '.metadata.component.version' "dist/${web_sbom}")"
	if jq -e '[.components[].purl | select(startswith("pkg:npm/svelte@"))] | length == 1' \
		"dist/${web_sbom}" >/dev/null; then
		pass "${web_sbom} lists the bundled packages"
	else
		fail "${web_sbom} does not list the svelte package"
	fi
}

# check_embedded_bundle requires the binary to embed this build's web
# interface (ADR-032). The policy in its fallback page hashes the boot
# script, which names the commit, so no other build has the same one.
check_embedded_bundle() {
	local policy workdir
	policy="$(grep -o '<meta http-equiv="content-security-policy"[^>]*>' web/dist/app/index.html || true)"
	if [[ -z "${policy}" ]]; then
		fail "the web interface's fallback page carries no Content-Security-Policy"
		return
	fi
	workdir="$(mktemp -d)"
	tar -xzf "dist/patchcaster_${version}_linux_amd64.tar.gz" -C "${workdir}" patchcaster
	if grep -a -q -F -- "${policy}" "${workdir}/patchcaster"; then
		pass "the binary embeds the web interface of this build"
	else
		fail "the binary does not embed the web interface of this build"
	fi
	rm -rf "${workdir}"
}

# check_bundle_reproducible installs and builds the web interface once
# more and requires the same bytes, its SBOM included (ADR-021).
check_bundle_reproducible() {
	local first
	first="$(mktemp -d)"
	cp -r web/dist/app "${first}/app"
	if ! { just web-install && just web-build; } >/dev/null; then
		fail "the second build of the web interface failed"
		rm -rf "${first}"
		return
	fi
	if diff -r "${first}/app" web/dist/app >/dev/null; then
		pass "a second build of the web interface is identical"
	else
		fail "a second build of the web interface differs"
	fi
	if cmp -s "dist/${web_sbom}" web/sbom/web.cdx.json; then
		pass "a second build writes the same SBOM"
	else
		fail "a second build writes a different SBOM"
	fi
	rm -rf "${first}"
}

check_binary_version() {
	local workdir reported
	workdir="$(mktemp -d)"
	tar -xzf "dist/patchcaster_${version}_linux_amd64.tar.gz" -C "${workdir}" patchcaster
	reported="$("${workdir}/patchcaster" --version)"
	rm -rf "${workdir}"
	expect_equal "binary --version" "${version}" "${reported}"
}

echo "Checking the rehearsed release ${tag} in ${repo}"
check_index
check_latest
check_sbom_attestations
check_sbom_content
check_labels
check_archives
check_image_licence_files
check_web_sbom
check_embedded_bundle
check_bundle_reproducible
check_binary_version

if ((failures > 0)); then
	echo "${failures} check(s) failed" >&2
	exit 1
fi
echo "The rehearsed release meets the contract."
