-- Own WireGuard VPN management (spec 024, ADR-0018, 0.64.0): peers and access rules.
--
-- The server is the hub. A peer is a device (sends data through the tunnel), a user (a technician's laptop or phone)
-- or a site (a router or Raspberry Pi that connects the LAN behind it: routed subnets). Access from peers is default
-- deny: only access rules open something, towards the server itself or towards other peers and site subnets. The web
-- service owns these rows and sends the complete state to the privileged agent, which applies the WireGuard
-- interface and an nftables table together. Private keys of peers are never stored (shown once); preshared keys stay
-- with the agent (root only). Rows are not deleted: removal sets removed_at, so the audit trail keeps its subjects.

CREATE TABLE core.vpn_peer (
  id             uuid PRIMARY KEY DEFAULT uuidv7(),
  name           text NOT NULL CHECK (name ~ '^[A-Za-z0-9][A-Za-z0-9 ._-]{0,62}$'),
  kind           text NOT NULL CHECK (kind IN ('device', 'user', 'site')),
  org_id         uuid REFERENCES core.organization(id),          -- owner organization; NULL = the whole server
  device_id      uuid REFERENCES core.device(id),                -- optional link to a device
  address        inet NOT NULL CHECK (family(address) = 4 AND masklen(address) = 32),
  public_key     text NOT NULL CHECK (public_key ~ '^[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]=$'),
  subnets        cidr[] NOT NULL DEFAULT '{}',                   -- routed subnets of a site (site-to-site)
  enabled        boolean NOT NULL DEFAULT true,
  expires_at     timestamptz,                                    -- time-limited access: disabled automatically
  note           text NOT NULL DEFAULT '' CHECK (length(note) <= 200),
  created_at     timestamptz NOT NULL DEFAULT now(),
  created_by     text NOT NULL DEFAULT '',
  updated_at     timestamptz NOT NULL DEFAULT now(),
  key_rotated_at timestamptz,
  removed_at     timestamptz,
  removed_by     text,
  CHECK (kind = 'site' OR cardinality(subnets) = 0),
  CHECK (cardinality(subnets) <= 16)
);
CREATE UNIQUE INDEX vpn_peer_address ON core.vpn_peer (address) WHERE removed_at IS NULL;
CREATE UNIQUE INDEX vpn_peer_name ON core.vpn_peer (lower(name)) WHERE removed_at IS NULL;
CREATE UNIQUE INDEX vpn_peer_key ON core.vpn_peer (public_key) WHERE removed_at IS NULL;
CREATE INDEX ON core.vpn_peer (org_id) WHERE removed_at IS NULL;

-- Access rule: sources -> destination : protocol/ports. Sources is a JSON list of
-- {"type":"all"} | {"type":"kind","kind":"user"} | {"type":"peer","id":…} | {"type":"org","id":…}.
CREATE TABLE core.vpn_rule (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  sources      jsonb NOT NULL CHECK (jsonb_typeof(sources) = 'array' AND jsonb_array_length(sources) BETWEEN 1 AND 20),
  dest_type    text NOT NULL CHECK (dest_type IN ('server', 'peer', 'site')),
  dest_peer_id uuid REFERENCES core.vpn_peer(id),
  dest_cidr    cidr,
  protocol     text NOT NULL CHECK (protocol IN ('tcp', 'udp', 'icmp', 'any')),
  ports        text NOT NULL DEFAULT '' CHECK (ports ~ '^[0-9, -]*$' AND length(ports) <= 200),
  enabled      boolean NOT NULL DEFAULT true,
  created_at   timestamptz NOT NULL DEFAULT now(),
  created_by   text NOT NULL DEFAULT '',
  updated_at   timestamptz NOT NULL DEFAULT now(),
  removed_at   timestamptz,
  removed_by   text,
  CHECK ((dest_type = 'server') = (dest_peer_id IS NULL)),
  CHECK (dest_type = 'site' OR dest_cidr IS NULL)
);

GRANT SELECT, INSERT, UPDATE ON core.vpn_peer, core.vpn_rule TO ctrl32_app;
