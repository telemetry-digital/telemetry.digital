// ctrl32 telemetry API client — generated from the OpenAPI document by tools/apigen. Do not edit; run: go run ./tools/apigen
// Usage: const api = new Ctrl32Client({ baseUrl: 'https://telemetry.example.com', token: 'c32_…' });
//        const devices = await api.getDevices();
// In a browser signed in with a session, pass { csrfToken } from getAuthMe() instead of a token.

export type APIToken = {
  created_at?: string;
  expires_at?: string | null;
  id?: string;
  last_used_at?: string | null;
  name?: string;
  revoked_at?: string | null;
  scopes?: string[];
};

export type AccountRequest = {
  created_at?: string;
  email?: string;
  full_name?: string;
  id?: string;
  message?: string;
};

export type Alarm = {
  cleared_at?: string | null;
  id?: string;
  /** datastream key, or input name for rule alarms */
  key?: string;
  raised_at?: string;
  /** raised on backfilled data */
  retrospective?: boolean;
  severity?: "warning" | "action" | "critical";
  state?: "active" | "acknowledged" | "suppressed" | "cleared";
  value?: number | null;
};

export type AlarmRule = {
  action?: number | null;
  asset?: string;
  datastream_id?: string;
  delay_seconds?: number;
  escalation_policy_id?: string | null;
  hysteresis?: number;
  id?: string;
  key?: string;
  reason?: string;
  type?: string;
  valid_from?: string;
  version?: number;
  warning?: number | null;
};

export type Asset = {
  attributes?: Record<string, unknown>;
  created_at?: string;
  datastreams?: number;
  id?: string;
  name?: string;
  parent_id?: string | null;
  site?: string;
  site_id?: string;
  type?: string;
};

export type Branding = {
  /** product footer: text, github, license, links, show_version, white_label, intro */
  footer?: Record<string, unknown>;
  /** browser icon */
  icon_url?: string;
  logo_custom?: boolean;
  logo_url?: string;
  map?: MapSettings;
  org_name?: string;
  product?: string;
  public_read?: boolean;
  theme?: Theme;
  version?: string;
  white_label?: {
    licensed?: boolean;
  };
};

export type Camera = {
  audio?: boolean;
  audio_record?: boolean;
  enabled?: boolean;
  /** camera group */
  group?: string;
  /** video.manage only */
  has_password?: boolean;
  has_sub?: boolean;
  id?: string;
  location?: string;
  motion_detect?: boolean;
  motion_sensitivity?: number;
  motion_zones?: number[][][];
  name?: string;
  privacy_masks?: number[][][];
  push?: {
    /** video.push_listen configured */
    enabled?: boolean;
    /** sub-stream */
    sub_url?: string;
    /** where the relay publishes the main stream */
    url?: string;
    /** = camera id */
    user?: string;
  };
  record?: "continuous" | "off";
  retention_days?: number;
  /** video.manage only */
  snapshot_url?: string;
  sort?: number;
  source?: "pull" | "push";
  status?: CameraStatus;
  /** video.manage only */
  sub_url?: string;
  /** video.manage only */
  transport?: string;
  /** video.manage only */
  url?: string;
  /** video.manage only */
  username?: string;
};

export type CameraStatus = {
  active?: string[];
  codec?: "h264" | "h265" | "mjpeg";
  /** password removed */
  error?: string;
  /** ONVIF event connection: connected, connecting or the error */
  events?: string;
  fps?: number;
  height?: number;
  kbps?: number;
  /** push cameras with ONVIF: the relay polls for camera control requests */
  onvif_relay?: boolean;
  /** events when recording only around events */
  record_mode?: string;
  recording?: boolean;
  since?: string;
  state?: "idle" | "connecting" | "online" | "offline" | "disabled";
  sub?: StreamInfo;
  viewers?: number;
  width?: number;
};

export type Campaign = {
  close_reason?: string | null;
  closed_at?: string | null;
  /** status → number of devices */
  counts?: Record<string, unknown>;
  created_at?: string;
  created_by?: string;
  firmware?: string;
  firmware_id?: string;
  hw?: string;
  id?: string;
  name?: string;
};

export type ChangeRequest = {
  /** the original request body */
  body?: Record<string, unknown>;
  /** the caller may approve it */
  can_approve?: boolean;
  decided_at?: string | null;
  decided_by?: string | null;
  decision_reason?: string | null;
  expires_at?: string;
  id?: string;
  kind?: "limits" | "assignments" | "calibration" | "flows" | "vpn";
  method?: string;
  path?: string;
  reason?: string;
  requested_at?: string;
  requested_by?: string;
  /** code and body of the replayed call */
  result?: unknown;
  status?: "pending" | "approved" | "rejected" | "expired" | "failed";
  /** what the change does, in words */
  summary?: string;
};

export type Channel = {
  created_at?: string;
  enabled?: boolean;
  has_secret?: boolean;
  id?: string;
  kind?: "email" | "webhook" | "sms_http";
  name?: string;
  revoked_at?: string | null;
  settings?: ChannelSettings;
};

export type ChannelSettings = {
  /** sms_http POST body template */
  body?: string;
  headers?: Record<string, string>;
  /** sms_http */
  method?: "GET" | "POST";
  recipients?: string[];
  /** webhook: HMAC-SHA256 key (X-Ctrl32-Signature) */
  secret?: string;
  /** webhook or SMS gateway URL; {to} and {text} placeholders for sms_http */
  url?: string;
};

export type Command = {
  acked_at?: string | null;
  approved_by?: string | null;
  args?: Record<string, unknown>;
  error?: string | null;
  expires_at?: string | null;
  id?: string;
  issued_at?: string;
  /** username or rule:<name> */
  issued_by?: string;
  method?: string;
  reason?: string;
  result?: unknown;
  sent_at?: string | null;
  status?: "pending" | "queued" | "sent" | "acked" | "failed" | "expired" | "rejected";
};

export type CommandResult = {
  error?: string;
  expires_at?: string;
  id?: string;
  method?: string;
  /** connector writes: {readback} */
  result?: unknown;
  status?: string;
};

export type Connector = {
  created_at?: string;
  id?: string;
  kind?: string;
  last_seen_at?: string | null;
  messages?: number;
  name?: string;
  revoked_at?: string | null;
  settings?: ConnectorSettings;
  /** opcua/modbus: connection state, last values, errors, writes */
  status?: Record<string, unknown>;
  /** chirpstack: ingest URL */
  url?: string;
};

export type ConnectorCreated = {
  /** chirpstack: integration hints */
  chirpstack?: Record<string, unknown>;
  id?: string;
  kind?: string;
  name?: string;
  /** chirpstack: bearer secret, shown once */
  secret?: string;
  settings?: ConnectorSettings;
  /** chirpstack: ingest URL */
  url?: string;
};

export type ConnectorSettings = {
  /** opcua: anonymous | username */
  auth?: string;
  /** modbus */
  byte_order?: "big" | "big_swap" | "little" | "little_swap";
  /** virtual device the values arrive under */
  device_external_id?: string;
  /** opcua: opc.tcp://host:4840 */
  endpoint?: string;
  /** modbus */
  framing?: "tcp" | "rtuovertcp";
  /** modbus */
  host?: string;
  /** polling interval */
  interval_s?: number;
  nodes?: (Record<string, unknown>)[];
  password?: string;
  /** modbus, default 502 */
  port?: number;
  /** modbus: register profile version the registers come from */
  profile_id?: string;
  /** modbus, informational */
  profile_name?: string;
  /** modbus, informational */
  profile_version?: number;
  registers?: ModbusRegister[];
  /** opcua */
  security_mode?: string;
  /** opcua */
  security_policy?: string;
  /** modbus */
  timeout_ms?: number;
  /** modbus */
  unit_id?: number;
  username?: string;
};

export type ConsoleSession = {
  close_reason?: string | null;
  closed_at?: string | null;
  id?: string;
  last_activity_at?: string;
  lines?: number;
  opened_at?: string;
  opened_by?: string;
};

export type Dashboard = {
  checksum?: string;
  created_at?: string;
  id?: string;
  is_default?: boolean;
  name?: string;
  slug?: string;
  spec?: DashboardSpec;
  version?: number;
};

export type DashboardSpec = {
  /** screen-v1 */
  format?: string;
  /** {type: grid, columns: 12, row_height} */
  layout?: Record<string, unknown>;
  widgets?: (Record<string, unknown>)[];
};

export type DashboardSummary = {
  created_at?: string;
  id?: string;
  is_default?: boolean;
  name?: string;
  slug?: string;
  version?: number;
  widgets?: number;
};

export type Datastream = {
  /** asset name */
  asset?: string;
  asset_id?: string;
  expected_interval?: string;
  id?: string;
  key?: string;
  kind?: "gauge" | "counter" | "state" | "event";
  quantity?: string;
  unit?: string;
};

export type Device = {
  /** MQTT session open */
  connected?: boolean;
  external_id?: string;
  id?: string;
  last_seen?: string | null;
  location?: {
    lat?: number;
    lon?: number;
    /** manual | reported */
    source?: string;
    updated_at?: string;
  } | null;
  name?: string;
  revoked_at?: string | null;
  /** last_seen, fw, battery, rssi, last_values, … */
  status?: Record<string, unknown>;
  transport?: "mqtt" | "http" | "lorawan" | "opcua" | "modbus";
};

export type Display = {
  /** current operator command */
  command?: Record<string, unknown>;
  content?: DisplayContent;
  created_at?: string;
  id?: string;
  last_agent?: string;
  last_ip?: string;
  last_seen_at?: string;
  location?: string;
  /** current message */
  message?: Record<string, unknown>;
  name?: string;
  offline_alert?: boolean;
  /** reported within 90 s */
  online?: boolean;
  /** the device collected its credential */
  paired?: boolean;
  updated_at?: string;
};

export type DisplayContent = {
  items: ({
    kind: "dashboard" | "wall";
    /** screen of the wall, from 0 */
    screen?: number;
    /** 10–3600, when shown in turn */
    seconds?: number;
    /** dashboard or process screen */
    slug?: string;
    /** video wall */
    wall?: string;
  })[];
  /** language of the display */
  locale?: string;
  /** enlarge cameras with an event */
  popup?: "none" | "wall" | "all";
  /** 5–600 */
  popup_seconds?: number;
};

export type Error = {
  /** human-readable message */
  error: string;
};

export type EscalationPolicy = {
  id?: string;
  name?: string;
  revoked_at?: string | null;
  /** alarm rules using it */
  rules?: number;
  steps?: EscalationStep[];
  version?: number;
};

export type EscalationStep = {
  /** seconds after the alarm */
  after_s: number;
  channel_id: string;
  /** in responses */
  channel_name?: string;
};

export type Firmware = {
  campaigns?: number;
  hw?: string;
  id?: string;
  min_version?: string;
  name?: string;
  notes?: string;
  sha256?: string;
  signed?: boolean;
  size?: number;
  uploaded_at?: string;
  uploaded_by?: string;
  version?: string;
};

export type Flow = {
  checksum?: string;
  created_at?: string;
  created_by?: string;
  deployed_version?: number | null;
  has_commands?: boolean;
  /** this version */
  id?: string;
  name: string;
  nodes?: number;
  reason?: string;
  slug: string;
  spec?: FlowSpec;
  state: "draft" | "deployed" | "disabled";
  version: number;
};

export type FlowProblem = {
  message: string;
  node?: string;
};

export type FlowSpec = {
  nodes: ({
    disabled?: boolean;
    id: string;
    name?: string;
    settings?: Record<string, unknown>;
    /** node type from the catalogue */
    type: string;
    x?: number;
    y?: number;
  })[];
  wires: ({
    from: string;
    /** output index */
    port: number;
    to: string;
  })[];
};

export type Incident = {
  /** Corrective and preventive action */
  capa?: string;
  cause?: string;
  closed_at?: string | null;
  closed_by?: string;
  /** Source data: alarm id and step, channel, error, sequence number … */
  detail?: Record<string, unknown>;
  id?: string;
  impact?: string;
  /** alarm_undelivered, audit_chain, backup_failed, manual, replica, flow or mqtt_source */
  kind?: string;
  opened_at?: string;
  /** Username or system */
  opened_by?: string;
  /** Empty for instance-level incidents */
  org_id?: string;
  org_name?: string;
  resolved_at?: string | null;
  /** minor, major or critical */
  severity?: string;
  /** open, investigating, resolved or closed */
  status?: string;
  title?: string;
  updated_at?: string;
};

export type IncidentEvent = {
  actor?: string;
  at?: string;
  detail?: Record<string, unknown>;
  /** opened, fields, note, status or notified */
  event?: string;
  id?: string;
};

export type LandingBlock = {
  address?: string;
  align?: "left" | "center";
  /** #rrggbb */
  background?: string;
  /** #rrggbb gradient end */
  background2?: string;
  buttons?: ({
    /** /path, https://…, mailto:, tel: */
    href?: string;
    label?: string;
    style?: "primary" | "secondary";
  })[];
  /** 1–4 */
  columns?: number;
  email?: string;
  /** [a-z0-9_-]{1,24}, unique */
  id: string;
  /** "logo" or an asset id */
  image?: string;
  image_side?: "left" | "right";
  items?: ({
    /** catalogue icon */
    icon?: string;
    text?: string;
    title?: string;
    value?: string;
  })[];
  phone?: string;
  size?: "small" | "medium" | "large";
  /** ≤ 300 */
  subtitle?: string;
  /** ≤ 4000; blank line = paragraph, "- " = list, **bold** */
  text?: string;
  /** #rrggbb */
  text_color?: string;
  /** ≤ 160 */
  title?: string;
  type: "hero" | "features" | "text" | "image" | "split" | "stats" | "cta" | "contact" | "spacer";
};

export type LandingSpec = {
  blocks: LandingBlock[];
  /** ≤ 200 */
  description?: string;
  theme?: {
    /** #rrggbb */
    accent?: string;
    /** #rrggbb */
    background?: string;
    /** #rrggbb */
    text?: string;
    width?: "normal" | "wide" | "full";
  };
  /** ≤ 80 */
  title?: string;
  /** 1 */
  version: number;
};

export type LatestValue = {
  asset?: string;
  key?: string;
  measured_at?: string;
  quality?: string;
  unit?: string;
  value?: number | null;
  value_json?: unknown;
};

export type LoginResponse = {
  /** MFA step needed */
  mfa_required?: boolean;
  must_change_password?: boolean;
  /** for POST /api/v1/auth/mfa */
  pre_auth_token?: string;
  user?: Me;
};

export type MCPConsent = {
  can_enable?: boolean;
  client_name?: string;
  client_uri?: string;
  /** AI access of the organization */
  enabled?: boolean;
  /** OAuth error to report to the client */
  error?: string;
  full_name?: string;
  organization?: string;
  read_only_requested?: boolean;
  redirect?: string;
  /** where the code goes */
  redirect_host?: string;
  user?: string;
  /** without the license only reading can be approved */
  write_licensed?: boolean;
};

export type MCPGrant = {
  client_name?: string;
  client_uri?: string;
  created_at?: string;
  id?: string;
  last_used_at?: string | null;
  read_only?: boolean;
  redirect_host?: string;
  revoked_at?: string | null;
  revoked_reason?: string;
  user?: string;
};

export type MCPSettings = {
  /** OAuth metadata */
  authorization_server?: string;
  buy_url?: string;
  /** user.admin */
  can_edit?: boolean;
  enabled?: boolean;
  /** API operations offered as tools */
  operations?: number;
  tools?: number;
  /** the MCP address to add in an AI assistant */
  url?: string;
  /** changes through assistants are licensed (the white-labeling license); without it assistants only read */
  write_licensed?: boolean;
};

export type MapSettings = {
  attribution?: string;
  /** built-in OpenStreetMap source in use (read-only) */
  builtin?: boolean;
  center_lat?: number;
  center_lon?: number;
  /** the server's PMTiles archive in use (read-only) */
  local?: boolean;
  max_zoom?: number;
  /** custom, pmtiles or builtin (read-only) */
  source?: string;
  /** XYZ tile URL template; empty = the server's PMTiles archive when configured, otherwise built-in OpenStreetMap */
  tile_url?: string;
  zoom?: number;
};

export type Me = {
  auth?: "session" | "token";
  /** send as X-CSRF-Token with cookie sessions */
  csrf_token?: string;
  full_name?: string;
  locale?: string;
  mfa_enabled?: boolean;
  mfa_required?: boolean;
  must_change_password?: boolean;
  org_id?: string;
  permissions?: string[];
  roles?: string[];
  user_id?: string;
  username?: string;
};

export type Message = {
  connector?: string;
  decode_error?: string | null;
  decode_status?: string;
  direction?: "in" | "out";
  id?: string;
  meta?: Record<string, unknown>;
  payload_json?: unknown;
  payload_text?: string;
  received_at?: string;
  topic?: string;
};

export type ModbusRegister = {
  /** 0-based */
  address: number;
  /** ^[a-z][a-z0-9_]{0,31}$ */
  key: string;
  /** limit for writes */
  max?: number;
  /** limit for writes */
  min?: number;
  offset?: number;
  scale?: number;
  table: "holding" | "input" | "coil" | "discrete";
  /** default int16 (bool for coils) */
  type?: "bool" | "int16" | "uint16" | "int32" | "uint32" | "float32" | "int64" | "uint64" | "float64";
  /** holding registers and coils only */
  writable?: boolean;
};

export type Notification = {
  alarm_id?: string | null;
  attempts?: number;
  channel?: string | null;
  due_at?: string;
  error?: string | null;
  event?: "raised" | "escalated" | "cleared" | "test" | "rule";
  id?: string;
  kind?: string | null;
  sent_at?: string | null;
  status?: "scheduled" | "sent" | "failed" | "cancelled";
  step?: number;
  subject?: string;
};

export type Observation = {
  /** source time */
  measured_at: string;
  quality: "ok" | "backfilled" | "sensor_fault" | "out_of_range" | "clock_suspect" | "stale" | "simulated";
  /** server time */
  received_at: string;
  time_source: "device" | "device_relative" | "server";
  value?: number | null;
  /** structured/string value */
  value_json?: unknown;
};

export type ObservationBuckets = {
  bucket_s: number;
  from: string;
  points: ({
    avg?: number | null;
    /** any row with quality other than ok or backfilled */
    flagged: boolean;
    max?: number | null;
    min?: number | null;
    /** rows in the bucket */
    n: number;
    /** bucket start */
    t: string;
  })[];
  to: string;
};

export type Organization = {
  admin_username?: string;
  created_at?: string;
  /** the caller's organization */
  current?: boolean;
  devices?: number;
  disabled_at?: string | null;
  disabled_reason?: string | null;
  id?: string;
  locale?: string;
  name?: string;
  profile?: string;
  slug?: string;
  tz?: string;
  users?: number;
};

export type PWAApp = {
  /** live view and video walls: auto = one camera per row on narrow screens, single = always one per row, grid = always the grid */
  camera_layout?: "auto" | "single" | "grid";
  created_at?: string;
  enabled?: boolean;
  /** id of this version */
  id?: string;
  install_url?: string;
  /** the app window stays on its sections: no way into the full application (UI confinement, not a permission boundary) */
  locked?: boolean;
  manifest_url?: string;
  name?: string;
  /** false: pinch and double-tap do not zoom the page in the app (the video player still zooms its picture) */
  page_zoom?: boolean;
  sections?: string[];
  /** home-screen label */
  short_name?: string;
  /** address /app/{slug} */
  slug?: string;
  start_url?: string;
  /** #rrggbb or empty (the organization's colour) */
  theme_color?: string;
  version?: number;
  warnings?: string[];
};

export type PWAAppInput = {
  /** default auto; omitted or empty on update keeps the value */
  camera_layout?: "auto" | "single" | "grid";
  /** default true */
  enabled?: boolean;
  /** default false; omitted on update keeps the value */
  locked?: boolean;
  /** 1–64 characters */
  name: string;
  /** default true; omitted on update keeps the value */
  page_zoom?: boolean;
  /** Reason for the change, written to the audit trail (required, 3–200 characters). */
  reason: string;
  sections: string[];
  /** ≤ 24 characters, ≤ 12 recommended; empty = name */
  short_name?: string;
  /** create only: 1–40 of a-z, 0-9, - */
  slug?: string;
  /** path of this application */
  start_url: string;
  /** #rrggbb or empty */
  theme_color?: string;
};

export type PeriodCell = {
  avg?: number;
  /** samples */
  count?: number;
  /** consumption */
  delta?: number;
  /** first reading */
  first?: number;
  /** a sample with quality other than ok or backfilled */
  flagged?: boolean;
  /** last reading */
  last?: number;
  max?: number;
  min?: number;
  /** samples outside low/high */
  out?: number;
  /** sum of the values */
  sum?: number;
};

export type PeriodTable = {
  /** datastream id → a PeriodCell per period (same order as periods) */
  cells?: Record<string, unknown>;
  columns?: ({
    datastream_id?: string;
    kind?: string;
    label?: string;
    unit?: string;
  })[];
  from?: string;
  high?: number;
  interval?: string;
  /** e.g. 2026-09 */
  label?: string;
  low?: number;
  period?: string;
  periods?: ({
    end?: string;
    label?: string;
    start?: string;
  })[];
  /** exclusive */
  to?: string;
  /** datastream id → PeriodCell over the whole range */
  totals?: Record<string, unknown>;
  /** time zone of the periods */
  tz?: string;
  values?: string[];
};

export type Permission = {
  description?: string;
  name?: string;
};

export type Policy = {
  four_eyes_assignments?: boolean;
  four_eyes_commands?: boolean;
  four_eyes_limits?: boolean;
  four_eyes_vpn?: boolean;
  mfa_required_all?: boolean;
  mfa_required_roles?: string[];
  password_min_length?: number;
};

export type PushSubscription = {
  created_at?: string;
  /** push service URL */
  endpoint?: string;
  failed_at?: string | null;
  id?: string;
  last_used_at?: string | null;
  revoked_at?: string | null;
  user_agent?: string;
  user_id?: string;
};

export type RegisterProfile = {
  changelog?: string;
  /** connectors on any version */
  connectors?: number;
  created_by?: string;
  description?: string;
  /** current version id */
  id?: string;
  /** same across versions */
  lineage?: string;
  name?: string;
  /** connectors on an older version */
  outdated?: number;
  reason?: string;
  registers?: number;
  retired_at?: string | null;
  spec?: RegisterProfileSpec;
  valid_from?: string;
  version?: number;
  writable?: number;
};

export type RegisterProfileDetail = {
  changelog?: string;
  created_by?: string;
  description?: string;
  id?: string;
  lineage?: string;
  name?: string;
  reason?: string;
  retired_at?: string | null;
  spec?: RegisterProfileSpec;
  valid_from?: string;
  valid_to?: string | null;
  version?: number;
  versions?: ({
    changelog?: string;
    connectors?: number;
    created_by?: string;
    id?: string;
    reason?: string;
    valid_from?: string;
    valid_to?: string | null;
    version?: number;
  })[];
};

export type RegisterProfileSpec = {
  byte_order?: "big" | "big_swap" | "little" | "little_swap";
  registers: ModbusRegister[];
};

export type RelayRequest = {
  /** base64 */
  body?: string;
  /** Content-Type, SOAPAction, Accept */
  headers?: Record<string, unknown>;
  method?: "POST" | "GET";
  /** path and query at the camera's ONVIF origin: /onvif/…, or a SOAP service the camera named (e.g. Tapo /event-…) */
  path?: string;
  /** request id */
  rid?: string;
};

export type Replica = {
  /** total_bytes, done_bytes, percent, eta_s */
  copy?: Record<string, unknown>;
  created_at?: string;
  created_by?: string;
  /** detachment record with verification */
  detach?: Record<string, unknown>;
  host_info?: Record<string, unknown>;
  id: string;
  lag_bytes?: number | null;
  lag_limit_s?: number;
  lag_s?: number | null;
  last_contact?: string | null;
  max_retained_mb?: number;
  mode: "copy" | "continuous" | "standby";
  name: string;
  replay_lsn?: string;
  standby_url?: string;
  state: "pairing" | "copying" | "in_sync" | "lagging" | "disconnected" | "detaching" | "detached" | "failed" | "lost" | "dropped";
};

export type ReplicationNode = {
  /** +1 on every promotion */
  epoch?: number;
  node_id: string;
  promoted_from?: string | null;
  role: "primary" | "replica" | "fenced";
};

export type Report = {
  created_at?: string;
  created_by?: string;
  file_hash?: string;
  id?: string;
  kind?: string;
  params?: Record<string, unknown>;
  signed?: boolean;
  size_bytes?: number;
};

export type Role = {
  id?: string;
  is_admin?: boolean;
  is_qa?: boolean;
  name?: string;
  /** empty for built-in roles */
  org_id?: string;
  permissions?: string[];
  /** members */
  users?: number;
};

export type Rule = {
  approved_at?: string | null;
  approved_by?: string | null;
  created_at?: string;
  created_by?: string;
  description?: string;
  enabled?: boolean;
  id?: string;
  last_event?: string | null;
  last_run?: string | null;
  name?: string;
  revoked_at?: string | null;
  spec?: RuleSpec;
  state?: {
    active?: boolean;
    loaded?: boolean;
    /** holding / cooldown */
    pending?: boolean;
    since?: string | null;
  };
  status?: "active" | "pending" | "rejected";
  updated_at?: string;
  updated_by?: string;
  version?: number;
};

export type RuleAction = {
  /** command: custom RPC arguments instead of {key, value} */
  args?: Record<string, unknown>;
  /** notify */
  channel_id?: string;
  /** derive: target datastream */
  datastream_id?: string;
  /** command */
  device_id?: string;
  /** alarm */
  escalation_policy_id?: string;
  /** derive: expression, e.g. t_in - t_out */
  expr?: string;
  /** command: writable register/node */
  key?: string;
  /** command: default write */
  method?: string;
  /** webhook: HMAC key */
  secret?: string;
  /** alarm */
  severity?: "warning" | "action" | "critical";
  /** notify: template with {rule} {event} {inputs} {<input>} */
  subject?: string;
  /** notify: template */
  text?: string;
  type: "notify" | "webhook" | "command" | "alarm" | "derive";
  /** webhook */
  url?: string;
  /** command: number, boolean or expression string */
  value?: unknown;
};

export type RuleCondition = {
  /** input name */
  input: string;
  op: ">" | ">=" | "<" | "<=" | "==" | "!=" | "stale";
  /** number, or an expression string over the inputs; stale: seconds without data */
  value: unknown;
};

export type RuleEvaluation = {
  active?: boolean;
  ages_s?: Record<string, number>;
  condition?: boolean;
  errors?: string[];
  inputs?: Record<string, number>;
  missing?: string[];
  pending?: boolean;
};

export type RuleInput = {
  datastream_id: string;
  /** ^[a-z][a-z0-9_]{0,31}$, used in conditions and expressions */
  name: string;
};

export type RuleRun = {
  actions?: ({
    alarm_id?: string;
    command_id?: string;
    detail?: string;
    status?: "done" | "failed";
    type?: string;
  })[];
  event?: "enter" | "exit";
  fired_at?: string;
  id?: string;
  inputs?: Record<string, number>;
  ok?: boolean;
  rule?: string;
  rule_id?: string;
  version?: number;
};

export type RuleSpec = {
  actions?: RuleAction[];
  clear_actions?: RuleAction[];
  conditions?: RuleCondition[];
  /** minimum seconds between firings */
  cooldown_s?: number;
  /** hold time before firing */
  for_s?: number;
  inputs: RuleInput[];
  /** default all */
  match?: "all" | "any";
};

export type Scene = {
  actions?: ({
    component?: string;
    entity?: string;
    entity_id?: string;
    /** brightness, rgb, color_temp, position, temperature, mode, value, percentage */
    params?: Record<string, unknown>;
    service?: string;
  })[];
  icon?: string;
  id?: string;
  name?: string;
  room?: string | null;
  room_id?: string | null;
  sort?: number;
};

export type SceneAction = {
  entity_id: string;
  /** brightness, rgb, color_temp, position, temperature, mode, value, percentage */
  params?: Record<string, unknown>;
  /** turn_on, turn_off, set_position, … */
  service: string;
};

export type Share = {
  public?: boolean;
  /** data URL of the QR code */
  qr?: string;
  slug?: string;
  token_prefix?: string;
  /** public URL */
  url?: string;
};

export type Site = {
  address?: string;
  assets?: number;
  created_at?: string;
  customer_id?: string | null;
  id?: string;
  name?: string;
  tz?: string;
};

export type StreamInfo = {
  codec?: "h264" | "h265" | "mjpeg";
  /** frames per second arriving */
  fps?: number;
  height?: number;
  /** kbit/s arriving */
  kbps?: number;
  state?: string;
  width?: number;
};

export type Theme = {
  font?: string;
  font_scale?: "small" | "normal" | "large";
  mode?: "auto" | "light" | "dark";
  /** hex colour */
  primary?: string;
  radius?: "none" | "small" | "medium" | "large";
};

export type TilesInfo = {
  attribution?: string;
  bounds?: number[];
  center?: number[];
  /** [map] pmtiles is set in config.toml */
  configured?: boolean;
  /** why the archive is not usable */
  error?: string;
  max_zoom?: number;
  min_zoom?: number;
  /** from the archive metadata */
  name?: string;
  /** the archive opened and holds raster tiles */
  ok?: boolean;
  size_bytes?: number;
  /** png, jpg, webp or avif */
  tile_type?: string;
  /** template served by this server */
  tile_url?: string;
  /** addressed tiles */
  tiles?: number;
};

