-- Smart home, phase 2 (spec 006).
-- An entity renamed by a user keeps its name when the device announces its discovery config again (zigbee2mqtt does so
-- at every start); until then the device's name is used.
ALTER TABLE core.entity ADD COLUMN name_locked boolean NOT NULL DEFAULT false;

-- An MQTT source (bridge or account) disconnected longer than its limit opens an incident.
ALTER TABLE core.incident DROP CONSTRAINT IF EXISTS incident_kind_check;
ALTER TABLE core.incident ADD CONSTRAINT incident_kind_check CHECK (kind IN ('alarm_undelivered', 'audit_chain', 'backup_failed', 'manual', 'replica', 'flow', 'mqtt_source'));
