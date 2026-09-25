-- Connector kinds beyond the ChirpStack HTTP integration (URS E-*): OPC UA client polling (0.13.0) and Modbus TCP (later).
ALTER TABLE core.connector DROP CONSTRAINT IF EXISTS connector_kind_check;
ALTER TABLE core.connector ADD CONSTRAINT connector_kind_check CHECK (kind IN ('chirpstack_http', 'opcua', 'modbus_tcp'));