export type User = {
  created_at?: string;
  disabled?: boolean;
  email?: string;
  full_name?: string;
  id?: string;
  last_login_at?: string | null;
  locked_until?: string | null;
  mfa_enabled?: boolean;
  must_change_password?: boolean;
  roles?: string[];
  username?: string;
};

export type VPNPeer = {
  /** enabled and not expired: configured on the server */
  active?: boolean;
  /** VPN address */
  address?: string;
  created_at?: string;
  created_by?: string;
  device_id?: string;
  device_name?: string;
  enabled?: boolean;
  /** the peer's public address */
  endpoint?: string;
  expired?: boolean;
  expires_at?: string | null;
  /** open access window: starts_at, ends_at, requested_by, approved_by */
  grant?: Record<string, unknown>;
  id?: string;
  key_rotated_at?: string | null;
  kind?: "device" | "user" | "site";
  latest_handshake?: string;
  name?: string;
  note?: string;
  /** handshake within 3 minutes */
  online?: boolean;
  org_id?: string;
  org_name?: string;
  /** grant request waiting for a second person: change_request_id, requested_by, requested_at, summary */
  pending_grant?: Record<string, unknown>;
  public_key?: string;
  rx_bytes?: number;
  subnets?: string[];
  tx_bytes?: number;
  updated_at?: string;
};

export type VPNRule = {
  created_at?: string;
  created_by?: string;
  dest_cidr?: string;
  dest_peer_id?: string;
  dest_type?: "server" | "peer" | "site";
  enabled?: boolean;
  id?: string;
  name?: string;
  /** owner organization; empty = the whole server */
  org_id?: string;
  ports?: string;
  protocol?: "tcp" | "udp" | "icmp" | "any";
  sources?: VPNSource[];
  updated_at?: string;
};

export type VPNRuleInput = {
  /** site: a host or subnet inside its routed subnets (empty = all of them) */
  dest_cidr?: string;
  /** peer or site */
  dest_peer_id?: string;
  dest_type: "server" | "peer" | "site";
  /** default true */
  enabled?: boolean;
  /** 1–80 */
  name: string;
  /** tcp/udp: "102, 502, 8000-8100"; empty = all */
  ports?: string;
  protocol: "tcp" | "udp" | "icmp" | "any";
  /** Reason for the change, written to the audit trail (required, 3–200 characters). */
  reason: string;
  sources: VPNSource[];
};

export type VPNSource = {
  /** peer or organization */
  id?: string;
  /** type kind */
  kind?: "device" | "user" | "site";
  type: "all" | "kind" | "peer" | "org";
};

export type VideoSettings = {
  camera_offline_minutes?: number;
  discovery_seconds?: number;
  disk_free_percent?: number;
  display_offline_minutes?: number;
  event_merge_seconds?: number;
  linger_seconds?: number;
  max_disk_gb?: number;
  ptz_audit_minutes?: number;
  push_block_minutes?: number;
  push_max_fails?: number;
  push_window_minutes?: number;
  segment_seconds?: number;
  timeshift_seconds?: number;
  wall_sub_stream_above?: number;
};

export type VideoWall = {
  id?: string;
  name?: string;
  screens?: WallScreen[];
  /** tiles above which automatic screens use the sub-stream */
  sub_stream_above?: number;
  updated_at?: string;
};

export type WallScreen = {
  /** 1–8 */
  cols: number;
  /** ≤ 40 */
  name?: string;
  /** auto: sub-stream above the video setting wall_sub_stream_above */
  quality?: "auto" | "main" | "sub";
  /** 1–8 */
  rows: number;
  tiles?: (string | null)[];
};

export interface Ctrl32Options {
  /** Server origin, e.g. https://telemetry.example.com (default: the page's origin) */
  baseUrl?: string;
  /** Personal API token; when absent the browser session cookie is used */
  token?: string;
  /** csrf_token from getAuthMe() — required for mutating calls with a session cookie */
  csrfToken?: string;
  /** Custom fetch (Node < 18, tests) */
  fetch?: typeof fetch;
}

export class Ctrl32Error extends Error {
  constructor(public readonly status: number, message: string, public readonly body?: unknown) {
    super(message);
    this.name = 'Ctrl32Error';
  }
}

export class Ctrl32Client {
  constructor(private readonly opts: Ctrl32Options = {}) {}

  /** Low-level request; returns the parsed JSON body, or the Response when raw is true. */
  async request<T>(method: string, path: string, query?: Record<string, unknown>, body?: unknown, raw = false): Promise<T> {
    const base = this.opts.baseUrl ?? (typeof location !== 'undefined' ? location.origin : 'http://127.0.0.1:8080');
    const url = new URL(path, base);
    if (query) for (const [k, v] of Object.entries(query)) if (v !== undefined && v !== null) url.searchParams.set(k, String(v));
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (this.opts.token) headers['Authorization'] = 'Bearer ' + this.opts.token;
    if (this.opts.csrfToken && method !== 'GET') headers['X-CSRF-Token'] = this.opts.csrfToken;
    let payload: BodyInit | undefined;
    if (typeof FormData !== 'undefined' && body instanceof FormData) payload = body;
    else if (body !== undefined) { headers['Content-Type'] = 'application/json'; payload = JSON.stringify(body); }
    const f = this.opts.fetch ?? fetch;
    const res = await f(url.toString(), { method, headers, body: payload, credentials: this.opts.token ? 'omit' : 'include' });
    if (!res.ok) {
      let message = res.status + ' ' + res.statusText;
      let data: unknown;
      try { data = await res.json(); const e = (data as { error?: string }).error; if (e) message = e; } catch { /* no JSON body */ }
      throw new Ctrl32Error(res.status, message, data);
    }
    if (raw) return res as unknown as T;
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  /** Cursor pagination helper: follows X-Next-Cursor until the server stops sending one. */
  async *pages<T>(path: string, query: Record<string, unknown>): AsyncGenerator<T[]> {
    let cursor: string | undefined;
    for (;;) {
      const res = await this.request<Response>('GET', path, { ...query, cursor }, undefined, true);
      const rows = (await res.json()) as T[];
      yield rows;
      const next = res.headers.get('X-Next-Cursor');
      if (!next) return;
      cursor = next;
    }
  }

  /** Open account requests
   *
   * Requires permission `user.admin`. */
  getAccountRequests(): Promise<AccountRequest[]> {
    return this.request<AccountRequest[]>('GET', `/api/v1/account-requests`, undefined, undefined, false);
  }

  /** Close an account request as fulfilled
   *
   * Requires permission `user.admin`. */
  postAccountRequestsIdCreated(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/account-requests/${encodeURIComponent(id)}/created`, undefined, body, false);
  }

  /** Dismiss an account request
   *
   * Requires permission `user.admin`. */
  postAccountRequestsIdDismiss(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/account-requests/${encodeURIComponent(id)}/dismiss`, undefined, body, false);
  }

  /** Current alarm rules
   *
   * Requires permission `data.read`. */
  getAlarmRules(query?: { datastream_id?: string; /** only rules of this datastream */ }): Promise<AlarmRule[]> {
    return this.request<AlarmRule[]>('GET', `/api/v1/alarm-rules`, query, undefined, false);
  }

  /** Create an alarm rule (new version)
   *
   * A rule of the same type on the same datastream is superseded (its `valid_to` is set); the new row carries the next version number. With the policy `four_eyes_limits` the call answers 202 with a pending change request instead. Types: `high`/`low` (value at or above/below the limits), `low_battery` (battery level at or below the limits), `power_loss` (value 0 = power lost; no limits), `door_open` (value other than 0 = open; no limits), `comm_loss` (no new value for longer than the limits in seconds; without limits 1.5 × the expected interval). `sensor_fault` is refused (devices do not report sensor faults). The condition must hold for `delay_seconds` before the alarm is raised.
   * 
   * Requires permission `config.write`. */
  postAlarmRules(body: {
    /** action limit (comm_loss: seconds) */
    action?: number | null;
    datastream_id: string;
    /** the condition must hold this long before the alarm is raised */
    delay_seconds?: number;
    /** optional; empty = global e-mail recipients */
    escalation_policy_id?: string;
    hysteresis?: number;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    type: "high" | "low" | "low_battery" | "power_loss" | "door_open" | "comm_loss";
    /** warning limit (comm_loss: seconds) */
    warning?: number | null;
  }): Promise<AlarmRule> {
    return this.request<AlarmRule>('POST', `/api/v1/alarm-rules`, undefined, body, false);
  }

  /** Disable an alarm rule
   *
   * Requires permission `config.write`. */
  deleteAlarmRulesId(id: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/alarm-rules/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Alarms, newest first (200)
   *
   * Requires permission `data.read`. */
  getAlarms(query?: { datastream_id?: string; }): Promise<Alarm[]> {
    return this.request<Alarm[]>('GET', `/api/v1/alarms`, query, undefined, false);
  }

  /** Acknowledge an active alarm
   *
   * Stops the escalation ladder; recorded in the alarm log with the actor and reason.
   * 
   * Requires permission `alarm.ack`. */
  postAlarmsIdAck(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/alarms/${encodeURIComponent(id)}/ack`, undefined, body, false);
  }

  /** List assets
   *
   * Requires permission `data.read`. */
  getAssets(query?: { site_id?: string; /** only assets of this site */ }): Promise<Asset[]> {
    return this.request<Asset[]>('GET', `/api/v1/assets`, query, undefined, false);
  }

  /** Create an asset
   *
   * Requires permission `config.write`. */
  postAssets(body: {
    /** free JSON */
    attributes?: Record<string, unknown>;
    name: string;
    /** parent asset */
    parent_id?: string | null;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    site_id: string;
    /** fridge, room, meter, machine, … */
    type: string;
  }): Promise<Asset> {
    return this.request<Asset>('POST', `/api/v1/assets`, undefined, body, false);
  }

  /** Update an asset
   *
   * Requires permission `config.write`. */
  patchAssetsId(id: string, body: {
    attributes?: Record<string, unknown>;
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    type?: string;
  }): Promise<Asset> {
    return this.request<Asset>('PATCH', `/api/v1/assets/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Verify the audit hash chain
   *
   * Requires permission `data.read`. */
  getAuditVerify(): Promise<{
    /** rows */
    checked?: number;
    first_bad_seq?: number | null;
    head_hash?: string;
    head_seq?: number;
    ok?: boolean;
  }> {
    return this.request<{
    /** rows */
    checked?: number;
    first_bad_seq?: number | null;
    head_hash?: string;
    head_seq?: number;
    ok?: boolean;
  }>('GET', `/api/v1/audit/verify`, undefined, undefined, false);
  }

  /** Request a password reset link
   *
   * Always answers 200 (no account enumeration). Sends an e-mail when SMTP is configured and the login matches an account with an e-mail address. */
  postAuthForgot(body: {
    /** Username or e-mail */
    login: string;
  }): Promise<{
    /** whether the server can send e-mail */
    mail_configured?: boolean;
    ok?: boolean;
  }> {
    return this.request<{
    /** whether the server can send e-mail */
    mail_configured?: boolean;
    ok?: boolean;
  }>('POST', `/api/v1/auth/forgot`, undefined, body, false);
  }

  /** Set my UI language
   *
   * Requires a signed-in user or an API token (any permission). */
  putAuthLocale(body: {
    /** Enabled locale code, e.g. sk */
    locale: string;
  }): Promise<{
    locale?: string;
  }> {
    return this.request<{
    locale?: string;
  }>('PUT', `/api/v1/auth/locale`, undefined, body, false);
  }

  /** Sign in
   *
   * Starts a browser session (cookie `ctrl32_session`). When the user has MFA enrolled the response carries `mfa_required` and a short-lived `pre_auth_token` for `POST /api/v1/auth/mfa`. Rate limited per IP; repeated failures lock the account per the security policy. */
  postAuthLogin(body: {
    /** Password */
    password: string;
    /** Username */
    username: string;
  }): Promise<LoginResponse> {
    return this.request<LoginResponse>('POST', `/api/v1/auth/login`, undefined, body, false);
  }

  /** Sign out
   *
   * Ends the session; safe to call without one. */
  postAuthLogout(): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/auth/logout`, undefined, undefined, false);
  }

  /** Who am I
   *
   * The signed-in principal: user, organization, roles, effective permissions and the CSRF token for cookie sessions.
   * 
   * Requires a signed-in user or an API token (any permission). */
  getAuthMe(): Promise<Me> {
    return this.request<Me>('GET', `/api/v1/auth/me`, undefined, undefined, false);
  }

  /** Complete sign-in with a TOTP or recovery code */
  postAuthMfa(body: {
    /** 6-digit TOTP code or a recovery code */
    code: string;
    /** From the login response */
    pre_auth_token: string;
  }): Promise<LoginResponse> {
    return this.request<LoginResponse>('POST', `/api/v1/auth/mfa`, undefined, body, false);
  }

  /** Change my password
   *
   * Requires a signed-in user or an API token (any permission). */
  postAuthPassword(body: {
    current: string;
    new: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/auth/password`, undefined, body, false);
  }

  /** Ask for an account
   *
   * Self-service request handled by an administrator under *Users → Account requests*. Rate limited. */
  postAuthRegisterRequest(body: {
    email: string;
    full_name: string;
    /** optional */
    message?: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/auth/register-request`, undefined, body, false);
  }

  /** Set a new password with a reset token */
  postAuthReset(body: {
    /** New password (policy: minimum length, mixed classes) */
    password: string;
    /** Token from the reset link */
    token: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/auth/reset`, undefined, body, false);
  }

  /** Confirm TOTP enrolment
   *
   * Returns the one-time recovery codes; store them safely.
   * 
   * Requires a signed-in user or an API token (any permission). */
  postAuthTotpConfirm(body: {
    /** Current TOTP code */
    code: string;
  }): Promise<{
    ok?: boolean;
    recovery_codes?: string[];
  }> {
    return this.request<{
    ok?: boolean;
    recovery_codes?: string[];
  }>('POST', `/api/v1/auth/totp/confirm`, undefined, body, false);
  }

  /** Start TOTP enrolment
   *
   * Returns the secret, an otpauth URL and a QR code (data URL). Confirm with `POST /api/v1/auth/totp/confirm`.
   * 
   * Requires a signed-in user or an API token (any permission). */
  postAuthTotpSetup(): Promise<{
    otpauth_url?: string;
    /** data:image/png;base64,… */
    qr?: string;
    secret?: string;
  }> {
    return this.request<{
    otpauth_url?: string;
    /** data:image/png;base64,… */
    qr?: string;
    secret?: string;
  }>('POST', `/api/v1/auth/totp/setup`, undefined, undefined, false);
  }

  /** Branding, theme, map settings and version
   *
   * Public: the sign-in page uses it. */
  getBranding(): Promise<Branding> {
    return this.request<Branding>('GET', `/api/v1/branding`, undefined, undefined, false);
  }

