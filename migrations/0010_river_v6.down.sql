DROP INDEX river_job_unique_idx;

ALTER TABLE river_job
    DROP COLUMN unique_states;

CREATE UNIQUE INDEX IF NOT EXISTS river_job_kind_unique_key_idx ON river_job (kind, unique_key) WHERE unique_key IS NOT NULL;

DROP FUNCTION river_job_state_in_bitmask;
