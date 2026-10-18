-- Siemens S7 connectors (0.83.0): the server reads data blocks, merkers, inputs and outputs of an S7-300/400/1200/1500
-- or LOGO! over S7comm (ISO-on-TCP, port 102) and feeds a virtual device (transport s7); writable tags accept the
-- command write {key, value} like Modbus registers and OPC UA nodes (limits, read back, audit trail).
ALTER TABLE core.connector DROP CONSTRAINT IF EXISTS connector_kind_check;
ALTER TABLE core.connector ADD CONSTRAINT connector_kind_check CHECK (kind IN ('chirpstack_http', 'opcua', 'modbus_tcp', 'mqtt_account', 'mqtt_bridge', 's7'));
ALTER TABLE core.device DROP CONSTRAINT IF EXISTS device_transport_check;
ALTER TABLE core.device ADD CONSTRAINT device_transport_check CHECK (transport IN ('mqtt', 'http', 'lorawan', 'opcua', 'modbus', 'mqtt_discovery', 's7'));
