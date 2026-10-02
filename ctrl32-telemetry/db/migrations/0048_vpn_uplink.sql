-- 0.69.0: VPN phase 3 — uplink to a hub (ADR-0021, spec 027).
--
-- A customer server without a public address connects out to a hub (another telemetry server with a public address).
-- On the hub the customer server is a site peer marked as an uplink: its VPN address and the LAN subnets behind it are
-- routed like any site's (an uplink may offer no subnets: then only the server itself is reachable). The optional
-- HTTPS transport (WireGuard datagrams over a WebSocket through the hub's HTTPS port when UDP is blocked) is
-- authenticated with a per-uplink secret; the hub stores its SHA-256 only.
ALTER TABLE core.vpn_peer ADD COLUMN uplink boolean NOT NULL DEFAULT false;
ALTER TABLE core.vpn_peer ADD COLUMN tunnel_key_hash text CHECK (tunnel_key_hash IS NULL OR tunnel_key_hash ~ '^[0-9a-f]{64}$');
ALTER TABLE core.vpn_peer ADD CONSTRAINT vpn_peer_uplink_is_site CHECK (NOT uplink OR kind = 'site');
ALTER TABLE core.vpn_peer ADD CONSTRAINT vpn_peer_tunnel_key_uplink CHECK (tunnel_key_hash IS NULL OR uplink);
CREATE UNIQUE INDEX vpn_peer_tunnel_key ON core.vpn_peer (tunnel_key_hash) WHERE removed_at IS NULL AND tunnel_key_hash IS NOT NULL;

-- On the customer server: access rules for traffic arriving from the hub through the uplink (default deny). Source is
-- the hub's VPN range or an address/subnet inside it (NULL = the whole range); destination this server's uplink address
-- or a host/subnet of the networks it offers to the hub. Managed with system.admin only; rows are never deleted.
CREATE TABLE core.vpn_uplink_rule (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  src_cidr    cidr,
  dest_type   text NOT NULL CHECK (dest_type IN ('server', 'lan')),
  dest_cidr   cidr,
  protocol    text NOT NULL CHECK (protocol IN ('tcp', 'udp', 'icmp', 'any')),
  ports       text NOT NULL DEFAULT '' CHECK (ports ~ '^[0-9, -]*$' AND length(ports) <= 200),
  enabled     boolean NOT NULL DEFAULT true,
  created_at  timestamptz NOT NULL DEFAULT now(),
  created_by  text NOT NULL DEFAULT '',
  updated_at  timestamptz NOT NULL DEFAULT now(),
  removed_at  timestamptz,
  removed_by  text,
  CHECK (src_cidr IS NULL OR family(src_cidr) = 4),
  CHECK (dest_cidr IS NULL OR (dest_type = 'lan' AND family(dest_cidr) = 4))
);
GRANT SELECT, INSERT, UPDATE ON core.vpn_uplink_rule TO ctrl32_app;
