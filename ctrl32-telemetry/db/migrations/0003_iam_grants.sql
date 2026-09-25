-- ctrl32 telemetry — grants for tables introduced in 0002 (default privileges give the app role SELECT/INSERT only).
-- Rule for future migrations: every new mutable table needs an explicit GRANT UPDATE (and DELETE only if truly needed).
BEGIN;
GRANT UPDATE ON core.user_recovery_code TO ctrl32_app;   -- used_at is set when a recovery code is consumed
COMMIT;
