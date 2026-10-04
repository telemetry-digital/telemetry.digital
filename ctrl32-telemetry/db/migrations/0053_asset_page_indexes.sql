-- Asset and site pages (0.74.0): the assets list counts the devices feeding each asset and its active alarms, the
-- asset page walks the hierarchy and lists the devices of its datastreams. Indexes for those lookups only.
CREATE INDEX IF NOT EXISTS assignment_current_to_idx ON core.assignment (to_id) WHERE valid_to IS NULL;
CREATE INDEX IF NOT EXISTS assignment_current_from_idx ON core.assignment (from_id) WHERE valid_to IS NULL;
CREATE INDEX IF NOT EXISTS asset_parent_idx ON core.asset (parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS alarm_open_datastream_idx ON core.alarm (datastream_id) WHERE cleared_at IS NULL;
