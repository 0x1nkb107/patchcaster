-- name: StateEnsure :exec
-- StateEnsure creates the board row of a merge request if it is missing.
-- The first delivery that names the merge request creates it, even a note or
-- pipeline delivery that arrives before the merge request event
-- (SYSREQ-027).
INSERT INTO mr_state (mr_ref, updated_at)
VALUES ($1, $2)
ON CONFLICT (mr_ref) DO NOTHING;

-- name: StateGet :one
-- StateGet reads the board of a merge request for rendering.
SELECT mr_ref, title, author, url, iid, source_branch, target_branch,
    project_path, source_project_id, open, pipeline_status, comment_count,
    edit_count, reviewers_done, reviewers_required, last_push_count
FROM mr_state
WHERE mr_ref = $1;

-- name: StateGetForUpdate :one
-- StateGetForUpdate reads the board of a merge request and locks its row, so
-- deliveries of one merge request apply their state one after another.
SELECT mr_ref, title, author, url, iid, source_branch, target_branch,
    project_path, source_project_id, open, pipeline_status, comment_count,
    edit_count, reviewers_done, reviewers_required, last_push_count
FROM mr_state
WHERE mr_ref = $1
FOR UPDATE;

-- name: StateApplyMR :exec
-- StateApplyMR stores the latest merge request facts of a merge request
-- delivery and adds its edit count.
UPDATE mr_state
SET title = $2, author = $3, url = $4, iid = $5, source_branch = $6,
    target_branch = $7, source_project_id = $8, open = $9,
    reviewers_done = $10, reviewers_required = $11,
    edit_count = edit_count + $12, updated_at = $13, project_path = $14
WHERE mr_ref = $1;

-- name: StateSetPipeline :execrows
-- StateSetPipeline stores a pipeline status only if it differs from the
-- stored one, so the affected row count tells whether the board moved.
UPDATE mr_state
SET pipeline_status = $2, updated_at = $3
WHERE mr_ref = $1 AND pipeline_status <> $2;

-- name: StateIncrComment :execrows
-- StateIncrComment counts one more human comment on a merge request.
UPDATE mr_state
SET comment_count = comment_count + 1, updated_at = $2
WHERE mr_ref = $1;

-- name: StateAttributePush :execrows
-- StateAttributePush gives a push's commit count to the open merge requests
-- of the pushed project and branch (SYSREQ-030).
UPDATE mr_state
SET last_push_count = $3, updated_at = $4
WHERE open AND source_project_id = $1 AND source_branch = $2;

-- name: StateClose :exec
-- StateClose marks the board of a merged or closed merge request.
UPDATE mr_state
SET open = FALSE, updated_at = $2
WHERE mr_ref = $1;
