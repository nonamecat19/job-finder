-- name: GetHostRetrievalState :one
SELECT * FROM host_retrieval_state WHERE host = $1;

-- name: UpsertHostRetrievalState :exec
INSERT INTO host_retrieval_state (
    host, identity_version, current_rung, rung_last_verified_at,
    cookies, consecutive_blocks, cooling_off_until,
    last_block_at, last_block_reason, crawl_delay_seconds
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    $8, $9, $10
) ON CONFLICT (host) DO UPDATE SET
    identity_version = EXCLUDED.identity_version,
    current_rung = EXCLUDED.current_rung,
    rung_last_verified_at = EXCLUDED.rung_last_verified_at,
    cookies = EXCLUDED.cookies,
    consecutive_blocks = EXCLUDED.consecutive_blocks,
    cooling_off_until = EXCLUDED.cooling_off_until,
    last_block_at = EXCLUDED.last_block_at,
    last_block_reason = EXCLUDED.last_block_reason,
    crawl_delay_seconds = EXCLUDED.crawl_delay_seconds,
    updated_at = now();

-- name: ClearHostCookies :exec
UPDATE host_retrieval_state SET cookies = NULL, updated_at = now() WHERE host = $1;

-- name: ClearHostRung :exec
UPDATE host_retrieval_state SET current_rung = 'direct', rung_last_verified_at = NULL, updated_at = now() WHERE host = $1;

-- name: RecordHostBlock :exec
UPDATE host_retrieval_state SET
    consecutive_blocks = consecutive_blocks + 1,
    last_block_at = now(),
    last_block_reason = $2,
    updated_at = now()
WHERE host = $1;

-- name: RecordHostSuccess :exec
UPDATE host_retrieval_state SET
    current_rung = $2,
    consecutive_blocks = 0,
    rung_last_verified_at = now(),
    updated_at = now()
WHERE host = $1;

-- name: UpdateHostCookies :exec
UPDATE host_retrieval_state SET cookies = $2, updated_at = now() WHERE host = $1;

-- name: SetHostCrawlDelay :exec
UPDATE host_retrieval_state SET crawl_delay_seconds = $2, updated_at = now() WHERE host = $1;