  /** Set branding, theme and map tiles
   *
   * Logo as base64 (PNG/SVG/JPEG, max 256 KiB).
   * 
   * Requires permission `content.write`. */
  putBranding(body: {
    clear_logo?: boolean;
    logo?: {
      /** base64 */
      data?: string;
      mime?: string;
    } | null;
    map?: MapSettings;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    theme?: Theme;
    white_label?: {
      clear_icon?: boolean;
      /** needs a license with white_label (402 otherwise) */
      enabled?: boolean;
      footer_links?: ({
        /** ≤ 40 */
        label?: string;
        /** http(s)://, mailto: or /path */
        url?: string;
      })[];
      /** ≤ 120 */
      footer_text?: string;
      icon?: {
        /** base64, ≤ 64 KiB */
        data?: string;
        /** image/png or image/svg+xml */
        mime?: string;
      } | null;
      /** start page introduction, ≤ 400 */
      intro?: string;
      /** ≤ 64 */
      product_name?: string;
      /** PWA name, ≤ 24 */
      short_name?: string;
      show_version?: boolean;
    };
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('PUT', `/api/v1/branding`, undefined, body, false);
  }

  /** Browser icon
   *
   * The white-label icon when active, otherwise a redirect to /favicon.svg. */
  getBrandingIcon(): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/branding/icon`, undefined, undefined, true);
  }

  /** Organization logo image */
  getBrandingLogo(): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/branding/logo`, undefined, undefined, true);
  }

  /** White-label settings of the organization
   *
   * Stored settings (applied or not), whether the server is licensed and whether they are active.
   * 
   * Requires permission `content.write`. */
  getBrandingWhiteLabel(): Promise<{
    active?: boolean;
    enabled?: boolean;
    footer_links?: ({
      label?: string;
      url?: string;
    })[];
    footer_text?: string;
    icon?: boolean;
    icon_url?: string;
    intro?: string;
    licensed?: boolean;
    product_name?: string;
    short_name?: string;
    show_version?: boolean;
  }> {
    return this.request<{
    active?: boolean;
    enabled?: boolean;
    footer_links?: ({
      label?: string;
      url?: string;
    })[];
    footer_text?: string;
    icon?: boolean;
    icon_url?: string;
    intro?: string;
    licensed?: boolean;
    product_name?: string;
    short_name?: string;
    show_version?: boolean;
  }>('GET', `/api/v1/branding/white-label`, undefined, undefined, false);
  }

  /** Change requests waiting for a second person
   *
   * Alarm rule and assignment changes held by the four-eyes policies (`status=pending`, default) or the full history (`status=all`). VPN access grants (kind `vpn`) appear here with four eyes for VPN access. Requires config.write, device.manage, vpn.manage or system.admin.
   * 
   * Requires a signed-in user or an API token (any permission). */
  getChangeRequests(query?: { status?: string; /** pending (default) | approved | rejected | expired | failed | all */ }): Promise<ChangeRequest[]> {
    return this.request<ChangeRequest[]>('GET', `/api/v1/change-requests`, query, undefined, false);
  }

  /** Approve a change request
   *
   * A different user than the requester, holding the permission of the change (config.write for limits, device.manage for assignments, vpn.manage or system.admin for VPN access grants). The original call is replayed under the approver's identity; `code` and `result` are its response.
   * 
   * Requires a signed-in user or an API token (any permission). */
  postChangeRequestsIdApprove(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** HTTP status of the replayed call */
    code?: number;
    id?: string;
    result?: unknown;
    status?: "approved" | "failed";
  }> {
    return this.request<{
    /** HTTP status of the replayed call */
    code?: number;
    id?: string;
    result?: unknown;
    status?: "approved" | "failed";
  }>('POST', `/api/v1/change-requests/${encodeURIComponent(id)}/approve`, undefined, body, false);
  }

  /** Reject a change request
   *
   * Requires a signed-in user or an API token (any permission). */
  postChangeRequestsIdReject(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    id?: string;
    status?: string;
  }> {
    return this.request<{
    id?: string;
    status?: string;
  }>('POST', `/api/v1/change-requests/${encodeURIComponent(id)}/reject`, undefined, body, false);
  }

  /** List connectors
   *
   * Requires permission `config.write`. */
  getConnectors(): Promise<Connector[]> {
    return this.request<Connector[]>('GET', `/api/v1/connectors`, undefined, undefined, false);
  }

  /** Create a connector
   *
   * `kind` chirpstack: returns the ingest URL and a bearer secret (shown once) for the ChirpStack HTTP integration. `kind` opcua / modbus_tcp: `settings` describe the endpoint and the nodes/registers; values arrive under a virtual device (`settings.device_external_id`).
   * 
   * Requires permission `config.write`. */
  postConnectors(body: {
    /** chirpstack: register unknown DevEUIs automatically */
    auto_create_devices?: boolean;
    kind: "chirpstack" | "opcua" | "modbus_tcp";
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    settings?: ConnectorSettings;
  }): Promise<ConnectorCreated> {
    return this.request<ConnectorCreated>('POST', `/api/v1/connectors`, undefined, body, false);
  }

  /** This server's OPC UA client certificate (PEM)
   *
   * Trust it on the OPC UA server for signed/encrypted sessions.
   * 
   * Requires permission `config.write`. */
  getConnectorsOpcuaCertificate(): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/connectors/opcua/certificate`, undefined, undefined, true);
  }

  /** Test connector settings without saving
   *
   * Connects once and reads the configured nodes/registers.
   * 
   * Requires permission `config.write`. */
  postConnectorsTest(body: {
    /** existing connector id (reuse its stored password) */
    id?: string;
    /** default opcua */
    kind?: "opcua" | "modbus_tcp";
    settings: ConnectorSettings;
  }): Promise<{
    bad_nodes?: string[];
    endpoint?: string;
    ok?: boolean;
    /** OPC UA: negotiated policy */
    security?: string;
    /** key → value */
    values?: Record<string, unknown>;
  }> {
    return this.request<{
    bad_nodes?: string[];
    endpoint?: string;
    ok?: boolean;
    /** OPC UA: negotiated policy */
    security?: string;
    /** key → value */
    values?: Record<string, unknown>;
  }>('POST', `/api/v1/connectors/test`, undefined, body, false);
  }

  /** Update connector settings (OPC UA, Modbus)
   *
   * Requires permission `config.write`. */
  putConnectorsId(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    settings: ConnectorSettings;
  }): Promise<Connector> {
    return this.request<Connector>('PUT', `/api/v1/connectors/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Revoke a connector
   *
   * Stops polling and rejects its secret.
   * 
   * Requires permission `config.write`. */
  deleteConnectorsId(id: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/connectors/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Counting areas
   *
   * Areas with their entrances, the occupancy now and today's passages per group, and whether the detector for cameras is installed on this server.
   * 
   * Requires permission `data.read`. */
  getCountingAreas(): Promise<{
    areas?: ({
      /** the area's asset (its datastreams) */
      asset_id?: string;
      /** group → departures counted while empty, since the reset */
      clamped?: Record<string, unknown>;
      entrances?: ({
        asset_id?: string;
        id?: string;
        name?: string;
        sort?: number;
        /** group → {in, out} */
        today?: Record<string, unknown>;
      })[];
      groups?: string[];
      id?: string;
      kind?: "building" | "car_park" | "zone";
      last_reset?: Record<string, unknown>;
      /** group → capacity */
      limits?: Record<string, unknown>;
      name?: string;
      /** group → occupancy now */
      occupancy?: Record<string, unknown>;
      /** HH:MM local, empty = no daily reset */
      reset_time?: string;
      site?: string;
      site_id?: string;
      /** group → {in, out} */
      today?: Record<string, unknown>;
    })[];
    cameras_installed?: boolean;
    cameras_note?: string;
    classes?: string[];
    groups?: string[];
  }> {
    return this.request<{
    areas?: ({
      /** the area's asset (its datastreams) */
      asset_id?: string;
      /** group → departures counted while empty, since the reset */
      clamped?: Record<string, unknown>;
      entrances?: ({
        asset_id?: string;
        id?: string;
        name?: string;
        sort?: number;
        /** group → {in, out} */
        today?: Record<string, unknown>;
      })[];
      groups?: string[];
      id?: string;
      kind?: "building" | "car_park" | "zone";
      last_reset?: Record<string, unknown>;
      /** group → capacity */
      limits?: Record<string, unknown>;
      name?: string;
      /** group → occupancy now */
      occupancy?: Record<string, unknown>;
      /** HH:MM local, empty = no daily reset */
      reset_time?: string;
      site?: string;
      site_id?: string;
      /** group → {in, out} */
      today?: Record<string, unknown>;
    })[];
    cameras_installed?: boolean;
    cameras_note?: string;
    classes?: string[];
    groups?: string[];
  }>('GET', `/api/v1/counting/areas`, undefined, undefined, false);
  }

  /** Create a counting area
   *
   * Creates the area's asset (type counting_area) on the site, an asset per entrance (type entrance) and the datastreams of the groups.
   * 
   * Requires permission `config.write`. */
  postCountingAreas(body: {
    entrances?: string[];
    groups?: string[];
    /** default building */
    kind?: "building" | "car_park" | "zone";
    /** group → capacity */
    limits?: Record<string, unknown>;
    /** 1–80 */
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** HH:MM, empty = none */
    reset_time?: string;
    /** new areas */
    site_id?: string;
  }): Promise<{
    asset_id?: string;
    id?: string;
  }> {
    return this.request<{
    asset_id?: string;
    id?: string;
  }>('POST', `/api/v1/counting/areas`, undefined, body, false);
  }

  /** Change a counting area
   *
   * Name, kind, groups (new groups get their datastreams), daily reset time and capacities.
   * 
   * Requires permission `config.write`. */
  putCountingAreasId(id: string, body: {
    entrances?: string[];
    groups?: string[];
    /** default building */
    kind?: "building" | "car_park" | "zone";
    /** group → capacity */
    limits?: Record<string, unknown>;
    /** 1–80 */
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** HH:MM, empty = none */
    reset_time?: string;
    /** new areas */
    site_id?: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('PUT', `/api/v1/counting/areas/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Archive a counting area
   *
   * The area and its entrances stop counting; their datastreams and observations stay.
   * 
   * Requires permission `config.write`. */
  deleteCountingAreasId(id: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/counting/areas/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Add an entrance
   *
   * Requires permission `config.write`. */
  postCountingAreasIdEntrances(id: string, body: {
    /** 1–80 */
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    sort?: number;
  }): Promise<{
    id?: string;
  }> {
    return this.request<{
    id?: string;
  }>('POST', `/api/v1/counting/areas/${encodeURIComponent(id)}/entrances`, undefined, body, false);
  }

  /** Correct the occupancy
   *
   * Sets the occupancy of a group (e.g. after a count by hand); recorded in the counting log and the audit trail with the reason, stored as an observation of `<group>_occupancy`.
   * 
   * Requires permission `config.write`. */
  postCountingAreasIdOccupancy(id: string, body: {
    group: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** 0–10 000 000 */
    value: number;
  }): Promise<{
    before?: number;
    occupancy?: number;
    ok?: boolean;
  }> {
    return this.request<{
    before?: number;
    occupancy?: number;
    ok?: boolean;
  }>('POST', `/api/v1/counting/areas/${encodeURIComponent(id)}/occupancy`, undefined, body, false);
  }

  /** Cameras and their object counting
   *
   * Every camera the user may see, with its counting configuration and state.
   * 
   * Requires permission `data.read`. */
  getCountingCameras(): Promise<(Record<string, unknown>)[]> {
    return this.request<(Record<string, unknown>)[]>('GET', `/api/v1/counting/cameras`, undefined, undefined, false);
  }

  /** Object counting of a camera
   *
   * Configuration and the state of the worker: analysed frames per second, inference time, CPU use of the worker and the decoder, objects followed, the boxes of the last analysed frame (for the line editor), counts since the start.
   * 
   * Requires permission `data.read`. */
  getCountingCamerasId(id: string): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/counting/cameras/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Configure object counting on a camera
   *
   * Lines (each counts for an entrance; the side where the area lies decides in and out), speed sections (two lines and the distance between them on the ground; a speed is an indicative measurement, not certified enforcement), analysed frames per second (1–15), model nano (fast) or tiny (more accurate), detection threshold. Off by default; the camera's picture is analysed on this server and never stored.
   * 
   * Requires permission `config.write`. */
  putCountingCamerasId(id: string, body: {
    config: {
      anchor?: "bottom" | "centre";
      lines?: ({
        a?: {
          /** 0–1 */
          x?: number;
          /** 0–1 */
          y?: number;
        };
        b?: {
          x?: number;
          y?: number;
        };
        classes?: string[];
        /** empty = a speed line only */
        entrance_id?: string;
        id?: string;
        /** side of A→B on the picture where the area lies */
        in?: "left" | "right";
        name?: string;
      })[];
      /** 0–0.1 */
      margin?: number;
      sections?: ({
        area_id?: string;
        /** set by the server */
        asset_id?: string;
        classes?: string[];
        /** 1–500, on the ground */
        distance_m?: number;
        id?: string;
        /** 0 = none */
        limit_kmh?: number;
        line_a?: string;
        line_b?: string;
        name?: string;
      })[];
    };
    enabled?: boolean;
    /** 1–15, default 5 */
    fps?: number;
    /** 0.1–0.95, default 0.35 */
    min_score?: number;
    model?: "nano" | "tiny";
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('PUT', `/api/v1/counting/cameras/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Rename an entrance
   *
   * Requires permission `config.write`. */
  putCountingEntrancesId(id: string, body: {
    /** 1–80 */
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    sort?: number;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('PUT', `/api/v1/counting/entrances/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Archive an entrance
   *
   * Requires permission `config.write`. */
  deleteCountingEntrancesId(id: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/counting/entrances/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Sensor inputs
   *
   * Requires permission `data.read`. */
  getCountingInputs(): Promise<(Record<string, unknown>)[]> {
    return this.request<(Record<string, unknown>)[]>('GET', `/api/v1/counting/inputs`, undefined, undefined, false);
  }

  /** Add a sensor input
   *
   * A datastream of a device (light barrier, turnstile, door counter …) whose values are passages in or out of an entrance: `increment` — each value is a number of passages, `total` — a running total (the difference counts; a restart from zero counts the new value).
   * 
   * Requires permission `config.write`. */
  postCountingInputs(body: {
    enabled?: boolean;
    entrance_id: string;
    /** people (default), vehicles, cars … */
    group?: string;
    in_datastream_id?: string;
    mode?: "increment" | "total";
    name: string;
    out_datastream_id?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    id?: string;
  }> {
    return this.request<{
    id?: string;
  }>('POST', `/api/v1/counting/inputs`, undefined, body, false);
  }

  /** Change a sensor input
   *
   * Requires permission `config.write`. */
  putCountingInputsId(id: string, body: {
    enabled?: boolean;
    entrance_id: string;
    group?: string;
    in_datastream_id?: string;
    mode?: "increment" | "total";
    name: string;
    out_datastream_id?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('PUT', `/api/v1/counting/inputs/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Remove a sensor input
   *
   * Requires permission `config.write`. */
  deleteCountingInputsId(id: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/counting/inputs/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Resets, corrections and clamps
   *
   * Requires permission `data.read`. */
  getCountingLog(query: { area: string; limit?: number; /** 1–500, default 50 */ }): Promise<({
    actor?: string;
    after?: number;
    at?: string;
    before?: number;
    group?: string;
    kind?: "reset" | "correction" | "clamp";
    reason?: string;
  })[]> {
    return this.request<({
    actor?: string;
    after?: number;
    at?: string;
    before?: number;
    group?: string;
    kind?: "reset" | "correction" | "clamp";
    reason?: string;
  })[]>('GET', `/api/v1/counting/log`, query, undefined, false);
  }

  /** Summary of an area
   *
   * One local day of an area: occupancy now, passages per group, per entrance with shares, per hour (with the occupancy at the end of each hour), the days before for comparison, the peak occupancy and the speed sections (count, average, maximum, over the limit, last vehicles, histogram per 10 km/h). The area is given directly or by one of its datastreams (widgets).
   * 
   * Requires permission `data.read`. */
  getCountingSummary(query?: { area?: string; /** area id */ datastream?: string; /** or a datastream of the area, an entrance or a speed section */ date?: string; /** YYYY-MM-DD, default today */ days?: number; /** days for comparison, 1–62, default 7 */ }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/counting/summary`, query, undefined, false);
  }

  /** Report client attributes (device)
   *
   * JSON object such as `{"fw":"1.4.2","hw":"esp32-s3","model":"…","ip":"10.0.0.12"}`; merged into the device's client attributes. */
  postDAttributes(body: Record<string, unknown>): Promise<{
    ok?: boolean;
  }> {
    return this.request<{
    ok?: boolean;
  }>('POST', `/api/v1/d/attributes`, undefined, body, false);
  }

  /** Shared attributes (device)
   *
   * The configuration the server holds for the device (the retained `attributes/shared` document over MQTT). */
  getDAttributesShared(): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/d/attributes/shared`, undefined, undefined, false);
  }

  /** Fetch queued commands (device, long polling)
   *
   * Returns the queued commands of the device and marks them sent. `wait` (0–60 s) keeps the request open until a command appears. Devices registered with transport `http` receive commands only this way; acknowledge each with `POST /api/v1/d/cmd/{id}/ack`. */
  getDCmd(query?: { wait?: string; /** Seconds to wait for a command (0–60) */ }): Promise<({
    /** Arguments */
    a?: Record<string, unknown>;
    /** Unix time after which the command must not be executed */
    exp?: number;
    id?: string;
    /** Method */
    m?: string;
  })[]> {
    return this.request<({
    /** Arguments */
    a?: Record<string, unknown>;
    /** Unix time after which the command must not be executed */
    exp?: number;
    id?: string;
    /** Method */
    m?: string;
  })[]>('GET', `/api/v1/d/cmd`, query, undefined, false);
  }

  /** Acknowledge a command (device)
   *
   * `{"ok":true,"r":{…}}` or `{"ok":false,"err":"…"}`; 404 when the command is not open for this device. */
  postDCmdIdAck(id: string, body: {
    err?: string;
    ok: boolean;
    /** Result */
    r?: unknown;
  }): Promise<{
    id?: string;
    /** acked or failed */
    status?: string;
  }> {
    return this.request<{
    id?: string;
    /** acked or failed */
    status?: string;
  }>('POST', `/api/v1/d/cmd/${encodeURIComponent(id)}/ack`, undefined, body, false);
  }

  /** Report firmware update progress (device)
   *
   * `{"fw_id":"…","st":"downloading|verifying|applying|updated|failed|rolled_back","pct":42,"ver":"1.5.0","err":"…"}`. */
  postDFotaStatus(body: Record<string, unknown>): Promise<{
    ok?: boolean;
  }> {
    return this.request<{
    ok?: boolean;
  }>('POST', `/api/v1/d/fota/status`, undefined, body, false);
  }

  /** Download a firmware image (device)
   *
   * Called by the device with its device token (`Authorization: Bearer`), supports HTTP range requests. */
  getDFotaFwBlob(fw: string): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/d/fota/${encodeURIComponent(fw)}/blob`, undefined, undefined, true);
  }

  /** Heartbeat (device)
   *
   * `{"online":true,"bat":3.61,"rssi":-71,"up_s":86400,"free":120000,"fw":"1.4.2","buf":12}`; `lat`/`lon` update the reported position. */
  postDStatus(body: Record<string, unknown>): Promise<{
    ok?: boolean;
  }> {
    return this.request<{
    ok?: boolean;
  }>('POST', `/api/v1/d/status`, undefined, body, false);
  }

  /** Send telemetry (device)
   *
   * One sample or an array. Accepted shapes: ctrl32 `{"ts":1700000000,"v":{"temp":4.2}}`, ThingsBoard `{"ts":1700000000000,"values":{…}}`, or a flat object `{"temp":4.2}` (every non-reserved key is a value). `ts` in seconds or milliseconds, `bf: 1` for backfill, `seq` for deduplication. Keys are normalised (lower case, `-`/`.`/space → `_`) and must match a datastream assigned to the device; unknown keys are reported and stored as a raw message. Up to 64 KiB, 2000 samples. */
  postDTelemetry(body: unknown): Promise<{
    /** Observations stored */
    accepted?: number;
    error?: string;
    /** ok, unknown_key or error */
    status?: string;
    unknown_keys?: string[];
  }> {
    return this.request<{
    /** Observations stored */
    accepted?: number;
    error?: string;
    /** ok, unknown_key or error */
    status?: string;
    unknown_keys?: string[];
  }>('POST', `/api/v1/d/telemetry`, undefined, body, false);
  }

  /** List dashboards
   *
   * Requires permission `data.read`. */
  getDashboards(): Promise<DashboardSummary[]> {
    return this.request<DashboardSummary[]>('GET', `/api/v1/dashboards`, undefined, undefined, false);
  }

  /** Create a dashboard
   *
   * `spec` is screen format v1 (`GET /api/v1/dashboards/catalog` lists the widget types; `GET /api/v1/dashboards/starter` returns a ready-made spec).
   * 
   * Requires permission `content.write`. */
  postDashboards(body: {
    is_default?: boolean;
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** URL-safe; generated from the name when empty */
    slug?: string;
    spec: DashboardSpec;
  }): Promise<Dashboard> {
    return this.request<Dashboard>('POST', `/api/v1/dashboards`, undefined, body, false);
  }

  /** Widget catalog and time ranges
   *
   * Requires permission `data.read`. */
  getDashboardsCatalog(): Promise<{
    ranges?: string[];
    widgets?: (Record<string, unknown>)[];
  }> {
    return this.request<{
    ranges?: string[];
    widgets?: (Record<string, unknown>)[];
  }>('GET', `/api/v1/dashboards/catalog`, undefined, undefined, false);
  }

  /** A starter dashboard spec built from the organization's data
   *
   * Requires permission `data.read`. */
  getDashboardsStarter(query?: { kind?: string; /** overview (default), showcase, mimic (widget symbols on a canvas) or plant (mixing plant process picture) */ }): Promise<{
    kind?: string;
    name?: string;
    spec?: DashboardSpec;
  }> {
    return this.request<{
    kind?: string;
    name?: string;
    spec?: DashboardSpec;
  }>('GET', `/api/v1/dashboards/starter`, query, undefined, false);
  }

  /** Dashboard with its spec
   *
   * Requires permission `data.read`. */
  getDashboardsSlug(slug: string): Promise<Dashboard> {
    return this.request<Dashboard>('GET', `/api/v1/dashboards/${encodeURIComponent(slug)}`, undefined, undefined, false);
  }

  /** Save a new version of a dashboard
   *
   * Widget settings are validated against the catalogue; invalid values are refused, settings the catalogue does not know are dropped and listed in `warnings` of the response.
   * 
   * Requires permission `content.write`. */
  putDashboardsSlug(slug: string, body: {
    is_default?: boolean;
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    spec?: DashboardSpec;
  }): Promise<Dashboard> {
    return this.request<Dashboard>('PUT', `/api/v1/dashboards/${encodeURIComponent(slug)}`, undefined, body, false);
  }

  /** Delete a dashboard
   *
   * Requires permission `content.write`. */
  deleteDashboardsSlug(slug: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/dashboards/${encodeURIComponent(slug)}`, query, undefined, false);
  }

  /** Public share of a dashboard
   *
   * Requires permission `content.write`. */
  getDashboardsSlugShare(slug: string): Promise<Share> {
    return this.request<Share>('GET', `/api/v1/dashboards/${encodeURIComponent(slug)}/share`, undefined, undefined, false);
  }

  /** Publish a dashboard under a public link
   *
   * Creates the share token; the response carries the public URL and a QR code.
   * 
   * Requires permission `content.write`. */
  postDashboardsSlugShare(slug: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<Share> {
    return this.request<Share>('POST', `/api/v1/dashboards/${encodeURIComponent(slug)}/share`, undefined, body, false);
  }

  /** Withdraw the public link
   *
   * Requires permission `content.write`. */
  deleteDashboardsSlugShare(slug: string, query: { reason: string; }): Promise<Share> {
    return this.request<Share>('DELETE', `/api/v1/dashboards/${encodeURIComponent(slug)}/share`, query, undefined, false);
  }

  /** List datastreams
   *
   * Requires permission `data.read`. */
  getDatastreams(): Promise<Datastream[]> {
    return this.request<Datastream[]>('GET', `/api/v1/datastreams`, undefined, undefined, false);
  }

  /** Create a datastream
   *
   * Requires permission `config.write`. */
  postDatastreams(body: {
    /** required accuracy in the unit */
    accuracy?: number | null;
    asset_id: string;
    /** PostgreSQL interval for gap detection, e.g. 5 minutes; empty = none */
    expected_interval?: string;
    /** what the device publishes, ^[a-z][a-z0-9_]{0,31}$ */
    key: string;
    kind: "gauge" | "counter" | "state" | "event";
    /** temperature, humidity, power_active, … */
    quantity: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    unit?: string;
  }): Promise<Datastream> {
    return this.request<Datastream>('POST', `/api/v1/datastreams`, undefined, body, false);
  }

  /** Latest value of several datastreams
   *
   * Requires permission `data.read`. */
  getDatastreamsLatest(query: { ids: string; /** comma-separated datastream ids */ }): Promise<Record<string, LatestValue>> {
    return this.request<Record<string, LatestValue>>('GET', `/api/v1/datastreams/latest`, query, undefined, false);
  }

  /** Datastream detail
   *
   * Datastream, asset, site, source device, last observation and current alarm rules.
   * 
   * Requires permission `data.read`. */
  getDatastreamsId(id: string): Promise<{
    alarm_rules?: AlarmRule[];
    asset?: Record<string, unknown>;
    datastream?: Datastream;
    /** assigned device */
    device?: Record<string, unknown> | null;
    last_observation?: Observation;
    rules?: ({
      enabled?: boolean;
      id?: string;
      name?: string;
      /** automation rules that read (input) or write (derive) this datastream */
      role?: "input" | "derive";
      status?: string;
    })[];
    site?: Record<string, unknown>;
  }> {
    return this.request<{
    alarm_rules?: AlarmRule[];
    asset?: Record<string, unknown>;
    datastream?: Datastream;
    /** assigned device */
    device?: Record<string, unknown> | null;
    last_observation?: Observation;
    rules?: ({
      enabled?: boolean;
      id?: string;
      name?: string;
      /** automation rules that read (input) or write (derive) this datastream */
      role?: "input" | "derive";
      status?: string;
    })[];
    site?: Record<string, unknown>;
  }>('GET', `/api/v1/datastreams/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Update a datastream
   *
   * Requires permission `config.write`. */
  patchDatastreamsId(id: string, body: {
    accuracy?: number | null;
    expected_interval?: string;
    quantity?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    unit?: string;
  }): Promise<Datastream> {
    return this.request<Datastream>('PATCH', `/api/v1/datastreams/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Devices waiting for approval
   *
   * Requires permission `device.manage`. */
  getDeviceApprovals(): Promise<(Record<string, unknown>)[]> {
    return this.request<(Record<string, unknown>)[]>('GET', `/api/v1/device-approvals`, undefined, undefined, false);
  }

  /** List devices
   *
   * Requires permission `data.read`. */
  getDevices(): Promise<Device[]> {
    return this.request<Device[]>('GET', `/api/v1/devices`, undefined, undefined, false);
  }

  /** Register a device
   *
   * Returns a claim code for self-provisioning (see `POST /api/v1/provision`); a token can also be issued with `POST /api/v1/devices/{id}/token`.
   * 
   * Requires permission `device.manage`. */
  postDevices(body: {
    /** MQTT client id / DevEUI / connector id */
    external_id: string;
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** default mqtt */
    transport?: "mqtt" | "http" | "lorawan" | "opcua" | "modbus";
  }): Promise<{
    claim_code?: string;
    claim_expires_in_hours?: number;
    external_id?: string;
    id?: string;
    name?: string;
    transport?: string;
  }> {
    return this.request<{
    claim_code?: string;
    claim_expires_in_hours?: number;
    external_id?: string;
    id?: string;
    name?: string;
    transport?: string;
  }>('POST', `/api/v1/devices`, undefined, body, false);
  }

  /** Device detail
   *
   * Device, its datastream assignments (current and past) and credentials (no secrets).
   * 
   * Requires permission `data.read`. */
  getDevicesId(id: string): Promise<{
    assignments?: (Record<string, unknown>)[];
    credentials?: (Record<string, unknown>)[];
    device?: Device;
  }> {
    return this.request<{
    assignments?: (Record<string, unknown>)[];
    credentials?: (Record<string, unknown>)[];
    device?: Device;
  }>('GET', `/api/v1/devices/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Rename a device or set its location
   *
   * Requires permission `device.manage`. */
  patchDevicesId(id: string, body: {
    clear_location?: boolean;
    location?: {
      lat?: number;
      lon?: number;
    } | null;
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<Device> {
    return this.request<Device>('PATCH', `/api/v1/devices/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Approve a self-provisioned device
   *
   * Its token works from now on.
   * 
   * Requires permission `device.manage`. */
  postDevicesIdApprove(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    approval?: string;
    id?: string;
  }> {
    return this.request<{
    approval?: string;
    id?: string;
  }>('POST', `/api/v1/devices/${encodeURIComponent(id)}/approve`, undefined, body, false);
  }

  /** Shared and client attributes
   *
   * Requires permission `data.read`. */
  getDevicesIdAttributes(id: string): Promise<{
    /** reported by the device */
    client?: Record<string, unknown>;
    /** set by the server, pushed to the device */
    shared?: Record<string, unknown>;
  }> {
    return this.request<{
    /** reported by the device */
    client?: Record<string, unknown>;
    /** set by the server, pushed to the device */
    shared?: Record<string, unknown>;
  }>('GET', `/api/v1/devices/${encodeURIComponent(id)}/attributes`, undefined, undefined, false);
  }

  /** Set or remove shared attributes
   *
   * Merged into the retained MQTT attributes message.
   * 
   * Requires permission `device.manage`. */
  putDevicesIdAttributes(id: string, body: {
    /** keys to set */
    attributes?: Record<string, unknown>;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    remove?: string[];
  }): Promise<{
    published?: boolean;
    shared?: Record<string, unknown>;
  }> {
    return this.request<{
    published?: boolean;
    shared?: Record<string, unknown>;
  }>('PUT', `/api/v1/devices/${encodeURIComponent(id)}/attributes`, undefined, body, false);
  }

  /** Command log of a device
   *
   * Newest first (default 50, max 500). Commands past their deadline are marked expired.
   * 
   * Requires permission `data.read`. */
  getDevicesIdCommands(id: string, query?: { limit?: number; }): Promise<Command[]> {
    return this.request<Command[]>('GET', `/api/v1/devices/${encodeURIComponent(id)}/commands`, query, undefined, false);
  }

  /** Send a command or write a value
   *
   * MQTT devices receive `{id, m, a, exp}` on `d/{external_id}/cmd` and acknowledge on `d/{external_id}/cmd/ack`. OPC UA and Modbus devices accept `method: write` with `args: {key, value}` for a writable node/register (limits checked, value read back). With the four-eyes policy the command is stored as `pending` (202) until another user approves it.
   * 
   * Requires permission `device.command`. */
  postDevicesIdCommands(id: string, body: {
    /** method arguments */
    args?: Record<string, unknown>;
    /** ^[a-z][a-z0-9_]{0,31}$ */
    method: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** deadline for the acknowledgement (default 300) */
    ttl_seconds?: number;
  }): Promise<CommandResult> {
    return this.request<CommandResult>('POST', `/api/v1/devices/${encodeURIComponent(id)}/commands`, undefined, body, false);
  }

  /** Approve a pending command (four eyes)
   *
   * Must be a different user than the issuer; the command is then executed.
   * 
   * Requires permission `device.command`. */
  postDevicesIdCommandsCidApprove(id: string, cid: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<CommandResult> {
    return this.request<CommandResult>('POST', `/api/v1/devices/${encodeURIComponent(id)}/commands/${encodeURIComponent(cid)}/approve`, undefined, body, false);
  }

  /** Reject a pending command
   *
   * Requires permission `device.command`. */
  postDevicesIdCommandsCidReject(id: string, cid: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<CommandResult> {
    return this.request<CommandResult>('POST', `/api/v1/devices/${encodeURIComponent(id)}/commands/${encodeURIComponent(cid)}/reject`, undefined, body, false);
  }

  /** Console sessions of a device
   *
   * Requires permission `device.console`. */
  getDevicesIdConsole(id: string): Promise<{
    idle_timeout_minutes?: number;
    open?: ConsoleSession;
    sessions?: ConsoleSession[];
  }> {
    return this.request<{
    idle_timeout_minutes?: number;
    open?: ConsoleSession;
    sessions?: ConsoleSession[];
  }>('GET', `/api/v1/devices/${encodeURIComponent(id)}/console`, undefined, undefined, false);
  }

  /** Open a console session
   *
   * Requires permission `device.console`. */
  postDevicesIdConsole(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    external_id?: string;
    session_id?: string;
  }> {
    return this.request<{
    external_id?: string;
    session_id?: string;
  }>('POST', `/api/v1/devices/${encodeURIComponent(id)}/console`, undefined, body, false);
  }

  /** Read console output
   *
   * Lines after a sequence number (poll with `after`).
   * 
   * Requires permission `device.console`. */
  getDevicesIdConsoleSid(id: string, sid: string, query?: { after?: number; /** last sequence number seen */ limit?: number; }): Promise<{
    lines?: ({
      dir?: "in" | "out";
      seq?: number;
      text?: string;
      ts?: string;
    })[];
    session?: ConsoleSession;
  }> {
    return this.request<{
    lines?: ({
      dir?: "in" | "out";
      seq?: number;
      text?: string;
      ts?: string;
    })[];
    session?: ConsoleSession;
  }>('GET', `/api/v1/devices/${encodeURIComponent(id)}/console/${encodeURIComponent(sid)}`, query, undefined, false);
  }

  /** Close a console session
   *
   * Requires permission `device.console`. */
  deleteDevicesIdConsoleSid(id: string, sid: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/devices/${encodeURIComponent(id)}/console/${encodeURIComponent(sid)}`, query, undefined, false);
  }

  /** Send a line to the device
   *
   * Requires permission `device.console`. */
  postDevicesIdConsoleSidIn(id: string, sid: string, body: {
    line: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/devices/${encodeURIComponent(id)}/console/${encodeURIComponent(sid)}/in`, undefined, body, false);
  }

  /** Assign a datastream to the device
   *
   * The device's value with the datastream's key is stored into it; a previous assignment of the datastream is closed. With the policy `four_eyes_assignments` the call answers 202 with a pending change request instead.
   * 
   * Requires permission `device.manage`. */
  postDevicesIdDatastreams(id: string, body: {
    datastream_id: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    closed_previous?: boolean;
    datastream_id?: string;
    device_id?: string;
    /** assignment id */
    id?: string;
  }> {
    return this.request<{
    closed_previous?: boolean;
    datastream_id?: string;
    device_id?: string;
    /** assignment id */
    id?: string;
  }>('POST', `/api/v1/devices/${encodeURIComponent(id)}/datastreams`, undefined, body, false);
  }

  /** End a datastream assignment
   *
   * Requires permission `device.manage`. */
  deleteDevicesIdDatastreamsDs(id: string, ds: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/devices/${encodeURIComponent(id)}/datastreams/${encodeURIComponent(ds)}`, query, undefined, false);
  }

  /** Raw messages of a device
   *
   * Every stored message in both directions with decode status (default 50, max 500).
   * 
   * Requires permission `data.read`. */
  getDevicesIdMessages(id: string, query?: { limit?: number; }): Promise<Message[]> {
    return this.request<Message[]>('GET', `/api/v1/devices/${encodeURIComponent(id)}/messages`, query, undefined, false);
  }

  /** Reject a self-provisioned device
   *
   * The device and its token are revoked.
   * 
   * Requires permission `device.manage`. */
  postDevicesIdReject(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    approval?: string;
    id?: string;
  }> {
    return this.request<{
    approval?: string;
    id?: string;
  }>('POST', `/api/v1/devices/${encodeURIComponent(id)}/reject`, undefined, body, false);
  }

  /** Revoke a device
   *
   * Disconnects it and rejects its credentials; data stays.
   * 
   * Requires permission `device.manage`. */
  postDevicesIdRevoke(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/devices/${encodeURIComponent(id)}/revoke`, undefined, body, false);
  }

  /** Issue or rotate the device token
   *
   * The previous token stops working at once; the new one is returned once.
   * 
   * Requires permission `device.manage`. */
  postDevicesIdToken(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    external_id?: string;
    /** shown once */
    token?: string;
  }> {
    return this.request<{
    external_id?: string;
    /** shown once */
    token?: string;
  }>('POST', `/api/v1/devices/${encodeURIComponent(id)}/token`, undefined, body, false);
  }

  /** Discovered devices (inbox)
   *
   * Requires permission `device.manage`. */
  getDiscovery(query?: { status?: string; /** pending | adopted | ignored */ }): Promise<(Record<string, unknown>)[]> {
    return this.request<(Record<string, unknown>)[]>('GET', `/api/v1/discovery`, query, undefined, false);
  }

  /** Adopt a discovered device
   *
   * Creates the device, its room (from the suggested area) and its entities.
   * 
   * Requires permission `device.manage`. */
  postDiscoveryIdAdopt(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    device_id?: string;
    id?: string;
    status?: string;
  }> {
    return this.request<{
    device_id?: string;
    id?: string;
    status?: string;
  }>('POST', `/api/v1/discovery/${encodeURIComponent(id)}/adopt`, undefined, body, false);
  }

  /** Ignore a discovered device
   *
   * Requires permission `device.manage`. */
  postDiscoveryIdIgnore(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    id?: string;
    status?: string;
  }> {
    return this.request<{
    id?: string;
    status?: string;
  }>('POST', `/api/v1/discovery/${encodeURIComponent(id)}/ignore`, undefined, body, false);
  }

  /** Kiosk: live video
   *
   * WebSocket like /video/cameras/{id}/live, only for cameras on the display's walls, in camera widgets of its dashboards, sent by an operator, or (popup all) with an active event.
   * 
   * Requires permission `display`. */
  getDisplayCamerasIdLive(id: string): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/display/cameras/${encodeURIComponent(id)}/live`, undefined, undefined, true);
  }

  /** Kiosk: paired, or a pairing code
   *
   * Called by the kiosk page. A paired kiosk (display cookie) gets paired = true. Otherwise a pairing is created (15 minutes, at most 20 waiting per address): the code to show is returned once and a secret is set as an HttpOnly cookie; after an administrator entered the code, the next call with that cookie sets the display cookie. With code_known the waiting pairing is kept. */
  postDisplayHello(body: {
    code_known?: boolean;
  }): Promise<{
    /** XXXX-XXXX, only when created */
    code?: string;
    expires_at?: string;
    name?: string;
    paired?: boolean;
  }> {
    return this.request<{
    /** XXXX-XXXX, only when created */
    code?: string;
    expires_at?: string;
    name?: string;
    paired?: boolean;
  }>('POST', `/api/v1/display/hello`, undefined, body, false);
  }

  /** Kiosk: content, command and camera events
   *
   * Display cookie only. Also records when the display was last seen.
   * 
   * Requires permission `display`. */
  getDisplayState(): Promise<{
    /** id → {name, kinds} of cameras with events */
    active?: Record<string, unknown>;
    /** id → name of the cameras it may show */
    cameras?: Record<string, unknown>;
    /** current command */
    command?: Record<string, unknown>;
    id?: string;
    items?: (Record<string, unknown>)[];
    locale?: string;
    name?: string;
    popup?: string;
    popup_seconds?: number;
    server_time?: string;
    /** changes when the content changes */
    version?: number;
  }> {
    return this.request<{
    /** id → {name, kinds} of cameras with events */
    active?: Record<string, unknown>;
    /** id → name of the cameras it may show */
    cameras?: Record<string, unknown>;
    /** current command */
    command?: Record<string, unknown>;
    id?: string;
    items?: (Record<string, unknown>)[];
    locale?: string;
    name?: string;
    popup?: string;
    popup_seconds?: number;
    server_time?: string;
    /** changes when the content changes */
    version?: number;
  }>('GET', `/api/v1/display/state`, undefined, undefined, false);
  }

  /** Kiosk: a video wall of its content
   *
   * Requires permission `display`. */
  getDisplayWallsId(id: string): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/display/walls/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Displays
   *
   * Requires permission `display.control`. */
  getDisplays(): Promise<Display[]> {
    return this.request<Display[]>('GET', `/api/v1/displays`, undefined, undefined, false);
  }

  /** Send a command or message to several displays
   *
   * The same actions for the listed displays, those of a location, or all displays of the organization. One audit entry lists the displays.
   * 
   * Requires permission `display.control`. */
  postDisplaysCommand(body: {
    action: "camera" | "identify" | "reload" | "clear" | "message";
    all?: boolean;
    camera?: string;
    ids?: string[];
    level?: "info" | "warning" | "alarm";
    location?: string;
    seconds?: number;
    text?: string;
  }): Promise<{
    /** how many got it */
    displays?: number;
  }> {
    return this.request<{
    /** how many got it */
    displays?: number;
  }>('POST', `/api/v1/displays/command`, undefined, body, false);
  }

  /** Pair a display
   *
   * Enter the code the kiosk shows (case-insensitive; 10 wrong codes in 10 minutes block the user for a while). Returns the device's address and browser. Audited.
   * 
   * Requires permission `display.manage`. */
  postDisplaysPair(body: {
    /** XXXX-XXXX */
    code: string;
    content: DisplayContent;
    /** hall, line ≤ 60 */
    location?: string;
    /** ≤ 80 */
    name: string;
    /** default true */
    offline_alert?: boolean;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<Display> {
    return this.request<Display>('POST', `/api/v1/displays/pair`, undefined, body, false);
  }

  /** Change a display
   *
   * Requires permission `display.manage`. */
  putDisplaysId(id: string, body: {
    content?: DisplayContent;
    location?: string;
    name?: string;
    offline_alert?: boolean;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<Display> {
    return this.request<Display>('PUT', `/api/v1/displays/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Disconnect a display
   *
   * The display is locked out at once and must be paired again.
   * 
   * Requires permission `display.manage`. */
  deleteDisplaysId(id: string, query?: { reason?: string; /** required */ }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/displays/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Send a command to a display
   *
   * camera: show a camera the operator may see (seconds 5–3600); identify: show the display's name; reload; clear: back to the content (also ends a message); message: an announcement (info as a band at the bottom, warning at the top, alarm over the whole screen) shown apart from cameras. Audited.
   * 
   * Requires permission `display.control`. */
  postDisplaysIdCommand(id: string, body: {
    action: "camera" | "identify" | "reload" | "clear" | "message";
    camera?: string;
    level?: "info" | "warning" | "alarm";
    quality?: "main" | "sub";
    /** 5–3600, default 60 */
    seconds?: number;
    /** message, 1–200 characters, shown as text */
    text?: string;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/displays/${encodeURIComponent(id)}/command`, undefined, body, false);
  }

  /** Energy overview
   *
   * Consumption and production of energy meters (entities with device_class energy, datastreams in Wh/kWh/MWh) per hour (day), day (week, month) or month (year) in the organization's time zone, with cost.
   * 
   * Requires permission `data.read`. */
  getEnergy(query?: { period?: string; /** day (default), week, month, year */ date?: string; /** YYYY-MM-DD inside the period (default today) */ }): Promise<{
    /** hour, day, month */
    bucket?: string;
    buckets?: ({
      consumption?: number;
      cost?: number;
      production?: number;
      start?: string;
    })[];
    currency?: string;
    feed_in_per_kwh?: number;
    from?: string;
    period?: string;
    /** sum of the current states of power entities */
    power_w?: number;
    price_per_kwh?: number;
    sources?: ({
      datastream_id?: string;
      device?: string;
      entity_id?: string;
      kwh?: number;
      name?: string;
      /** consumption, production, off */
      role?: string;
      /** % of consumption */
      share?: number;
      unit?: string;
    })[];
    to?: string;
    total?: {
      consumption?: number;
      cost?: number;
      production?: number;
    };
    tz?: string;
    /** kWh */
    unit?: string;
  }> {
    return this.request<{
    /** hour, day, month */
    bucket?: string;
    buckets?: ({
      consumption?: number;
      cost?: number;
      production?: number;
      start?: string;
    })[];
    currency?: string;
    feed_in_per_kwh?: number;
    from?: string;
    period?: string;
    /** sum of the current states of power entities */
    power_w?: number;
    price_per_kwh?: number;
    sources?: ({
      datastream_id?: string;
      device?: string;
      entity_id?: string;
      kwh?: number;
      name?: string;
      /** consumption, production, off */
      role?: string;
      /** % of consumption */
      share?: number;
      unit?: string;
    })[];
    to?: string;
    total?: {
      consumption?: number;
      cost?: number;
      production?: number;
    };
    tz?: string;
    /** kWh */
    unit?: string;
  }>('GET', `/api/v1/energy`, query, undefined, false);
  }

  /** Energy settings and meters
   *
   * Requires permission `data.read`. */
  getEnergySettings(): Promise<{
    currency?: string;
    exclude_others?: boolean;
    feed_in_per_kwh?: number;
    meters?: ({
      datastream_id?: string;
      device?: string;
      name?: string;
      role?: string;
      unit?: string;
    })[];
    price_per_kwh?: number;
  }> {
    return this.request<{
    currency?: string;
    exclude_others?: boolean;
    feed_in_per_kwh?: number;
    meters?: ({
      datastream_id?: string;
      device?: string;
      name?: string;
      role?: string;
      unit?: string;
    })[];
    price_per_kwh?: number;
  }>('GET', `/api/v1/energy/settings`, undefined, undefined, false);
  }

  /** Set energy price and meter roles
   *
   * Meters without an explicit role count as consumption; a device with several energy sensors counts the one named total.
   * 
   * Requires permission `content.write`. */
  putEnergySettings(body: {
    /** ≤ 4 */
    currency?: string;
    /** count only the listed meters */
    exclude_others?: boolean;
    /** 0–100 */
    feed_in_per_kwh?: number;
    /** 0–100 */
    price_per_kwh?: number;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    sources?: ({
      datastream_id?: string;
      role?: "consumption" | "production" | "off";
    })[];
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('PUT', `/api/v1/energy/settings`, undefined, body, false);
  }

  /** Entities with their state
   *
   * Requires permission `data.read`. */
  getEntities(query?: { room?: string; device?: string; component?: string; source?: string; /** MQTT source id */ }): Promise<({
    attributes?: Record<string, unknown>;
    available?: boolean;
    /** sensor, binary_sensor, switch, light, cover, fan, lock, climate, number, select, button, text, … */
    component?: string;
    /** history of the state */
    datastream_id?: string;
    device?: string;
    device_id?: string;
    error?: string;
    id?: string;
    name?: string;
    room?: string;
    room_id?: string;
    services?: string[];
    state?: string;
    status?: "active" | "unsupported" | "removed";
    /** control hints: min, max, step, options, modes, …; lights: color {rgb, color_temp, min_mireds, max_mireds} */
    ui?: Record<string, unknown>;
    unit?: string;
  })[]> {
    return this.request<({
    attributes?: Record<string, unknown>;
    available?: boolean;
    /** sensor, binary_sensor, switch, light, cover, fan, lock, climate, number, select, button, text, … */
    component?: string;
    /** history of the state */
    datastream_id?: string;
    device?: string;
    device_id?: string;
    error?: string;
    id?: string;
    name?: string;
    room?: string;
    room_id?: string;
    services?: string[];
    state?: string;
    status?: "active" | "unsupported" | "removed";
    /** control hints: min, max, step, options, modes, …; lights: color {rgb, color_temp, min_mireds, max_mireds} */
    ui?: Record<string, unknown>;
    unit?: string;
  })[]>('GET', `/api/v1/entities`, query, undefined, false);
  }

  /** Entity with its discovery config
   *
   * Requires permission `data.read`. */
  getEntitiesId(id: string): Promise<{
    attributes?: Record<string, unknown>;
    available?: boolean;
    /** sensor, binary_sensor, switch, light, cover, fan, lock, climate, number, select, button, text, … */
    component?: string;
    /** history of the state */
    datastream_id?: string;
    device?: string;
    device_id?: string;
    error?: string;
    id?: string;
    name?: string;
    room?: string;
    room_id?: string;
    services?: string[];
    state?: string;
    status?: "active" | "unsupported" | "removed";
    /** control hints: min, max, step, options, modes, …; lights: color {rgb, color_temp, min_mireds, max_mireds} */
    ui?: Record<string, unknown>;
    unit?: string;
  }> {
    return this.request<{
    attributes?: Record<string, unknown>;
    available?: boolean;
    /** sensor, binary_sensor, switch, light, cover, fan, lock, climate, number, select, button, text, … */
    component?: string;
    /** history of the state */
    datastream_id?: string;
    device?: string;
    device_id?: string;
    error?: string;
    id?: string;
    name?: string;
    room?: string;
    room_id?: string;
    services?: string[];
    state?: string;
    status?: "active" | "unsupported" | "removed";
    /** control hints: min, max, step, options, modes, …; lights: color {rgb, color_temp, min_mireds, max_mireds} */
    ui?: Record<string, unknown>;
    unit?: string;
  }>('GET', `/api/v1/entities/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Rename, move to a room, hide
   *
   * Requires permission `device.manage`. */
  patchEntitiesId(id: string, body: {
    event_kinds?: string[];
    /** 1–600 */
    event_post_s?: number;
    /** 0–60, record = events */
    event_pre_s?: number;
    /** camera group ≤ 60; users limited to groups see only their cameras (spec 014) */
    group?: string;
    hidden?: boolean;
    name?: string;
    /** http://camera/onvif/device_service: events and PTZ (spec 013); for source push the camera's address in its own network, reached through the relay (onvif = … in relay.toml) */
    onvif_url?: string;
    privacy_masks?: number[][][];
    /** empty = the first profile with PTZ */
    ptz_profile?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    room_id?: string;
    sort?: number;
    /** pull: the server connects to the camera; push: a relay at a remote site sends the video (secret returned once as push_secret with relay_config) */
    source?: "pull" | "push";
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('PATCH', `/api/v1/entities/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Control an entity
   *
   * Becomes a device command (permissions, four eyes, audit): `sent` until the device reports the requested state, then `acked`; optimistic entities and buttons are `acked` at once. The reason defaults to "Home: <service> <entity>".
   * 
   * Requires permission `device.command`. */
  postEntitiesIdCommand(id: string, body: {
    /** 0–100 % */
    brightness?: number;
    /** mireds */
    color_temp?: number;
    mode?: string;
    percentage?: number;
    /** 0–100 % */
    position?: number;
    /** optional */
    reason?: string;
    rgb?: number[];
    /** turn_on, turn_off, toggle, open, close, stop, set_position, lock, unlock, set_temperature, set_hvac_mode, set_value, select_option, press, set_percentage */
    service: string;
    temperature?: number;
    /** number, text or option */
    value?: Record<string, unknown>;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/entities/${encodeURIComponent(id)}/command`, undefined, body, false);
  }

  /** List escalation policies
   *
   * Requires permission `config.write`. */
  getEscalationPolicies(): Promise<EscalationPolicy[]> {
    return this.request<EscalationPolicy[]>('GET', `/api/v1/escalation-policies`, undefined, undefined, false);
  }

  /** Create an escalation policy
   *
   * Step 0 (after_s = 0) is delivered at once; later steps only while the alarm is still active and unacknowledged.
   * 
   * Requires permission `config.write`. */
  postEscalationPolicies(body: {
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    steps: EscalationStep[];
  }): Promise<EscalationPolicy> {
    return this.request<EscalationPolicy>('POST', `/api/v1/escalation-policies`, undefined, body, false);
  }

  /** Update a policy (new version in place)
   *
   * Requires permission `config.write`. */
  putEscalationPoliciesId(id: string, body: {
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    steps: EscalationStep[];
  }): Promise<EscalationPolicy> {
    return this.request<EscalationPolicy>('PUT', `/api/v1/escalation-policies/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Revoke a policy
   *
   * Refused (409) while alarm rules use it.
   * 
   * Requires permission `config.write`. */
  deleteEscalationPoliciesId(id: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/escalation-policies/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Export the access log
   *
   * Sign-ins, failures, token use, session events.
   * 
   * Requires permission `audit.read`. */
  getExportAccessLog(query?: { from?: string; to?: string; format?: string; /** csv | jsonl */ limit?: number; }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/export/access-log`, query, undefined, true);
  }

  /** Export the alarm log
   *
   * Requires permission `data.export`. */
  getExportAlarms(query?: { from?: string; to?: string; format?: string; /** csv | jsonl */ limit?: number; }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/export/alarms`, query, undefined, true);
  }

  /** Export the audit trail
   *
   * Hash-chained rows: seq, hash, prev_hash, actor, action, entity, old/new values, reason, request id.
   * 
   * Requires permission `audit.read`. */
  getExportAudit(query?: { from?: string; to?: string; format?: string; /** csv | jsonl */ limit?: number; }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/export/audit`, query, undefined, true);
  }

  /** Export raw messages
   *
   * Requires permission `data.export`. */
  getExportMessages(query?: { device_id?: string; from?: string; to?: string; format?: string; /** csv | jsonl */ limit?: number; }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/export/messages`, query, undefined, true);
  }

  /** Export observations
   *
   * `format` csv (default), jsonl or pdf (a signed report stored under Reports). Flagged rows (quality other than ok) are marked.
   * 
   * Requires permission `data.export`. */
  getExportObservations(query: { datastream_id: string; from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ format?: string; /** csv | jsonl | pdf */ limit?: number; }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/export/observations`, query, undefined, true);
  }

  /** Export an organization as a ZIP archive
   *
   * CSV files of every table of the caller's organization for a period, with a manifest and SHA-256 checksums. An organization administrator task (user.admin) since 0.65.0.
   * 
   * Requires permission `user.admin`. */
  getExportOrganization(query?: { from?: string; /** RFC 3339 start (optional) */ to?: string; /** RFC 3339 end (optional) */ }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/export/organization`, query, undefined, true);
  }

  /** List firmware images
   *
   * Requires permission `fota.read`. */
  getFirmware(): Promise<Firmware[]> {
    return this.request<Firmware[]>('GET', `/api/v1/firmware`, undefined, undefined, false);
  }

  /** Upload a firmware image
   *
   * multipart/form-data. The SHA-256 is computed on the server; an Ed25519 signature may be attached for devices that verify it.
   * 
   * Requires permission `fota.manage`. */
  postFirmware(body: FormData): Promise<Firmware> {
    return this.request<Firmware>('POST', `/api/v1/firmware`, undefined, body, false);
  }

  /** Incoming webhook of a flow
   *
   * The JSON body's value and topic become the message, other fields msg.<name>. With a secret on the node, X-Signature: sha256=<HMAC-SHA256 of the body> is required. */
  postFlowHooksToken(token: string, body: Record<string, unknown>): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/flow-hooks/${encodeURIComponent(token)}`, undefined, body, false);
  }

  /** Flows
   *
   * Requires permission `data.read`. */
  getFlows(): Promise<({
    dropped?: number;
    errors?: number;
    flow?: Flow;
    running?: boolean;
  })[]> {
    return this.request<({
    dropped?: number;
    errors?: number;
    flow?: Flow;
    running?: boolean;
  })[]>('GET', `/api/v1/flows`, undefined, undefined, false);
  }

  /** Create a flow (draft)
   *
   * Requires permission `content.write`. */
  postFlows(body: {
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    spec?: FlowSpec;
  }): Promise<{
    flow?: Flow;
    problems?: FlowProblem[];
  }> {
    return this.request<{
    flow?: Flow;
    problems?: FlowProblem[];
  }>('POST', `/api/v1/flows`, undefined, body, false);
  }

  /** Node library
   *
   * Every node type with its group, ports and settings schema, and the expression functions.
   * 
   * Requires permission `data.read`. */
  getFlowsCatalog(): Promise<{
    functions?: string[];
    limits?: Record<string, unknown>;
    nodes?: (Record<string, unknown>)[];
  }> {
    return this.request<{
    functions?: string[];
    limits?: Record<string, unknown>;
    nodes?: (Record<string, unknown>)[];
  }>('GET', `/api/v1/flows/catalog`, undefined, undefined, false);
  }

  /** Import a flow
   *
   * Requires permission `content.write`. */
  postFlowsImport(body: {
    /** ctrl32-flow/1 */
    format: string;
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    slug?: string;
    spec: FlowSpec;
  }): Promise<{
    flow?: Flow;
    problems?: FlowProblem[];
  }> {
    return this.request<{
    flow?: Flow;
    problems?: FlowProblem[];
  }>('POST', `/api/v1/flows/import`, undefined, body, false);
  }

  /** Flow settings
   *
   * Requires permission `data.read`. */
  getFlowsSettings(): Promise<{
    allowed_hosts?: string[];
  }> {
    return this.request<{
    allowed_hosts?: string[];
  }>('GET', `/api/v1/flows/settings`, undefined, undefined, false);
  }

  /** Change flow settings
   *
   * Requires permission `config.write`. */
  putFlowsSettings(body: {
    allowed_hosts: string[];
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    allowed_hosts?: string[];
  }> {
    return this.request<{
    allowed_hosts?: string[];
  }>('PUT', `/api/v1/flows/settings`, undefined, body, false);
  }

  /** Validate a flow
   *
   * Requires permission `data.read`. */
  postFlowsValidate(body: {
    spec: FlowSpec;
  }): Promise<{
    has_commands?: boolean;
    ok?: boolean;
    problems?: FlowProblem[];
  }> {
    return this.request<{
    has_commands?: boolean;
    ok?: boolean;
    problems?: FlowProblem[];
  }>('POST', `/api/v1/flows/validate`, undefined, body, false);
  }

  /** A flow (current version)
   *
   * With validation problems, running status per node and the URLs of its webhook nodes.
   * 
   * Requires permission `data.read`. */
  getFlowsSlug(slug: string): Promise<{
    flow?: Flow;
    /** node id → URL */
    hooks?: Record<string, unknown>;
    problems?: FlowProblem[];
    /** running, version, queue, dropped, nodes {in, out, errors, last} */
    status?: Record<string, unknown>;
  }> {
    return this.request<{
    flow?: Flow;
    /** node id → URL */
    hooks?: Record<string, unknown>;
    problems?: FlowProblem[];
    /** running, version, queue, dropped, nodes {in, out, errors, last} */
    status?: Record<string, unknown>;
  }>('GET', `/api/v1/flows/${encodeURIComponent(slug)}`, undefined, undefined, false);
  }

  /** Save a new version (draft)
   *
   * The deployed version keeps running until the new one is deployed.
   * 
   * Requires permission `content.write`. */
  putFlowsSlug(slug: string, body: {
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    spec: FlowSpec;
  }): Promise<{
    flow?: Flow;
    problems?: FlowProblem[];
  }> {
    return this.request<{
    flow?: Flow;
    problems?: FlowProblem[];
  }>('PUT', `/api/v1/flows/${encodeURIComponent(slug)}`, undefined, body, false);
  }

  /** Delete a flow
   *
   * Requires permission `content.write`. */
  deleteFlowsSlug(slug: string, query: { reason: string; }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('DELETE', `/api/v1/flows/${encodeURIComponent(slug)}`, query, undefined, false);
  }

  /** Debug panel entries
   *
   * The last 500 entries: debug nodes, errors, actions, dry-run actions, drops.
   * 
   * Requires permission `data.read`. */
  getFlowsSlugDebug(slug: string, query?: { after?: number; /** sequence number of the last entry seen */ }): Promise<({
    dry?: boolean;
    /** debug, error, action, dry_action, drop, info */
    kind?: string;
    msg?: Record<string, unknown>;
    node?: string;
    seq?: number;
    text?: string;
    ts?: string;
  })[]> {
    return this.request<({
    dry?: boolean;
    /** debug, error, action, dry_action, drop, info */
    kind?: string;
    msg?: Record<string, unknown>;
    node?: string;
    seq?: number;
    text?: string;
    ts?: string;
  })[]>('GET', `/api/v1/flows/${encodeURIComponent(slug)}/debug`, query, undefined, false);
  }

  /** Deploy the current version
   *
   * Refused (422) with the problems when the flow is not valid. A flow that sends commands needs device.command and, with the four-eyes policy for commands, becomes a change request (202) that another user approves.
   * 
   * Requires permission `content.write`. */
  postFlowsSlugDeploy(slug: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    flow?: Flow;
    status?: Record<string, unknown>;
  }> {
    return this.request<{
    flow?: Flow;
    status?: Record<string, unknown>;
  }>('POST', `/api/v1/flows/${encodeURIComponent(slug)}/deploy`, undefined, body, false);
  }

  /** Stop a deployed flow
   *
   * Requires permission `content.write`. */
  postFlowsSlugDisable(slug: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    flow?: Flow;
  }> {
    return this.request<{
    flow?: Flow;
  }>('POST', `/api/v1/flows/${encodeURIComponent(slug)}/disable`, undefined, body, false);
  }

  /** Export a flow (JSON)
   *
   * Requires permission `data.read`. */
  getFlowsSlugExport(slug: string): Promise<{
    checksum?: string;
    /** ctrl32-flow/1 */
    format?: string;
    name?: string;
    slug?: string;
    spec?: FlowSpec;
    version?: number;
  }> {
    return this.request<{
    checksum?: string;
    /** ctrl32-flow/1 */
    format?: string;
    name?: string;
    slug?: string;
    spec?: FlowSpec;
    version?: number;
  }>('GET', `/api/v1/flows/${encodeURIComponent(slug)}/export`, undefined, undefined, false);
  }

  /** Send a test message
   *
   * Into a node of the deployed flow (dry_run: actions are shown, not executed), or with `spec` into the edited flow (always a dry run).
   * 
   * Requires permission `content.write`. */
  postFlowsSlugInject(slug: string, body: {
    dry_run?: boolean;
    node: string;
    spec?: FlowSpec;
    topic?: string;
    /** number, text, bool or null */
    value?: Record<string, unknown>;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/flows/${encodeURIComponent(slug)}/inject`, undefined, body, false);
  }

  /** Restore a version
   *
   * Saves the old content as a new version (deploy it separately).
   * 
   * Requires permission `content.write`. */
  postFlowsSlugRestore(slug: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    version: number;
  }): Promise<{
    flow?: Flow;
    problems?: FlowProblem[];
  }> {
    return this.request<{
    flow?: Flow;
    problems?: FlowProblem[];
  }>('POST', `/api/v1/flows/${encodeURIComponent(slug)}/restore`, undefined, body, false);
  }

  /** Counters of a running flow
   *
   * Requires permission `data.read`. */
  getFlowsSlugStatus(slug: string): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/flows/${encodeURIComponent(slug)}/status`, undefined, undefined, false);
  }

  /** Versions of a flow
   *
   * Requires permission `data.read`. */
  getFlowsSlugVersions(slug: string): Promise<Flow[]> {
    return this.request<Flow[]>('GET', `/api/v1/flows/${encodeURIComponent(slug)}/versions`, undefined, undefined, false);
  }

  /** List update campaigns
   *
   * Requires permission `fota.read`. */
  getFotaCampaigns(): Promise<Campaign[]> {
    return this.request<Campaign[]>('GET', `/api/v1/fota/campaigns`, undefined, undefined, false);
  }

  /** Start an update campaign
   *
   * Publishes the update offer to the selected devices; progress arrives on `d/{id}/fota/status`.
   * 
   * Requires permission `fota.manage`. */
  postFotaCampaigns(body: {
    device_ids: string[];
    firmware_id: string;
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    devices?: number;
    firmware_id?: string;
    id?: string;
    name?: string;
    published?: number;
    version?: string;
  }> {
    return this.request<{
    devices?: number;
    firmware_id?: string;
    id?: string;
    name?: string;
    published?: number;
    version?: string;
  }>('POST', `/api/v1/fota/campaigns`, undefined, body, false);
  }

  /** Campaign detail with per-device status
   *
   * Requires permission `fota.read`. */
  getFotaCampaignsId(id: string): Promise<{
    campaign?: Campaign;
    devices?: ({
      device_fw?: string;
      device_id?: string;
      error?: string | null;
      external_id?: string;
      name?: string;
      pct?: number;
      status?: string;
      updated_at?: string;
      version?: string;
    })[];
  }> {
    return this.request<{
    campaign?: Campaign;
    devices?: ({
      device_fw?: string;
      device_id?: string;
      error?: string | null;
      external_id?: string;
      name?: string;
      pct?: number;
      status?: string;
      updated_at?: string;
      version?: string;
    })[];
  }>('GET', `/api/v1/fota/campaigns/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Cancel a campaign
   *
   * Requires permission `fota.manage`. */
  deleteFotaCampaignsId(id: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/fota/campaigns/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Library objects for process pictures (current versions)
   *
   * Requires permission `data.read`. */
  getHmiObjects(query?: { kind?: string; /** svg or composite (empty = all) */ }): Promise<(Record<string, unknown>)[]> {
    return this.request<(Record<string, unknown>)[]>('GET', `/api/v1/hmi/objects`, query, undefined, false);
  }

  /** Create a library object
   *
   * kind `composite`: body {elements, tags, width, height} — drawing elements (screen format v1) whose tags may be placeholders `$name` listed in `tags`; kind `svg`: body {svg}, sanitised like an import.
   * 
   * Requires permission `content.write`. */
  postHmiObjects(body: {
    /** object body */
    body: Record<string, unknown>;
    category?: string;
    /** svg | composite */
    kind: string;
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    slug?: string;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/hmi/objects`, undefined, body, false);
  }

  /** Import an SVG graphic
   *
   * The SVG is sanitised by a whitelist (no script, event handlers, foreignObject, images, links, animations or external references); the response lists what was removed.
   * 
   * Requires permission `content.write`. */
  postHmiObjectsImportSvg(body: {
    category?: string;
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    slug?: string;
    /** SVG document, at most 512 KiB */
    svg: string;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/hmi/objects/import-svg`, undefined, body, false);
  }

  /** Library object with its body
   *
   * Requires permission `data.read`. */
  getHmiObjectsSlug(slug: string): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/hmi/objects/${encodeURIComponent(slug)}`, undefined, undefined, false);
  }

  /** Save a new version of a library object
   *
   * Requires permission `content.write`. */
  putHmiObjectsSlug(slug: string, body: {
    /** object body */
    body: Record<string, unknown>;
    category?: string;
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('PUT', `/api/v1/hmi/objects/${encodeURIComponent(slug)}`, undefined, body, false);
  }

  /** Retire a library object
   *
   * Requires permission `content.write`. */
  deleteHmiObjectsSlug(slug: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/hmi/objects/${encodeURIComponent(slug)}`, query, undefined, false);
  }

  /** The sanitised SVG of a graphic
   *
   * Served as image/svg+xml with a sandbox Content-Security-Policy.
   * 
   * Requires permission `data.read`. */
  getHmiObjectsSlugSvg(slug: string): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/hmi/objects/${encodeURIComponent(slug)}/svg`, undefined, undefined, true);
  }

  /** Rooms with their entities (Home page)
   *
   * Requires permission `data.read`. */
  getHome(): Promise<{
    can_control?: boolean;
    can_manage?: boolean;
    pending_devices?: number;
    rooms?: ({
      entities?: ({
        attributes?: Record<string, unknown>;
        available?: boolean;
        /** sensor, binary_sensor, switch, light, cover, fan, lock, climate, number, select, button, text, … */
        component?: string;
        /** history of the state */
        datastream_id?: string;
        device?: string;
        device_id?: string;
        error?: string;
        id?: string;
        name?: string;
        room?: string;
        room_id?: string;
        services?: string[];
        state?: string;
        status?: "active" | "unsupported" | "removed";
        /** control hints: min, max, step, options, modes, …; lights: color {rgb, color_temp, min_mireds, max_mireds} */
        ui?: Record<string, unknown>;
        unit?: string;
      })[];
      id?: string;
      name?: string;
    })[];
  }> {
    return this.request<{
    can_control?: boolean;
    can_manage?: boolean;
    pending_devices?: number;
    rooms?: ({
      entities?: ({
        attributes?: Record<string, unknown>;
        available?: boolean;
        /** sensor, binary_sensor, switch, light, cover, fan, lock, climate, number, select, button, text, … */
        component?: string;
        /** history of the state */
        datastream_id?: string;
        device?: string;
        device_id?: string;
        error?: string;
        id?: string;
        name?: string;
        room?: string;
        room_id?: string;
        services?: string[];
        state?: string;
        status?: "active" | "unsupported" | "removed";
        /** control hints: min, max, step, options, modes, …; lights: color {rgb, color_temp, min_mireds, max_mireds} */
        ui?: Record<string, unknown>;
        unit?: string;
      })[];
      id?: string;
      name?: string;
    })[];
  }>('GET', `/api/v1/home`, undefined, undefined, false);
  }

  /** Supported and enabled languages */
  getI18nLanguages(): Promise<{
    default?: string;
    enabled?: string[];
    supported?: ({
      /** built-in translated strings */
      builtin?: number;
      code?: string;
      name?: string;
    })[];
  }> {
    return this.request<{
    default?: string;
    enabled?: string[];
    supported?: ({
      /** built-in translated strings */
      builtin?: number;
      code?: string;
      name?: string;
    })[];
  }>('GET', `/api/v1/i18n/languages`, undefined, undefined, false);
  }

  /** Enable languages and set the default
   *
   * Requires permission `content.write`. */
  putI18nLanguages(body: {
    default: string;
    enabled: string[];
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    default?: string;
    enabled?: string[];
  }> {
    return this.request<{
    default?: string;
    enabled?: string[];
  }>('PUT', `/api/v1/i18n/languages`, undefined, body, false);
  }

  /** Translation catalog of a locale
   *
   * Built-in strings merged with the organization's overrides; key = English text. */
  getI18nLocale(locale: string): Promise<Record<string, string>> {
    return this.request<Record<string, string>>('GET', `/api/v1/i18n/${encodeURIComponent(locale)}`, undefined, undefined, false);
  }

  /** Set translation overrides
   *
   * An empty value removes the override.
   * 
   * Requires permission `content.write`. */
  putI18nLocale(locale: string, body: {
    entries: Record<string, string>;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    locale?: string;
    removed?: number;
    set?: number;
  }> {
    return this.request<{
    locale?: string;
    removed?: number;
    set?: number;
  }>('PUT', `/api/v1/i18n/${encodeURIComponent(locale)}`, undefined, body, false);
  }

  /** Catalog for editing (built-in and override per key)
   *
   * Requires permission `content.write`. */
  getI18nLocaleEdit(locale: string): Promise<{
    entries?: ({
      builtin?: string;
      key?: string;
      override?: string;
    })[];
    locale?: string;
  }> {
    return this.request<{
    entries?: ({
      builtin?: string;
      key?: string;
      override?: string;
    })[];
    locale?: string;
  }>('GET', `/api/v1/i18n/${encodeURIComponent(locale)}/edit`, undefined, undefined, false);
  }

  /** List incidents
   *
   * Incident log (URS C-07, I-09): an alarm whose escalation step could not be delivered and a broken audit trail chain open incidents automatically; operators add their own. Each incident carries impact, cause and CAPA (corrective and preventive action). `status` = `open` (default: open + investigating), `all`, or one status. System administrators also see instance-level incidents (no organization).
   * 
   * Requires a signed-in user or an API token (any permission). */
  getIncidents(query?: { status?: string; /** open, all, investigating, resolved or closed */ }): Promise<Incident[]> {
    return this.request<Incident[]>('GET', `/api/v1/incidents`, query, undefined, false);
  }

  /** Open an incident by hand
   *
   * Requires permission `alarm.ack`. */
  postIncidents(body: {
    capa?: string;
    cause?: string;
    impact?: string;
    /** Audit trail */
    reason: string;
    /** minor, major (default) or critical */
    severity?: string;
    /** Up to 200 characters */
    title: string;
  }): Promise<Incident> {
    return this.request<Incident>('POST', `/api/v1/incidents`, undefined, body, false);
  }

  /** Incident with its history
   *
   * Requires a signed-in user or an API token (any permission). */
  getIncidentsId(id: string): Promise<{
    events?: IncidentEvent[];
    incident?: Incident;
  }> {
    return this.request<{
    events?: IncidentEvent[];
    incident?: Incident;
  }>('GET', `/api/v1/incidents/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Update an incident: fields, a note, the status
   *
   * Transitions: open → investigating, resolved or closed; investigating → open, resolved or closed; resolved → investigating or closed; closed → investigating (reopen). Resolving needs a cause, closing a cause and a CAPA. Every change becomes an incident event and an audit trail entry.
   * 
   * Requires permission `alarm.ack`. */
  patchIncidentsId(id: string, body: {
    capa?: string;
    cause?: string;
    impact?: string;
    /** Free text appended to the history */
    note?: string;
    /** Audit trail */
    reason: string;
    /** New status (optional) */
    status?: string;
  }): Promise<Incident> {
    return this.request<Incident>('PATCH', `/api/v1/incidents/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** ChirpStack HTTP integration endpoint
   *
   * Called by ChirpStack with `?event=up` (and others) and the connector's bearer secret. Uplinks are decoded per device profile and stored like MQTT telemetry. */
  postIngestChirpstackId(id: string, body: Record<string, unknown>, query: { event: string; /** ChirpStack event type, e.g. up */ }): Promise<{
    accepted?: boolean;
    decoded_keys?: string[];
    device?: string;
    event?: string;
    note?: string;
  }> {
    return this.request<{
    accepted?: boolean;
    decoded_keys?: string[];
    device?: string;
    event?: string;
    note?: string;
  }>('POST', `/api/v1/ingest/chirpstack/${encodeURIComponent(id)}`, query, body, false);
  }

  /** The published front page
   *
   * Public. `spec` is null while the built-in page is shown. */
  getLanding(): Promise<{
    spec?: LandingSpec;
    version?: number;
  }> {
    return this.request<{
    spec?: LandingSpec;
    version?: number;
  }>('GET', `/api/v1/landing`, undefined, undefined, false);
  }

  /** Uploaded images
   *
   * Requires permission `system.admin`. */
  getLandingAssets(): Promise<({
    bytes?: number;
    created_at?: string;
    id?: string;
    mime?: string;
    name?: string;
    url?: string;
  })[]> {
    return this.request<({
    bytes?: number;
    created_at?: string;
    id?: string;
    mime?: string;
    name?: string;
    url?: string;
  })[]>('GET', `/api/v1/landing/assets`, undefined, undefined, false);
  }

  /** Upload an image
   *
   * PNG, JPEG or WebP (checked by content) or SVG (sanitised), at most 2 MiB.
   * 
   * Requires permission `system.admin`. */
  postLandingAssets(body: FormData): Promise<{
    bytes?: number;
    id?: string;
    mime?: string;
    name?: string;
    url?: string;
  }> {
    return this.request<{
    bytes?: number;
    id?: string;
    mime?: string;
    name?: string;
    url?: string;
  }>('POST', `/api/v1/landing/assets`, undefined, body, false);
  }

  /** An image of the front page
   *
   * Public; served with a sandbox Content-Security-Policy. */
  getLandingAssetsId(id: string): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/landing/assets/${encodeURIComponent(id)}`, undefined, undefined, true);
  }

  /** The newest version for the editor
   *
   * The built-in page as blocks when nothing was saved yet.
   * 
   * Requires permission `system.admin`. */
  getLandingDraft(): Promise<{
    published_version?: number;
    spec?: LandingSpec;
    /** null when nothing was saved */
    version?: number;
  }> {
    return this.request<{
    published_version?: number;
    spec?: LandingSpec;
    /** null when nothing was saved */
    version?: number;
  }>('GET', `/api/v1/landing/draft`, undefined, undefined, false);
  }

  /** Show the built-in page again
   *
   * Requires permission `system.admin`. */
  postLandingUnpublish(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    published_version?: number | null;
  }> {
    return this.request<{
    published_version?: number | null;
  }>('POST', `/api/v1/landing/unpublish`, undefined, body, false);
  }

  /** Saved versions
   *
   * Requires permission `system.admin`. */
  getLandingVersions(): Promise<({
    blocks?: number;
    created_at?: string;
    created_by?: string;
    published?: boolean;
    reason?: string;
    version?: number;
  })[]> {
    return this.request<({
    blocks?: number;
    created_at?: string;
    created_by?: string;
    published?: boolean;
    reason?: string;
    version?: number;
  })[]>('GET', `/api/v1/landing/versions`, undefined, undefined, false);
  }

  /** Save a version (and publish it)
   *
   * Strict validation: known block types and fields only, #rrggbb colours, catalogue icons, links to a page here, https://, mailto: or tel:, own images.
   * 
   * Requires permission `system.admin`. */
  postLandingVersions(body: {
    /** publish at once */
    publish?: boolean;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    spec: LandingSpec;
  }): Promise<{
    published?: boolean;
    version?: number;
  }> {
    return this.request<{
    published?: boolean;
    version?: number;
  }>('POST', `/api/v1/landing/versions`, undefined, body, false);
  }

  /** One version
   *
   * Requires permission `system.admin`. */
  getLandingVersionsV(v: string): Promise<{
    spec?: LandingSpec;
    version?: number;
  }> {
    return this.request<{
    spec?: LandingSpec;
    version?: number;
  }>('GET', `/api/v1/landing/versions/${encodeURIComponent(v)}`, undefined, undefined, false);
  }

  /** Publish a version
   *
   * Requires permission `system.admin`. */
  postLandingVersionsVPublish(v: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    published_version?: number;
  }> {
    return this.request<{
    published_version?: number;
  }>('POST', `/api/v1/landing/versions/${encodeURIComponent(v)}/publish`, undefined, body, false);
  }

  /** Consent page: the authorization request
   *
   * Used by the consent page `/oauth/authorize` in a signed-in browser: validates the OAuth request (client, redirect address, PKCE S256, scope, resource) and names the client. `error` and `redirect` when the request is refused in a way the client must learn.
   * 
   * Requires a signed-in user or an API token (any permission). */
  getMcpAuthorize(query: { client_id: string; redirect_uri: string; response_type: string; /** code */ code_challenge: string; /** PKCE S256 */ code_challenge_method: string; /** S256 */ state?: string; scope?: string; /** mcp or mcp:read */ resource?: string; /** the MCP address */ }): Promise<MCPConsent> {
    return this.request<MCPConsent>('GET', `/api/v1/mcp/authorize`, query, undefined, false);
  }

  /** Consent page: approve or deny
   *
   * Browser session only. Approving creates the grant (the user's permissions at this time, or reading only) and a single-use code valid for ten minutes; the answer names the address the browser goes to next (the client's redirect with `code` or `error`).
   * 
   * Requires a signed-in user or an API token (any permission). */
  postMcpAuthorize(body: {
    approve?: boolean;
    client_id: string;
    code_challenge: string;
    /** S256 */
    code_challenge_method: string;
    /** reading only */
    read_only?: boolean;
    redirect_uri: string;
    resource?: string;
    /** code */
    response_type: string;
    scope?: string;
    state?: string;
  }): Promise<{
    redirect?: string;
  }> {
    return this.request<{
    redirect?: string;
  }>('POST', `/api/v1/mcp/authorize`, undefined, body, false);
  }

  /** Connected AI assistants
   *
   * The assistants I approved (with all=1 the whole organization's, user.admin); revoked ones are listed for 30 days.
   * 
   * Requires a signed-in user or an API token (any permission). */
  getMcpGrants(query?: { all?: string; /** 1: every user of the organization (user.admin) */ }): Promise<{
    grants?: MCPGrant[];
  }> {
    return this.request<{
    grants?: MCPGrant[];
  }>('GET', `/api/v1/mcp/grants`, query, undefined, false);
  }

  /** Disconnect an AI assistant
   *
   * Ends the approval and every token issued under it. The owner, or user.admin for anyone in the organization.
   * 
   * Requires a signed-in user or an API token (any permission). */
  deleteMcpGrantsId(id: string, query: { reason: string; /** reason for the audit trail */ }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/mcp/grants/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** AI access of the organization
   *
   * Whether AI assistants may connect (off by default), the MCP address to add in the assistant, and the number of tools and operations offered.
   * 
   * Requires a signed-in user or an API token (any permission). */
  getMcpSettings(): Promise<MCPSettings> {
    return this.request<MCPSettings>('GET', `/api/v1/mcp/settings`, undefined, undefined, false);
  }

  /** Switch AI access on or off
   *
   * Audited with the reason. Switching off stops every connected assistant at once (their tokens are refused); switching on again lets them continue.
   * 
   * Requires permission `user.admin`. */
  putMcpSettings(body: {
    enabled: boolean;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<MCPSettings> {
    return this.request<MCPSettings>('PUT', `/api/v1/mcp/settings`, undefined, body, false);
  }

  /** MQTT accounts and bridges
   *
   * Requires permission `device.manage`. */
  getMqttSources(): Promise<({
    id?: string;
    kind?: "account" | "bridge";
    name?: string;
    /** devices waiting in the inbox */
    pending?: number;
    /** host, port, tls, username, client_id, discovery_prefix, adopt (auto|inbox), site_id, namespace; never passwords */
    settings?: Record<string, unknown>;
    status?: {
      connected?: boolean;
      entities?: number;
      error?: string;
    };
  })[]> {
    return this.request<({
    id?: string;
    kind?: "account" | "bridge";
    name?: string;
    /** devices waiting in the inbox */
    pending?: number;
    /** host, port, tls, username, client_id, discovery_prefix, adopt (auto|inbox), site_id, namespace; never passwords */
    settings?: Record<string, unknown>;
    status?: {
      connected?: boolean;
      entities?: number;
      error?: string;
    };
  })[]>('GET', `/api/v1/mqtt-sources`, undefined, undefined, false);
  }

  /** Create an MQTT account or a bridge
   *
   * An account gets a generated user name and password (returned once) and its own topic namespace: its devices use any topics (zigbee2mqtt/…, cmnd/…, homeassistant/…) without seeing other accounts. A bridge connects ctrl32 to a foreign broker.
   * 
   * Requires permission `device.manage`. */
  postMqttSources(body: {
    adopt?: "auto" | "inbox";
    /** bridge */
    client_id?: string;
    /** default homeassistant */
    discovery_prefix?: string;
    /** bridge */
    host?: string;
    /** bridge, self-signed brokers */
    insecure_skip_verify?: boolean;
    kind?: "account" | "bridge";
    name?: string;
    /** minutes disconnected before an incident opens (default 10, 0 = never; accounts only once they have entities) */
    outage_incident_min?: number;
    /** bridge; stored encrypted */
    password?: string;
    /** bridge */
    port?: number;
    publish_datastreams?: string[];
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    site_id?: string;
    /** bridge */
    tls?: boolean;
    /** bridge */
    username?: string;
  }): Promise<{
    id?: string;
    kind?: "account" | "bridge";
    name?: string;
    /** devices waiting in the inbox */
    pending?: number;
    /** host, port, tls, username, client_id, discovery_prefix, adopt (auto|inbox), site_id, namespace; never passwords */
    settings?: Record<string, unknown>;
    status?: {
      connected?: boolean;
      entities?: number;
      error?: string;
    };
  }> {
    return this.request<{
    id?: string;
    kind?: "account" | "bridge";
    name?: string;
    /** devices waiting in the inbox */
    pending?: number;
    /** host, port, tls, username, client_id, discovery_prefix, adopt (auto|inbox), site_id, namespace; never passwords */
    settings?: Record<string, unknown>;
    status?: {
      connected?: boolean;
      entities?: number;
      error?: string;
    };
  }>('POST', `/api/v1/mqtt-sources`, undefined, body, false);
  }

  /** Change an MQTT source
   *
   * Requires permission `device.manage`. */
  putMqttSourcesId(id: string, body: {
    adopt?: "auto" | "inbox";
    /** bridge */
    client_id?: string;
    /** default homeassistant */
    discovery_prefix?: string;
    /** bridge */
    host?: string;
    /** bridge, self-signed brokers */
    insecure_skip_verify?: boolean;
    kind?: "account" | "bridge";
    name?: string;
    /** minutes disconnected before an incident opens (default 10, 0 = never; accounts only once they have entities) */
    outage_incident_min?: number;
    /** bridge; stored encrypted */
    password?: string;
    /** bridge */
    port?: number;
    publish_datastreams?: string[];
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    site_id?: string;
    /** bridge */
    tls?: boolean;
    /** bridge */
    username?: string;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('PUT', `/api/v1/mqtt-sources/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Delete an MQTT source
   *
   * Its entities are marked removed; their history stays.
   * 
   * Requires permission `device.manage`. */
  deleteMqttSourcesId(id: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/mqtt-sources/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** New password for an MQTT account
   *
   * Connected clients of the account are disconnected.
   * 
   * Requires permission `device.manage`. */
  postMqttSourcesIdPassword(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    id?: string;
    /** shown once */
    password?: string;
    username?: string;
  }> {
    return this.request<{
    id?: string;
    /** shown once */
    password?: string;
    username?: string;
  }>('POST', `/api/v1/mqtt-sources/${encodeURIComponent(id)}/password`, undefined, body, false);
  }

  /** List channels
   *
   * Requires permission `config.write`. */
  getNotificationChannels(): Promise<Channel[]> {
    return this.request<Channel[]>('GET', `/api/v1/notification-channels`, undefined, undefined, false);
  }

  /** Create a channel
   *
   * Requires permission `config.write`. */
  postNotificationChannels(body: {
    kind: "email" | "webhook" | "sms_http" | "voice_http" | "web_push";
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    settings: ChannelSettings;
  }): Promise<Channel> {
    return this.request<Channel>('POST', `/api/v1/notification-channels`, undefined, body, false);
  }

  /** Update a channel
   *
   * A blank webhook secret keeps the stored one.
   * 
   * Requires permission `config.write`. */
  putNotificationChannelsId(id: string, body: {
    enabled?: boolean;
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    settings?: ChannelSettings;
  }): Promise<Channel> {
    return this.request<Channel>('PUT', `/api/v1/notification-channels/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Revoke a channel
   *
   * Requires permission `config.write`. */
  deleteNotificationChannelsId(id: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/notification-channels/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Send a test message
   *
   * Logged in the delivery log like any notification.
   * 
   * Requires permission `config.write`. */
  postNotificationChannelsIdTest(id: string, body: Record<string, unknown>): Promise<{
    channel?: string;
    kind?: string;
    ok?: boolean;
  }> {
    return this.request<{
    channel?: string;
    kind?: string;
    ok?: boolean;
  }>('POST', `/api/v1/notification-channels/${encodeURIComponent(id)}/test`, undefined, body, false);
  }

  /** Delivery log, newest first
   *
   * Requires permission `config.write`. */
  getNotifications(query?: { limit?: number; /** default 50, max 500 */ }): Promise<Notification[]> {
    return this.request<Notification[]>('GET', `/api/v1/notifications`, query, undefined, false);
  }

  /** Observations of a datastream
   *
   * Raw rows ordered by time, or aggregated per bucket (`bucket` = seconds: avg/min/max/count per bucket, at most 20000 buckets). Raw pages are bounded by `limit` (default 1000, max 10000) and 366 days per request; when more rows exist the response carries the header `X-Next-Cursor` (and a `Link: <…>; rel="next"`), pass it as `cursor` to continue.
   * 
   * Requires permission `data.read`. */
  getObservations(query: { datastream_id: string; from?: string; /** RFC 3339 (default: 24 hours ago) */ to?: string; /** RFC 3339 (default: now) */ bucket?: number; /** aggregation bucket in seconds; omitted = raw rows */ limit?: number; /** max rows per page */ cursor?: string; /** X-Next-Cursor of the previous page */ }): Promise<Observation[] | ObservationBuckets> {
    return this.request<Observation[] | ObservationBuckets>('GET', `/api/v1/observations`, query, undefined, false);
  }

  /** Connection details for devices
   *
   * Requires permission `device.manage`. */
  getOnboarding(): Promise<{
    base_url?: string;
    mqtt?: {
      host?: string;
      port?: number;
      tls_port?: number;
      ws_port?: number;
    };
    provision_url?: string;
  }> {
    return this.request<{
    base_url?: string;
    mqtt?: {
      host?: string;
      port?: number;
      tls_port?: number;
      ws_port?: number;
    };
    provision_url?: string;
  }>('GET', `/api/v1/onboarding`, undefined, undefined, false);
  }

  /** OpenAPI 3.1 document
   *
   * This description. The reference page at `/api/docs` renders it. */
  getOpenapiJson(): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/openapi.json`, undefined, undefined, false);
  }

  /** List organizations
   *
   * Requires permission `system.admin`. */
  getOrganizations(): Promise<Organization[]> {
    return this.request<Organization[]>('GET', `/api/v1/organizations`, undefined, undefined, false);
  }

  /** Create an organization with its first administrator
   *
   * Requires permission `system.admin`. */
  postOrganizations(body: {
    admin_email?: string;
    admin_full_name?: string;
    admin_username: string;
    locale?: string;
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** URL-safe identifier */
    slug: string;
    /** IANA time zone */
    tz?: string;
  }): Promise<{
    admin?: {
      /** shown once */
      initial_password?: string;
      username?: string;
    };
    organization?: Organization;
  }> {
    return this.request<{
    admin?: {
      /** shown once */
      initial_password?: string;
      username?: string;
    };
    organization?: Organization;
  }>('POST', `/api/v1/organizations`, undefined, body, false);
  }

  /** Update or disable an organization
   *
   * Requires permission `system.admin`. */
  patchOrganizationsId(id: string, body: {
    disabled?: boolean;
    locale?: string;
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    tz?: string;
  }): Promise<Organization> {
    return this.request<Organization>('PATCH', `/api/v1/organizations/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Reset the organization administrator's password
   *
   * Requires permission `system.admin`. */
  postOrganizationsIdResetAdmin(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** shown once */
    initial_password?: string;
    username?: string;
  }> {
    return this.request<{
    /** shown once */
    initial_password?: string;
    username?: string;
  }>('POST', `/api/v1/organizations/${encodeURIComponent(id)}/reset-admin`, undefined, body, false);
  }

  /** Values per period (daily log, monthly consumption)
   *
   * Aggregates of datastreams per local hour, day, week, month or year in the organization's time zone: minimum, maximum, average, first and last reading, consumption of a counter (sum of increases, a meter reset counts the new reading, the last reading before the period is the baseline), sample count and samples outside the limits. Every period of the range has a row, empty when there is no data. Used by the dashboard widget *Period table* (spec 023).
   * 
   * Requires permission `data.read`. */
  getPeriodTable(query: { datastreams: string; /** comma-separated datastream ids (1–50) */ interval?: string; /** hour | day (default) | week | month | year — local periods of the organization's time zone */ values?: string; /** comma-separated: min, max, avg, first, last, delta (consumption of a counter), sum (total of the values, e.g. people counted), count, out (samples outside low/high); default min,max */ period?: string; /** today | yesterday | this_week | last_week | this_month (default) | last_month | this_year | last_year | last_7d | last_30d | last_12m | custom */ from?: string; /** custom: first day YYYY-MM-DD */ to?: string; /** custom: last day YYYY-MM-DD (included) */ shift?: number; /** move the period by whole periods, e.g. -1 = the one before */ low?: string; /** lower limit (out, flagging) */ high?: string; /** upper limit */ orientation?: string; /** periods (a row per period, default) | datastreams */ total?: string; /** 1: a total row */ decimals?: number; /** 0–6 (exports) */ label?: string; /** display name of a datastream, repeated in the order of datastreams */ title?: string; /** title of an export */ }): Promise<PeriodTable> {
    return this.request<PeriodTable>('GET', `/api/v1/period-table`, query, undefined, false);
  }

  /** Export a period table
   *
   * The same table as `GET /api/v1/period-table` as `format` xlsx (default; title, period, organization and author above the table), csv (UTF-8 with BOM) or pdf (stored under Reports with hashes, like other PDF exports). Audited.
   * 
   * Requires permission `data.export`. */
  getPeriodTableExport(query: { datastreams: string; /** comma-separated datastream ids (1–50) */ interval?: string; /** hour | day (default) | week | month | year — local periods of the organization's time zone */ values?: string; /** comma-separated: min, max, avg, first, last, delta (consumption of a counter), sum (total of the values, e.g. people counted), count, out (samples outside low/high); default min,max */ period?: string; /** today | yesterday | this_week | last_week | this_month (default) | last_month | this_year | last_year | last_7d | last_30d | last_12m | custom */ from?: string; /** custom: first day YYYY-MM-DD */ to?: string; /** custom: last day YYYY-MM-DD (included) */ shift?: number; /** move the period by whole periods, e.g. -1 = the one before */ low?: string; /** lower limit (out, flagging) */ high?: string; /** upper limit */ orientation?: string; /** periods (a row per period, default) | datastreams */ total?: string; /** 1: a total row */ decimals?: number; /** 0–6 (exports) */ label?: string; /** display name of a datastream, repeated in the order of datastreams */ title?: string; /** title of an export */ format?: string; /** xlsx | csv | pdf */ }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/period-table/export`, query, undefined, true);
  }

  /** Permission catalog
   *
   * Requires a signed-in user or an API token (any permission). */
  getPermissions(): Promise<Permission[]> {
    return this.request<Permission[]>('GET', `/api/v1/permissions`, undefined, undefined, false);
  }

  /** Device self-provisioning with a claim code or a provisioning key
   *
   * Either the claim code from device registration (response: token and MQTT connection details), or ThingsBoard-compatible `{deviceName, provisionDeviceKey, provisionDeviceSecret}` of a provisioning profile (response: `{status, credentialsType: ACCESS_TOKEN, credentialsValue}`; the same works over MQTT: user name `provision`, publish `/provision/request`, answer on `/provision/response`). Rate limited per IP and key. */
  postProvision(body: {
    /** claim code */
    claim?: string;
    /** provisioning profile variant */
    deviceName?: string;
    /** firmware version */
    fw?: string;
    /** hardware id */
    hw?: string;
    provisionDeviceKey?: string;
    provisionDeviceSecret?: string;
  }): Promise<{
    device_id?: string;
    external_id?: string;
    mqtt?: {
      host?: string;
      port?: number;
      tls?: boolean;
    };
    protocol?: string;
    /** device token, shown once */
    token?: string;
  }> {
    return this.request<{
    device_id?: string;
    external_id?: string;
    mqtt?: {
      host?: string;
      port?: number;
      tls?: boolean;
    };
    protocol?: string;
    /** device token, shown once */
    token?: string;
  }>('POST', `/api/v1/provision`, undefined, body, false);
  }

  /** Provisioning profiles
   *
   * Requires permission `device.manage`. */
  getProvisionProfiles(): Promise<({
    /** created devices wait for approval */
    approval?: boolean;
    auto_datastreams?: boolean;
    devices?: number;
    id?: string;
    /** pk_…, identifies the profile */
    key?: string;
    /** 0 = unlimited */
    max_devices?: number;
    name?: string;
    /** regular expression */
    name_pattern?: string;
    room_id?: string;
    site_id?: string;
    strategy?: "create" | "preprovisioned";
    transport?: "mqtt" | "http";
  })[]> {
    return this.request<({
    /** created devices wait for approval */
    approval?: boolean;
    auto_datastreams?: boolean;
    devices?: number;
    id?: string;
    /** pk_…, identifies the profile */
    key?: string;
    /** 0 = unlimited */
    max_devices?: number;
    name?: string;
    /** regular expression */
    name_pattern?: string;
    room_id?: string;
    site_id?: string;
    strategy?: "create" | "preprovisioned";
    transport?: "mqtt" | "http";
  })[]>('GET', `/api/v1/provision-profiles`, undefined, undefined, false);
  }

  /** Create a provisioning profile
   *
   * The response carries the `secret` once; devices use key and secret to register themselves.
   * 
   * Requires permission `device.manage`. */
  postProvisionProfiles(body: {
    approval?: boolean;
    auto_datastreams?: boolean;
    max_devices?: number;
    name?: string;
    name_pattern?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    room_id?: string;
    site_id?: string;
    strategy?: "create" | "preprovisioned";
    transport?: "mqtt" | "http";
  }): Promise<{
    /** created devices wait for approval */
    approval?: boolean;
    auto_datastreams?: boolean;
    devices?: number;
    id?: string;
    /** pk_…, identifies the profile */
    key?: string;
    /** 0 = unlimited */
    max_devices?: number;
    name?: string;
    /** regular expression */
    name_pattern?: string;
    room_id?: string;
    site_id?: string;
    strategy?: "create" | "preprovisioned";
    transport?: "mqtt" | "http";
  }> {
    return this.request<{
    /** created devices wait for approval */
    approval?: boolean;
    auto_datastreams?: boolean;
    devices?: number;
    id?: string;
    /** pk_…, identifies the profile */
    key?: string;
    /** 0 = unlimited */
    max_devices?: number;
    name?: string;
    /** regular expression */
    name_pattern?: string;
    room_id?: string;
    site_id?: string;
    strategy?: "create" | "preprovisioned";
    transport?: "mqtt" | "http";
  }>('POST', `/api/v1/provision-profiles`, undefined, body, false);
  }

  /** Change a provisioning profile
   *
   * Requires permission `device.manage`. */
  putProvisionProfilesId(id: string, body: {
    approval?: boolean;
    auto_datastreams?: boolean;
    max_devices?: number;
    name?: string;
    name_pattern?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    room_id?: string;
    site_id?: string;
    strategy?: "create" | "preprovisioned";
    transport?: "mqtt" | "http";
  }): Promise<{
    /** created devices wait for approval */
    approval?: boolean;
    auto_datastreams?: boolean;
    devices?: number;
    id?: string;
    /** pk_…, identifies the profile */
    key?: string;
    /** 0 = unlimited */
    max_devices?: number;
    name?: string;
    /** regular expression */
    name_pattern?: string;
    room_id?: string;
    site_id?: string;
    strategy?: "create" | "preprovisioned";
    transport?: "mqtt" | "http";
  }> {
    return this.request<{
    /** created devices wait for approval */
    approval?: boolean;
    auto_datastreams?: boolean;
    devices?: number;
    id?: string;
    /** pk_…, identifies the profile */
    key?: string;
    /** 0 = unlimited */
    max_devices?: number;
    name?: string;
    /** regular expression */
    name_pattern?: string;
    room_id?: string;
    site_id?: string;
    strategy?: "create" | "preprovisioned";
    transport?: "mqtt" | "http";
  }>('PUT', `/api/v1/provision-profiles/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Revoke a provisioning profile
   *
   * Requires permission `device.manage`. */
  deleteProvisionProfilesId(id: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/provision-profiles/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** New secret for a profile
   *
   * Devices already provisioned keep their tokens.
   * 
   * Requires permission `device.manage`. */
  postProvisionProfilesIdRotate(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    id?: string;
    /** shown once */
    secret?: string;
  }> {
    return this.request<{
    id?: string;
    /** shown once */
    secret?: string;
  }>('POST', `/api/v1/provision-profiles/${encodeURIComponent(id)}/rotate`, undefined, body, false);
  }

  /** Shared dashboard */
  getPublicToken(token: string): Promise<{
    name?: string;
    slug?: string;
    spec?: DashboardSpec;
  }> {
    return this.request<{
    name?: string;
    slug?: string;
    spec?: DashboardSpec;
  }>('GET', `/api/v1/public/${encodeURIComponent(token)}`, undefined, undefined, false);
  }

  /** Alarms of the bound datastreams */
  getPublicTokenAlarms(token: string): Promise<Alarm[]> {
    return this.request<Alarm[]>('GET', `/api/v1/public/${encodeURIComponent(token)}/alarms`, undefined, undefined, false);
  }

  /** Datastreams bound in the shared dashboard */
  getPublicTokenDatastreams(token: string): Promise<Datastream[]> {
    return this.request<Datastream[]>('GET', `/api/v1/public/${encodeURIComponent(token)}/datastreams`, undefined, undefined, false);
  }

  /** Latest values (bound datastreams only) */
  getPublicTokenDatastreamsLatest(token: string, query: { ids: string; }): Promise<Record<string, LatestValue>> {
    return this.request<Record<string, LatestValue>>('GET', `/api/v1/public/${encodeURIComponent(token)}/datastreams/latest`, query, undefined, false);
  }

  /** Devices shown in the shared dashboard */
  getPublicTokenDevices(token: string): Promise<Device[]> {
    return this.request<Device[]>('GET', `/api/v1/public/${encodeURIComponent(token)}/devices`, undefined, undefined, false);
  }

  /** A library graphic shown by the shared picture
   *
   * Only graphics that the linked process picture references. */
  getPublicTokenHmiObjectsSlugSvg(token: string, slug: string): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/public/${encodeURIComponent(token)}/hmi/objects/${encodeURIComponent(slug)}/svg`, undefined, undefined, true);
  }

  /** Observations (bound datastreams only)
   *
   * Same parameters and cursor pagination as `GET /api/v1/observations`. */
  getPublicTokenObservations(token: string, query: { datastream_id: string; from?: string; to?: string; bucket?: number; /** seconds */ limit?: number; cursor?: string; }): Promise<Observation[] | ObservationBuckets> {
    return this.request<Observation[] | ObservationBuckets>('GET', `/api/v1/public/${encodeURIComponent(token)}/observations`, query, undefined, false);
  }

  /** Period table (bound datastreams only)
   *
   * Same parameters and answer as `GET /api/v1/period-table`, only for datastreams bound in the shared dashboard. */
  getPublicTokenPeriodTable(token: string): Promise<PeriodTable> {
    return this.request<PeriodTable>('GET', `/api/v1/public/${encodeURIComponent(token)}/period-table`, undefined, undefined, false);
  }

  /** Live stream of the shared dashboard (SSE) */
  getPublicTokenStream(token: string): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/public/${encodeURIComponent(token)}/stream`, undefined, undefined, true);
  }

  /** VAPID public key for Web Push
   *
   * Pass it as `applicationServerKey` to `PushManager.subscribe` in the browser.
   * 
   * Requires a signed-in user or an API token (any permission). */
  getPushKey(): Promise<{
    /** base64url */
    public_key?: string;
  }> {
    return this.request<{
    /** base64url */
    public_key?: string;
  }>('GET', `/api/v1/push/key`, undefined, undefined, false);
  }

  /** My browsers subscribed to notifications
   *
   * Keys are never returned; subscriptions revoked in the last 7 days are listed with `revoked_at`.
   * 
   * Requires a signed-in user or an API token (any permission). */
  getPushSubscriptions(): Promise<PushSubscription[]> {
    return this.request<PushSubscription[]>('GET', `/api/v1/push/subscriptions`, undefined, undefined, false);
  }

  /** Register this browser for Web Push
   *
   * The JSON of the browser's `PushSubscription` (`endpoint`, `keys.p256dh`, `keys.auth`). Registering the same endpoint again refreshes the keys (200). Not allowed with an API token.
   * 
   * Requires a signed-in user or an API token (any permission). */
  postPushSubscriptions(body: {
    /** push service URL */
    endpoint: string;
    /** ignored */
    expirationTime?: unknown | null;
    keys: {
      /** base64url */
      auth: string;
      /** base64url */
      p256dh: string;
    };
    /** optional label of the browser */
    user_agent?: string;
  }): Promise<{
    created?: boolean;
    id?: string;
  }> {
    return this.request<{
    created?: boolean;
    id?: string;
  }>('POST', `/api/v1/push/subscriptions`, undefined, body, false);
  }

  /** Remove one of my browser subscriptions
   *
   * Requires a signed-in user or an API token (any permission). */
  deletePushSubscriptionsId(id: string): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/push/subscriptions/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Send a test notification to my browsers
   *
   * Requires a signed-in user or an API token (any permission). */
  postPushTest(body: Record<string, unknown>): Promise<{
    failed?: number;
    sent?: number;
  }> {
    return this.request<{
    failed?: number;
    sent?: number;
  }>('POST', `/api/v1/push/test`, undefined, body, false);
  }

  /** List app profiles
   *
   * Current version of every app of the organization, enabled or not.
   * 
   * Requires permission `content.write`. */
  getPwaApps(): Promise<PWAApp[]> {
    return this.request<PWAApp[]>('GET', `/api/v1/pwa-apps`, undefined, undefined, false);
  }

  /** Create an app profile
   *
   * `start_url` must be a path of this application (`/video`, `/video/walls/{id}`, `/dashboards/{slug}` …): no other site, no `//`, no `javascript:`, not `/api/v1/` or `/app/`. `sections`: keys from `GET /api/v1/pwa-apps/options`, at least one. `warnings` names a short name longer than 12 characters, and the start page of a locked app outside its sections. `locked`, `page_zoom`, `camera_layout` (auto, single, grid): see the PWAApp schema.
   * 
   * Requires permission `content.write`. */
  postPwaApps(body: PWAAppInput): Promise<PWAApp> {
    return this.request<PWAApp>('POST', `/api/v1/pwa-apps`, undefined, body, false);
  }

  /** Menu sections and start pages to choose from
   *
   * Sections are the top-level menu groups; start pages are the main pages plus the organization's dashboards, video walls and cameras (cameras only with `video.view`, limited to the user's camera groups).
   * 
   * Requires permission `content.write`. */
  getPwaAppsOptions(): Promise<{
    sections?: ({
      key?: string;
      label?: string;
      url?: string;
    })[];
    /** recommended maximum length of short_name */
    short_name_hint?: number;
    start_pages?: ({
      group?: string;
      label?: string;
      url?: string;
    })[];
  }> {
    return this.request<{
    sections?: ({
      key?: string;
      label?: string;
      url?: string;
    })[];
    /** recommended maximum length of short_name */
    short_name_hint?: number;
    start_pages?: ({
      group?: string;
      label?: string;
      url?: string;
    })[];
  }>('GET', `/api/v1/pwa-apps/options`, undefined, undefined, false);
  }

  /** One app profile
   *
   * Requires permission `content.write`. */
  getPwaAppsSlug(slug: string): Promise<PWAApp> {
    return this.request<PWAApp>('GET', `/api/v1/pwa-apps/${encodeURIComponent(slug)}`, undefined, undefined, false);
  }

  /** Save a new version of an app profile
   *
   * The slug stays; `enabled` omitted keeps the current state.
   * 
   * Requires permission `content.write`. */
  putPwaAppsSlug(slug: string, body: PWAAppInput): Promise<PWAApp> {
    return this.request<PWAApp>('PUT', `/api/v1/pwa-apps/${encodeURIComponent(slug)}`, undefined, body, false);
  }

  /** Delete an app profile
   *
   * The profile is retired (versions stay); its install page and manifest answer 404.
   * 
   * Requires permission `content.write`. */
  deletePwaAppsSlug(slug: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/pwa-apps/${encodeURIComponent(slug)}`, query, undefined, false);
  }

  /** Name, start page, sections and options of an enabled app
   *
   * Public (the app window needs it before sign-in); contains no data of the organization. 404 for an unknown or disabled app. A locked app keeps the window on its sections (app.js redirects other pages to the start page and offers no way into the full application); this is UI confinement, permissions stay the security boundary. */
  getPwaAppsSlugLaunch(slug: string): Promise<{
    camera_layout?: "auto" | "single" | "grid";
    locked?: boolean;
    name?: string;
    page_zoom?: boolean;
    sections?: string[];
    short_name?: string;
    slug?: string;
    start_url?: string;
    theme_color?: string;
  }> {
    return this.request<{
    camera_layout?: "auto" | "single" | "grid";
    locked?: boolean;
    name?: string;
    page_zoom?: boolean;
    sections?: string[];
    short_name?: string;
    slug?: string;
    start_url?: string;
    theme_color?: string;
  }>('GET', `/api/v1/pwa-apps/${encodeURIComponent(slug)}/launch`, undefined, undefined, false);
  }

  /** Register profiles (current versions)
   *
   * Versioned Modbus register maps shared by connectors of the same device type, with the number of connectors using any version and how many are on an older version.
   * 
   * Requires permission `config.write`. */
  getRegisterProfiles(): Promise<RegisterProfile[]> {
    return this.request<RegisterProfile[]>('GET', `/api/v1/register-profiles`, undefined, undefined, false);
  }

  /** Create a register profile
   *
   * Requires permission `config.write`. */
  postRegisterProfiles(body: {
    description?: string;
    /** unique among active profiles */
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    spec: RegisterProfileSpec;
  }): Promise<{
    id?: string;
    name?: string;
    registers?: number;
    version?: number;
  }> {
    return this.request<{
    id?: string;
    name?: string;
    registers?: number;
    version?: number;
  }>('POST', `/api/v1/register-profiles`, undefined, body, false);
  }

  /** One profile version with the history of its lineage
   *
   * Requires permission `config.write`. */
  getRegisterProfilesId(id: string): Promise<RegisterProfileDetail> {
    return this.request<RegisterProfileDetail>('GET', `/api/v1/register-profiles/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** New version of a profile
   *
   * The current version is closed (`valid_to`) and a new row with the next version number is created; connectors keep their version until it is applied.
   * 
   * Requires permission `config.write`. */
  putRegisterProfilesId(id: string, body: {
    /** what changed */
    changelog?: string;
    description?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    spec: RegisterProfileSpec;
  }): Promise<{
    /** new version id */
    id?: string;
    name?: string;
    previous_id?: string;
    registers?: number;
    version?: number;
  }> {
    return this.request<{
    /** new version id */
    id?: string;
    name?: string;
    previous_id?: string;
    registers?: number;
    version?: number;
  }>('PUT', `/api/v1/register-profiles/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Retire a profile
   *
   * Refused (409) while connectors use any version of it.
   * 
   * Requires permission `config.write`. */
  deleteRegisterProfilesId(id: string, query: { reason: string; }): Promise<{
    id?: string;
    retired?: boolean;
  }> {
    return this.request<{
    id?: string;
    retired?: boolean;
  }>('DELETE', `/api/v1/register-profiles/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Apply this version to the connectors on older versions
   *
   * Each connector's registers and byte order are replaced from the profile (audited as connector.update) and its poller restarts.
   * 
   * Requires permission `config.write`. */
  postRegisterProfilesIdApply(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    applied?: number;
    connectors?: ({
      id?: string;
      name?: string;
    })[];
    id?: string;
    version?: number;
  }> {
    return this.request<{
    applied?: number;
    connectors?: ({
      id?: string;
      name?: string;
    })[];
    id?: string;
    version?: number;
  }>('POST', `/api/v1/register-profiles/${encodeURIComponent(id)}/apply`, undefined, body, false);
  }

  /** This node and its replicas
   *
   * Node identity and role (primary, replica, fenced) and every replica with mode, state, lag, copy progress, limits and detachment record.
   * 
   * Requires permission `system.admin`. */
  getReplication(): Promise<{
    max_replicas?: number;
    modes?: string[];
    node?: ReplicationNode;
    pending?: (Record<string, unknown>)[];
    replicas?: Replica[];
  }> {
    return this.request<{
    max_replicas?: number;
    modes?: string[];
    node?: ReplicationNode;
    pending?: (Record<string, unknown>)[];
    replicas?: Replica[];
  }>('GET', `/api/v1/replication`, undefined, undefined, false);
  }

  /** Pending command for the replica (long poll)
   *
   * Requires permission `replica token`. */
  getReplicationAgent(): Promise<{
    /** detach command: lsn, seq, hash, snapshot, counts, partial */
    command?: Record<string, unknown> | null;
  }> {
    return this.request<{
    /** detach command: lsn, seq, hash, snapshot, counts, partial */
    command?: Record<string, unknown> | null;
  }>('GET', `/api/v1/replication/agent`, undefined, undefined, false);
  }

  /** Report from the replica
   *
   * Requires permission `replica token`. */
  postReplicationAgent(body: {
    /** detach result: promoted, chain_ok, entry_ok, counts, errors */
    result?: Record<string, unknown>;
    /** replica's own view */
    status?: Record<string, unknown>;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/replication/agent`, undefined, body, false);
  }

  /** Pair a replica with a one-time code
   *
   * Used by `ctrl32-telemetry replica attach`. Rate-limited; a wrong or expired code is a security event (403). Versions of PostgreSQL (major) and ctrl32 must match (400). */
  postReplicationPair(body: {
    code: string;
    ctrl32_version: string;
    hostname?: string;
    name?: string;
    os?: string;
    pg_major: number;
    standby_url?: string;
  }): Promise<{
    mode?: string;
    name?: string;
    /** its password (shown once) */
    password?: string;
    pg_major?: number;
    primary_node?: string;
    replica_id?: string;
    /** replication role */
    role?: string;
    slot?: string;
    /** bearer token of the replica (shown once) */
    token?: string;
  }> {
    return this.request<{
    mode?: string;
    name?: string;
    /** its password (shown once) */
    password?: string;
    pg_major?: number;
    primary_node?: string;
    replica_id?: string;
    /** replication role */
    role?: string;
    slot?: string;
    /** bearer token of the replica (shown once) */
    token?: string;
  }>('POST', `/api/v1/replication/pair`, undefined, body, false);
  }

  /** Create a one-time pairing code
   *
   * The code (XXXX-XXXX-XXXX) is shown once and is valid 15 minutes; on the second machine run `ctrl32-telemetry replica attach -primary <this URL> -code <code>`.
   * 
   * Requires permission `system.admin`. */
  postReplicationPairings(body: {
    /** alarm when the replica is further behind (default 10) */
    lag_limit_s?: number;
    mode: "copy" | "continuous" | "standby";
    /** ≤ 64 */
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    code?: string;
    expires_at?: string;
    mode?: string;
    name?: string;
  }> {
    return this.request<{
    code?: string;
    expires_at?: string;
    mode?: string;
    name?: string;
  }>('POST', `/api/v1/replication/pairings`, undefined, body, false);
  }

  /** Change a replica's mode or limits
   *
   * Requires permission `system.admin`. */
  patchReplicationReplicasId(id: string, body: {
    lag_limit_s?: number;
    /** maximum WAL the primary keeps for it (64 MB – 10 TB) */
    max_retained_mb?: number;
    mode?: "copy" | "continuous" | "standby";
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** hot standby: its public URL */
    standby_url?: string;
  }): Promise<Replica> {
    return this.request<Replica>('PATCH', `/api/v1/replication/replicas/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Drop a replica
   *
   * Revokes its token and removes its replication role, slot and pg_hba rule; the copy stays as it is and is no longer fed.
   * 
   * Requires permission `system.admin`. */
  deleteReplicationReplicasId(id: string, query: { reason: string; }): Promise<Replica> {
    return this.request<Replica>('DELETE', `/api/v1/replication/replicas/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Detach a replica safely
   *
   * Counts the record tables in one snapshot, writes the `replica.detach` audit entry, then the replica catches up to it, becomes independent and verifies the audit chain and the counts; the result is in `detach.verification`. Refused (409) when not in sync unless `accept_partial`.
   * 
   * Requires permission `system.admin`. */
  postReplicationReplicasIdDetach(id: string, body: {
    /** detach a replica that is not in sync; the copy is marked partial */
    accept_partial?: boolean;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<Replica> {
    return this.request<Replica>('POST', `/api/v1/replication/replicas/${encodeURIComponent(id)}/detach`, undefined, body, false);
  }

  /** History of a replica
   *
   * Requires permission `system.admin`. */
  getReplicationReplicasIdEvents(id: string, query?: { limit?: number; /** default 100 */ }): Promise<({
    detail?: Record<string, unknown>;
    /** state, alarm, command, detach, promote, security, config */
    kind?: string;
    ts?: string;
  })[]> {
    return this.request<({
    detail?: Record<string, unknown>;
    /** state, alarm, command, detach, promote, security, config */
    kind?: string;
    ts?: string;
  })[]>('GET', `/api/v1/replication/replicas/${encodeURIComponent(id)}/events`, query, undefined, false);
  }

  /** Who this node is
   *
   * No authentication: a returning primary asks its hot standby whether it was promoted (fencing). */
  getReplicationRole(): Promise<ReplicationNode> {
    return this.request<ReplicationNode>('GET', `/api/v1/replication/role`, undefined, undefined, false);
  }

  /** Replication tunnel (HTTP upgrade)
   *
   * With `Connection: Upgrade`, `Upgrade: ctrl32-pgrepl` and the replica token the connection becomes a byte stream to the primary's PostgreSQL (101 Switching Protocols).
   * 
   * Requires permission `replica token`. */
  getReplicationTunnel(): Promise<unknown> {
    return this.request<unknown>('GET', `/api/v1/replication/tunnel`, undefined, undefined, false);
  }

  /** Report kinds, overrides and engine info
   *
   * Requires permission `data.export`. */
  getReportTemplates(): Promise<{
    builtin_kinds?: string[];
    engine?: string;
    engine_version?: string;
    max_rows?: number;
    overrides?: (Record<string, unknown>)[];
  }> {
    return this.request<{
    builtin_kinds?: string[];
    engine?: string;
    engine_version?: string;
    max_rows?: number;
    overrides?: (Record<string, unknown>)[];
  }>('GET', `/api/v1/report-templates`, undefined, undefined, false);
  }

  /** Compile a block spec to Typst source
   *
   * Requires permission `content.write`. */
  postReportTemplatesCompile(body: {
    spec: Record<string, unknown>;
  }): Promise<{
    /** Typst source */
    body?: string;
    spec?: Record<string, unknown>;
  }> {
    return this.request<{
    /** Typst source */
    body?: string;
    spec?: Record<string, unknown>;
  }>('POST', `/api/v1/report-templates/compile`, undefined, body, false);
  }

  /** Render Typst source with sample data (PDF)
   *
   * Requires permission `content.write`. */
  postReportTemplatesPreview(body: {
    /** Typst source */
    body: string;
    kind: string;
    locale?: string;
  }): Promise<Response> {
    return this.request<Response>('POST', `/api/v1/report-templates/preview`, undefined, body, true);
  }

  /** Template of a report kind
   *
   * The effective template (organization override or built-in) as Typst source and, when designed with blocks, its block spec.
   * 
   * Requires permission `data.export`. */
  getReportTemplatesKind(kind: string, query?: { locale?: string; }): Promise<{
    blocks?: (Record<string, unknown>)[];
    layout?: string;
    template?: Record<string, unknown>;
  }> {
    return this.request<{
    blocks?: (Record<string, unknown>)[];
    layout?: string;
    template?: Record<string, unknown>;
  }>('GET', `/api/v1/report-templates/${encodeURIComponent(kind)}`, query, undefined, false);
  }

  /** Override a report template
   *
   * Either a block `spec` (compiled to Typst on the server) or a full Typst `body`.
   * 
   * Requires permission `content.write`. */
  putReportTemplatesKind(kind: string, body: {
    /** Typst source */
    body?: string;
    locale?: string;
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** block designer spec */
    spec?: Record<string, unknown>;
  }): Promise<{
    checksum?: string;
    id?: string;
    kind?: string;
    locale?: string;
    name?: string;
    version?: number;
  }> {
    return this.request<{
    checksum?: string;
    id?: string;
    kind?: string;
    locale?: string;
    name?: string;
    version?: number;
  }>('PUT', `/api/v1/report-templates/${encodeURIComponent(kind)}`, undefined, body, false);
  }

  /** Revert to the built-in template
   *
   * Requires permission `content.write`. */
  deleteReportTemplatesKind(kind: string, query: { locale?: string; reason: string; }): Promise<{
    kind?: string;
    locale?: string;
    reverted?: boolean;
  }> {
    return this.request<{
    kind?: string;
    locale?: string;
    reverted?: boolean;
  }>('DELETE', `/api/v1/report-templates/${encodeURIComponent(kind)}`, query, undefined, false);
  }

  /** Data contract of a report kind
   *
   * The variables and tables a template of this kind receives.
   * 
   * Requires permission `data.export`. */
  getReportTemplatesKindContract(kind: string): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/report-templates/${encodeURIComponent(kind)}/contract`, undefined, undefined, false);
  }

  /** Default block spec of a report kind
   *
   * Requires permission `data.export`. */
  getReportTemplatesKindDefaultSpec(kind: string): Promise<{
    blocks?: (Record<string, unknown>)[];
    spec?: Record<string, unknown>;
  }> {
    return this.request<{
    blocks?: (Record<string, unknown>)[];
    spec?: Record<string, unknown>;
  }>('GET', `/api/v1/report-templates/${encodeURIComponent(kind)}/default-spec`, undefined, undefined, false);
  }

  /** Render the effective template with sample data (PDF)
   *
   * Requires permission `data.export`. */
  getReportTemplatesKindPreview(kind: string, query?: { locale?: string; }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/report-templates/${encodeURIComponent(kind)}/preview`, query, undefined, true);
  }

  /** Stored reports, newest first
   *
   * Requires permission `data.export`. */
  getReports(query?: { limit?: number; }): Promise<Report[]> {
    return this.request<Report[]>('GET', `/api/v1/reports`, query, undefined, false);
  }

  /** Report metadata
   *
   * Requires permission `data.export`. */
  getReportsId(id: string): Promise<Report> {
    return this.request<Report>('GET', `/api/v1/reports/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Download the report PDF
   *
   * Requires permission `data.export`. */
  getReportsIdFile(id: string): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/reports/${encodeURIComponent(id)}/file`, undefined, undefined, true);
  }

  /** Roles of the organization
   *
   * Built-in roles (immutable) and organization roles with their permission sets and member counts.
   * 
   * Requires a signed-in user or an API token (any permission). */
  getRoles(): Promise<Role[]> {
    return this.request<Role[]>('GET', `/api/v1/roles`, undefined, undefined, false);
  }

  /** Create an organization role
   *
   * Requires permission `user.admin`. */
  postRoles(body: {
    name: string;
    permissions: string[];
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<Role> {
    return this.request<Role>('POST', `/api/v1/roles`, undefined, body, false);
  }

  /** Replace the permissions of an organization role
   *
   * Built-in roles cannot be changed.
   * 
   * Requires permission `user.admin`. */
  putRolesId(id: string, body: {
    permissions: string[];
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<Role> {
    return this.request<Role>('PUT', `/api/v1/roles/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Rule transitions with their action results
   *
   * Requires permission `config.write`. */
  getRuleRuns(query?: { rule_id?: string; limit?: number; /** default 50, max 500 */ }): Promise<RuleRun[]> {
    return this.request<RuleRun[]>('GET', `/api/v1/rule-runs`, query, undefined, false);
  }

  /** List automation rules
   *
   * With the live state of each loaded rule (`state.active` = condition currently true).
   * 
   * Requires permission `config.write`. */
  getRules(): Promise<Rule[]> {
    return this.request<Rule[]>('GET', `/api/v1/rules`, undefined, undefined, false);
  }

  /** Create an automation rule
   *
   * Rules that send commands need `device.command`; with the four-eyes policy they are stored as `pending` (202) until a different user approves them.
   * 
   * Requires permission `config.write`. */
  postRules(body: {
    description?: string;
    /** unique in the organization */
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    spec: RuleSpec;
  }): Promise<{
    id?: string;
    name?: string;
    status?: "active" | "pending";
    version?: number;
  }> {
    return this.request<{
    id?: string;
    name?: string;
    status?: "active" | "pending";
    version?: number;
  }>('POST', `/api/v1/rules`, undefined, body, false);
  }

  /** Dry run a rule spec
   *
   * Evaluates the condition against the current values; no action runs.
   * 
   * Requires permission `config.write`. */
  postRulesTest(body: {
    /** optional: include the live state of this rule */
    rule_id?: string;
    spec: RuleSpec;
  }): Promise<RuleEvaluation> {
    return this.request<RuleEvaluation>('POST', `/api/v1/rules/test`, undefined, body, false);
  }

  /** Update a rule (new version in place when the spec changes)
   *
   * Requires permission `config.write`. */
  putRulesId(id: string, body: {
    description?: string;
    enabled?: boolean;
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    spec?: RuleSpec;
  }): Promise<{
    enabled?: boolean;
    id?: string;
    name?: string;
    status?: string;
    version?: number;
  }> {
    return this.request<{
    enabled?: boolean;
    id?: string;
    name?: string;
    status?: string;
    version?: number;
  }>('PUT', `/api/v1/rules/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Revoke a rule
   *
   * Alarms raised by the rule are cleared.
   * 
   * Requires permission `config.write`. */
  deleteRulesId(id: string, query: { reason: string; }): Promise<{
    id?: string;
    revoked?: boolean;
  }> {
    return this.request<{
    id?: string;
    revoked?: boolean;
  }>('DELETE', `/api/v1/rules/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Approve a pending rule (four eyes)
   *
   * Requires permission `device.command`. */
  postRulesIdApprove(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    id?: string;
    status?: string;
  }> {
    return this.request<{
    id?: string;
    status?: string;
  }>('POST', `/api/v1/rules/${encodeURIComponent(id)}/approve`, undefined, body, false);
  }

  /** Convert a rule into a flow spec
   *
   * Returns an equivalent flow (datastream triggers → join → "true for" → actions) to create as a draft with `POST /api/v1/flows`; parts without a flow equivalent (staleness, webhook secrets, custom RPC arguments) are listed in `notes` and in a comment node. The rule itself is not changed.
   * 
   * Requires permission `config.write`. */
  getRulesIdAsFlow(id: string): Promise<{
    name?: string;
    notes?: string[];
    problems?: FlowProblem[];
    spec?: FlowSpec;
  }> {
    return this.request<{
    name?: string;
    notes?: string[];
    problems?: FlowProblem[];
    spec?: FlowSpec;
  }>('GET', `/api/v1/rules/${encodeURIComponent(id)}/as-flow`, undefined, undefined, false);
  }

  /** Reject a pending rule
   *
   * Requires permission `device.command`. */
  postRulesIdReject(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    id?: string;
    status?: string;
  }> {
    return this.request<{
    id?: string;
    status?: string;
  }>('POST', `/api/v1/rules/${encodeURIComponent(id)}/reject`, undefined, body, false);
  }

  /** Scenes
   *
   * Requires permission `data.read`. */
  getScenes(): Promise<Scene[]> {
    return this.request<Scene[]>('GET', `/api/v1/scenes`, undefined, undefined, false);
  }

  /** Create a scene
   *
   * Actions explicitly, or capture: entity ids whose current states (on/off, brightness, colour, position, temperature, mode, value) become the actions.
   * 
   * Requires permission `device.manage`. */
  postScenes(body: {
    actions?: SceneAction[];
    capture?: string[];
    event_kinds?: string[];
    /** 1–600 */
    event_post_s?: number;
    /** 0–60, record = events */
    event_pre_s?: number;
    /** camera group ≤ 60; users limited to groups see only their cameras (spec 014) */
    group?: string;
    /** icon name */
    icon?: string;
    /** ≤ 80 */
    name: string;
    /** http://camera/onvif/device_service: events and PTZ (spec 013); for source push the camera's address in its own network, reached through the relay (onvif = … in relay.toml) */
    onvif_url?: string;
    privacy_masks?: number[][][];
    /** empty = the first profile with PTZ */
    ptz_profile?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    room_id?: string;
    sort?: number;
    /** pull: the server connects to the camera; push: a relay at a remote site sends the video (secret returned once as push_secret with relay_config) */
    source?: "pull" | "push";
  }): Promise<Scene> {
    return this.request<Scene>('POST', `/api/v1/scenes`, undefined, body, false);
  }

  /** Change a scene
   *
   * Requires permission `device.manage`. */
  putScenesId(id: string, body: {
    actions?: SceneAction[];
    capture?: string[];
    event_kinds?: string[];
    /** 1–600 */
    event_post_s?: number;
    /** 0–60, record = events */
    event_pre_s?: number;
    /** camera group ≤ 60; users limited to groups see only their cameras (spec 014) */
    group?: string;
    icon?: string;
    name?: string;
    /** http://camera/onvif/device_service: events and PTZ (spec 013); for source push the camera's address in its own network, reached through the relay (onvif = … in relay.toml) */
    onvif_url?: string;
    privacy_masks?: number[][][];
    /** empty = the first profile with PTZ */
    ptz_profile?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    room_id?: string;
    sort?: number;
    /** pull: the server connects to the camera; push: a relay at a remote site sends the video (secret returned once as push_secret with relay_config) */
    source?: "pull" | "push";
  }): Promise<Scene> {
    return this.request<Scene>('PUT', `/api/v1/scenes/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Delete a scene
   *
   * Requires permission `device.manage`. */
  deleteScenesId(id: string, query?: { reason?: string; /** required */ }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/scenes/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Activate a scene
   *
   * Each action is an entity command of the signed-in user (four eyes when required, audit with reason "Scene: <name>"). 409 when every command failed.
   * 
   * Requires permission `device.command`. */
  postScenesIdActivate(id: string, body: {
    /** optional */
    reason?: string;
  }): Promise<{
    failed?: number;
    name?: string;
    results?: ({
      command_id?: string;
      entity_id?: string;
      error?: string;
      http_status?: number;
      service?: string;
      /** sent, acked, pending, failed */
      status?: string;
    })[];
    scene?: string;
  }> {
    return this.request<{
    failed?: number;
    name?: string;
    results?: ({
      command_id?: string;
      entity_id?: string;
      error?: string;
      http_status?: number;
      service?: string;
      /** sent, acked, pending, failed */
      status?: string;
    })[];
    scene?: string;
  }>('POST', `/api/v1/scenes/${encodeURIComponent(id)}/activate`, undefined, body, false);
  }

  /** Scheduled reports
   *
   * Reports mailed on a cron schedule in the organization's time zone.
   * 
   * Requires permission `config.write`. */
  getScheduledReports(): Promise<{
    reports?: ({
      attach_pdf?: boolean;
      attach_xlsx?: boolean;
      content?: Record<string, unknown>;
      created_by?: string;
      cron?: string;
      enabled?: boolean;
      id?: string;
      last_run_at?: string | null;
      /** sent, partial, failed */
      last_status?: string;
      name?: string;
      next_run_at?: string | null;
      period?: string;
      recipients?: Record<string, unknown>;
      /** time zone of the organization */
      tz?: string;
      updated_at?: string;
    })[];
    /** e-mail is configured on this server */
    smtp?: boolean;
  }> {
    return this.request<{
    reports?: ({
      attach_pdf?: boolean;
      attach_xlsx?: boolean;
      content?: Record<string, unknown>;
      created_by?: string;
      cron?: string;
      enabled?: boolean;
      id?: string;
      last_run_at?: string | null;
      /** sent, partial, failed */
      last_status?: string;
      name?: string;
      next_run_at?: string | null;
      period?: string;
      recipients?: Record<string, unknown>;
      /** time zone of the organization */
      tz?: string;
      updated_at?: string;
    })[];
    /** e-mail is configured on this server */
    smtp?: boolean;
  }>('GET', `/api/v1/scheduled-reports`, undefined, undefined, false);
  }

  /** Create a scheduled report
   *
   * Content: a summary of an object counting area, a period table widget of a dashboard, or a report template (observations of a datastream, alarms); period: the previous day, week (Monday to Sunday) or month; attachments PDF and/or XLSX (CSV for templates). The files are made like the exports of the user interface, as the report's author.
   * 
   * Requires permission `config.write`. */
  postScheduledReports(body: {
    /** default true */
    attach_pdf?: boolean;
    /** default true (CSV for templates) */
    attach_xlsx?: boolean;
    content: {
      /** counting */
      area_id?: string;
      /** period_table: dashboard slug */
      dashboard?: string;
      /** template observations */
      datastream_id?: string;
      /** template */
      kind?: "observations" | "alarms";
      type?: "counting" | "period_table" | "template";
      /** period_table: widget id */
      widget?: string;
    };
    /** five fields, e.g. 0 7 * * 1, or a preset daily_0700, monday_0700, monthly_0700 */
    cron: string;
    /** default true */
    enabled?: boolean;
    /** 1–80 */
    name: string;
    period: "previous_day" | "previous_week" | "previous_month";
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    recipients: {
      addresses?: string[];
      channels?: string[];
    };
  }): Promise<{
    id?: string;
    next_run_at?: string | null;
    tz?: string;
  }> {
    return this.request<{
    id?: string;
    next_run_at?: string | null;
    tz?: string;
  }>('POST', `/api/v1/scheduled-reports`, undefined, body, false);
  }

  /** Choices for a scheduled report
   *
   * Requires permission `config.write`. */
  getScheduledReportsOptions(): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/scheduled-reports/options`, undefined, undefined, false);
  }

  /** Next runs of a cron expression
   *
   * In the organization's time zone, across daylight-saving changes.
   * 
   * Requires permission `config.write`. */
  postScheduledReportsPreviewCron(body: {
    /** 1–20, default 5 */
    count?: number;
    cron: string;
  }): Promise<{
    /** normalised */
    cron?: string;
    next?: string[];
    tz?: string;
  }> {
    return this.request<{
    /** normalised */
    cron?: string;
    next?: string[];
    tz?: string;
  }>('POST', `/api/v1/scheduled-reports/preview-cron`, undefined, body, false);
  }

  /** A scheduled report
   *
   * Requires permission `config.write`. */
  getScheduledReportsId(id: string): Promise<{
    attach_pdf?: boolean;
    attach_xlsx?: boolean;
    content?: Record<string, unknown>;
    created_by?: string;
    cron?: string;
    enabled?: boolean;
    id?: string;
    last_run_at?: string | null;
    /** sent, partial, failed */
    last_status?: string;
    name?: string;
    next_run_at?: string | null;
    period?: string;
    recipients?: Record<string, unknown>;
    /** time zone of the organization */
    tz?: string;
    updated_at?: string;
  }> {
    return this.request<{
    attach_pdf?: boolean;
    attach_xlsx?: boolean;
    content?: Record<string, unknown>;
    created_by?: string;
    cron?: string;
    enabled?: boolean;
    id?: string;
    last_run_at?: string | null;
    /** sent, partial, failed */
    last_status?: string;
    name?: string;
    next_run_at?: string | null;
    period?: string;
    recipients?: Record<string, unknown>;
    /** time zone of the organization */
    tz?: string;
    updated_at?: string;
  }>('GET', `/api/v1/scheduled-reports/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Change a scheduled report
   *
   * Requires permission `config.write`. */
  putScheduledReportsId(id: string, body: {
    /** default true */
    attach_pdf?: boolean;
    /** default true (CSV for templates) */
    attach_xlsx?: boolean;
    content: {
      /** counting */
      area_id?: string;
      /** period_table: dashboard slug */
      dashboard?: string;
      /** template observations */
      datastream_id?: string;
      /** template */
      kind?: "observations" | "alarms";
      type?: "counting" | "period_table" | "template";
      /** period_table: widget id */
      widget?: string;
    };
    /** five fields, e.g. 0 7 * * 1, or a preset daily_0700, monday_0700, monthly_0700 */
    cron: string;
    /** default true */
    enabled?: boolean;
    /** 1–80 */
    name: string;
    period: "previous_day" | "previous_week" | "previous_month";
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    recipients: {
      addresses?: string[];
      channels?: string[];
    };
  }): Promise<{
    next_run_at?: string | null;
    ok?: boolean;
  }> {
    return this.request<{
    next_run_at?: string | null;
    ok?: boolean;
  }>('PUT', `/api/v1/scheduled-reports/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Remove a scheduled report
   *
   * The delivery log stays.
   * 
   * Requires permission `config.write`. */
  deleteScheduledReportsId(id: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/scheduled-reports/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Delivery log of a scheduled report
   *
   * Requires permission `config.write`. */
  getScheduledReportsIdRuns(id: string): Promise<(Record<string, unknown>)[]> {
    return this.request<(Record<string, unknown>)[]>('GET', `/api/v1/scheduled-reports/${encodeURIComponent(id)}/runs`, undefined, undefined, false);
  }

  /** Send a scheduled report now
   *
   * A test: the files for the period before now are mailed to the recipients and the delivery is recorded like a scheduled one.
   * 
   * Requires permission `config.write`. */
  postScheduledReportsIdSend(id: string): Promise<{
    attachments?: ({
      bytes?: number;
      name?: string;
      sha256?: string;
    })[];
    error?: string;
    period_from?: string;
    period_to?: string;
    /** delivered */
    recipients?: number;
    status?: "sent" | "partial" | "failed";
  }> {
    return this.request<{
    attachments?: ({
      bytes?: number;
      name?: string;
      sha256?: string;
    })[];
    error?: string;
    period_from?: string;
    period_to?: string;
    /** delivered */
    recipients?: number;
    status?: "sent" | "partial" | "failed";
  }>('POST', `/api/v1/scheduled-reports/${encodeURIComponent(id)}/send`, undefined, undefined, false);
  }

  /** Server status and configuration summary
   *
   * Version, listeners, database, SMTP, storage paths, connected MQTT clients, update availability (`update`: current, available, release_date of this version, updates_until and updates_ended of the license).
   * 
   * Requires permission `system.admin`. */
  getServer(): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/server`, undefined, undefined, false);
  }

  /** Backups and schedule
   *
   * Requires permission `system.admin`. */
  getServerBackups(): Promise<{
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      size_bytes?: number;
    })[];
    running?: boolean;
    schedule?: Record<string, unknown>;
  }> {
    return this.request<{
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      size_bytes?: number;
    })[];
    running?: boolean;
    schedule?: Record<string, unknown>;
  }>('GET', `/api/v1/server/backups`, undefined, undefined, false);
  }

  /** Start a backup now
   *
   * Requires permission `system.admin`. */
  postServerBackups(body: Record<string, unknown>): Promise<{
    encrypted?: boolean;
  }> {
    return this.request<{
    encrypted?: boolean;
  }>('POST', `/api/v1/server/backups`, undefined, body, false);
  }

  /** Set the backup schedule
   *
   * Requires permission `system.admin`. */
  putServerBackupsSchedule(body: {
    enabled?: boolean;
    hour?: number;
    /** files to keep */
    keep?: number;
    minute?: number;
    /** encrypt backups (blank keeps the stored one) */
    passphrase?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    enabled?: boolean;
    hour?: number;
    keep?: number;
    minute?: number;
    passphrase_changed?: boolean;
  }> {
    return this.request<{
    enabled?: boolean;
    hour?: number;
    keep?: number;
    minute?: number;
    passphrase_changed?: boolean;
  }>('PUT', `/api/v1/server/backups/schedule`, undefined, body, false);
  }

  /** Download a backup archive
   *
   * Requires permission `system.admin`. */
  getServerBackupsName(name: string): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/server/backups/${encodeURIComponent(name)}`, undefined, undefined, true);
  }

  /** Delete a backup archive
   *
   * Requires permission `system.admin`. */
  postServerBackupsNameDelete(name: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/server/backups/${encodeURIComponent(name)}/delete`, undefined, body, false);
  }

  /** Configure the domain and TLS (Caddy)
   *
   * Requires permission `system.admin`. */
  putServerCaddy(body: {
    /** optional DNS challenge provider */
    dns_provider?: string;
    domain: string;
    /** ACME account e-mail */
    email?: string;
    extra_domains?: string[];
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** DNS provider token (blank keeps the stored one) */
    token?: string;
  }): Promise<{
    dns_provider?: string;
    domain?: string;
    email?: string;
    extra_domains?: string[];
    token_changed?: boolean;
  }> {
    return this.request<{
    dns_provider?: string;
    domain?: string;
    email?: string;
    extra_domains?: string[];
    token_changed?: boolean;
  }>('PUT', `/api/v1/server/caddy`, undefined, body, false);
  }

  /** Installed product license
   *
   * The system is free including up to 4 cameras. A license (one per server) issued by the licensor and verified offline against the public keys built into the binary covers more than 4 cameras, white labeling, changes through AI assistants and the VPN extras, and includes the updates released until `updates_until`; afterwards the system keeps running with everything it has and only newer versions are not installed.
   * 
   * Requires permission `system.admin`. */
  getServerLicense(): Promise<{
    activation?: {
      activated_at?: string;
      activated_by?: string;
      host?: string;
      /** starter | permanent */
      kind?: string;
      last_attempt?: string;
      last_error?: string;
      last_ok?: string;
      /** order number of the ctrl32 licence */
      order?: string;
      /** the licence service refused the order; automatic renewal stopped */
      refused?: boolean;
    };
    /** server name sent with the order number */
    activation_host?: string;
    /** the stored order renews the license automatically */
    auto_renew?: boolean;
    /** where the ctrl32 licence is sold (license.buy_url) */
    buy_url?: string;
    cameras?: {
      /** enabled cameras of all organizations */
      enabled?: number;
      /** 4: enabled cameras without a license */
      free_limit?: number;
      licensed?: boolean;
    };
    domains?: string[];
    email?: string;
    /** absent = no expiry date */
    expires?: string;
    features?: string[];
    id?: string;
    installed?: boolean;
    installed_at?: string;
    installed_by?: string;
    issued?: string;
    licensee?: string;
    /** the license has no expiry date */
    no_expiry?: boolean;
    /** a 30-day license of the online activation (signed with the online key) */
    online?: boolean;
    /** older name of no_expiry */
    perpetual?: boolean;
    /** why not */
    problem?: string;
    /** release date of this version (YYYY-MM-DD; empty = unknown) */
    release_date?: string;
    /** host of http.base_url */
    server_host?: string;
    /** licensor keys in this build */
    trusted_keys?: number;
    /** the included updates ended; the system keeps running */
    updates_ended?: boolean;
    /** YYYY-MM-DD: last release date the license includes updates for; absent = no limit */
    updates_until?: string;
    /** usable on this server now */
    valid?: boolean;
    white_label?: boolean;
  }> {
    return this.request<{
    activation?: {
      activated_at?: string;
      activated_by?: string;
      host?: string;
      /** starter | permanent */
      kind?: string;
      last_attempt?: string;
      last_error?: string;
      last_ok?: string;
      /** order number of the ctrl32 licence */
      order?: string;
      /** the licence service refused the order; automatic renewal stopped */
      refused?: boolean;
    };
    /** server name sent with the order number */
    activation_host?: string;
    /** the stored order renews the license automatically */
    auto_renew?: boolean;
    /** where the ctrl32 licence is sold (license.buy_url) */
    buy_url?: string;
    cameras?: {
      /** enabled cameras of all organizations */
      enabled?: number;
      /** 4: enabled cameras without a license */
      free_limit?: number;
      licensed?: boolean;
    };
    domains?: string[];
    email?: string;
    /** absent = no expiry date */
    expires?: string;
    features?: string[];
    id?: string;
    installed?: boolean;
    installed_at?: string;
    installed_by?: string;
    issued?: string;
    licensee?: string;
    /** the license has no expiry date */
    no_expiry?: boolean;
    /** a 30-day license of the online activation (signed with the online key) */
    online?: boolean;
    /** older name of no_expiry */
    perpetual?: boolean;
    /** why not */
    problem?: string;
    /** release date of this version (YYYY-MM-DD; empty = unknown) */
    release_date?: string;
    /** host of http.base_url */
    server_host?: string;
    /** licensor keys in this build */
    trusted_keys?: number;
    /** the included updates ended; the system keeps running */
    updates_ended?: boolean;
    /** YYYY-MM-DD: last release date the license includes updates for; absent = no limit */
    updates_until?: string;
    /** usable on this server now */
    valid?: boolean;
    white_label?: boolean;
  }>('GET', `/api/v1/server/license`, undefined, undefined, false);
  }

  /** Install a product license
   *
   * 409 `updates_ended` when the updates the license includes ended before this version was released (renew it, or use it with a version it covers).
   * 
   * Requires permission `system.admin`. */
  putServerLicense(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** c32l1.… */
    token: string;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('PUT', `/api/v1/server/license`, undefined, body, false);
  }

  /** Remove the product license
   *
   * Also forgets the stored order number, so nothing is renewed.
   * 
   * Requires permission `system.admin`. */
  deleteServerLicense(query?: { reason?: string; /** required */ }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('DELETE', `/api/v1/server/license`, query, undefined, false);
  }

  /** Activate the license with an order number
   *
   * Sends the order number of the ctrl32 licence (bought on ctrl32.com; it unlocks either the ctrl32 editor or the licensed features of one server) and the host name of http.base_url to the licence service (license.activation_url), verifies the license it returns and installs it. Inside the 14-day refund period the license runs 30 days and is renewed daily while it expires within 20 days; afterwards the license without expiry date is returned. 409 = refused by the service (refunded, used for the editor, server limit), a license without expiry date is already installed, or the updates of the returned license ended before this version was released (`updates_ended`); 502 = the service is not reachable or its answer was not accepted.
   * 
   * Requires permission `system.admin`. */
  postServerLicenseActivate(body: {
    /** order number (Paddle transaction id or ctrl32.com order id) */
    order: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/server/license/activate`, undefined, body, false);
  }

  /** Renew the activated license now
   *
   * Asks the licence service for the stored order now (the server does it daily on its own) and installs the answer when it is better than the installed license; renewed tells whether it did.
   * 
   * Requires permission `system.admin`. */
  postServerLicenseRenew(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** a new license was installed */
    renewed?: boolean;
  }> {
    return this.request<{
    /** a new license was installed */
    renewed?: boolean;
  }>('POST', `/api/v1/server/license/renew`, undefined, body, false);
  }

  /** Restart the service (through the agent)
   *
   * Requires permission `system.admin`. */
  postServerRestart(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    unit?: string;
  }> {
    return this.request<{
    unit?: string;
  }>('POST', `/api/v1/server/restart`, undefined, body, false);
  }

  /** Download, verify and install a release
   *
   * The agent fetches the binary and SHA256SUMS from the update URL, verifies, installs and restarts. With a license whose included updates end (`updates_until`), a binary released later (release date reported by the binary) is refused with 402 `updates_ended` and the running system stays as it is; servers without a license always update.
   * 
   * Requires permission `system.admin`. */
  postServerUpdate(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** release version, e.g. 0.24.0 */
    version: string;
  }): Promise<{
    sha256?: string;
    version?: string;
  }> {
    return this.request<{
    sha256?: string;
    version?: string;
  }>('POST', `/api/v1/server/update`, undefined, body, false);
  }

  /** WireGuard status and peers
   *
   * Requires permission `system.admin`. */
  getServerWireguard(): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/server/wireguard`, undefined, undefined, false);
  }

  /** Enable or configure WireGuard
   *
   * Requires permission `system.admin`. */
  putServerWireguard(body: {
    /** server address in the tunnel, CIDR */
    address?: string;
    enabled?: boolean;
    /** public endpoint host:port */
    endpoint?: string;
    listen_port?: number;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    address?: string;
    enabled?: boolean;
    endpoint?: string;
    listen_port?: number;
  }> {
    return this.request<{
    address?: string;
    enabled?: boolean;
    endpoint?: string;
    listen_port?: number;
  }>('PUT', `/api/v1/server/wireguard`, undefined, body, false);
  }

  /** Organization security policy
   *
   * Requires permission `config.write`. */
  getSettingsPolicy(): Promise<Policy> {
    return this.request<Policy>('GET', `/api/v1/settings/policy`, undefined, undefined, false);
  }

  /** Set the four-eyes policies
   *
   * `four_eyes_commands`: commands wait for approval. `four_eyes_limits`: alarm rule changes become change requests. `four_eyes_assignments`: device ↔ datastream assignments become change requests. `four_eyes_vpn`: VPN access of technicians needs a grant approved by a second person. At least one field.
   * 
   * Requires permission `user.admin`. */
  putSettingsPolicy(body: {
    four_eyes_assignments?: boolean;
    four_eyes_commands?: boolean;
    four_eyes_limits?: boolean;
    four_eyes_vpn?: boolean;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    four_eyes_assignments?: boolean;
    four_eyes_commands?: boolean;
    four_eyes_limits?: boolean;
    four_eyes_vpn?: boolean;
  }> {
    return this.request<{
    four_eyes_assignments?: boolean;
    four_eyes_commands?: boolean;
    four_eyes_limits?: boolean;
    four_eyes_vpn?: boolean;
  }>('PUT', `/api/v1/settings/policy`, undefined, body, false);
  }

  /** List sites
   *
   * Requires permission `data.read`. */
  getSites(): Promise<Site[]> {
    return this.request<Site[]>('GET', `/api/v1/sites`, undefined, undefined, false);
  }

  /** Create a site
   *
   * Requires permission `config.write`. */
  postSites(body: {
    address?: string;
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** IANA time zone, default Europe/Bratislava */
    tz?: string;
  }): Promise<Site> {
    return this.request<Site>('POST', `/api/v1/sites`, undefined, body, false);
  }

  /** Update a site
   *
   * Requires permission `config.write`. */
  patchSitesId(id: string, body: {
    address?: string;
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    tz?: string;
  }): Promise<Site> {
    return this.request<Site>('PATCH', `/api/v1/sites/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Live events (Server-Sent Events)
   *
   * Long-lived `text/event-stream`. Events: `hello`, `observation` {datastream_id, key, value, measured_at}, `device` {device_id, connected}, `alarm` {alarm_id, datastream_id, state, severity, value}, `command` {device_id, command_id, method, status, detail}; `ping` every 25 s with the count of dropped events. Filters: `datastreams` (comma-separated ids), `devices` (`all` or ids), `alarms=1`.
   * 
   * Requires permission `data.read`. */
  getStream(query?: { datastreams?: string; /** comma-separated datastream ids */ devices?: string; /** all, or comma-separated device ids */ alarms?: string; /** 1 to include alarm events */ }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/stream`, query, undefined, true);
  }

  /** Self-hosted map tiles (PMTiles archive)
   *
   * Status of the server's local map: a raster PMTiles v3 archive named by `[map] pmtiles` in config.toml. When it is configured and the organization has no tile URL of its own, the branding map points at `/tiles/{z}/{x}/{y}.<ext>` on this server (public, cacheable; 404 outside the archive), so map widgets work without the Internet. Create an archive with `pmtiles convert map.mbtiles map.pmtiles` (go-pmtiles) or try `go run ./tools/pmtilesgen`. */
  getTiles(): Promise<TilesInfo> {
    return this.request<TilesInfo>('GET', `/api/v1/tiles`, undefined, undefined, false);
  }

  /** My API tokens
   *
   * Requires permission `token.manage`. */
  getTokens(): Promise<APIToken[]> {
    return this.request<APIToken[]>('GET', `/api/v1/tokens`, undefined, undefined, false);
  }

  /** Create an API token
   *
   * The raw token is returned once. Scopes must be a subset of the caller's permissions.
   * 
   * Requires permission `token.manage`. */
  postTokens(body: {
    /** optional expiry */
    expires_at?: string | null;
    name: string;
    scopes: string[];
  }): Promise<{
    info?: APIToken;
    /** raw token, shown once */
    token?: string;
  }> {
    return this.request<{
    info?: APIToken;
    /** raw token, shown once */
    token?: string;
  }>('POST', `/api/v1/tokens`, undefined, body, false);
  }

  /** Revoke an API token
   *
   * Requires permission `token.manage`. */
  deleteTokensId(id: string): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/tokens/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** List users
   *
   * Requires permission `user.admin`. */
  getUsers(): Promise<User[]> {
    return this.request<User[]>('GET', `/api/v1/users`, undefined, undefined, false);
  }

  /** Create a user
   *
   * Returns a one-time initial password that must be changed at first sign-in.
   * 
   * Requires permission `user.admin`. */
  postUsers(body: {
    email?: string;
    full_name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    roles: string[];
    username: string;
  }): Promise<{
    /** shown once */
    initial_password?: string;
    user?: User;
  }> {
    return this.request<{
    /** shown once */
    initial_password?: string;
    user?: User;
  }>('POST', `/api/v1/users`, undefined, body, false);
  }

  /** User detail
   *
   * Profile, effective permissions, role history and API tokens.
   * 
   * Requires permission `user.admin`. */
  getUsersId(id: string): Promise<{
    api_tokens?: APIToken[];
    permissions?: string[];
    role_history?: (Record<string, unknown>)[];
    user?: User;
  }> {
    return this.request<{
    api_tokens?: APIToken[];
    permissions?: string[];
    role_history?: (Record<string, unknown>)[];
    user?: User;
  }>('GET', `/api/v1/users/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Update a user
   *
   * Roles, disabled flag, name, e-mail. Every role change is a row in the role history.
   * 
   * Requires permission `user.admin`. */
  patchUsersId(id: string, body: {
    disabled?: boolean;
    email?: string;
    full_name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    roles?: string[];
  }): Promise<User> {
    return this.request<User>('PATCH', `/api/v1/users/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** A user's camera access
   *
   * limited = false: all cameras; otherwise only the cameras of the groups ("" = cameras without a group).
   * 
   * Requires permission `user.admin`. */
  getUsersIdCameraGroups(id: string): Promise<{
    groups?: string[];
    limited?: boolean;
  }> {
    return this.request<{
    groups?: string[];
    limited?: boolean;
  }>('GET', `/api/v1/users/${encodeURIComponent(id)}/camera-groups`, undefined, undefined, false);
  }

  /** Limit a user to camera groups
   *
   * Applies at once to every camera endpoint. An administrator limited to some groups can hand out only those. Audited.
   * 
   * Requires permission `user.admin`. */
  putUsersIdCameraGroups(id: string, body: {
    groups?: string[];
    limited: boolean;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    groups?: string[];
    limited?: boolean;
  }> {
    return this.request<{
    groups?: string[];
    limited?: boolean;
  }>('PUT', `/api/v1/users/${encodeURIComponent(id)}/camera-groups`, undefined, body, false);
  }

  /** Remove a user's MFA enrolment
   *
   * Requires permission `user.admin`. */
  postUsersIdMfaReset(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    by?: string;
    username?: string;
  }> {
    return this.request<{
    by?: string;
    username?: string;
  }>('POST', `/api/v1/users/${encodeURIComponent(id)}/mfa-reset`, undefined, body, false);
  }

  /** Reset a user's password
   *
   * Returns a new one-time password.
   * 
   * Requires permission `user.admin`. */
  postUsersIdResetPassword(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    by?: string;
    initial_password?: string;
  }> {
    return this.request<{
    by?: string;
    initial_password?: string;
  }>('POST', `/api/v1/users/${encodeURIComponent(id)}/reset-password`, undefined, body, false);
  }

  /** Sign a user out everywhere
   *
   * Requires permission `user.admin`. */
  postUsersIdSessionsRevoke(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    by?: string;
    /** revoked sessions */
    sessions?: number;
    username?: string;
  }> {
    return this.request<{
    by?: string;
    /** revoked sessions */
    sessions?: number;
    username?: string;
  }>('POST', `/api/v1/users/${encodeURIComponent(id)}/sessions/revoke`, undefined, body, false);
  }

  /** Unlock a locked-out user
   *
   * Requires permission `user.admin`. */
  postUsersIdUnlock(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    by?: string;
    username?: string;
  }> {
    return this.request<{
    by?: string;
    username?: string;
  }>('POST', `/api/v1/users/${encodeURIComponent(id)}/unlock`, undefined, body, false);
  }

  /** Camera groups in use
   *
   * Groups of the organization's cameras with their counts; a limited user sees only their groups.
   * 
   * Requires permission `video.view`. */
  getVideoCameraGroups(): Promise<({
    cameras?: number;
    group?: string;
  })[]> {
    return this.request<({
    cameras?: number;
    group?: string;
  })[]>('GET', `/api/v1/video/camera-groups`, undefined, undefined, false);
  }

  /** Cameras
   *
   * Stream addresses, user name and transport only for video.manage.
   * 
   * Requires permission `video.view`. */
  getVideoCameras(): Promise<Camera[]> {
    return this.request<Camera[]>('GET', `/api/v1/video/cameras`, undefined, undefined, false);
  }

  /** Add a camera
   *
   * Without a license a server runs up to 4 enabled cameras (all organizations together): a fifth enabled camera is refused with 402 `license_required`; a disabled camera can be added.
   * 
   * Requires permission `video.manage`. */
  postVideoCameras(body: {
    /** take the camera's sound (AAC, Opus or G.711); spec 020 */
    audio?: boolean;
    /** record the sound too (default true; false = heard live only, saves disk space) */
    audio_record?: boolean;
    enabled?: boolean;
    event_kinds?: string[];
    /** 1–600 */
    event_post_s?: number;
    /** 0–60, record = events */
    event_pre_s?: number;
    /** camera group ≤ 60; users limited to groups see only their cameras (spec 014) */
    group?: string;
    /** ≤ 120 */
    location?: string;
    /** motion detection on the server (spec 019) */
    motion_detect?: boolean;
    /** 1 to 100, default 50 */
    motion_sensitivity?: number;
    motion_zones?: number[][][];
    /** ≤ 80 */
    name: string;
    /** http://camera/onvif/device_service: events and PTZ (spec 013); for source push the camera's address in its own network, reached through the relay (onvif = … in relay.toml) */
    onvif_url?: string;
    /** stored encrypted; omit to keep, "" to clear */
    password?: string;
    privacy_masks?: number[][][];
    /** empty = the first profile with PTZ */
    ptz_profile?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** continuous; events: only around events (pre- and post-roll); off */
    record?: "continuous" | "events" | "off";
    /** 1–3650, default 7 */
    retention_days?: number;
    /** http(s) address of one JPEG picture, for H.264/H.265 cameras; without credentials */
    snapshot_url?: string;
    sort?: number;
    /** pull: the server connects to the camera; push: a relay at a remote site sends the video (secret returned once as push_secret with relay_config) */
    source?: "pull" | "push";
    /** smaller stream for tiles (optional) */
    sub_url?: string;
    transport?: "auto" | "tcp" | "udp";
    /** rtsp://, rtsps://, http:// or https:// main stream; user:password in the URL are moved to username/password */
    url: string;
    username?: string;
  }): Promise<Camera> {
    return this.request<Camera>('POST', `/api/v1/video/cameras`, undefined, body, false);
  }

  /** Test a stream
   *
   * Connects once (at most 8 s) and reports codec and picture size or the error. With id and no password the stored password is used.
   * 
   * Requires permission `video.manage`. */
  postVideoCamerasTest(body: {
    /** camera whose stored password to use */
    id?: string;
    password?: string;
    transport?: "auto" | "tcp" | "udp";
    url: string;
    username?: string;
  }): Promise<CameraStatus> {
    return this.request<CameraStatus>('POST', `/api/v1/video/cameras/test`, undefined, body, false);
  }

  /** A camera
   *
   * Requires permission `video.view`. */
  getVideoCamerasId(id: string): Promise<Camera> {
    return this.request<Camera>('GET', `/api/v1/video/cameras/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Change a camera
   *
   * Enabling a camera without a license when 4 cameras are enabled is refused with 402 `license_required`; cameras already enabled keep working and can be changed.
   * 
   * Requires permission `video.manage`. */
  putVideoCamerasId(id: string, body: {
    /** take the camera's sound (AAC, Opus or G.711); spec 020 */
    audio?: boolean;
    /** record the sound too (default true; false = heard live only, saves disk space) */
    audio_record?: boolean;
    enabled?: boolean;
    event_kinds?: string[];
    /** 1–600 */
    event_post_s?: number;
    /** 0–60, record = events */
    event_pre_s?: number;
    /** camera group ≤ 60; users limited to groups see only their cameras (spec 014) */
    group?: string;
    /** ≤ 120 */
    location?: string;
    /** motion detection on the server (spec 019) */
    motion_detect?: boolean;
    /** 1 to 100, default 50 */
    motion_sensitivity?: number;
    motion_zones?: number[][][];
    /** ≤ 80 */
    name?: string;
    /** http://camera/onvif/device_service: events and PTZ (spec 013); for source push the camera's address in its own network, reached through the relay (onvif = … in relay.toml) */
    onvif_url?: string;
    /** stored encrypted; omit to keep, "" to clear */
    password?: string;
    privacy_masks?: number[][][];
    /** empty = the first profile with PTZ */
    ptz_profile?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** continuous; events: only around events (pre- and post-roll); off */
    record?: "continuous" | "events" | "off";
    /** 1–3650, default 7 */
    retention_days?: number;
    /** http(s) address of one JPEG picture, for H.264/H.265 cameras; without credentials */
    snapshot_url?: string;
    sort?: number;
    /** pull: the server connects to the camera; push: a relay at a remote site sends the video (secret returned once as push_secret with relay_config) */
    source?: "pull" | "push";
    /** smaller stream for tiles (optional) */
    sub_url?: string;
    transport?: "auto" | "tcp" | "udp";
    /** rtsp://, rtsps://, http:// or https:// main stream; user:password in the URL are moved to username/password */
    url?: string;
    username?: string;
  }): Promise<Camera> {
    return this.request<Camera>('PUT', `/api/v1/video/cameras/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Remove a camera
   *
   * Recordings stay until the camera's retention ends.
   * 
   * Requires permission `video.manage`. */
  deleteVideoCamerasId(id: string, query?: { reason?: string; /** required */ }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/video/cameras/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Events of a camera
   *
   * Motion, tampering, line crossing, intrusion and inputs reported by the camera over ONVIF, and marks from flows and people.
   * 
   * Requires permission `video.playback`. */
  getVideoCamerasIdEvents(id: string, query?: { from?: string; /** RFC 3339, required */ to?: string; /** RFC 3339 */ kind?: string; }): Promise<({
    camera?: string;
    camera_id?: string;
    /** null while it lasts */
    ended_at?: string | null;
    id?: number;
    kind?: "motion" | "tamper" | "line" | "intrusion" | "input" | "mark" | "other";
    label?: string;
    source?: "camera" | "flow" | "user";
    started_at?: string;
  })[]> {
    return this.request<({
    camera?: string;
    camera_id?: string;
    /** null while it lasts */
    ended_at?: string | null;
    id?: number;
    kind?: "motion" | "tamper" | "line" | "intrusion" | "input" | "mark" | "other";
    label?: string;
    source?: "camera" | "flow" | "user";
    started_at?: string;
  })[]>('GET', `/api/v1/video/cameras/${encodeURIComponent(id)}/events`, query, undefined, false);
  }

  /** Mark a camera
   *
   * A mark on the timeline; on a camera recording on events it records the given seconds with the pre-roll before. Audited.
   * 
   * Requires permission `video.view`. */
  postVideoCamerasIdEvents(id: string, body: {
    /** 1–200 characters */
    label: string;
    /** 0–3600 */
    seconds?: number;
  }): Promise<{
    id?: number;
  }> {
    return this.request<{
    id?: number;
  }>('POST', `/api/v1/video/cameras/${encodeURIComponent(id)}/events`, undefined, body, false);
  }

  /** Signed evidence package
   *
   * ZIP with video.mp4 (remuxed, not re-encoded), manifest.json (SHA-256 of the video and of every recorded file as computed while recording and now, events, who exported it and why), manifest.sig (Ed25519 by the server key), public_key.txt and README.txt. At most one hour. Verify with `ctrl32-telemetry verify-evidence`. Audited with the video hash.
   * 
   * Requires permission `video.evidence`. */
  getVideoCamerasIdEvidence(id: string, query: { from: string; /** RFC 3339 */ to: string; /** RFC 3339 */ reason: string; /** written into the manifest */ case_ref?: string; }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/video/cameras/${encodeURIComponent(id)}/evidence`, query, undefined, true);
  }

  /** Download recorded video
   *
   * One MP4 file (fragmented) of at most one hour; a change of resolution or codec ends it early. The recorded sound is the second track only for people with video.audio. Written to the audit trail (video.export).
   * 
   * Requires permission `video.playback`. */
  getVideoCamerasIdExport(id: string, query?: { from?: string; /** RFC 3339, required */ to?: string; /** RFC 3339, required */ }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/video/cameras/${encodeURIComponent(id)}/export`, query, undefined, true);
  }

  /** Live video (WebSocket)
   *
   * Upgrade to a WebSocket. Text messages: `{type:"init", codec, mime, width, height}`, `{type:"time", t (ms), pos (s)}` at key frames, `{type:"status", state, error}`, `{type:"audio", mime, codec}` when the sound comes (spec 020: `audio=1`, main stream, camera with sound, video.audio; audited as video.listen): binary messages starting with the byte `A` then carry the sound as its own fragmented MP4 stream on the picture's time line; for G.711 cameras `{type:"audio", codec:"pcm", rate, channels}` and binary messages `P` + float64 position (s, big-endian) + 16-bit big-endian samples, for WebAudio. Binary messages: for H.264/H.265 the MP4 initialization, then one fragment (moof+mdat) per frame, for Media Source Extensions; for MJPEG one JPEG picture each.
   * 
   * Requires permission `video.view`. */
  getVideoCamerasIdLive(id: string, query?: { audio?: string; /** 1 = with the sound (needs video.audio) */ quality?: string; /** sub (default; falls back to main) or main */ }): Promise<unknown> {
    return this.request<unknown>('GET', `/api/v1/video/cameras/${encodeURIComponent(id)}/live`, query, undefined, false);
  }

  /** Record a picture saved in the player
   *
   * The player in the browser burns the privacy masks into a saved picture. A picture without them (a stream opened with unmask=1) may be saved only with video.unmask; it is audited (video.unmask).
   * 
   * Requires permission `video.view`. */
  postVideoCamerasIdPictureSaved(id: string, body: {
    /** time of the picture, RFC 3339 */
    at?: string;
    /** the picture carries no privacy masks */
    unmasked?: boolean;
  }): Promise<{
    audited?: boolean;
    /** the camera has privacy masks */
    masked?: boolean;
  }> {
    return this.request<{
    audited?: boolean;
    /** the camera has privacy masks */
    masked?: boolean;
  }>('POST', `/api/v1/video/cameras/${encodeURIComponent(id)}/picture-saved`, undefined, body, false);
  }

  /** Recorded video (WebSocket)
   *
   * Same messages as live, paced by the recording from the key frame before `from`; periods without recording are skipped; `{type:"end"}` after the newest recording. Written to the audit trail (video.playback).
   * 
   * Requires permission `video.playback`. */
  getVideoCamerasIdPlayback(id: string, query?: { audio?: string; /** 1 = with the recorded sound at speed 1 (needs video.audio) */ from?: string; /** RFC 3339, required */ speed?: string; /** 0.25–16, default 1 */ }): Promise<unknown> {
    return this.request<unknown>('GET', `/api/v1/video/cameras/${encodeURIComponent(id)}/playback`, query, undefined, false);
  }

  /** Turn and zoom a PTZ camera
   *
   * move starts a continuous movement (speeds -1…1) until stop; preset goes to a stored position. Audited (moves once per the video setting ptz_audit_minutes per user and camera, presets always).
   * 
   * Requires permission `video.ptz`. */
  postVideoCamerasIdPtz(id: string, body: {
    action: "move" | "stop" | "preset";
    pan?: number;
    /** token */
    preset?: string;
    tilt?: number;
    zoom?: number;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/video/cameras/${encodeURIComponent(id)}/ptz`, undefined, body, false);
  }

  /** PTZ presets
   *
   * Requires permission `video.ptz`. */
  getVideoCamerasIdPtzPresets(id: string): Promise<({
    name?: string;
    token?: string;
  })[]> {
    return this.request<({
    name?: string;
    token?: string;
  })[]>('GET', `/api/v1/video/cameras/${encodeURIComponent(id)}/ptz/presets`, undefined, undefined, false);
  }

  /** New secret of a remote camera
   *
   * The relay with the old secret is disconnected at once. The new secret and a relay configuration are returned only in this response. Push cameras only (409 otherwise).
   * 
   * Requires permission `video.manage`. */
  postVideoCamerasIdPushSecret(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** shown once */
    push_secret?: string;
    /** relay.toml for camera-relay */
    relay_config?: string;
  }> {
    return this.request<{
    /** shown once */
    push_secret?: string;
    /** relay.toml for camera-relay */
    relay_config?: string;
  }>('POST', `/api/v1/video/cameras/${encodeURIComponent(id)}/push-secret`, undefined, body, false);
  }

  /** Recorded spans
   *
   * Adjacent files are merged into spans for a timeline. Range at most 32 days.
   * 
   * Requires permission `video.playback`. */
  getVideoCamerasIdRecordings(id: string, query?: { from?: string; /** RFC 3339, required */ to?: string; /** RFC 3339, default now */ }): Promise<{
    bytes?: number;
    camera?: string;
    files?: number;
    from?: string;
    spans?: ({
      from?: string;
      to?: string;
    })[];
    to?: string;
  }> {
    return this.request<{
    bytes?: number;
    camera?: string;
    files?: number;
    from?: string;
    spans?: ({
      from?: string;
      to?: string;
    })[];
    to?: string;
  }>('GET', `/api/v1/video/cameras/${encodeURIComponent(id)}/recordings`, query, undefined, false);
  }

  /** The current picture
   *
   * MJPEG streams only (the server does not decode H.264/H.265).
   * 
   * Requires permission `video.view`. */
  getVideoCamerasIdSnapshot(id: string, query?: { quality?: string; /** sub or main */ }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/video/cameras/${encodeURIComponent(id)}/snapshot`, query, undefined, true);
  }

  /** Find ONVIF cameras
   *
   * WS-Discovery in the server's network (3 s); extra IP addresses (other subnets) are probed directly.
   * 
   * Requires permission `video.manage`. */
  postVideoDiscover(body: {
    hosts?: string[];
  }): Promise<({
    /** device service */
    address?: string;
    hardware?: string;
    host?: string;
    name?: string;
  })[]> {
    return this.request<({
    /** device service */
    address?: string;
    hardware?: string;
    host?: string;
    name?: string;
  })[]>('POST', `/api/v1/video/discover`, undefined, body, false);
  }

  /** Events of all cameras
   *
   * Requires permission `video.playback`. */
  getVideoEvents(query?: { from?: string; /** RFC 3339, required */ to?: string; /** RFC 3339 */ kind?: string; /** motion, tamper, line, intrusion, input, mark, other */ }): Promise<({
    camera?: string;
    camera_id?: string;
    /** null while it lasts */
    ended_at?: string | null;
    id?: number;
    kind?: "motion" | "tamper" | "line" | "intrusion" | "input" | "mark" | "other";
    label?: string;
    source?: "camera" | "flow" | "user";
    started_at?: string;
  })[]> {
    return this.request<({
    camera?: string;
    camera_id?: string;
    /** null while it lasts */
    ended_at?: string | null;
    id?: number;
    kind?: "motion" | "tamper" | "line" | "intrusion" | "input" | "mark" | "other";
    label?: string;
    source?: "camera" | "flow" | "user";
    started_at?: string;
  })[]>('GET', `/api/v1/video/events`, query, undefined, false);
  }

  /** Evidence signing key
   *
   * The server's Ed25519 public key and its fingerprint, to verify evidence packages.
   * 
   * Requires permission `video.playback`. */
  getVideoEvidenceKey(): Promise<{
    /** ed25519 */
    algorithm?: string;
    fingerprint?: string;
    /** base64 */
    public_key?: string;
  }> {
    return this.request<{
    /** ed25519 */
    algorithm?: string;
    fingerprint?: string;
    /** base64 */
    public_key?: string;
  }>('GET', `/api/v1/video/evidence/key`, undefined, undefined, false);
  }

  /** Recordings held as evidence
   *
   * Held ranges with their size; all=1 includes released ones (who released them and why). Limited users see only their cameras.
   * 
   * Requires permission `video.playback`. */
  getVideoHolds(query?: { camera_id?: string; all?: string; /** 1 = include released */ }): Promise<(Record<string, unknown>)[]> {
    return this.request<(Record<string, unknown>)[]>('GET', `/api/v1/video/holds`, query, undefined, false);
  }

  /** Hold recordings as evidence
   *
   * Recordings of the camera in the range are never deleted — not by the camera's retention nor to free disk space — until released. At most 31 days. Audited.
   * 
   * Requires permission `video.evidence`. */
  postVideoHolds(body: {
    camera_id: string;
    /** ≤ 80 */
    case_ref?: string;
    from: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** 1–120 */
    title: string;
    to: string;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/video/holds`, undefined, body, false);
  }

  /** Release held recordings
   *
   * The normal retention deletes them afterwards. Audited with the reason.
   * 
   * Requires permission `video.evidence`. */
  deleteVideoHoldsId(id: string, query?: { reason?: string; /** required */ }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/video/holds/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Media profiles of an ONVIF camera
   *
   * With the stream addresses, so the camera form fills itself. With id and no password the stored password is used. 401 when the camera refuses the credentials.
   * 
   * Requires permission `video.manage`. */
  postVideoOnvifProfiles(body: {
    id?: string;
    password?: string;
    /** http://camera/onvif/device_service */
    url: string;
    username?: string;
  }): Promise<{
    events?: boolean;
    profiles?: ({
      encoding?: string;
      height?: number;
      name?: string;
      ptz?: boolean;
      token?: string;
      uri?: string;
      width?: number;
    })[];
    ptz?: boolean;
  }> {
    return this.request<{
    events?: boolean;
    profiles?: ({
      encoding?: string;
      height?: number;
      name?: string;
      ptz?: boolean;
      token?: string;
      uri?: string;
      width?: number;
    })[];
    ptz?: boolean;
  }>('POST', `/api/v1/video/onvif/profiles`, undefined, body, false);
  }

  /** Next camera control request (relay)
   *
   * Long poll of the relay of a camera that sends its video (source push): answers one pending ONVIF request of the camera as soon as there is one, or 204 after `wait` seconds (at most 25). The relay performs it against the camera's ONVIF address from its relay.toml (under /onvif/ POST or GET, elsewhere at that origin only SOAP calls: POST, application/soap+xml or text/xml, a SOAP envelope) and posts the answer back. `wait=0` only tests the secret and never hands out a request. Authenticated with the camera's push secret in `X-Camera-Secret`; wrong secrets count towards blocking the address (video settings push_max_fails, push_window_minutes, push_block_minutes) and are security events. 429 while the address is blocked. */
  getVideoRelayIdOnvifNext(id: string, query?: { wait?: number; /** seconds to wait for a request, 0 to 25 (default 25) */ }): Promise<RelayRequest> {
    return this.request<RelayRequest>('GET', `/api/v1/video/relay/${encodeURIComponent(id)}/onvif/next`, query, undefined, false);
  }

  /** Answer of a camera control request (relay)
   *
   * The camera's answer (status, Content-Type, body up to 1 MiB) or the relay's error. 410 when the request no longer waits (the server gives up after 10 s). */
  postVideoRelayIdOnvifRid(id: string, rid: string, body: {
    /** base64, at most 1 MiB */
    body?: string;
    /** why the relay has no answer */
    error?: string;
    /** Content-Type only */
    headers?: Record<string, unknown>;
    /** HTTP status of the camera */
    status?: number;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/video/relay/${encodeURIComponent(id)}/onvif/${encodeURIComponent(rid)}`, undefined, body, false);
  }

  /** Server-wide video settings
   *
   * Current settings, their defaults and whether this user may change them (system.admin).
   * 
   * Requires permission `video.manage`. */
  getVideoSettings(): Promise<{
    can_edit?: boolean;
    defaults?: VideoSettings;
    settings?: VideoSettings;
    storage_dir?: string;
  }> {
    return this.request<{
    can_edit?: boolean;
    defaults?: VideoSettings;
    settings?: VideoSettings;
    storage_dir?: string;
  }>('GET', `/api/v1/video/settings`, undefined, undefined, false);
  }

  /** Change the video settings
   *
   * Fields left out keep their value; every field has bounds (400 names the first one out of bounds). Audited with the old and new values.
   * 
   * Requires permission `system.admin`. */
  putVideoSettings(body: {
    /** 0–1440, 0 = no incident */
    camera_offline_minutes?: number;
    /** 1–15 */
    discovery_seconds?: number;
    /** 0–50 */
    disk_free_percent?: number;
    /** 0–1440 */
    display_offline_minutes?: number;
    /** 0–120 */
    event_merge_seconds?: number;
    /** 0–600 */
    linger_seconds?: number;
    /** 1–1048576 */
    max_disk_gb?: number;
    /** 0–60, 0 = every move */
    ptz_audit_minutes?: number;
    /** 1–10080 */
    push_block_minutes?: number;
    /** 3–100 */
    push_max_fails?: number;
    /** 1–1440 */
    push_window_minutes?: number;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** 10–600 */
    segment_seconds?: number;
    /** 5–300 */
    timeshift_seconds?: number;
    /** 0–64 */
    wall_sub_stream_above?: number;
  }): Promise<VideoSettings> {
    return this.request<VideoSettings>('PUT', `/api/v1/video/settings`, undefined, body, false);
  }

  /** Space used by recordings
   *
   * Requires permission `video.manage`. */
  getVideoStorage(): Promise<{
    camera_limit?: {
      /** enabled cameras of all organizations */
      enabled?: number;
      /** 4 */
      free_limit?: number;
      licensed?: boolean;
    };
    cameras?: ({
      bytes?: number;
      files?: number;
      id?: string;
      name?: string;
      oldest?: string | null;
      removed?: boolean;
    })[];
    /** video.max_disk_gb */
    max_bytes?: number;
    /** all organizations */
    used_bytes?: number;
  }> {
    return this.request<{
    camera_limit?: {
      /** enabled cameras of all organizations */
      enabled?: number;
      /** 4 */
      free_limit?: number;
      licensed?: boolean;
    };
    cameras?: ({
      bytes?: number;
      files?: number;
      id?: string;
      name?: string;
      oldest?: string | null;
      removed?: boolean;
    })[];
    /** video.max_disk_gb */
    max_bytes?: number;
    /** all organizations */
    used_bytes?: number;
  }>('GET', `/api/v1/video/storage`, undefined, undefined, false);
  }

  /** Video walls
   *
   * Requires permission `video.view`. */
  getVideoWalls(): Promise<VideoWall[]> {
    return this.request<VideoWall[]>('GET', `/api/v1/video/walls`, undefined, undefined, false);
  }

  /** Create a video wall
   *
   * Requires permission `video.manage`. */
  postVideoWalls(body: {
    /** ≤ 80 */
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    screens: WallScreen[];
  }): Promise<VideoWall> {
    return this.request<VideoWall>('POST', `/api/v1/video/walls`, undefined, body, false);
  }

  /** A video wall
   *
   * Requires permission `video.view`. */
  getVideoWallsId(id: string): Promise<VideoWall> {
    return this.request<VideoWall>('GET', `/api/v1/video/walls/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Change a video wall
   *
   * Requires permission `video.manage`. */
  putVideoWallsId(id: string, body: {
    /** ≤ 80 */
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    screens?: WallScreen[];
  }): Promise<VideoWall> {
    return this.request<VideoWall>('PUT', `/api/v1/video/walls/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Delete a video wall
   *
   * Requires permission `video.manage`. */
  deleteVideoWallsId(id: string, query?: { reason?: string; /** required */ }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/video/walls/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** VPN: interface, firewall, license, peers and access rules
   *
   * The interface and the nftables table from the agent (`interface.firewall.ruleset` is the generated table), whether the server is licensed for site peers, time-limited access and more than `free_limit` peers, every peer with its live state (handshake, traffic) and every access rule. `services` are the server's ports a rule can open to the VPN. A server administrator (system.admin) sees everything (`scope` server); an organization's VPN manager (vpn.manage) sees only the peers and rules of the own organization, no generated table and no host administration service (`scope` organization). `four_eyes`: technicians' access needs a second person's approval (policy four_eyes_vpn); peers then carry `grant` (the open access window) and `pending_grant` (a request waiting for approval).
   * 
   * Requires one of the permissions `system.admin`, `vpn.manage`. */
  getVpn(): Promise<{
    /** configured, connected, error */
    agent?: Record<string, unknown>;
    buy_url?: string;
    /** policy four_eyes_vpn of the caller's organization */
    four_eyes?: boolean;
    /** peers allowed without the license */
    free_limit?: number;
    /** min and max length of an access grant */
    grant_minutes?: Record<string, unknown>;
    /** WireGuard state of the agent without peers, with firewall {applied, rules, forward, applied_at, error, ruleset} */
    interface?: Record<string, unknown>;
    licensed?: boolean;
    peers?: VPNPeer[];
    peers_used?: number;
    rules?: VPNRule[];
    scope?: "server" | "organization";
    services?: ({
      key?: string;
      name?: string;
      ports?: string;
      protocol?: string;
    })[];
    /** development agent: nothing is applied to a host */
    simulated?: boolean;
    /** the agent manages the VPN on this host (Linux: kernel WireGuard and nftables; Windows: userspace hub) */
    supported?: boolean;
  }> {
    return this.request<{
    /** configured, connected, error */
    agent?: Record<string, unknown>;
    buy_url?: string;
    /** policy four_eyes_vpn of the caller's organization */
    four_eyes?: boolean;
    /** peers allowed without the license */
    free_limit?: number;
    /** min and max length of an access grant */
    grant_minutes?: Record<string, unknown>;
    /** WireGuard state of the agent without peers, with firewall {applied, rules, forward, applied_at, error, ruleset} */
    interface?: Record<string, unknown>;
    licensed?: boolean;
    peers?: VPNPeer[];
    peers_used?: number;
    rules?: VPNRule[];
    scope?: "server" | "organization";
    services?: ({
      key?: string;
      name?: string;
      ports?: string;
      protocol?: string;
    })[];
    /** development agent: nothing is applied to a host */
    simulated?: boolean;
    /** the agent manages the VPN on this host (Linux: kernel WireGuard and nftables; Windows: userspace hub) */
    supported?: boolean;
  }>('GET', `/api/v1/vpn`, undefined, undefined, false);
  }

  /** Apply the stored VPN state again
   *
   * Sends peers and rules to the agent again (it applies the interface and the firewall together and rolls back on failure). The server does this on its own every five minutes and whenever an access expires.
   * 
   * Requires permission `system.admin`. */
  postVpnApply(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    firewall?: Record<string, unknown>;
    ok?: boolean;
  }> {
    return this.request<{
    firewall?: Record<string, unknown>;
    ok?: boolean;
  }>('POST', `/api/v1/vpn/apply`, undefined, body, false);
  }

  /** Add a VPN peer (configuration shown once)
   *
   * Kinds: `device` (sends data through the tunnel), `user` (a technician's laptop or phone), `site` (a router that connects its LAN: `subnets`). Without the license up to 5 peers of kinds device and user without an expiry (402 `license_required` otherwise). The private key is generated, returned once in `config` (and as a QR code; for a site also router templates) and never stored. Needs the caller's `password` again, and `code` from the authenticator app with two-factor sign-in (403 `step_up_required` / `step_up_failed`). A new peer reaches nothing until an access rule allows it. An organization's VPN manager adds peers of the own organization only. With four eyes for VPN access (policy four_eyes_vpn) a technician (`user`) is added disabled and gets access through an approved grant.
   * 
   * Requires one of the permissions `system.admin`, `vpn.manage`. */
  postVpnPeers(body: {
    /** authenticator code when two-factor sign-in is on */
    code?: string;
    /** linked device (optional; implies its organization) */
    device_id?: string;
    /** time-limited access: disabled automatically (license) */
    expires_at?: string;
    kind: "device" | "user" | "site";
    /** 1–63: letters, digits, space . _ - */
    name: string;
    /** ≤ 200 */
    note?: string;
    /** owner organization (optional; empty = the whole server) */
    org_id?: string;
    /** your password (step-up) */
    password: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    subnets?: string[];
  }): Promise<{
    allowed_ips?: string[];
    /** four eyes: the technician is added disabled; request a grant */
    approval_required?: boolean;
    client_template_keys?: string[];
    client_templates?: {
      /** Android and iPhone steps */
      mobile?: string;
      /** .conf for the WireGuard app for Windows */
      windows?: string;
    };
    /** wg-quick configuration with the private key (shown once) */
    config?: string;
    peer?: VPNPeer;
    /** data URL of a PNG QR code of the configuration */
    qr?: string;
    /** the server's VPN address */
    server?: string;
    template_keys?: string[];
    templates?: {
      edgerouter?: string;
      mikrotik?: string;
      openwrt?: string;
      opnsense?: string;
      pfsense?: string;
      rutos?: string;
      wg_quick?: string;
    };
  }> {
    return this.request<{
    allowed_ips?: string[];
    /** four eyes: the technician is added disabled; request a grant */
    approval_required?: boolean;
    client_template_keys?: string[];
    client_templates?: {
      /** Android and iPhone steps */
      mobile?: string;
      /** .conf for the WireGuard app for Windows */
      windows?: string;
    };
    /** wg-quick configuration with the private key (shown once) */
    config?: string;
    peer?: VPNPeer;
    /** data URL of a PNG QR code of the configuration */
    qr?: string;
    /** the server's VPN address */
    server?: string;
    template_keys?: string[];
    templates?: {
      edgerouter?: string;
      mikrotik?: string;
      openwrt?: string;
      opnsense?: string;
      pfsense?: string;
      rutos?: string;
      wg_quick?: string;
    };
  }>('POST', `/api/v1/vpn/peers`, undefined, body, false);
  }

  /** Change, disable or enable a VPN peer
   *
   * Fields that are present change. `expires_at` "" clears the expiry; enabling an expired peer needs a new expiry or none. New routed subnets and setting an expiry need the license; renaming, disabling, enabling and clearing never do. With four eyes for VPN access, enabling a technician or changing its expiry is refused (409 `approval_required`): request a grant.
   * 
   * Requires one of the permissions `system.admin`, `vpn.manage`. */
  patchVpnPeersId(id: string, body: {
    /** uuid or "" */
    device_id?: string;
    enabled?: boolean;
    /** RFC 3339 or "" */
    expires_at?: string;
    name?: string;
    note?: string;
    /** uuid or "" */
    org_id?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    subnets?: string[];
  }): Promise<{
    ok?: boolean;
    peer?: Record<string, unknown>;
  }> {
    return this.request<{
    ok?: boolean;
    peer?: Record<string, unknown>;
  }>('PATCH', `/api/v1/vpn/peers/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Time-limited access for a VPN peer
   *
   * Enables the peer — and every rule that names it — for `minutes` (15 to 43200); afterwards it is disabled again (audited `vpn.peer_expire`). A new grant replaces the open one. With four eyes for VPN access (policy four_eyes_vpn) a grant for a technician (`user` peer) is held as a change request (202, Settings → Approvals) and starts when a different user approves it. Needs the license (402 `license_required`).
   * 
   * Requires one of the permissions `system.admin`, `vpn.manage`. */
  postVpnPeersIdGrant(id: string, body: {
    /** length of the access window, 15 to 43200 */
    minutes: number;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    grant?: {
      /** the second person (four eyes) */
      approved_by?: string;
      ends_at?: string;
      id?: string;
      minutes?: number;
      peer?: string;
      peer_id?: string;
      requested_by?: string;
      starts_at?: string;
    };
    ok?: boolean;
  }> {
    return this.request<{
    grant?: {
      /** the second person (four eyes) */
      approved_by?: string;
      ends_at?: string;
      id?: string;
      minutes?: number;
      peer?: string;
      peer_id?: string;
      requested_by?: string;
      starts_at?: string;
    };
    ok?: boolean;
  }>('POST', `/api/v1/vpn/peers/${encodeURIComponent(id)}/grant`, undefined, body, false);
  }

  /** Remove a VPN peer
   *
   * Access ends at once. Rules with the peer as destination are removed; the peer is dropped from the sources of other rules (a rule left without sources is removed).
   * 
   * Requires one of the permissions `system.admin`, `vpn.manage`. */
  postVpnPeersIdRemove(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    ok?: boolean;
    rules_changed?: number;
    rules_removed?: number;
  }> {
    return this.request<{
    ok?: boolean;
    rules_changed?: number;
    rules_removed?: number;
  }>('POST', `/api/v1/vpn/peers/${encodeURIComponent(id)}/remove`, undefined, body, false);
  }

  /** New key pair for a VPN peer (configuration shown once)
   *
   * The old key stops working at once. Needs the password (and authenticator code) again.
   * 
   * Requires one of the permissions `system.admin`, `vpn.manage`. */
  postVpnPeersIdRotate(id: string, body: {
    code?: string;
    password: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    allowed_ips?: string[];
    config?: string;
    peer?: Record<string, unknown>;
    qr?: string;
    server?: string;
    /** site only */
    templates?: Record<string, unknown>;
  }> {
    return this.request<{
    allowed_ips?: string[];
    config?: string;
    peer?: Record<string, unknown>;
    qr?: string;
    server?: string;
    /** site only */
    templates?: Record<string, unknown>;
  }>('POST', `/api/v1/vpn/peers/${encodeURIComponent(id)}/rotate`, undefined, body, false);
  }

  /** Add a VPN access rule
   *
   * Default deny: peers reach only what a rule allows. Sources: `{type: all}`, `{type: kind, kind}`, `{type: peer, id}`, `{type: org, id}`. Destination: `server` (this server's VPN address, e.g. MQTT 1883), `peer` (a peer's VPN address) or `site` (the routed subnets of a site, or `dest_cidr` inside them, e.g. one PLC). Ports for tcp/udp: "102, 502, 8000-8100" (empty = all). An organization's VPN manager creates rules of the own organization: sources are its peers or the organization, destinations its peers and sites or the offered services of this server (tcp to their ports, or icmp).
   * 
   * Requires one of the permissions `system.admin`, `vpn.manage`. */
  postVpnRules(body: VPNRuleInput): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/vpn/rules`, undefined, body, false);
  }

  /** Change a VPN access rule
   *
   * Requires one of the permissions `system.admin`, `vpn.manage`. */
  putVpnRulesId(id: string, body: VPNRuleInput): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('PUT', `/api/v1/vpn/rules/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Remove a VPN access rule
   *
   * Requires one of the permissions `system.admin`, `vpn.manage`. */
  postVpnRulesIdRemove(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/vpn/rules/${encodeURIComponent(id)}/remove`, undefined, body, false);
  }
}
