-- 0.65.0: server administration separated from organization administration (ADR-0019), VPN phase 2 (ADR-0018
-- amendments: four eyes and time-limited access grants, management per organization).
--
-- Until now the built-in role org_admin of every organization contained system.admin, so the administrator of any
-- tenant controlled the whole server (all organizations, server settings, VPN, backups, updates, licence). From now on
-- system.admin belongs to the new built-in role server_admin only. Nobody is locked out: every user who holds
-- org_admin at this moment also gets server_admin (granted by this migration, recorded in the role history with the
-- reason below), so effective permissions do not change today. New organization administrators do not get it. The
-- operator should review who holds server_admin afterwards (Users → Roles) and revoke it where it is not needed.

-- an organization's own role named server_admin would shadow the built-in one when roles are granted by name
UPDATE core.role SET name = 'server_admin_' || substr(replace(id::text, '-', ''), 1, 6)
  WHERE org_id IS NOT NULL AND name = 'server_admin';

INSERT INTO core.permission (name, description) VALUES
  ('vpn.manage', 'Manage VPN peers, access rules and access grants of the own organization')
ON CONFLICT (name) DO NOTHING;
UPDATE core.permission SET description = 'Server administration: organizations, server settings, VPN of the whole server, backups, updates, licence'
  WHERE name = 'system.admin';

INSERT INTO core.role (org_id, name, permissions, is_admin, is_qa)
  SELECT NULL, 'server_admin', ARRAY['system.admin'], false, false
  WHERE NOT EXISTS (SELECT 1 FROM core.role WHERE org_id IS NULL AND name = 'server_admin');

-- keep today's effective permissions: current holders of org_admin (disabled accounts included) get server_admin too
INSERT INTO core.user_role_history (user_id, role_id, event, actor_id, reason)
SELECT cur.user_id, (SELECT id FROM core.role WHERE org_id IS NULL AND name = 'server_admin'), 'granted', NULL,
       'migration 0045 (0.65.0): server administration separated from org_admin; granted to keep existing access — review'
FROM (
  SELECT user_id, role_id, (array_agg(event ORDER BY ts DESC, id DESC))[1] AS ev
  FROM core.user_role_history GROUP BY user_id, role_id
) cur
WHERE cur.role_id = (SELECT id FROM core.role WHERE org_id IS NULL AND name = 'org_admin') AND cur.ev = 'granted'
  AND NOT EXISTS (
    SELECT 1 FROM core.user_role_history h
    WHERE h.user_id = cur.user_id AND h.role_id = (SELECT id FROM core.role WHERE org_id IS NULL AND name = 'server_admin'));

UPDATE core.role SET permissions = array_append(array_remove(permissions, 'system.admin'), 'vpn.manage')
  WHERE org_id IS NULL AND name = 'org_admin' AND NOT ('vpn.manage' = ANY (permissions));
UPDATE core.role SET permissions = array_remove(permissions, 'system.admin')
  WHERE org_id IS NULL AND name = 'org_admin';

-- server administrators need two-factor sign-in wherever organization administrators need it
UPDATE core.organization
SET settings = jsonb_set(settings, '{security,mfa_required_roles}', (settings->'security'->'mfa_required_roles') || '["server_admin"]'::jsonb)
WHERE jsonb_typeof(settings->'security'->'mfa_required_roles') = 'array'
  AND (settings->'security'->'mfa_required_roles') ? 'org_admin'
  AND NOT ((settings->'security'->'mfa_required_roles') ? 'server_admin');

-- VPN: rules owned by an organization (NULL = the whole server, managed with system.admin only)
ALTER TABLE core.vpn_rule ADD COLUMN org_id uuid REFERENCES core.organization(id);
CREATE INDEX ON core.vpn_rule (org_id) WHERE removed_at IS NULL;

-- VPN: time-limited access grants (a window in which a peer and its rules are active), optionally approved by a
-- second person (four eyes, organization policy four_eyes_vpn). Rows are history: never deleted.
CREATE TABLE core.vpn_grant (
  id                uuid PRIMARY KEY DEFAULT uuidv7(),
  peer_id           uuid NOT NULL REFERENCES core.vpn_peer(id),
  org_id            uuid REFERENCES core.organization(id),
  starts_at         timestamptz NOT NULL DEFAULT now(),
  ends_at           timestamptz NOT NULL,
  reason            text NOT NULL,
  requested_by      text NOT NULL,
  approved_by       text,                                         -- the second person (four eyes), NULL without approval
  change_request_id uuid REFERENCES core.change_request(id),
  created_at        timestamptz NOT NULL DEFAULT now(),
  ended_at          timestamptz,                                  -- revoked early or expired
  ended_by          text,
  end_reason        text,
  CHECK (ends_at > starts_at)
);
CREATE INDEX ON core.vpn_grant (peer_id, ends_at DESC);
GRANT SELECT, INSERT, UPDATE ON core.vpn_grant TO ctrl32_app;

ALTER TABLE core.change_request DROP CONSTRAINT IF EXISTS change_request_kind_check;
ALTER TABLE core.change_request ADD CONSTRAINT change_request_kind_check CHECK (kind IN ('limits', 'assignments', 'calibration', 'flows', 'vpn'));
