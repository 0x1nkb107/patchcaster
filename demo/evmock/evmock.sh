#!/usr/bin/env bash
# evmock - send a signed GitLab webhook to a locally running PatchCaster.
#
#   ./demo/evmock/evmock.sh <kind>
#
# Kinds follow internal/plan: open, comment, pipeline-pass, pipeline-fail,
# reviewed, changes, approval, push, merge, close. Payloads follow
# internal/event's wire contract (note timestamps live in created_at, review
# verdicts travel in changes.reviewers, a push is a Push Hook *plus* an MR
# update with oldrev - GitLab's real behaviour, mirrored here).
#
# Everything is overridable by env; defaults match contrib/routing.example.yaml
# (project group/application) and the service defaults in demo/README.md.
set -euo pipefail

URL="${URL:-http://127.0.0.1:8080/webhook}"
SIGNING_SECRET="${SIGNING_SECRET:-demo-sec}"
PROJECT="${PROJECT:-group/application}"
GITLAB_URL="${GITLAB_URL:-http://gitlab.example}"
MR_ID="${MR_ID:-101}"
MR_IID="${MR_IID:-7}"
MR_TITLE="${MR_TITLE:-Add token refresh}"
SOURCE_BRANCH="${SOURCE_BRANCH:-feature/auth}"
TARGET_BRANCH="${TARGET_BRANCH:-main}"
AUTHOR="${AUTHOR:-alice}"
REVIEWER="${REVIEWER:-carol}"
REVIEWER_ID="${REVIEWER_ID:-2}"
COMMITTER="${COMMITTER:-dave}"
N_COMMITS="${N_COMMITS:-3}"

now() { date -u +%Y-%m-%dT%H:%M:%S.000Z; }
MR_URL_BASE="${GITLAB_URL}/${PROJECT}/-/merge_requests/${MR_IID}"

# send <label>: reads one JSON payload on stdin, signs it per Standard
# Webhooks (v1,{base64 hmac-sha256 over "{id}.{timestamp}.{body}"}) and posts.
send() {
	local label="$1" body id ts sig code
	body="$(cat)"
	id="$(cat /proc/sys/kernel/random/uuid)"
	ts="$(date +%s)"
	sig="$(printf '%s.%s.%s' "$id" "$ts" "$body" \
		| openssl dgst -sha256 -hmac "$SIGNING_SECRET" -binary | base64 -w0)"
	code="$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$URL" \
		-H 'Content-Type: application/json' \
		-H "Webhook-Id: $id" -H "Webhook-Timestamp: $ts" \
		-H "Webhook-Signature: v1,$sig" --data-binary "$body")"
	echo "$label -> HTTP $code"
}

case "${1:-}" in
open)
	cat <<-EOF | send open
	{"object_kind":"merge_request","event_name":"merge_request",
	 "project":{"id":9,"full_path":"$PROJECT","path_with_namespace":"$PROJECT"},
	 "user":{"id":1,"username":"$AUTHOR"},
	 "object_attributes":{"id":$MR_ID,"iid":$MR_IID,"title":"$MR_TITLE",
	   "description":"Refreshes OAuth tokens proactively.",
	   "source_branch":"$SOURCE_BRANCH","target_branch":"$TARGET_BRANCH",
	   "action":"open","state":"opened","url":"$MR_URL_BASE","timestamp":"$(now)"},
	 "reviewers":[]}
	EOF
	;;
comment)
	cat <<-EOF | send comment
	{"object_kind":"note","event_name":"note",
	 "project":{"id":9,"full_path":"$PROJECT","path_with_namespace":"$PROJECT"},
	 "user":{"id":3,"username":"bob"},
	 "object_attributes":{"id":501,"action":"create","noteable_type":"MergeRequest",
	   "created_at":"$(now)","note":{"id":501,"note":"ship it :shipit:","noteable_type":"MergeRequest"}},
	 "merge_request":{"id":$MR_ID,"iid":$MR_IID}}
	EOF
	;;
pipeline-pass | pipeline-fail)
	status=success
	[[ "${1}" == pipeline-fail ]] && status=failed
	cat <<-EOF | send "$1"
	{"object_kind":"pipeline","event_name":"pipeline",
	 "project":{"id":9,"full_path":"$PROJECT","path_with_namespace":"$PROJECT"},
	 "user":{"id":1,"username":"$AUTHOR"},
	 "object_attributes":{"id":7001,"status":"$status","ref":"$SOURCE_BRANCH",
	   "created_at":"$(now)","finished_at":"$(now)"},
	 "merge_request":{"id":$MR_ID,"iid":$MR_IID}}
	EOF
	;;
