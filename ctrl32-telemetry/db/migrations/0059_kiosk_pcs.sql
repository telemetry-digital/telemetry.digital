-- Kiosk PCs and display health (0.86.0, spec 034, ADR-0025). A kiosk PC is the Windows computer behind one or more
-- displays (one Chrome kiosk per monitor, each paired as its own display). Its small agent — a readable PowerShell
-- script running as a scheduled task — reports the PC's state and performs a closed list of operations the
-- operators ask for: restart the browsers, reboot, refresh the report, update its scripts, and a screenshot. A
-- screenshot can show unmasked camera pictures: it is off per PC until an administrator turns it on with a reason,
-- each one needs display.screenshot and a reason, is audited, and is kept in memory only until it is fetched.
-- The agent's credential is a random token; only its hash is stored.
CREATE TABLE core.kiosk_pc (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id          uuid NOT NULL REFERENCES core.organization(id),
  name            text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  monitors        int NOT NULL DEFAULT 1 CHECK (monitors BETWEEN 1 AND 8),
  server_url      text NOT NULL DEFAULT '' CHECK (length(server_url) <= 200), -- the server as this PC reaches it
  token_hash      text UNIQUE,
  screenshots     boolean NOT NULL DEFAULT false,
  mesh_node       text NOT NULL DEFAULT '' CHECK (length(mesh_node) <= 120),   -- MeshCentral node (emergency remote desktop)
  report          jsonb,                       -- the last report of its agent (PC, Windows, Chrome, load, monitors)
  command         jsonb,                       -- the pending operation {seq, action, by, until}
  result          jsonb,                       -- the agent's answer to the last operation
  created_at      timestamptz NOT NULL DEFAULT now(),
  created_by      uuid,
  updated_at      timestamptz NOT NULL DEFAULT now(),
  last_seen_at    timestamptz,
  last_ip         text,
  revoked_at      timestamptz
);
CREATE INDEX ON core.kiosk_pc (org_id) WHERE revoked_at IS NULL;
GRANT SELECT, INSERT, UPDATE ON core.kiosk_pc TO ctrl32_app;

-- a display runs on a monitor of a kiosk PC; the display page reports what it shows (camera tiles playing or not,
-- screen size, page version) with its poll
ALTER TABLE core.display ADD COLUMN IF NOT EXISTS kiosk_pc_id uuid REFERENCES core.kiosk_pc(id);
ALTER TABLE core.display ADD COLUMN IF NOT EXISTS kiosk_monitor int CHECK (kiosk_monitor BETWEEN 0 AND 7);
ALTER TABLE core.display ADD COLUMN IF NOT EXISTS health jsonb;
ALTER TABLE core.display ADD COLUMN IF NOT EXISTS health_at timestamptz;

-- the MeshCentral address and node of this server: the emergency remote desktop (System → Server)
ALTER TABLE core.server_setting DROP CONSTRAINT IF EXISTS server_setting_key_check;
ALTER TABLE core.server_setting ADD CONSTRAINT server_setting_key_check CHECK (key IN ('updates', 'smtp', 'remote_management'));

INSERT INTO core.permission (name, description) VALUES
  ('display.screenshot', 'See screenshots of kiosk PCs (may show unmasked camera pictures)')
ON CONFLICT (name) DO NOTHING;
UPDATE core.role SET permissions = array_append(permissions, 'display.screenshot')
  WHERE org_id IS NULL AND name IN ('org_admin') AND NOT ('display.screenshot' = ANY (permissions));
