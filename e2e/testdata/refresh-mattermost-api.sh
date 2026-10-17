#!/usr/bin/env bash
# Refreshes mattermost-openapi-v4.yaml next to this script: the endpoints
# PatchCaster calls, for posts, users and channels, of the Mattermost release
# whose API the e2e fake validates every request against (ADR-027). To move to a newer release, change both pins
# below, run this script and commit the regenerated description.
#
# The description is assembled from the upstream sources the way Mattermost's
# own build concatenates them. The same commit always gives the same bytes, so
# a rerun without a new pin changes nothing.
set -euo pipefail

# The release is pinned by commit because tags can move.
release="v11.11.1"
commit="3acb3a7f684d11ccfcec4e5bd11c79f64e3eabf9"

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
out="${here}/mattermost-openapi-v4.yaml"
base="https://raw.githubusercontent.com/mattermost/mattermost/${commit}/api/v4/source"

# A failed download leaves the committed description as it was.
trap 'rm -f "${out}.partial"' EXIT
{
	printf '# Mattermost REST API v4, posts, users and channels endpoints only\n'
	printf '# (ADR-027). Assembled by refresh-mattermost-api.sh, next to this file,\n'
	printf '# from mattermost/mattermost %s (commit %s):\n' "${release}" "${commit}"
	printf '# introduction.yaml, posts.yaml, users.yaml, channels.yaml and\n'
	printf "# definitions.yaml, concatenated in that order as Mattermost's own build\n"
	printf '# does. Do not edit by hand.\n'
	for part in introduction posts users channels definitions; do
		curl -fsSL "${base}/${part}.yaml"
	done
} > "${out}.partial"
mv "${out}.partial" "${out}"
echo "wrote ${out} from Mattermost ${release}"