reviewed | changes)
	# A submitted review arrives as an MR update whose acting user moves from
	# unreviewed to the verdict inside changes.reviewers. The object form is
	# what GitLab really sends (docs notwithstanding; see event.go).
	verdict=reviewed
	[[ "${1}" == changes ]] && verdict=requested_changes
	cat <<-EOF | send "$1"
	{"object_kind":"merge_request","event_name":"merge_request",
	 "project":{"id":9,"full_path":"$PROJECT","path_with_namespace":"$PROJECT"},
	 "user":{"id":$REVIEWER_ID,"username":"$REVIEWER"},
	 "object_attributes":{"id":$MR_ID,"iid":$MR_IID,"title":"$MR_TITLE",
	   "source_branch":"$SOURCE_BRANCH","target_branch":"$TARGET_BRANCH",
	   "action":"update","state":"opened","url":"$MR_URL_BASE","timestamp":"$(now)"},
	 "reviewers":[{"id":$REVIEWER_ID,"state":"$verdict"}],
	 "changes":{"reviewers":{"previous":[{"id":$REVIEWER_ID,"state":"unreviewed"}],
	                           "current":[{"id":$REVIEWER_ID,"state":"$verdict"}]}}}
	EOF
	;;
approval)
	cat <<-EOF | send approval
	{"object_kind":"merge_request","event_name":"merge_request",
	 "project":{"id":9,"full_path":"$PROJECT","path_with_namespace":"$PROJECT"},
	 "user":{"id":$REVIEWER_ID,"username":"$REVIEWER"},
	 "object_attributes":{"id":$MR_ID,"iid":$MR_IID,"title":"$MR_TITLE",
	   "source_branch":"$SOURCE_BRANCH","target_branch":"$TARGET_BRANCH",
	   "action":"approval","state":"opened","url":"$MR_URL_BASE","timestamp":"$(now)"},
	 "reviewers":[{"id":$REVIEWER_ID,"state":"approved"}]}
	EOF
	;;
push)
	# GitLab sends a Push Hook (commit-count attribution) *and* an MR update
	# carrying oldrev (the source of the notice); delivery order is arbitrary.
	commits="$(
		for _ in $(seq "$N_COMMITS"); do printf '{"timestamp":"%s"},' "$(now)"; done | sed 's/,$//'
	)"
	cat <<-EOF | send "push hook"
	{"object_kind":"push","event_name":"push",
	 "project":{"id":9,"full_path":"$PROJECT","path_with_namespace":"$PROJECT"},
	 "user_name":"$COMMITTER","user_username":"$COMMITTER",
	 "ref":"refs/heads/$SOURCE_BRANCH",
	 "before":"0000000000000000000000000000000000000000",
	 "after":"1111111111111111111111111111111111111111",
	 "total_commits_count":$N_COMMITS,"commits":[$commits]}
	EOF
	cat <<-EOF | send "push update"
	{"object_kind":"merge_request","event_name":"merge_request",
	 "project":{"id":9,"full_path":"$PROJECT","path_with_namespace":"$PROJECT"},
	 "user":{"id":1,"username":"$AUTHOR"},
	 "object_attributes":{"id":$MR_ID,"iid":$MR_IID,"title":"$MR_TITLE",
	   "source_branch":"$SOURCE_BRANCH","target_branch":"$TARGET_BRANCH",
	   "action":"update","state":"opened","url":"$MR_URL_BASE","timestamp":"$(now)",
	   "oldrev":"0000000000000000000000000000000000000000"},
	 "reviewers":[]}
	EOF
	;;
merge)
	cat <<-EOF | send merge
	{"object_kind":"merge_request","event_name":"merge_request",
	 "project":{"id":9,"full_path":"$PROJECT","path_with_namespace":"$PROJECT"},
	 "user":{"id":1,"username":"$AUTHOR"},
	 "object_attributes":{"id":$MR_ID,"iid":$MR_IID,"title":"$MR_TITLE",
	   "source_branch":"$SOURCE_BRANCH","target_branch":"$TARGET_BRANCH",
	   "action":"merge","state":"merged","url":"$MR_URL_BASE","timestamp":"$(now)"},
	 "reviewers":[]}
	EOF
	;;
close)
	cat <<-EOF | send close
	{"object_kind":"merge_request","event_name":"merge_request",
	 "project":{"id":9,"full_path":"$PROJECT","path_with_namespace":"$PROJECT"},
	 "user":{"id":1,"username":"$AUTHOR"},
	 "object_attributes":{"id":$MR_ID,"iid":$MR_IID,"title":"$MR_TITLE",
	   "source_branch":"$SOURCE_BRANCH","target_branch":"$TARGET_BRANCH",
	   "action":"close","state":"closed","url":"$MR_URL_BASE","timestamp":"$(now)"},
	 "reviewers":[]}
	EOF
	;;
*)
	cat <<'EOF'
usage: ./demo/evmock/evmock.sh <kind>

  open              create the thread's root post
  comment           💬 comment reply + status refresh
  pipeline-pass     🟢 pipeline passed
  pipeline-fail     🔴 pipeline failed
  reviewed          👀 review submitted
  changes           🚨 changes requested
  approval          ✅ individual approval
  push              📝 N new commits (Push Hook + MR update, like GitLab)
  merge             🎉 merged notice; thread closes
  close             thread closes

Env knobs: URL, SIGNING_SECRET, PROJECT, GITLAB_URL, MR_ID, MR_IID,
MR_TITLE, SOURCE_BRANCH, TARGET_BRANCH, AUTHOR, REVIEWER, REVIEWER_ID,
COMMITTER, N_COMMITS. PROJECT must exist in the routing table.
EOF
	exit 2
	;;
esac
