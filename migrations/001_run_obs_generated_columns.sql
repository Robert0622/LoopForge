-- 001: generated columns for run observability (MySQL only)
--
-- run_telemetry and run_logs store each event as a JSON payload. On MySQL, the
-- telemetry filter in run details (internal/repo/runs.go adapterSQLFilter) and
-- the analytics coverage query (internal/repo/analytics.go
-- queryAnalyticsCoverage) read the generated columns adapter and level added
-- here. Tables are created automatically on startup; apply this script once
-- afterwards. SQLite needs nothing: it parses the JSON in code.
--
-- Notes:
-- - Adding a STORED generated column rebuilds the table; on large tables, run
--   it in a maintenance window.
-- - MySQL 8.0 has no ADD COLUMN IF NOT EXISTS, so the script is not
--   re-runnable. Check information_schema.columns first if unsure.
-- - Startup migrations never drop these columns.

ALTER TABLE run_telemetry
  ADD COLUMN adapter VARCHAR(255)
    GENERATED ALWAYS AS (COALESCE(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(payload, '$.source.adapter')), ''), 'unknown')) STORED,
  ADD KEY idx_run_telemetry_run_adapter (run_id, adapter);

ALTER TABLE run_logs
  ADD COLUMN level VARCHAR(32)
    GENERATED ALWAYS AS (COALESCE(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(payload, '$.level')), ''), 'info')) STORED,
  ADD KEY idx_run_logs_run_level (run_id, level);
