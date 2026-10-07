CREATE TABLE IF NOT EXISTS llm_requests (
    id                BIGSERIAL   PRIMARY KEY,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    model             TEXT        NOT NULL,
    prompt            TEXT        NOT NULL,
    response          TEXT        NOT NULL DEFAULT '',
    prompt_tokens     INTEGER     NOT NULL DEFAULT 0,
    completion_tokens INTEGER     NOT NULL DEFAULT 0,
    total_tokens      INTEGER     NOT NULL DEFAULT 0,
    latency_ms        INTEGER     NOT NULL DEFAULT 0,
    status            TEXT        NOT NULL DEFAULT 'ok',
    error_message     TEXT        NOT NULL DEFAULT ''
);

-- История всегда читается «последние N», поэтому индекс по убыванию даты.
CREATE INDEX IF NOT EXISTS llm_requests_created_at_idx
    ON llm_requests (created_at DESC);
