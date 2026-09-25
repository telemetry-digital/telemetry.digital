-- Four eyes for commands (URS H-08, A-12): when the organization policy four_eyes_commands is on, a command waits
-- as 'pending' until a different user approves it (then it is executed) or rejects it. Approval identity is kept
-- on the row; the audit trail records both steps.
ALTER TABLE core.device_command DROP CONSTRAINT IF EXISTS device_command_status_check;
ALTER TABLE core.device_command ADD CONSTRAINT device_command_status_check
  CHECK (status IN ('pending', 'queued', 'sent', 'acked', 'failed', 'expired', 'rejected'));
ALTER TABLE core.device_command
  ADD COLUMN IF NOT EXISTS approved_by      uuid,
  ADD COLUMN IF NOT EXISTS approved_by_name text,
  ADD COLUMN IF NOT EXISTS approved_at      timestamptz;
