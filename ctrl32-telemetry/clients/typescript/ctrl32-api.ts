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

export type ActiveAlarm = {
  asset?: string;
  asset_id?: string;
  datastream_id?: string;
  id?: string;
  key?: string;
  raised_at?: string;
  severity?: string;
  state?: string;
  value?: number | null;
};

export type Alarm = {
  acked_at?: string | null;
  acked_by?: string | null;
  class?: {
    ack_required?: boolean;
    code?: string;
    /** #rrggbb */
    color?: string;
    name?: string;
    /** 1 (highest) to 4 */
    priority?: number;
    sound?: "none" | "beep" | "siren";
  };
  cleared_at?: string | null;
  datastream_id?: string | null;
  id?: string;
  /** ISA-18.2 state */
  isa_state?: "unack_active" | "ack_active" | "unack_rtn" | "normal" | "shelved";
  /** datastream key, or input name for rule alarms */
  key?: string;
  /** the limit crossed */
  limit?: number | null;
  raised_at?: string;
  /** raised on backfilled data */
  retrospective?: boolean;
  severity?: "warning" | "action" | "critical";
  shelf?: {
    by?: string;
    reason?: string;
  };
  shelved_until?: string | null;
  source?: {
    /** asset name */
    asset?: string;
    /** automation rule name */
    rule?: string;
    /** rule type */
    type?: string;
    unit?: string;
  };
  state?: "active" | "acknowledged" | "suppressed" | "cleared";
  value?: number | null;
};

export type AlarmClass = {
  ack_required?: boolean;
  builtin?: boolean;
  code?: string;
  /** #rrggbb */
  color?: string;
  id?: string;
  name?: string;
  /** 1 (highest) to 4 */
  priority?: number;
  /** current alarm rules of the class */
  rules?: number;
  sound?: "none" | "beep" | "siren";
};

export type AlarmClassInput = {
  /** default true */
  ack_required?: boolean;
  /** 2 to 31 lower-case letters, digits or _ (create only) */
  code?: string;
  /** #rrggbb */
  color: string;
  name: string;
  /** 1 (highest) to 4 */
  priority: number;
  reason: string;
  sound?: "none" | "beep" | "siren";
};

export type AlarmRule = {
  action?: number | null;
  asset?: string;
  asset_id?: string;
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

export type AlarmSourceCount = {
  asset?: string;
  count?: number;
  datastream_id?: string | null;
  key?: string;
  rule?: string;
  /** of all alarms in the range, 0..1 */
  share?: number;
};

export type AlarmSummary = {
  /** all acknowledged */
  acknowledged?: boolean;
  count?: number;
  /** worst active alarm */
  severity?: "warning" | "action" | "critical";
};

export type Annotation = {
  category?: "note" | "expected_event" | "maintenance" | "calibration" | "sensor_fault" | "data_issue";
  created_at?: string;
  created_by?: string;
  datastream_id?: string;
  from?: string;
  id?: string;
  retract_reason?: string;
  retracted_at?: string;
  retracted_by?: string;
  text?: string;
  /** = from for one observation */
  to?: string;
};

export type Asset = {
  alarm?: AlarmSummary;
  attributes?: Record<string, unknown>;
  /** child assets */
  children?: number;
  created_at?: string;
  datastreams?: number;
  /** devices feeding its datastreams now */
  devices?: number;
  id?: string;
  /** newest value of its datastreams */
  last_at?: string | null;
  name?: string;
  parent_id?: string | null;
  site?: string;
  site_id?: string;
  type?: string;
};

export type AssetDevice = {
  connected?: boolean;
  external_id?: string;
  id?: string;
  keys?: string[];
  last_seen?: string | null;
  location?: unknown;
  name?: string;
  revoked_at?: string | null;
  transport?: string;
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
  /** the server can convert downloads to H.264 (ffmpeg installed) */
  export_h264?: boolean;
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
  /** video.manage only */
  note?: string;
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
  /** s7 */
  conn_type?: "pg" | "op" | "s7basic";
  /** virtual device the values arrive under */
  device_external_id?: string;
  /** opcua: opc.tcp://host:4840 */
  endpoint?: string;
  /** modbus */
  framing?: "tcp" | "rtuovertcp";
  /** modbus, s7 */
  host?: string;
  /** polling interval */
  interval_s?: number;
  /** s7: hex, e.g. 01.00 (LOGO!) */
  local_tsap?: string;
  /** opcua */
  mode?: "poll" | "subscribe";
  nodes?: (Record<string, unknown>)[];
  password?: string;
  /** s7 */
  plc?: "s7-300" | "s7-400" | "s7-1200" | "s7-1500" | "et200sp" | "logo" | "other";
  /** modbus, default 502; s7, default 102 */
  port?: number;
  /** modbus: register profile version the registers come from */
  profile_id?: string;
  /** modbus, informational */
  profile_name?: string;
  /** modbus, informational */
  profile_version?: number;
  /** opcua subscribe: 100..60000, default 1000 */
  publishing_ms?: number;
  /** s7: 0..7 (default from plc) */
  rack?: number;
  registers?: ModbusRegister[];
  /** s7: hex, e.g. 02.00 (LOGO!) */
  remote_tsap?: string;
  /** opcua */
  security_mode?: string;
  /** opcua */
  security_policy?: string;
  /** opcua: pin the server certificate (SHA-1 hex) */
  server_thumbprint?: string;
  /** s7: 0..31 (default from plc) */
  slot?: number;
  tags?: S7Tag[];
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
  aliases?: (Record<string, unknown>)[];
  /** faceplate type: {asset (the asset it was drawn for), prefix (beginning of its datastream keys)}; see GET /dashboards/{slug}/instance */
  faceplate?: Record<string, unknown>;
  filters?: (Record<string, unknown>)[];
  /** screen-v1 */
  format?: string;
  key_filters?: (Record<string, unknown>)[];
  /** {type: grid, columns: 12, row_height} */
  layout?: Record<string, unknown>;
  /** place in the picture tree of the operator view: {parent (slug), order 0..9999, label up to 24 characters}; absent = not in the tree */
  nav?: Record<string, unknown>;
  /** viewers of a public link and displays see the annotations of the charts (default false) */
  public_annotations?: boolean;
  states?: (Record<string, unknown>)[];
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
  range_max?: number | null;
  /** physical range (detail only); outside = sensor_fault */
  range_min?: number | null;
  unit?: string;
};

export type Device = {
  assets?: ({
    id?: string;
    name?: string;
  })[];
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
  transport?: "mqtt" | "http" | "lorawan" | "opcua" | "modbus" | "s7";
};

export type Display = {
  /** recordings may be played on the display */
  allow_playback?: boolean;
  /** current operator command */
  command?: Record<string, unknown>;
  content?: DisplayContent;
  created_at?: string;
  /** what the page reported in the last 3 minutes: page_version, screen, item, tiles, playing, problems [{camera, name, state, since_s}], kiosk, monitor */
  health?: Record<string, unknown>;
  health_at?: string;
  id?: string;
  /** its monitor (0-based) */
  kiosk_monitor?: number;
  /** kiosk PC it runs on (empty: none) */
  kiosk_pc?: string;
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

export type ESignature = {
  comment?: string;
  id?: string;
  /** server key */
  key_id?: string;
  meaning?: "authored" | "reviewed" | "approved" | "verified" | "responsible";
  method?: "password" | "password_totp";
  /** why it is not valid */
  problem?: string;
  /** SHA-256 of the signed record version */
  record_hash?: string;
  record_id?: string;
  record_type?: "report" | "audit_review";
  roles?: string[];
  signed_at?: string;
  /** time in the organization's time zone */
  signed_at_local?: string;
  /** user name */
  signer?: string;
  /** full name */
  signer_name?: string;
  tz?: string;
  valid?: boolean;
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

export type KioskPC = {
  /** the pending operation */
  command?: Record<string, unknown>;
  created_at?: string;
  displays?: ({
    id?: string;
    monitor?: number;
    name?: string;
    online?: boolean;
  })[];
  id?: string;
  last_ip?: string;
  last_seen_at?: string;
  mesh_node?: string;
  monitors?: number;
  name?: string;
  /** reported within 90 s */
  online?: boolean;
  /** the last report of its agent */
  report?: Record<string, unknown>;
  /** the answer to the last operation: seq, action, ok, message, at */
  result?: Record<string, unknown>;
  /** screenshots allowed */
  screenshots?: boolean;
  /** the PC runs them */
  scripts_current?: boolean;
  /** version of the scripts of this server */
  scripts_version?: string;
  server_url?: string;
  /** has a credential */
  set_up?: boolean;
  updated_at?: string;
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
  /** stored quality, or stale (computed, never stored) when a good value is older than 1.5 × the datastream's expected interval */
  quality?: string;
  /** older than 1.5 × the expected interval */
  stale?: boolean;
  /** the stored quality when quality shows stale */
  stored_quality?: string;
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
  /** days the session stays signed in on this device (Stay signed in); 0 = a normal session */
  remembered_days?: number;
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

export type OSJob = {
  actor?: string;
  error?: string;
  exit_code?: number;
  finished_at?: string;
  id?: string;
  kind?: "check" | "upgrade";
  /** tail of the log (state only) */
  log?: string;
  phase?: "running" | "done" | "failed";
  reason?: string;
  reboot_required?: boolean;
  started_at?: string;
  /** check: available; upgrade: upgraded or newly installed */
  upgrades?: number;
};

export type OSReboot = {
  actor?: string;
  /** when it reboots */
  at?: string;
  id?: string;
  reason?: string;
  requested_at?: string;
};

export type OSState = {
  booted_at?: string;
  checked_at?: string;
  checking?: boolean;
  history?: OSJob[];
  job?: OSJob;
  last?: OSJob;
  list_error?: string;
  /** package lists downloaded */
  lists_updated_at?: string;
  /** apt */
  package_manager?: string;
  /** why not */
  reason?: string;
  reboot?: OSReboot;
  reboot_packages?: string[];
  reboot_required?: boolean;
  security_count?: number;
  supported?: boolean;
  upgrade_count?: number;
  upgrades?: ({
    from?: string;
    name?: string;
    new?: boolean;
    origin?: string;
    security?: boolean;
    to?: string;
  })[];
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
  /** the app leaves out the pages that configure: camera management, camera settings, the setup of object counting (UI confinement, not a permission boundary) */
  view_only?: boolean;
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
  /** default false; omitted on update keeps the value */
  view_only?: boolean;
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

export type PhysicalRange = {
  /** highest value */
  max?: number | null;
  /** lowest value the sensor can measure */
  min?: number | null;
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
  /** audit chain head printed in the report */
  audit_seq?: number;
  bytes?: number;
  /** SHA-256 of data.json */
  data_hash?: string;
  /** SHA-256 of the PDF */
  file_hash?: string;
  generated_at?: string;
  generated_by?: string;
  id?: string;
  kind?: string;
  /** parameters of the report */
  scope?: Record<string, unknown>;
  signatures?: ({
    /** the report still has the signed hash */
    hash_matches?: boolean;
    meaning?: string;
    signed_at?: string;
    signer?: string;
    signer_name?: string;
  })[];
  template_id?: string;
  template_ver?: number;
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

export type S7Tag = {
  /** DB1.DBD0, DB1.DBX6.0, DB1.DBS20.20, MW10, M0.1, IW64, Q0.0, DB1,REAL4, VW100 (LOGO!) */
  address: string;
  /** route the value to another virtual device */
  device?: string;
  /** ^[a-z][a-z0-9_]{0,31}$ */
  key: string;
  /** write limit (scaled) */
  max?: number;
  /** write limit (scaled) */
  min?: number;
  offset?: number;
  scale?: number;
  /** default implied by the address (X bool, B byte, W int, D real, S string) */
  type?: "bool" | "byte" | "char" | "sint" | "usint" | "int" | "uint" | "word" | "dint" | "udint" | "dword" | "real" | "lreal" | "string";
  /** not for inputs */
  writable?: boolean;
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
  alarm?: AlarmSummary;
  assets?: number;
  created_at?: string;
  customer_id?: string | null;
  datastreams?: number;
  /** devices feeding its datastreams now */
  devices?: number;
  id?: string;
  location?: {
    lat?: number;
    lon?: number;
  } | null;
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

export type TelemetryKey = {
  asset?: string;
  asset_id?: string;
  datastream_id?: string;
  device?: string;
  /** asset page: the device feeding it now */
  device_id?: string;
  key?: string;
  kind?: string;
  last?: {
    at?: string;
    /** stored quality, or stale (computed) for a good value older than 1.5 × the expected interval */
    quality?: string;
    received_at?: string;
    stale?: boolean;
    stored_quality?: string;
    value?: number | null;
    value_json?: unknown;
  } | null;
  quantity?: string;
  /** assignment start */
  since?: string;
  source?: "datastream" | "unassigned" | "status";
  spark?: number[][];
  stats?: TelemetryStats;
  unit?: string;
};

export type TelemetryStats = {
  avg?: number | null;
  /** of them with quality sensor_fault (not in min/max/avg) */
  faults?: number;
  max?: number | null;
  min?: number | null;
  /** values in the range */
  n?: number;
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
  /** a site that is another telemetry server connected through an uplink */
  uplink?: boolean;
  /** the uplink uses the HTTPS transport */
  via_https?: boolean;
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

export type VPNUplink = {
  /** this server's address in the hub's VPN */
  address?: string;
  /** this server is connected to a hub */
  configured?: boolean;
  connected_at?: string;
  endpoint?: string;
  /** applied, rules, forward, applied_at, error, ruleset (table inet ctrl32_uplink, or the agent's rules text on Windows) */
  firewall?: Record<string, unknown>;
  /** the token carries the HTTPS transport */
  https_available?: boolean;
  hub_address?: string;
  hub_endpoint?: string;
  hub_public_key?: string;
  lan_forward?: string[];
  latest_handshake?: string;
  mode?: "kernel" | "userspace" | "simulation";
  /** this server's name on the hub */
  name?: string;
  /** handshake within 3 minutes */
  online?: boolean;
  /** this server's uplink key */
  public_key?: string;
  running?: boolean;
  rx_bytes?: number;
  subnets?: string[];
  transport?: "udp" | "https";
  /** HTTPS transport: connected, since, error */
  tunnel?: Record<string, unknown>;
  tunnel_host?: string;
  tx_bytes?: number;
  /** the hub's VPN range */
  vpn_net?: string;
};

export type VPNUplinkRuleInput = {
  /** lan: a host or subnet inside the offered networks (empty = all of them) */
  dest_cidr?: string;
  dest_type: "server" | "lan";
  /** default true */
  enabled?: boolean;
  /** 1–80 */
  name: string;
  /** tcp/udp: "443, 502"; empty = all */
  ports?: string;
  protocol: "tcp" | "udp" | "icmp" | "any";
  /** Reason for the change, written to the audit trail (required, 3–200 characters). */
  reason: string;
  /** an address or subnet inside the hub's VPN range; empty = the whole range */
  src_cidr?: string;
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
  /** the enlarged camera offers its recordings (users with video.playback) */
  playback_on_click?: boolean;
  screens?: WallScreen[];
  /** tiles above which automatic screens use the sub-stream */
  sub_stream_above?: number;
  updated_at?: string;
};

export type WallRect = {
  /** height in cells, 1–rows */
  h: number;
  /** width in cells, 1–cols */
  w: number;
  /** column of the top left cell, 0-based */
  x: number;
  /** row of the top left cell, 0-based */
  y: number;
};

export type WallScreen = {
  /** 1–8 */
  cols: number;
  /** ≤ 40 */
  name?: string;
  /** auto: sub-stream above the video setting wall_sub_stream_above */
  quality?: "auto" | "main" | "sub";
  rects?: WallRect[];
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

  /** Alarm classes
   *
   * The classes of the organization: the built-in Alarm (priority 1), Warning (2) and Message (4, no acknowledgement) and its own. The class decides the colour, the order, the sound and whether an alarm waits for an acknowledgement.
   * 
   * Requires permission `data.read`. */
  getAlarmClasses(): Promise<AlarmClass[]> {
    return this.request<AlarmClass[]>('GET', `/api/v1/alarm-classes`, undefined, undefined, false);
  }

  /** Create an alarm class
   *
   * Requires permission `config.write`. */
  postAlarmClasses(body: AlarmClassInput): Promise<{
    id?: string;
  }> {
    return this.request<{
    id?: string;
  }>('POST', `/api/v1/alarm-classes`, undefined, body, false);
  }

  /** Change an alarm class
   *
   * The code stays; `ack_required` left out keeps its value.
   * 
   * Requires permission `config.write`. */
  putAlarmClassesId(id: string, body: AlarmClassInput): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('PUT', `/api/v1/alarm-classes/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Delete an alarm class
   *
   * 409 for a built-in class and for a class used by alarm rules.
   * 
   * Requires permission `config.write`. */
  deleteAlarmClassesId(id: string, query: { reason: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/alarm-classes/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Current alarm rules
   *
   * Requires permission `data.read`. */
  getAlarmRules(query?: { datastream_id?: string; /** only rules of this datastream */ asset_id?: string; /** only rules of the datastreams of this asset */ }): Promise<AlarmRule[]> {
    return this.request<AlarmRule[]>('GET', `/api/v1/alarm-rules`, query, undefined, false);
  }

  /** Create an alarm rule (new version)
   *
   * A rule of the same type on the same datastream is superseded (its `valid_to` is set); the new row carries the next version number. With the policy `four_eyes_limits` the call answers 202 with a pending change request instead. Types: `high`/`low` (value at or above/below the limits), `low_battery` (battery level at or below the limits), `power_loss` (value 0 = power lost; no limits), `door_open` (value other than 0 = open; no limits), `comm_loss` (no new value for longer than the limits in seconds; without limits 1.5 × the expected interval), `sensor_fault` (a value with quality sensor_fault — flagged by the device with `fault` in its telemetry, or outside the datastream's physical range; no limits; the next value with another quality clears it). Value rules skip values with quality sensor_fault. The condition must hold for `delay_seconds` before the alarm is raised.
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

  /** Alarms
   *
   * Without `view` the newest 200 alarms. `view=current`: everything that needs attention — active alarms and alarms that returned to normal unacknowledged — the most important class first, unacknowledged before acknowledged (at most 500). `view=shelved`: shelved alarms. `view=history`: the alarms raised between `from` and `to` (default the last 24 hours, at most 1000), by class code and text. Every alarm carries its ISA-18.2 state (`isa_state`), class and source.
   * 
   * Requires permission `data.read`. */
  getAlarms(query?: { datastream_id?: string; view?: string; /** current, shelved or history */ from?: string; /** RFC 3339 (history) */ to?: string; /** RFC 3339 (history) */ class?: string; /** class code (history) */ q?: string; /** text in the source: asset, key or rule name (history) */ }): Promise<Alarm[]> {
    return this.request<Alarm[]>('GET', `/api/v1/alarms`, query, undefined, false);
  }

  /** Acknowledge several alarms
   *
   * Up to 500 alarms with one reason; alarms already acknowledged or of another organization are skipped. Each acknowledgement is written to the alarm log and the audit trail.
   * 
   * Requires permission `alarm.ack`. */
  postAlarmsAck(body: {
    ids: string[];
    /** at least 3 characters */
    reason: string;
  }): Promise<{
    /** how many were acknowledged now */
    acknowledged?: number;
  }> {
    return this.request<{
    /** how many were acknowledged now */
    acknowledged?: number;
  }>('POST', `/api/v1/alarms/ack`, undefined, body, false);
  }

  /** Alarm performance (ISA-18.2)
   *
   * Numbers of a range (default the last 7 days, at most 92): alarms in total, per hour and per day on average, the busiest hour, the mean time to acknowledge by a person, alarms per day (in the time zone `tz`), the ten most frequent alarms with their share, chattering alarms (three or more of one source within a minute), standing alarms (active for longer than 24 hours now) and the counts by class. Shelved alarms are left out of the load numbers.
   * 
   * Requires permission `data.read`. */
  getAlarmsStats(query?: { from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ tz?: string; /** IANA time zone of the days, default UTC */ }): Promise<{
    chattering?: AlarmSourceCount[];
    classes?: ({
      code?: string;
      color?: string;
      count?: number;
      name?: string;
    })[];
    days?: ({
      count?: number;
      /** YYYY-MM-DD */
      day?: string;
    })[];
    from?: string;
    /** seconds */
    mean_ack_s?: number | null;
    peak_hour?: number;
    per_day?: number;
    per_hour?: number;
    standing?: ({
      asset?: string;
      datastream_id?: string | null;
      hours?: number;
      key?: string;
      rule?: string;
    })[];
    /** 6 */
    target_per_hour?: number;
    to?: string;
    top?: AlarmSourceCount[];
    total?: number;
  }> {
    return this.request<{
    chattering?: AlarmSourceCount[];
    classes?: ({
      code?: string;
      color?: string;
      count?: number;
      name?: string;
    })[];
    days?: ({
      count?: number;
      /** YYYY-MM-DD */
      day?: string;
    })[];
    from?: string;
    /** seconds */
    mean_ack_s?: number | null;
    peak_hour?: number;
    per_day?: number;
    per_hour?: number;
    standing?: ({
      asset?: string;
      datastream_id?: string | null;
      hours?: number;
      key?: string;
      rule?: string;
    })[];
    /** 6 */
    target_per_hour?: number;
    to?: string;
    top?: AlarmSourceCount[];
    total?: number;
  }>('GET', `/api/v1/alarms/stats`, query, undefined, false);
  }

  /** Alarm summary for the alarm bar
   *
   * How many alarms wait for an acknowledgement, how many are active, how many are shelved, and the most important unacknowledged alarm (`top`, null when none). Shelved alarms are not counted as unacknowledged or active.
   * 
   * Requires permission `data.read`. */
  getAlarmsSummary(): Promise<{
    active?: number;
    shelved?: number;
    top?: Alarm;
    unack?: number;
  }> {
    return this.request<{
    active?: number;
    shelved?: number;
    top?: Alarm;
    unack?: number;
  }>('GET', `/api/v1/alarms/summary`, undefined, undefined, false);
  }

  /** Acknowledge an alarm
   *
   * An active alarm or one that returned to normal unacknowledged. Stops the escalation ladder; recorded in the alarm log with the actor and reason. 404 when the alarm is already acknowledged.
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

  /** Shelve an alarm
   *
   * Shelves the rule of the alarm for 15 minutes to 24 hours: its alarms that have not returned to normal, and every new one until then, are recorded (state `suppressed`, `isa_state` `shelved`) but not annunciated — no alarm bar, no notification, no escalation. The shelf ends by itself. 409 when already shelved.
   * 
   * Requires permission `alarm.ack`. */
  postAlarmsIdShelve(id: string, body: {
    /** 15 to 1440 */
    minutes: number;
    /** at least 3 characters */
    reason: string;
  }): Promise<{
    /** alarms shelved now */
    alarms?: number;
    shelved_until?: string;
  }> {
    return this.request<{
    /** alarms shelved now */
    alarms?: number;
    shelved_until?: string;
  }>('POST', `/api/v1/alarms/${encodeURIComponent(id)}/shelve`, undefined, body, false);
  }

  /** End the shelf of an alarm
   *
   * Alarms still active return to the current list (unacknowledged ones are annunciated again). 409 when the alarm is not shelved.
   * 
   * Requires permission `alarm.ack`. */
  postAlarmsIdUnshelve(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** alarms returned to the list */
    alarms?: number;
  }> {
    return this.request<{
    /** alarms returned to the list */
    alarms?: number;
  }>('POST', `/api/v1/alarms/${encodeURIComponent(id)}/unshelve`, undefined, body, false);
  }

  /** Annotations of a datastream
   *
   * Annotations overlapping `from`–`to` (default the last 30 days); retracted ones only with `include_retracted=1`.
   * 
   * Requires permission `data.read`. */
  getAnnotations(query: { datastream_id: string; from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ include_retracted?: string; /** 1 */ }): Promise<Annotation[]> {
    return this.request<Annotation[]>('GET', `/api/v1/annotations`, query, undefined, false);
  }

  /** Annotate observations
   *
   * A comment on one observation (`to` omitted or equal to `from`) or on a period of a datastream; the data does not change. Shown on charts and written to the observation exports. Audited.
   * 
   * Requires permission `data.annotate`. */
  postAnnotations(body: {
    /** default note */
    category?: "note" | "expected_event" | "maintenance" | "calibration" | "sensor_fault" | "data_issue";
    datastream_id: string;
    from: string;
    /** 1–1000 characters */
    text: string;
    /** optional, at most 366 days after from */
    to?: string;
  }): Promise<Annotation> {
    return this.request<Annotation>('POST', `/api/v1/annotations`, undefined, body, false);
  }

  /** Retract an annotation
   *
   * Annotations are records: a retraction is a new record with the reason; the retracted annotation is no longer shown or exported. Audited.
   * 
   * Requires permission `data.annotate`. */
  postAnnotationsIdRetract(id: string, body: {
    /** 3–1000 characters */
    reason: string;
  }): Promise<{
    id?: string;
    retraction_id?: string;
  }> {
    return this.request<{
    id?: string;
    retraction_id?: string;
  }>('POST', `/api/v1/annotations/${encodeURIComponent(id)}/retract`, undefined, body, false);
  }

  /** List assets
   *
   * Each asset with the number of its datastreams, of the devices feeding them now and of its child assets, its worst active alarm and the time of its newest value.
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

  /** An asset with everything around it
   *
   * The asset (as in the list), its site, its ancestors (the top-level asset first), its child assets, its datastreams with the latest value (quality stale when older than the expected interval allows), the source device and the number of current alarm rules, the devices feeding it, the active alarms and its map position (its own, else the site's).
   * 
   * Requires permission `data.read`. */
  getAssetsId(id: string): Promise<{
    alarms?: ActiveAlarm[];
    asset?: Asset;
    children?: Asset[];
    datastreams?: (Record<string, unknown>)[];
    devices?: AssetDevice[];
    location?: {
      lat?: number;
      lon?: number;
      /** asset | site */
      source?: string;
    } | null;
    path?: ({
      id?: string;
      name?: string;
      type?: string;
    })[];
    site?: Record<string, unknown>;
  }> {
    return this.request<{
    alarms?: ActiveAlarm[];
    asset?: Asset;
    children?: Asset[];
    datastreams?: (Record<string, unknown>)[];
    devices?: AssetDevice[];
    location?: {
      lat?: number;
      lon?: number;
      /** asset | site */
      source?: string;
    } | null;
    path?: ({
      id?: string;
      name?: string;
      type?: string;
    })[];
    site?: Record<string, unknown>;
  }>('GET', `/api/v1/assets/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Update an asset
   *
   * `parent_id` moves it under another asset of the same site ("" = top level; never under itself or an asset below it); `location` sets its map position (kept in the attributes as lat, lon), `clear_location` removes it.
   * 
   * Requires permission `config.write`. */
  patchAssetsId(id: string, body: {
    /** replaces the attributes */
    attributes?: Record<string, unknown>;
    clear_location?: boolean;
    location?: {
      lat?: number;
      lon?: number;
    };
    name?: string;
    /** parent asset, empty = top level */
    parent_id?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    type?: string;
  }): Promise<Asset> {
    return this.request<Asset>('PATCH', `/api/v1/assets/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Audit history of an asset
   *
   * Audit entries of the asset, its datastreams, their alarm rules and device assignments, newest first; page with `before` = `next_before`.
   * 
   * Requires permission `audit.read`. */
  getAssetsIdHistory(id: string, query?: { before?: string; /** sequence number */ limit?: number; /** at most 200, default 50 */ }): Promise<{
    entries?: ({
      action?: string;
      actor?: string;
      entity_id?: string;
      entity_type?: string;
      new_value?: unknown;
      old_value?: unknown;
      reason?: string;
      seq?: number;
      ts?: string;
    })[];
    /** empty on the last page */
    next_before?: string;
  }> {
    return this.request<{
    entries?: ({
      action?: string;
      actor?: string;
      entity_id?: string;
      entity_type?: string;
      new_value?: unknown;
      old_value?: unknown;
      reason?: string;
      seq?: number;
      ts?: string;
    })[];
    /** empty on the last page */
    next_before?: string;
  }>('GET', `/api/v1/assets/${encodeURIComponent(id)}/history`, query, undefined, false);
  }

  /** Every datastream of an asset
   *
   * Like the Telemetry tab of a device: each datastream with the latest value, its quality and time, statistics over `from`–`to` (default the last 24 hours, at most 366 days; sensor faults counted in `faults`, not in min/max/avg) and a sparkline of the last 24 hours. `children=1` adds the datastreams of every asset below it.
   * 
   * Requires permission `data.read`. */
  getAssetsIdTelemetry(id: string, query?: { from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ children?: string; /** 1 = include the assets below */ }): Promise<{
    asset_id?: string;
    /** assets included */
    assets?: number;
    children?: boolean;
    from?: string;
    generated_at?: string;
    keys?: TelemetryKey[];
    to?: string;
  }> {
    return this.request<{
    asset_id?: string;
    /** assets included */
    assets?: number;
    children?: boolean;
    from?: string;
    generated_at?: string;
    keys?: TelemetryKey[];
    to?: string;
  }>('GET', `/api/v1/assets/${encodeURIComponent(id)}/telemetry`, query, undefined, false);
  }

  /** Export the values of one datastream of an asset
   *
   * CSV or Excel, oldest first, at most 100 000 rows; the same columns as the device export. Audited against the asset.
   * 
   * Requires permission `data.export`. */
  getAssetsIdTelemetryExport(id: string, query: { datastream_id: string; from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ format?: string; /** csv (default) | xlsx */ }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/assets/${encodeURIComponent(id)}/telemetry/export`, query, undefined, true);
  }

  /** Chart of one datastream of an asset
   *
   * As `GET /api/v1/devices/{id}/telemetry/series` for a datastream of the asset or of an asset below it.
   * 
   * Requires permission `data.read`. */
  getAssetsIdTelemetrySeries(id: string, query: { datastream_id: string; from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ agg?: string; /** auto | none | avg | min | max */ bucket?: number; /** seconds, 0 = automatic */ }): Promise<{
    annotations?: Annotation[];
    /** 0 = raw points */
    bucket_s?: number;
    datastream_id?: string;
    from?: string;
    key?: string;
    points?: ({
      max?: number;
      min?: number;
      n?: number;
      quality?: string;
      t?: string;
      v?: number | null;
    })[];
    source?: string;
    stats?: TelemetryStats;
    to?: string;
    unit?: string;
  }> {
    return this.request<{
    annotations?: Annotation[];
    /** 0 = raw points */
    bucket_s?: number;
    datastream_id?: string;
    from?: string;
    key?: string;
    points?: ({
      max?: number;
      min?: number;
      n?: number;
      quality?: string;
      t?: string;
      v?: number | null;
    })[];
    source?: string;
    stats?: TelemetryStats;
    to?: string;
    unit?: string;
  }>('GET', `/api/v1/assets/${encodeURIComponent(id)}/telemetry/series`, query, undefined, false);
  }

  /** Values of one datastream of an asset, newest first
   *
   * Paged by `cursor` (from `next_cursor`), at most 500 a page (default 50).
   * 
   * Requires permission `data.read`. */
  getAssetsIdTelemetryValues(id: string, query: { datastream_id: string; from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ cursor?: string; limit?: number; }): Promise<{
    /** empty on the last page */
    next_cursor?: string;
    rows?: ({
      annotation?: string;
      at?: string;
      quality?: string;
      received_at?: string;
      time_source?: string;
      value?: number | null;
      value_json?: unknown;
    })[];
  }> {
    return this.request<{
    /** empty on the last page */
    next_cursor?: string;
    rows?: ({
      annotation?: string;
      at?: string;
      quality?: string;
      received_at?: string;
      time_source?: string;
      value?: number | null;
      value_json?: unknown;
    })[];
  }>('GET', `/api/v1/assets/${encodeURIComponent(id)}/telemetry/values`, query, undefined, false);
  }

  /** Audit trail of the organization
   *
   * Newest first, filtered by period (default the last 7 days), action, actor and entity type (substrings); `before` = the `next_before` of the previous page. Each entry shows the latest review that covered it.
   * 
   * Requires permission `audit.read`. */
  getAudit(query?: { from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ action?: string; actor?: string; entity_type?: string; before?: number; /** sequence number */ limit?: number; /** at most 500, default 100 */ }): Promise<{
    entries?: ({
      action?: string;
      actor?: string;
      actor_type?: string;
      entity_id?: string;
      entity_type?: string;
      hash?: string;
      new_value?: unknown;
      old_value?: unknown;
      reason?: string;
      /** latest review covering it: id, by, at, outcome */
      review?: Record<string, unknown>;
      seq?: number;
      ts?: string;
    })[];
    next_before?: string;
  }> {
    return this.request<{
    entries?: ({
      action?: string;
      actor?: string;
      actor_type?: string;
      entity_id?: string;
      entity_type?: string;
      hash?: string;
      new_value?: unknown;
      old_value?: unknown;
      reason?: string;
      /** latest review covering it: id, by, at, outcome */
      review?: Record<string, unknown>;
      seq?: number;
      ts?: string;
    })[];
    next_before?: string;
  }>('GET', `/api/v1/audit`, query, undefined, false);
  }

  /** Reviews of the audit trail
   *
   * Newest first, each with its signature and whether it is valid.
   * 
   * Requires permission `audit.read`. */
  getAuditReviews(query?: { limit?: number; /** at most 500, default 50 */ }): Promise<(Record<string, unknown>)[]> {
    return this.request<(Record<string, unknown>)[]>('GET', `/api/v1/audit/reviews`, query, undefined, false);
  }

  /** Record a review of the audit trail
   *
   * A period (`from`, `to`; ended, at most 366 days) or one entry (`seq`) of the organization's audit trail: the review records the first and last entry, their number and the hash of the last one, the outcome and a comment (required with findings), and is signed with the meaning reviewed. The reviewer confirms with the password and, with two-factor sign-in, the authenticator code; only in a browser session. Audited.
   * 
   * Requires permission `audit.review`. */
  postAuditReviews(body: {
    code?: string;
    /** at most 2000 characters */
    comment?: string;
    from?: string;
    outcome: "no_findings" | "findings";
    password: string;
    /** one entry instead of a period */
    seq?: number;
    to?: string;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/audit/reviews`, undefined, body, false);
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
    /** Stay signed in on this device: when the organization policy allows it (`remember_device_days`), the session lasts that many days with a persistent cookie; the response then carries `remembered_days` */
    remember?: boolean;
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
   * `kind` chirpstack: returns the ingest URL and a bearer secret (shown once) for the ChirpStack HTTP integration. `kind` opcua / modbus_tcp / s7: `settings` describe the endpoint and the nodes/registers/tags; values arrive under a virtual device (`settings.device_external_id`); OPC UA nodes and S7 tags with `device` feed another virtual device.
   * 
   * Requires permission `config.write`. */
  postConnectors(body: {
    /** chirpstack: register unknown DevEUIs automatically */
    auto_create_devices?: boolean;
    kind: "chirpstack" | "opcua" | "modbus_tcp" | "s7";
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    settings?: ConnectorSettings;
  }): Promise<ConnectorCreated> {
    return this.request<ConnectorCreated>('POST', `/api/v1/connectors`, undefined, body, false);
  }

  /** Browse the address space of an OPC UA server
   *
   * Children of a node (default the Objects folder i=85): objects and folders first, variables with data type, access (readable/writable) and current value; at most 2000 children. Uses the connection part of `settings` (endpoint, security, login); `id` reuses the stored password of a connector.
   * 
   * Requires permission `config.write`. */
  postConnectorsOpcuaBrowse(body: {
    /** existing connector id */
    id?: string;
    /** ns=2;s=Line1 (empty = i=85) */
    node_id?: string;
    settings: ConnectorSettings;
  }): Promise<{
    items?: ({
      browse_name?: string;
      data_type?: string;
      display_name?: string;
      has_children?: boolean;
      /** Object | Variable | Method | View */
      node_class?: string;
      node_id?: string;
      problem?: string;
      readable?: boolean;
      value?: Record<string, unknown>;
      writable?: boolean;
    })[];
    namespaces?: string[];
    node_id?: string;
    server_thumbprint?: string;
    truncated?: boolean;
  }> {
    return this.request<{
    items?: ({
      browse_name?: string;
      data_type?: string;
      display_name?: string;
      has_children?: boolean;
      /** Object | Variable | Method | View */
      node_class?: string;
      node_id?: string;
      problem?: string;
      readable?: boolean;
      value?: Record<string, unknown>;
      writable?: boolean;
    })[];
    namespaces?: string[];
    node_id?: string;
    server_thumbprint?: string;
    truncated?: boolean;
  }>('POST', `/api/v1/connectors/opcua/browse`, undefined, body, false);
  }

  /** This server's OPC UA client certificate (PEM)
   *
   * Trust it on the OPC UA server for signed/encrypted sessions.
   * 
   * Requires permission `config.write`. */
  getConnectorsOpcuaCertificate(): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/connectors/opcua/certificate`, undefined, undefined, true);
  }

  /** Security policies, modes and logins an OPC UA server offers
   *
   * GetEndpoints without a session; nothing is stored. `supported` tells whether this client can use the policy.
   * 
   * Requires permission `config.write`. */
  postConnectorsOpcuaEndpoints(body: {
    /** opc.tcp://host:4840/path */
    endpoint: string;
  }): Promise<{
    endpoint?: string;
    endpoints?: ({
      application_uri?: string;
      product_uri?: string;
      security_level?: number;
      /** None | Sign | SignAndEncrypt */
      security_mode?: string;
      security_policy?: string;
      server_name?: string;
      /** SHA-1 */
      server_thumbprint?: string;
      supported?: boolean;
      url?: string;
      user_tokens?: string[];
    })[];
  }> {
    return this.request<{
    endpoint?: string;
    endpoints?: ({
      application_uri?: string;
      product_uri?: string;
      security_level?: number;
      /** None | Sign | SignAndEncrypt */
      security_mode?: string;
      security_policy?: string;
      server_name?: string;
      /** SHA-1 */
      server_thumbprint?: string;
      supported?: boolean;
      url?: string;
      user_tokens?: string[];
    })[];
  }>('POST', `/api/v1/connectors/opcua/endpoints`, undefined, body, false);
  }

  /** Propose S7 tags from a TIA Portal or STEP 7 export
   *
   * multipart/form-data. Reads a PLC tag table of TIA Portal (.xlsx, .xml), a symbol table of STEP 7 (.sdf, .asc, .seq), a data block as source (.db, .awl, with its data types in the same file) or as Openness XML, or a table with name, address, type and comment (.csv), at most 8 MB. Variables become tags with a key made from the symbol, the absolute address and the data type; addresses of data blocks are calculated by the layout rules of blocks with standard access, optimized blocks are refused. Nothing is stored and no tag is writable: the answer is a proposal for the connector's tag list.
   * 
   * Requires permission `config.write`. */
  postConnectorsS7Import(body: FormData): Promise<{
    blocks?: ({
      name?: string;
      note?: string;
      number?: number;
      /** bytes */
      size?: number;
      skipped?: number;
      tags?: number;
    })[];
    file?: string;
    format?: string;
    /** tags one connector may read */
    max_tags?: number;
    skipped?: ({
      address?: string;
      name?: string;
      plc_type?: string;
      reason?: string;
    })[];
    tags?: ({
      /** DB5.DBD12, MW10 …; DB{db}.… when the block number is not known */
      address?: string;
      area?: "i" | "q" | "m" | "db";
      block?: string;
      comment?: string;
      key?: string;
      /** symbol in the project */
      name?: string;
      needs_db?: boolean;
      plc_type?: string;
      /** data type of the connector */
      type?: string;
    })[];
    warnings?: string[];
  }> {
    return this.request<{
    blocks?: ({
      name?: string;
      note?: string;
      number?: number;
      /** bytes */
      size?: number;
      skipped?: number;
      tags?: number;
    })[];
    file?: string;
    format?: string;
    /** tags one connector may read */
    max_tags?: number;
    skipped?: ({
      address?: string;
      name?: string;
      plc_type?: string;
      reason?: string;
    })[];
    tags?: ({
      /** DB5.DBD12, MW10 …; DB{db}.… when the block number is not known */
      address?: string;
      area?: "i" | "q" | "m" | "db";
      block?: string;
      comment?: string;
      key?: string;
      /** symbol in the project */
      name?: string;
      needs_db?: boolean;
      plc_type?: string;
      /** data type of the connector */
      type?: string;
    })[];
    warnings?: string[];
  }>('POST', `/api/v1/connectors/s7/import`, undefined, body, false);
  }

  /** Test connector settings without saving
   *
   * Connects once and reads the configured nodes/registers/tags (S7: also identifies the CPU).
   * 
   * Requires permission `config.write`. */
  postConnectorsTest(body: {
    /** existing connector id (reuse its stored password) */
    id?: string;
    /** default opcua */
    kind?: "opcua" | "modbus_tcp" | "s7";
    settings: ConnectorSettings;
  }): Promise<{
    bad_nodes?: string[];
    /** S7: order_code, module_type, module_name, plc_name, serial */
    cpu?: Record<string, unknown>;
    endpoint?: string;
    ok?: boolean;
    /** S7: negotiated PDU size */
    pdu?: number;
    /** OPC UA: negotiated policy */
    security?: string;
    /** OPC UA: SHA-1 of the server certificate */
    server_thumbprint?: string;
    /** key → value (device/key for nodes routed to another device) */
    values?: Record<string, unknown>;
  }> {
    return this.request<{
    bad_nodes?: string[];
    /** S7: order_code, module_type, module_name, plc_name, serial */
    cpu?: Record<string, unknown>;
    endpoint?: string;
    ok?: boolean;
    /** S7: negotiated PDU size */
    pdu?: number;
    /** OPC UA: negotiated policy */
    security?: string;
    /** OPC UA: SHA-1 of the server certificate */
    server_thumbprint?: string;
    /** key → value (device/key for nodes routed to another device) */
    values?: Record<string, unknown>;
  }>('POST', `/api/v1/connectors/test`, undefined, body, false);
  }

  /** Update connector settings (OPC UA, Modbus, S7)
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

  /** Detector installation by the server agent
   *
   * Whether the server agent can install the detector on this host, the pinned versions, the last installation and the running or last job with its progress (download x of y, unpack, verify). A finished job is applied to object counting at once (no restart).
   * 
   * Requires permission `system.admin`. */
  getCountingInstall(): Promise<{
    agent?: {
      configured?: boolean;
      connected?: boolean;
      error?: string;
    };
    /** last installation by the agent: time, versions, paths */
    installed?: Record<string, unknown>;
    job?: {
      action?: "install" | "remove";
      /** [analytics] paths written, null after a removal */
      config?: Record<string, unknown>;
      /** why it failed; nothing installed changed */
      error?: string;
      finished_at?: string;
      force?: boolean;
      id?: string;
      installed?: string[];
      kept?: string[];
      progress?: {
        bytes?: number;
        /** download item of items */
        item?: number;
        items?: number;
        /** the part */
        label?: string;
        step?: "prepare" | "download" | "unpack" | "verify" | "swap" | "done";
        /** 0 = unknown */
        total?: number;
      };
      started_at?: string;
      state?: "running" | "done" | "failed";
    };
    /** versions: onnxruntime, ffmpeg, ffmpeg_release, yolox */
    pins?: Record<string, unknown>;
    /** os/arch */
    platform?: string;
    /** development and tests: stand-in downloads */
    simulated?: boolean;
    /** Linux amd64/arm64 or Windows amd64 */
    supported?: boolean;
  }> {
    return this.request<{
    agent?: {
      configured?: boolean;
      connected?: boolean;
      error?: string;
    };
    /** last installation by the agent: time, versions, paths */
    installed?: Record<string, unknown>;
    job?: {
      action?: "install" | "remove";
      /** [analytics] paths written, null after a removal */
      config?: Record<string, unknown>;
      /** why it failed; nothing installed changed */
      error?: string;
      finished_at?: string;
      force?: boolean;
      id?: string;
      installed?: string[];
      kept?: string[];
      progress?: {
        bytes?: number;
        /** download item of items */
        item?: number;
        items?: number;
        /** the part */
        label?: string;
        step?: "prepare" | "download" | "unpack" | "verify" | "swap" | "done";
        /** 0 = unknown */
        total?: number;
      };
      started_at?: string;
      state?: "running" | "done" | "failed";
    };
    /** versions: onnxruntime, ffmpeg, ffmpeg_release, yolox */
    pins?: Record<string, unknown>;
    /** os/arch */
    platform?: string;
    /** development and tests: stand-in downloads */
    simulated?: boolean;
    /** Linux amd64/arm64 or Windows amd64 */
    supported?: boolean;
  }>('GET', `/api/v1/counting/install`, undefined, undefined, false);
  }

  /** Install the detector
   *
   * The server agent downloads ONNX Runtime, the LGPL build of ffmpeg and the YOLOX models pinned in this version from their official releases over HTTPS, keeps a file only when its SHA-256 matches the pin, refuses a GPL build of ffmpeg, swaps the parts in atomically (a failure changes nothing installed), writes [analytics] in config.toml (threads and max_cameras stay) and object counting uses them at once; the self-check then runs again. Installed parts are kept unless force. Asks for the password (and authenticator code) again, from a browser session only; audited as analytics.install (request and result). Camera counting pauses while it runs. 409 while another job runs.
   * 
   * Requires permission `system.admin`. */
  postCountingInstall(body: {
    /** authenticator code with two-factor sign-in */
    code?: string;
    /** repair: download and replace every part again */
    force?: boolean;
    /** the caller's password again */
    password: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    agent?: {
      configured?: boolean;
      connected?: boolean;
      error?: string;
    };
    /** last installation by the agent: time, versions, paths */
    installed?: Record<string, unknown>;
    job?: {
      action?: "install" | "remove";
      /** [analytics] paths written, null after a removal */
      config?: Record<string, unknown>;
      /** why it failed; nothing installed changed */
      error?: string;
      finished_at?: string;
      force?: boolean;
      id?: string;
      installed?: string[];
      kept?: string[];
      progress?: {
        bytes?: number;
        /** download item of items */
        item?: number;
        items?: number;
        /** the part */
        label?: string;
        step?: "prepare" | "download" | "unpack" | "verify" | "swap" | "done";
        /** 0 = unknown */
        total?: number;
      };
      started_at?: string;
      state?: "running" | "done" | "failed";
    };
    /** versions: onnxruntime, ffmpeg, ffmpeg_release, yolox */
    pins?: Record<string, unknown>;
    /** os/arch */
    platform?: string;
    /** development and tests: stand-in downloads */
    simulated?: boolean;
    /** Linux amd64/arm64 or Windows amd64 */
    supported?: boolean;
  }> {
    return this.request<{
    agent?: {
      configured?: boolean;
      connected?: boolean;
      error?: string;
    };
    /** last installation by the agent: time, versions, paths */
    installed?: Record<string, unknown>;
    job?: {
      action?: "install" | "remove";
      /** [analytics] paths written, null after a removal */
      config?: Record<string, unknown>;
      /** why it failed; nothing installed changed */
      error?: string;
      finished_at?: string;
      force?: boolean;
      id?: string;
      installed?: string[];
      kept?: string[];
      progress?: {
        bytes?: number;
        /** download item of items */
        item?: number;
        items?: number;
        /** the part */
        label?: string;
        step?: "prepare" | "download" | "unpack" | "verify" | "swap" | "done";
        /** 0 = unknown */
        total?: number;
      };
      started_at?: string;
      state?: "running" | "done" | "failed";
    };
    /** versions: onnxruntime, ffmpeg, ffmpeg_release, yolox */
    pins?: Record<string, unknown>;
    /** os/arch */
    platform?: string;
    /** development and tests: stand-in downloads */
    simulated?: boolean;
    /** Linux amd64/arm64 or Windows amd64 */
    supported?: boolean;
  }>('POST', `/api/v1/counting/install`, undefined, body, false);
  }

  /** Remove the detector
   *
   * The server agent removes the detector parts and their paths in [analytics]; counting with sensors keeps working. Password (and authenticator code) again, browser session only; audited as analytics.remove.
   * 
   * Requires permission `system.admin`. */
  deleteCountingInstall(body: {
    /** authenticator code with two-factor sign-in */
    code?: string;
    /** the caller's password again */
    password: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    agent?: {
      configured?: boolean;
      connected?: boolean;
      error?: string;
    };
    /** last installation by the agent: time, versions, paths */
    installed?: Record<string, unknown>;
    job?: {
      action?: "install" | "remove";
      /** [analytics] paths written, null after a removal */
      config?: Record<string, unknown>;
      /** why it failed; nothing installed changed */
      error?: string;
      finished_at?: string;
      force?: boolean;
      id?: string;
      installed?: string[];
      kept?: string[];
      progress?: {
        bytes?: number;
        /** download item of items */
        item?: number;
        items?: number;
        /** the part */
        label?: string;
        step?: "prepare" | "download" | "unpack" | "verify" | "swap" | "done";
        /** 0 = unknown */
        total?: number;
      };
      started_at?: string;
      state?: "running" | "done" | "failed";
    };
    /** versions: onnxruntime, ffmpeg, ffmpeg_release, yolox */
    pins?: Record<string, unknown>;
    /** os/arch */
    platform?: string;
    /** development and tests: stand-in downloads */
    simulated?: boolean;
    /** Linux amd64/arm64 or Windows amd64 */
    supported?: boolean;
  }> {
    return this.request<{
    agent?: {
      configured?: boolean;
      connected?: boolean;
      error?: string;
    };
    /** last installation by the agent: time, versions, paths */
    installed?: Record<string, unknown>;
    job?: {
      action?: "install" | "remove";
      /** [analytics] paths written, null after a removal */
      config?: Record<string, unknown>;
      /** why it failed; nothing installed changed */
      error?: string;
      finished_at?: string;
      force?: boolean;
      id?: string;
      installed?: string[];
      kept?: string[];
      progress?: {
        bytes?: number;
        /** download item of items */
        item?: number;
        items?: number;
        /** the part */
        label?: string;
        step?: "prepare" | "download" | "unpack" | "verify" | "swap" | "done";
        /** 0 = unknown */
        total?: number;
      };
      started_at?: string;
      state?: "running" | "done" | "failed";
    };
    /** versions: onnxruntime, ffmpeg, ffmpeg_release, yolox */
    pins?: Record<string, unknown>;
    /** os/arch */
    platform?: string;
    /** development and tests: stand-in downloads */
    simulated?: boolean;
    /** Linux amd64/arm64 or Windows amd64 */
    supported?: boolean;
  }>('DELETE', `/api/v1/counting/install`, undefined, body, false);
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

  /** Detector installation check
   *
   * Whether object counting on cameras can run on this server: ffmpeg (version, LGPL/GPL build), ONNX Runtime (version) and the YOLOX models (SHA-256 against the pins, one test inference). Checked in a separate process and cached for 5 minutes; refresh=1 checks again. Paths and technical details only for server administrators (system.admin). Installed with the button *Install the detector* (POST /api/v1/counting/install), `install.sh --analytics` or `install.ps1 -Analytics`.
   * 
   * Requires permission `data.read`. */
  getCountingSelfcheck(query?: { refresh?: string; /** 1 = check again now */ }): Promise<{
    checked?: string;
    components?: ({
      /** system.admin only */
      detail?: string;
      /** advice code */
      hint?: "install" | "gpl_build" | "sha_mismatch" | "platform" | "ort_version" | "model_broken";
      /** ffmpeg: LGPL, GPL or non-free */
      licence?: string;
      /** models: test inference time */
      ms?: number;
      name?: "ffmpeg" | "onnxruntime" | "model_nano" | "model_tiny" | "worker";
      /** system.admin only */
      path?: string;
      state?: "ok" | "warning" | "error" | "missing" | "not_configured";
      version?: string;
    })[];
    /** os/arch */
    platform?: string;
    /** cameras can count: ffmpeg, ONNX Runtime and at least one model work */
    ready?: boolean;
  }> {
    return this.request<{
    checked?: string;
    components?: ({
      /** system.admin only */
      detail?: string;
      /** advice code */
      hint?: "install" | "gpl_build" | "sha_mismatch" | "platform" | "ort_version" | "model_broken";
      /** ffmpeg: LGPL, GPL or non-free */
      licence?: string;
      /** models: test inference time */
      ms?: number;
      name?: "ffmpeg" | "onnxruntime" | "model_nano" | "model_tiny" | "worker";
      /** system.admin only */
      path?: string;
      state?: "ok" | "warning" | "error" | "missing" | "not_configured";
      version?: string;
    })[];
    /** os/arch */
    platform?: string;
    /** cameras can count: ffmpeg, ONNX Runtime and at least one model work */
    ready?: boolean;
  }>('GET', `/api/v1/counting/selfcheck`, query, undefined, false);
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
  getDashboardsStarter(query?: { kind?: string; /** overview (default), sensors (overview of all sensors), energy (energy overview), coldchain (cold chain overview), showcase, mimic (widget symbols on a canvas) or plant (mixing plant process picture) */ }): Promise<{
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

  /** Faceplate type bound to an instance
   *
   * A screen whose spec names `faceplate {asset, prefix}` is a faceplate type drawn for one asset. This returns its spec bound to another asset and/or key prefix: every tag of the example asset whose key starts with the type's prefix becomes the datastream of the instance asset with the key `prefix + rest`; a tag without a counterpart is unbound; devices and command keys follow, and an example device without a counterpart loses its commands. Without `asset` the asset of `tag` is used, without both the example itself; without `prefix` the type's prefix. 400 when the screen is not a faceplate type.
   * 
   * Requires permission `data.read`. */
  getDashboardsSlugInstance(slug: string, query?: { asset?: string; /** asset of the instance */ tag?: string; /** a datastream of the instance asset (used when asset is not given) */ prefix?: string; /** key prefix of the instance (a-z, 0-9, _) */ }): Promise<{
    instance?: {
      /** asset id */
      asset?: string;
      /** how many of them have a counterpart */
      bound?: number;
      /** asset name */
      name?: string;
      prefix?: string;
      /** tags of the example in the type */
      tags?: number;
    };
    name?: string;
    slug?: string;
    spec?: DashboardSpec;
  }> {
    return this.request<{
    instance?: {
      /** asset id */
      asset?: string;
      /** how many of them have a counterpart */
      bound?: number;
      /** asset name */
      name?: string;
      prefix?: string;
      /** tags of the example in the type */
      tags?: number;
    };
    name?: string;
    slug?: string;
    spec?: DashboardSpec;
  }>('GET', `/api/v1/dashboards/${encodeURIComponent(slug)}/instance`, query, undefined, false);
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
    range?: PhysicalRange;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    unit?: string;
  }): Promise<Datastream> {
    return this.request<Datastream>('POST', `/api/v1/datastreams`, undefined, body, false);
  }

  /** Export rows of an overview table
   *
   * The datastreams `ids` in this order (the filtered and sorted view of an overview table, at most 2000) with the chosen `columns` (site, asset, asset_type, device, datastream, quantity, value, measured, age, quality, alarm, limits, min, max, change; value adds a unit column) as `format` xlsx (default) or csv. min, max and change need `from`/`to`. Audited.
   * 
   * Requires permission `data.export`. */
  postDatastreamsExport(body: {
    columns?: string[];
    /** 0–6 */
    decimals?: number;
    format?: "xlsx" | "csv";
    from?: string;
    ids: string[];
    title?: string;
    to?: string;
  }): Promise<Response> {
    return this.request<Response>('POST', `/api/v1/datastreams/export`, undefined, body, true);
  }

  /** Choices of dashboard filters
   *
   * For each kind the values with the number of datastreams: `site`, `asset_type`, `quantity`, `device`, `attr:<key>` (values of an attribute) and `attr_keys` (attribute names). The other filters narrow the choices (a filter does not narrow its own).
   * 
   * Requires permission `data.read`. */
  getDatastreamsFacets(query?: { kinds?: string; /** comma-separated, default site,asset_type,quantity,device */ site?: string; /** comma-separated site ids */ asset_type?: string; /** comma-separated asset types (fridge, meter, …) */ quantity?: string; /** comma-separated quantities (temperature, energy, …) */ device?: string; /** comma-separated device ids (the device a datastream is assigned to) */ attr?: string; /** key=value of an asset or datastream attribute, repeatable (several values of one key: any of them) */ q?: string; /** free text: key, quantity, asset, asset type, site, device or name contains it */ name?: string; /** name pattern of asset / key, * and ? as wildcards (no wildcard = contains) */ ids?: string; /** comma-separated datastream ids (at most 500): only these */ f_site?: string; /** dashboard filter: like site */ f_asset_type?: string; /** dashboard filter */ f_quantity?: string; /** dashboard filter */ f_device?: string; /** dashboard filter */ f_attr?: string; /** dashboard filter, key=value, repeatable */ f_q?: string; /** dashboard filter: free text */ }): Promise<Record<string, ({
    label?: string;
    n?: number;
    value?: string;
  })[]>> {
    return this.request<Record<string, ({
    label?: string;
    n?: number;
    value?: string;
  })[]>>('GET', `/api/v1/datastreams/facets`, query, undefined, false);
  }

  /** Latest value of several datastreams
   *
   * Requires permission `data.read`. */
  getDatastreamsLatest(query: { ids: string; /** comma-separated datastream ids */ }): Promise<Record<string, LatestValue>> {
    return this.request<Record<string, LatestValue>>('GET', `/api/v1/datastreams/latest`, query, undefined, false);
  }

  /** Datastreams selected by a query (overviews)
   *
   * Datastreams of the organization matching sites, asset types, quantities, devices, attributes, free text and a name pattern — the dynamic data source of dashboard widgets and the filter bar of dashboards (0.68.0). Several values of one parameter match any of them; different parameters must all match; plain and `f_` (dashboard filter) parameters both apply. Ordered by site, asset and key. `latest=1` adds the latest value, the active alarm and the limits of the alarm rules; `from`/`to` add minimum, maximum and change (consumption of a meter) in that window.
   * 
   * Requires permission `data.read`. */
  getDatastreamsQuery(query?: { site?: string; /** comma-separated site ids */ asset_type?: string; /** comma-separated asset types (fridge, meter, …) */ quantity?: string; /** comma-separated quantities (temperature, energy, …) */ device?: string; /** comma-separated device ids (the device a datastream is assigned to) */ attr?: string; /** key=value of an asset or datastream attribute, repeatable (several values of one key: any of them) */ q?: string; /** free text: key, quantity, asset, asset type, site, device or name contains it */ name?: string; /** name pattern of asset / key, * and ? as wildcards (no wildcard = contains) */ ids?: string; /** comma-separated datastream ids (at most 500): only these */ f_site?: string; /** dashboard filter: like site */ f_asset_type?: string; /** dashboard filter */ f_quantity?: string; /** dashboard filter */ f_device?: string; /** dashboard filter */ f_attr?: string; /** dashboard filter, key=value, repeatable */ f_q?: string; /** dashboard filter: free text */ limit?: number; /** 1–500, default 100 */ latest?: string; /** 1: latest value, active alarm, limits */ from?: string; /** window start (RFC 3339) */ to?: string; /** window end (default now) */ }): Promise<{
    items?: ({
      /** active alarm {severity, count, acknowledged} */
      alarm?: Record<string, unknown>;
      asset?: string;
      asset_id?: string;
      asset_type?: string;
      device?: string;
      /** assigned device */
      device_id?: string;
      id?: string;
      /** expected interval */
      interval_s?: number;
      key?: string;
      kind?: string;
      /** display name (attribute name) */
      label?: string;
      /** {measured_at, value, value_json, quality, stale, stored_quality}; quality stale is computed (older than 1.5 × interval_s), never stored */
      latest?: Record<string, unknown> | null;
      /** alarm-rule limits {high_warn, high_action, low_warn, low_action} */
      limits?: Record<string, unknown>;
      quantity?: string;
      site?: string;
      site_id?: string;
      unit?: string;
      /** {min, max, first, last, change, n} between from and to */
      window?: Record<string, unknown>;
    })[];
    limit?: number;
    /** all matches */
    total?: number;
    /** more matches than limit */
    truncated?: boolean;
  }> {
    return this.request<{
    items?: ({
      /** active alarm {severity, count, acknowledged} */
      alarm?: Record<string, unknown>;
      asset?: string;
      asset_id?: string;
      asset_type?: string;
      device?: string;
      /** assigned device */
      device_id?: string;
      id?: string;
      /** expected interval */
      interval_s?: number;
      key?: string;
      kind?: string;
      /** display name (attribute name) */
      label?: string;
      /** {measured_at, value, value_json, quality, stale, stored_quality}; quality stale is computed (older than 1.5 × interval_s), never stored */
      latest?: Record<string, unknown> | null;
      /** alarm-rule limits {high_warn, high_action, low_warn, low_action} */
      limits?: Record<string, unknown>;
      quantity?: string;
      site?: string;
      site_id?: string;
      unit?: string;
      /** {min, max, first, last, change, n} between from and to */
      window?: Record<string, unknown>;
    })[];
    limit?: number;
    /** all matches */
    total?: number;
    /** more matches than limit */
    truncated?: boolean;
  }>('GET', `/api/v1/datastreams/query`, query, undefined, false);
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
   * `range` replaces the physical range (a null bound removes it); without `range` the range stays.
   * 
   * Requires permission `config.write`. */
  patchDatastreamsId(id: string, body: {
    accuracy?: number | null;
    expected_interval?: string;
    quantity?: string;
    range?: PhysicalRange;
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
    transport?: "mqtt" | "http" | "lorawan" | "opcua" | "modbus" | "s7";
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

  /** Everything the device sends
   *
   * Every key of the device: assigned datastreams (`source` datastream), keys without a datastream read from the raw messages of the last 90 days, at most 5000 messages (`unassigned`), and the keys of its status messages (`status`). Each with the latest value, its quality and time, statistics over `from`–`to` (default the last 24 hours, at most 366 days; values with quality sensor_fault are counted in `faults`, not in min/max/avg) and a sparkline of the last 24 hours (averages per 30 minutes).
   * 
   * Requires permission `data.read`. */
  getDevicesIdTelemetry(id: string, query?: { from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ }): Promise<{
    device_id?: string;
    from?: string;
    generated_at?: string;
    keys?: TelemetryKey[];
    raw_window_days?: number;
    to?: string;
  }> {
    return this.request<{
    device_id?: string;
    from?: string;
    generated_at?: string;
    keys?: TelemetryKey[];
    raw_window_days?: number;
    to?: string;
  }>('GET', `/api/v1/devices/${encodeURIComponent(id)}/telemetry`, query, undefined, false);
  }

  /** Create a datastream for a key and assign it
   *
   * For a key the device publishes without a datastream: a new datastream on the chosen asset, assigned to the device at once. Values stored from the next message; earlier ones stay in the raw messages. With the policy `four_eyes_assignments` the call answers 202 with a pending change request instead.
   * 
   * Requires permission `device.manage`. */
  postDevicesIdTelemetryAssign(id: string, body: {
    asset_id: string;
    /** the telemetry key */
    key: string;
    kind?: "gauge" | "counter" | "state" | "event";
    /** default: the key */
    quantity?: string;
    range?: {
      max?: number | null;
      min?: number | null;
    };
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    unit?: string;
  }): Promise<{
    asset_id?: string;
    assignment_id?: string;
    datastream_id?: string;
    key?: string;
    kind?: string;
    quantity?: string;
    unit?: string;
  }> {
    return this.request<{
    asset_id?: string;
    assignment_id?: string;
    datastream_id?: string;
    key?: string;
    kind?: string;
    quantity?: string;
    unit?: string;
  }>('POST', `/api/v1/devices/${encodeURIComponent(id)}/telemetry/assign`, undefined, body, false);
  }

  /** Export the values of one key
   *
   * CSV or Excel, oldest first, at most 100 000 rows; columns measured_at, received_at, value, value_json, unit, quality, time_source, annotation. Audited.
   * 
   * Requires permission `data.export`. */
  getDevicesIdTelemetryExport(id: string, query: { key: string; source?: string; /** datastream (default) | unassigned | status */ from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ format?: string; /** csv (default) | xlsx */ }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/devices/${encodeURIComponent(id)}/telemetry/export`, query, undefined, true);
  }

  /** Chart of one key
   *
   * Raw points while there are at most 5000 in the range (`agg` auto or none), otherwise buckets (`bucket` seconds, 0 = about 300 buckets) with the average, minimum and maximum; `agg` min or max shows that aggregate as the value. Datastream keys carry their annotations.
   * 
   * Requires permission `data.read`. */
  getDevicesIdTelemetrySeries(id: string, query: { key: string; source?: string; /** datastream (default) | unassigned | status */ from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ agg?: string; /** auto | none | avg | min | max */ bucket?: number; /** seconds, 0 = automatic */ }): Promise<{
    annotations?: Annotation[];
    /** 0 = raw points */
    bucket_s?: number;
    datastream_id?: string;
    from?: string;
    key?: string;
    points?: ({
      max?: number;
      min?: number;
      n?: number;
      quality?: string;
      t?: string;
      v?: number | null;
    })[];
    source?: string;
    stats?: TelemetryStats;
    to?: string;
    /** raw messages beyond the read limit were not read */
    truncated?: boolean;
    unit?: string;
  }> {
    return this.request<{
    annotations?: Annotation[];
    /** 0 = raw points */
    bucket_s?: number;
    datastream_id?: string;
    from?: string;
    key?: string;
    points?: ({
      max?: number;
      min?: number;
      n?: number;
      quality?: string;
      t?: string;
      v?: number | null;
    })[];
    source?: string;
    stats?: TelemetryStats;
    to?: string;
    /** raw messages beyond the read limit were not read */
    truncated?: boolean;
    unit?: string;
  }>('GET', `/api/v1/devices/${encodeURIComponent(id)}/telemetry/series`, query, undefined, false);
  }

  /** Values of one key, newest first
   *
   * Paged by `cursor` (from `next_cursor`), at most 500 a page (default 50).
   * 
   * Requires permission `data.read`. */
  getDevicesIdTelemetryValues(id: string, query: { key: string; source?: string; /** datastream (default) | unassigned | status */ from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ cursor?: string; limit?: number; }): Promise<{
    /** empty on the last page */
    next_cursor?: string;
    rows?: ({
      annotation?: string;
      /** measured (or received) time */
      at?: string;
      quality?: string;
      received_at?: string;
      time_source?: string;
      value?: number | null;
      value_json?: unknown;
    })[];
  }> {
    return this.request<{
    /** empty on the last page */
    next_cursor?: string;
    rows?: ({
      annotation?: string;
      /** measured (or received) time */
      at?: string;
      quality?: string;
      received_at?: string;
      time_source?: string;
      value?: number | null;
      value_json?: unknown;
    })[];
  }>('GET', `/api/v1/devices/${encodeURIComponent(id)}/telemetry/values`, query, undefined, false);
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

  /** Kiosk: play a recording
   *
   * WebSocket like /video/cameras/{id}/playback, for a display with `allow_playback` and a camera of its content, audited as the display. Masks stay on, there is no sound and a display has no way to download.
   * 
   * Requires permission `display`. */
  getDisplayCamerasIdPlayback(id: string, query?: { from?: string; /** RFC 3339, required */ speed?: string; /** 0.25–16, default 1 */ }): Promise<unknown> {
    return this.request<unknown>('GET', `/api/v1/display/cameras/${encodeURIComponent(id)}/playback`, query, undefined, false);
  }

  /** Kiosk: recorded spans
   *
   * Like /video/cameras/{id}/recordings, for a display with `allow_playback` and a camera of its content.
   * 
   * Requires permission `display`. */
  getDisplayCamerasIdRecordings(id: string, query?: { from?: string; /** RFC 3339, required */ to?: string; /** RFC 3339, default now */ }): Promise<{
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
  }>('GET', `/api/v1/display/cameras/${encodeURIComponent(id)}/recordings`, query, undefined, false);
  }

  /** Kiosk: what it shows
   *
   * The display page reports every 30 s (at most every 10 s, else 429): each camera tile with its state — playing (the picture moved on in the last 10 s), stalled (connected, the picture stands), connecting, offline (the camera), unsupported (the browser cannot play its codec) — and since when, the screen size, the version of the page, the title of what it shows, and the kiosk PC and monitor its launcher named in the address. Only cameras the display may show are kept. The Displays list shows it (`health`) for 3 minutes.
   * 
   * Requires permission `display`. */
  postDisplayHealth(body: {
    item?: string;
    /** kiosk PC from ?kiosk= */
    kiosk?: string;
    /** 0–7 from ?monitor= */
    monitor?: number;
    page_version?: string;
    /** w, h, vw, vh, dpr */
    screen?: Record<string, unknown>;
    tiles?: ({
      camera?: string;
      since_s?: number;
      state?: "playing" | "stalled" | "connecting" | "offline" | "unsupported";
    })[];
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/display/health`, undefined, body, false);
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
    /** version of the server; the display reloads its page when it changes (0.85.1) */
    server_version?: string;
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
    /** version of the server; the display reloads its page when it changes (0.85.1) */
    server_version?: string;
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
    /** anyone at the display may play the recordings of its cameras, without downloading; switching it on needs video.playback; off by default */
    allow_playback?: boolean;
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

  /** Entities of an alias, with a key filter
   *
   * Resolves an entity alias of a dashboard (0.73.0) within the organization: `alias` is the JSON of one alias — `{kind: single, entity: {type, id}}`, `{kind: list, entity_type, ids}`, `{kind: type, entity_type: device|asset|datastream, types}` (device profiles, asset types or quantities; `energy*` matches a prefix), `{kind: name, entity_type, match: starts|contains|ends, text}`, `{kind: hierarchy, root: {type: asset|site, id} or root_current: true, depth: 1–10, entity_type: device|asset|datastream, asset_type}`, `{kind: current, entity_type, default}` (the entity of the dashboard state given in `current`) or `{kind: query, source}` (a query source of 0.68.0). `filter` (JSON of a key filter `{match: all|any, rules: [{source: value|attribute|quality|stale|alarm, key, type: number|string|boolean, op: eq|ne|gt|ge|lt|le|between|contains|starts|ends|in, value, value2, dynamic: {from: filter|self|current|org, key}} or a group {match, rules}]}`) keeps only the matching entities; `highlight` (a key filter too) marks them with `match`. `want` answers with the entities, their datastreams (narrowed by `quantity` and `key`) or their devices. At most 2000 candidates, 500 rows.
   * 
   * Requires permission `data.read`. */
  getEntitiesQuery(query: { alias: string; /** JSON of the alias */ filter?: string; /** JSON of a key filter: only matching entities */ highlight?: string; /** JSON of a key filter: mark matching rows */ current?: string; /** type:id — the current entity of the dashboard state */ want?: string; /** entities (default), datastreams or devices */ quantity?: string; /** comma-separated quantities of the datastreams (want=datastreams|devices) */ key?: string; /** comma-separated datastream keys (want=datastreams|devices) */ values?: string; /** comma-separated datastream keys whose latest value each entity carries */ attrs?: string; /** comma-separated attribute names each entity carries (the entity's, else its asset's, else its site's) */ f_value?: string; /** key=value of a value filter of the dashboard (dynamic values of key filters), repeatable */ follow?: string; /** 1: narrow by the dashboard filters (f_site, f_asset_type, f_quantity, f_device, f_attr, f_q as in GET /api/v1/datastreams/query) */ limit?: number; /** 1–500, default 100 */ latest?: string; /** 1 (want=datastreams): latest value, alarm, limits */ from?: string; /** window start (want=datastreams) */ to?: string; /** window end */ }): Promise<{
    items?: (Record<string, unknown>)[];
    limit?: number;
    /** all matches */
    total?: number;
    /** more matches than limit */
    truncated?: boolean;
    /** entity type of the items: device, asset, datastream or site */
    type?: string;
  }> {
    return this.request<{
    items?: (Record<string, unknown>)[];
    limit?: number;
    /** all matches */
    total?: number;
    /** more matches than limit */
    truncated?: boolean;
    /** entity type of the items: device, asset, datastream or site */
    type?: string;
  }>('GET', `/api/v1/entities/query`, query, undefined, false);
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
   * `format` csv (default), jsonl or pdf (a signed report stored under Reports). Flagged rows (quality other than ok) are marked. The column `annotation` carries the comments of the annotations covering each observation (` | ` between several).
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
   * Served as image/svg+xml with a sandbox Content-Security-Policy. ?anim=1 animates the parts marked with c32-* effect classes (0.84.0).
   * 
   * Requires permission `data.read`. */
  getHmiObjectsSlugSvg(slug: string, query?: { anim?: string; /** 1 = add the server's style sheet for the c32-* effect classes */ speed?: string; /** effect speed 0.05–20, default 1 */ }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/hmi/objects/${encodeURIComponent(slug)}/svg`, query, undefined, true);
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

  /** Kiosk PC agent: report and get the pending operation
   *
   * Called by the agent on the PC every 15 s with its report (free form, at most 48 KiB) and the result of the last operation `{seq, action, ok, message}`; answers with the pending operation, the current version of the scripts and the poll interval. */
  postKioskReport(body: {
    /** hostname, windows, uptime_s, cpu_pct, gpu_decode_pct, ram_*, disk_*, browser, browser_windows, windows_updates, reboot_pending, scripts_version, launcher, … */
    report?: Record<string, unknown>;
    /** seq, action, ok, message */
    result?: Record<string, unknown>;
  }): Promise<{
    /** seq, action, by, until */
    command?: Record<string, unknown>;
    /** id, name, monitors */
    kiosk?: Record<string, unknown>;
    poll_s?: number;
    scripts_version?: string;
  }> {
    return this.request<{
    /** seq, action, by, until */
    command?: Record<string, unknown>;
    /** id, name, monitors */
    kiosk?: Record<string, unknown>;
    poll_s?: number;
    scripts_version?: string;
  }>('POST', `/api/v1/kiosk/report`, undefined, body, false);
  }

  /** Kiosk PC agent: deliver a screenshot
   *
   * Body image/jpeg or image/png, at most 8 MB, one picture per monitor (`monitor`, 0-based; without it monitor 0); accepted only for the screenshot asked for (`seq`), once per monitor, while the PC allows screenshots. */
  postKioskScreenshot(query: { seq: string; /** the operation */ monitor?: number; /** 0-based monitor of the PC */ }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/kiosk/screenshot`, query, undefined, false);
  }

  /** Kiosk PC agent: its scripts
   *
   * The agent, the launcher and the launcher settings of this PC (with `cdp_port`, the DevTools port of monitor 0 on 127.0.0.1 of the PC), with their version. `capture.ps1` is a stub kept for agents before 0.86.5, which refuse an update without it; the new agent deletes it. */
  getKioskScripts(): Promise<{
    /** name → text */
    files?: Record<string, unknown>;
    version?: string;
  }> {
    return this.request<{
    /** name → text */
    files?: Record<string, unknown>;
    version?: string;
  }>('GET', `/api/v1/kiosk/scripts`, undefined, undefined, false);
  }

  /** Kiosk PCs
   *
   * The Windows PCs behind the displays (0.86.0, ADR-0025) with the last report of their agent (PC, Windows, browser, load, monitors, Windows updates, launcher), the displays on their monitors, the pending operation and the last result, and whether their scripts are current.
   * 
   * Requires permission `display.manage`. */
  getKiosks(): Promise<KioskPC[]> {
    return this.request<KioskPC[]>('GET', `/api/v1/kiosks`, undefined, undefined, false);
  }

  /** Register a kiosk PC
   *
   * Returns the PC's credential once (`token`); its setup script (`POST /api/v1/kiosks/{id}/setup`) carries it. `server_url`: this server as the PC reaches it (empty: base_url). Audited as kiosk.create.
   * 
   * Requires permission `display.manage`. */
  postKiosks(body: {
    /** 1–8 */
    monitors?: number;
    /** ≤ 80 */
    name: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** http(s) address, e.g. http://10.0.0.5:8080 */
    server_url?: string;
  }): Promise<{
    kiosk?: KioskPC;
    /** credential of the PC's agent (shown once) */
    token?: string;
  }> {
    return this.request<{
    kiosk?: KioskPC;
    /** credential of the PC's agent (shown once) */
    token?: string;
  }>('POST', `/api/v1/kiosks`, undefined, body, false);
  }

  /** Change a kiosk PC
   *
   * Fields left out keep their value. `displays` links the displays to the monitors (one display per monitor, 0 = top left; replaces the links). `screenshots` allows screenshots of this PC — turning it on needs display.screenshot. `mesh_node`: the PC in MeshCentral (emergency remote desktop). Audited as kiosk.update.
   * 
   * Requires permission `display.manage`. */
  putKiosksId(id: string, body: {
    displays?: ({
      /** display */
      id?: string;
      /** 0-based */
      monitor?: number;
    })[];
    mesh_node?: string;
    /** 1–8 */
    monitors?: number;
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** off by default */
    screenshots?: boolean;
    server_url?: string;
  }): Promise<KioskPC> {
    return this.request<KioskPC>('PUT', `/api/v1/kiosks/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Remove a kiosk PC
   *
   * Its agent is locked out at once; its displays stay paired and are unlinked. Audited as kiosk.remove.
   * 
   * Requires permission `display.manage`. */
  deleteKiosksId(id: string, query?: { reason?: string; /** required */ }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('DELETE', `/api/v1/kiosks/${encodeURIComponent(id)}`, query, undefined, false);
  }

  /** Ask a kiosk PC for an operation
   *
   * A closed list the PC's agent performs within its poll (15 s): `report` (report now), `restart_browsers` (the launcher starts them again within 30 s), `reboot` (in 10 s), `update_scripts` (the agent and launcher of this server version), `screenshot` (needs display.screenshot and screenshots allowed for the PC; fetch it with GET …/screenshot). The operation waits 10 minutes for the PC. Audited as kiosk.command.
   * 
   * Requires permission `display.manage`. */
  postKiosksIdCommand(id: string, body: {
    action: "report" | "restart_browsers" | "reboot" | "update_scripts" | "screenshot";
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    action?: string;
    by?: string;
    /** ms */
    seq?: number;
    until?: string;
  }> {
    return this.request<{
    action?: string;
    by?: string;
    /** ms */
    seq?: number;
    until?: string;
  }>('POST', `/api/v1/kiosks/${encodeURIComponent(id)}/command`, undefined, body, false);
  }

  /** Open the emergency remote desktop of a kiosk PC
   *
   * The MeshCentral link to the PC (needs the MeshCentral address of the server and the PC's node). Audited as remote.mesh_open.
   * 
   * Requires permission `display.manage`. */
  postKiosksIdRemote(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    url?: string;
  }> {
    return this.request<{
    url?: string;
  }>('POST', `/api/v1/kiosks/${encodeURIComponent(id)}/remote`, undefined, body, false);
  }

  /** The screenshot asked for
   *
   * One picture per monitor, taken by the kiosk browser of that monitor (the display page only, not the rest of the desktop; since 0.86.5). 202 `{waiting: true, received, monitors}` until every monitor sent its picture or the agent reported its result; then the pictures once (they are not stored; after 3 minutes they are gone), with the agent's message when a monitor failed. 502 `capture_failed` with the agent's message when no monitor could be taken. Only for the user who asked for it. Before 0.86.5 this answered with one image/jpeg.
   * 
   * Requires permission `display.screenshot`. */
  getKiosksIdScreenshot(id: string): Promise<{
    /** the agent's result, e.g. which monitor failed */
    message?: string;
    monitors?: ({
      /** image/jpeg */
      content_type?: string;
      /** base64 */
      image?: string;
      /** 0-based */
      monitor?: number;
    })[];
    taken_at?: string;
  }> {
    return this.request<{
    /** the agent's result, e.g. which monitor failed */
    message?: string;
    monitors?: ({
      /** image/jpeg */
      content_type?: string;
      /** base64 */
      image?: string;
      /** 0-based */
      monitor?: number;
    })[];
    taken_at?: string;
  }>('GET', `/api/v1/kiosks/${encodeURIComponent(id)}/screenshot`, undefined, undefined, false);
  }

  /** Setup script of a kiosk PC
   *
   * The PowerShell setup script for the PC with its credential (the caller sends it: the server keeps only its hash). Run once as administrator on the PC: kiosk user with automatic logon (its password stays on the PC), power and lock screen, Windows Update at 03:00, browser policies, the launcher (one Chrome kiosk per monitor) and the agent as scheduled tasks.
   * 
   * Requires permission `display.manage`. */
  postKiosksIdSetup(id: string, body: {
    /** the PC's credential */
    token: string;
  }): Promise<Response> {
    return this.request<Response>('POST', `/api/v1/kiosks/${encodeURIComponent(id)}/setup`, undefined, body, true);
  }

  /** Replace the credential of a kiosk PC
   *
   * The old credential stops at once; the PC reports again after its setup script ran with the new one. Audited as kiosk.token.
   * 
   * Requires permission `display.manage`. */
  postKiosksIdToken(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** shown once */
    token?: string;
  }> {
    return this.request<{
    /** shown once */
    token?: string;
  }>('POST', `/api/v1/kiosks/${encodeURIComponent(id)}/token`, undefined, body, false);
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

  /** Operations log: commands and alarm events, newest first
   *
   * Who sent which command to which device (key, value, read-back, status, reason) and alarm events (raised, escalated, acknowledged with the actor and reason, cleared), merged by time. Optional filters: devices, keys (a trailing * matches a prefix), kinds (commands, alarms), before (paging).
   * 
   * Requires permission `data.read`. */
  getLogbook(query?: { devices?: string; /** comma-separated device ids */ keys?: string; /** comma-separated keys; a trailing * matches a prefix */ kinds?: string; /** commands, alarms or both */ limit?: string; /** 1..500, default 100 */ before?: string; /** RFC 3339 time, entries older than it */ }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('GET', `/api/v1/logbook`, query, undefined, false);
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

  /** Attributes of the organization
   *
   * Free attributes of the organization (for example `max_temp`), used as dynamic values of the key filters of dashboards.
   * 
   * Requires permission `data.read`. */
  getOrganizationAttributes(): Promise<{
    /** name → text, number or boolean */
    attributes?: Record<string, unknown>;
  }> {
    return this.request<{
    /** name → text, number or boolean */
    attributes?: Record<string, unknown>;
  }>('GET', `/api/v1/organization/attributes`, undefined, undefined, false);
  }

  /** Set the attributes of the organization
   *
   * Replaces all attributes (at most 50; names of letters, digits, . _ -; values text up to 200 characters, numbers or true/false). Audited.
   * 
   * Requires permission `config.write`. */
  putOrganizationAttributes(body: {
    /** name → text, number or boolean */
    attributes: Record<string, unknown>;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    attributes?: Record<string, unknown>;
  }> {
    return this.request<{
    attributes?: Record<string, unknown>;
  }>('PUT', `/api/v1/organization/attributes`, undefined, body, false);
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

  /** Annotations of a bound datastream
   *
   * Only when the dashboard owner enabled `public_annotations` in the spec; otherwise an empty list. Time, category and text, without the author. */
  getPublicTokenAnnotations(token: string, query: { datastream_id: string; from?: string; /** RFC 3339 */ to?: string; /** RFC 3339 */ }): Promise<({
    category?: string;
    datastream_id?: string;
    from?: string;
    id?: string;
    text?: string;
    to?: string;
  })[]> {
    return this.request<({
    category?: string;
    datastream_id?: string;
    from?: string;
    id?: string;
    text?: string;
    to?: string;
  })[]>('GET', `/api/v1/public/${encodeURIComponent(token)}/annotations`, query, undefined, false);
  }

  /** Datastreams bound in the shared dashboard */
  getPublicTokenDatastreams(token: string): Promise<Datastream[]> {
    return this.request<Datastream[]>('GET', `/api/v1/public/${encodeURIComponent(token)}/datastreams`, undefined, undefined, false);
  }

  /** Choices of the public filters
   *
   * Like `GET /api/v1/datastreams/facets`, only for filters the owner marked public and only within the dashboard's scope. */
  getPublicTokenDatastreamsFacets(token: string, query?: { kinds?: string; f_site?: string; f_asset_type?: string; f_quantity?: string; f_device?: string; f_attr?: string; f_q?: string; }): Promise<Record<string, ({
    label?: string;
    n?: number;
    value?: string;
  })[]>> {
    return this.request<Record<string, ({
    label?: string;
    n?: number;
    value?: string;
  })[]>>('GET', `/api/v1/public/${encodeURIComponent(token)}/datastreams/facets`, query, undefined, false);
  }

  /** Latest values (bound datastreams only) */
  getPublicTokenDatastreamsLatest(token: string, query: { ids: string; }): Promise<Record<string, LatestValue>> {
    return this.request<Record<string, LatestValue>>('GET', `/api/v1/public/${encodeURIComponent(token)}/datastreams/latest`, query, undefined, false);
  }

  /** Datastreams of a widget of the shared dashboard
   *
   * The datastreams of the widget `widget` — its query source or its bound datastreams — within the dashboard's scope, narrowed by the dashboard filters when the widget follows them: a filter marked public takes the viewer's `f_` value, any other filter its default. Answer as `GET /api/v1/datastreams/query`. */
  getPublicTokenDatastreamsQuery(token: string, query: { widget: string; /** widget id */ f_site?: string; f_asset_type?: string; f_quantity?: string; f_device?: string; f_attr?: string; /** key=value, repeatable */ f_q?: string; limit?: number; latest?: string; /** 1 */ from?: string; to?: string; }): Promise<{
    items?: ({
      /** active alarm {severity, count, acknowledged} */
      alarm?: Record<string, unknown>;
      asset?: string;
      asset_id?: string;
      asset_type?: string;
      device?: string;
      /** assigned device */
      device_id?: string;
      id?: string;
      /** expected interval */
      interval_s?: number;
      key?: string;
      kind?: string;
      /** display name (attribute name) */
      label?: string;
      /** {measured_at, value, value_json, quality, stale, stored_quality}; quality stale is computed (older than 1.5 × interval_s), never stored */
      latest?: Record<string, unknown> | null;
      /** alarm-rule limits {high_warn, high_action, low_warn, low_action} */
      limits?: Record<string, unknown>;
      quantity?: string;
      site?: string;
      site_id?: string;
      unit?: string;
      /** {min, max, first, last, change, n} between from and to */
      window?: Record<string, unknown>;
    })[];
    limit?: number;
    /** all matches */
    total?: number;
    /** more matches than limit */
    truncated?: boolean;
  }> {
    return this.request<{
    items?: ({
      /** active alarm {severity, count, acknowledged} */
      alarm?: Record<string, unknown>;
      asset?: string;
      asset_id?: string;
      asset_type?: string;
      device?: string;
      /** assigned device */
      device_id?: string;
      id?: string;
      /** expected interval */
      interval_s?: number;
      key?: string;
      kind?: string;
      /** display name (attribute name) */
      label?: string;
      /** {measured_at, value, value_json, quality, stale, stored_quality}; quality stale is computed (older than 1.5 × interval_s), never stored */
      latest?: Record<string, unknown> | null;
      /** alarm-rule limits {high_warn, high_action, low_warn, low_action} */
      limits?: Record<string, unknown>;
      quantity?: string;
      site?: string;
      site_id?: string;
      unit?: string;
      /** {min, max, first, last, change, n} between from and to */
      window?: Record<string, unknown>;
    })[];
    limit?: number;
    /** all matches */
    total?: number;
    /** more matches than limit */
    truncated?: boolean;
  }>('GET', `/api/v1/public/${encodeURIComponent(token)}/datastreams/query`, query, undefined, false);
  }

  /** Devices shown in the shared dashboard */
  getPublicTokenDevices(token: string): Promise<Device[]> {
    return this.request<Device[]>('GET', `/api/v1/public/${encodeURIComponent(token)}/devices`, undefined, undefined, false);
  }

  /** Entities of a widget of the shared dashboard
   *
   * The alias, key filter, highlight and columns of the widget `widget` come from the stored dashboard, never from the request. Everything stays within the dashboard's scope (what its aliases select without key filters, its bound and query datastreams); `current` (the entity of the viewer's dashboard state) is ignored outside it. Value filters marked public take the viewer's `f_value`, others their default. Answer as `GET /api/v1/entities/query`. */
  getPublicTokenEntitiesQuery(token: string, query: { widget: string; /** widget id */ current?: string; /** type:id */ f_site?: string; f_asset_type?: string; f_quantity?: string; f_device?: string; f_attr?: string; /** key=value, repeatable */ f_q?: string; f_value?: string; /** key=value, repeatable */ limit?: number; latest?: string; /** 1 */ from?: string; to?: string; }): Promise<{
    items?: (Record<string, unknown>)[];
    limit?: number;
    /** all matches */
    total?: number;
    /** more matches than limit */
    truncated?: boolean;
    /** entity type of the items: device, asset, datastream or site */
    type?: string;
  }> {
    return this.request<{
    items?: (Record<string, unknown>)[];
    limit?: number;
    /** all matches */
    total?: number;
    /** more matches than limit */
    truncated?: boolean;
    /** entity type of the items: device, asset, datastream or site */
    type?: string;
  }>('GET', `/api/v1/public/${encodeURIComponent(token)}/entities/query`, query, undefined, false);
  }

  /** A library graphic shown by the shared picture
   *
   * Only graphics that the linked process picture references. ?anim=1 animates the parts marked with c32-* effect classes (0.84.0). */
  getPublicTokenHmiObjectsSlugSvg(token: string, slug: string, query?: { anim?: string; /** 1 = add the server's style sheet for the c32-* effect classes */ speed?: string; /** effect speed 0.05–20, default 1 */ }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/public/${encodeURIComponent(token)}/hmi/objects/${encodeURIComponent(slug)}/svg`, query, undefined, true);
  }

  /** Logbook of a screen on a paired display
   *
   * Only for a paired display of the organization (token ~<slug>); public links get 403. Scope: the devices of the screen's logbook widgets. */
  getPublicTokenLogbook(token: string, query?: { devices?: string; keys?: string; kinds?: string; limit?: string; }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('GET', `/api/v1/public/${encodeURIComponent(token)}/logbook`, query, undefined, false);
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
   * `start_url` must be a path of this application (`/video`, `/video/walls/{id}`, `/dashboards/{slug}` …): no other site, no `//`, no `javascript:`, not `/api/v1/` or `/app/`. `sections`: keys from `GET /api/v1/pwa-apps/options`, at least one. `warnings` names a short name longer than 12 characters, and the start page of a locked app outside its sections. `locked`, `page_zoom`, `camera_layout` (auto, single, grid), `view_only`: see the PWAApp schema.
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
    view_only?: boolean;
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
    view_only?: boolean;
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

  /** Electronic signatures of a report
   *
   * Each signature with its manifestation (name, user name, roles, time in the organization's time zone, meaning) and whether it is valid: the manifest matches, the server's Ed25519 signature verifies and the stored PDF still has the signed hash.
   * 
   * Requires permission `data.export`. */
  getReportsIdSignatures(id: string): Promise<{
    file_hash?: string;
    /** the stored file matches its recorded hash */
    file_ok?: boolean;
    problem?: string;
    report_id?: string;
    signatures?: ESignature[];
  }> {
    return this.request<{
    file_hash?: string;
    /** the stored file matches its recorded hash */
    file_ok?: boolean;
    problem?: string;
    report_id?: string;
    signatures?: ESignature[];
  }>('GET', `/api/v1/reports/${encodeURIComponent(id)}/signatures`, undefined, undefined, false);
  }

  /** Sign a report electronically
   *
   * The signer confirms with the password and, with two-factor sign-in, the authenticator code (5 failures a minute at most). Only in a browser session: API tokens and AI assistants cannot sign. The signature covers the SHA-256 of the stored PDF; the same meaning twice by the same user is refused (409). Audited.
   * 
   * Requires permission `report.sign`. */
  postReportsIdSignatures(id: string, body: {
    /** authenticator code, with two-factor sign-in */
    code?: string;
    /** at most 500 characters */
    comment?: string;
    meaning: "authored" | "reviewed" | "approved" | "verified" | "responsible";
    password: string;
  }): Promise<ESignature> {
    return this.request<ESignature>('POST', `/api/v1/reports/${encodeURIComponent(id)}/signatures`, undefined, body, false);
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

  /** Picture tree of the operator view with group alarms
   *
   * The screens that name their place in `spec.nav {parent, order, label}`, depth-first (siblings by order, then name). Each node carries the alarms needing attention (not shelved) on the datastreams its picture uses and on those of every picture below it: `unack` wait for an acknowledgement, `active` have their condition active, `color` and `priority` are those of the most important one (an unacknowledged alarm first, then the class priority). `home` is the default screen when it is in the tree, else the first top-level picture.
   * 
   * Requires permission `data.read`. */
  getScreensTree(): Promise<{
    /** slug of the start picture, empty when the tree is empty */
    home?: string;
    nodes?: ({
      active?: number;
      children?: number;
      /** class colour of the most important alarm, empty when none */
      color?: string;
      /** 1 = top level */
      depth?: number;
      /** dashboard | mimic */
      kind?: string;
      /** button label */
      label?: string;
      name?: string;
      order?: number;
      /** slug of the parent, empty at the top level */
      parent?: string;
      /** class priority of the most important alarm, 0 when none */
      priority?: number;
      slug?: string;
      unack?: number;
    })[];
  }> {
    return this.request<{
    /** slug of the start picture, empty when the tree is empty */
    home?: string;
    nodes?: ({
      active?: number;
      children?: number;
      /** class colour of the most important alarm, empty when none */
      color?: string;
      /** 1 = top level */
      depth?: number;
      /** dashboard | mimic */
      kind?: string;
      /** button label */
      label?: string;
      name?: string;
      order?: number;
      /** slug of the parent, empty at the top level */
      parent?: string;
      /** class priority of the most important alarm, 0 when none */
      priority?: number;
      slug?: string;
      unack?: number;
    })[];
  }>('GET', `/api/v1/screens/tree`, undefined, undefined, false);
  }

  /** Search everything by name
   *
   * Devices, datastreams, assets, sites, dashboards, cameras, flows, incidents and users of the organization whose name (or external id, key, address, title, e-mail) contains `q`. Each kind only with the permission that shows it (data.read; cameras video.view and the caller's camera groups; users user.admin). At most `limit` hits per kind; `more` lists kinds with further hits.
   * 
   * Requires a signed-in user or an API token (any permission). */
  getSearch(query: { q: string; /** 1–100 characters */ limit?: number; /** hits per kind, 1–20, default 5 */ kinds?: string; /** comma-separated subset: device, datastream, asset, site, dashboard, camera, flow, incident, user */ }): Promise<{
    more?: string[];
    q?: string;
    results?: ({
      id?: string;
      kind?: string;
      subtitle?: string;
      title?: string;
      /** page of the object */
      url?: string;
    })[];
  }> {
    return this.request<{
    more?: string[];
    q?: string;
    results?: ({
      id?: string;
      kind?: string;
      subtitle?: string;
      title?: string;
      /** page of the object */
      url?: string;
    })[];
  }>('GET', `/api/v1/search`, query, undefined, false);
  }

  /** Server status and configuration summary
   *
   * Version, listeners, database, SMTP, storage paths, connected MQTT clients, update availability (`update`: current, available, release_date of this version, updates_until and updates_ended of the license).
   * 
   * Requires permission `system.admin`. */
  getServer(): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/server`, undefined, undefined, false);
  }

  /** Backups, schedule, offsite target, restore test and the history of runs
   *
   * Requires permission `system.admin`. */
  getServerBackups(): Promise<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }> {
    return this.request<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }>('GET', `/api/v1/server/backups`, undefined, undefined, false);
  }

  /** Start a backup now (copied to the offsite target when one is enabled)
   *
   * Requires permission `system.admin`. */
  postServerBackups(body: {
    /** encrypt this backup (blank: unencrypted, it then stays on the server) */
    passphrase?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** encrypt with the passphrase of the schedule */
    use_stored?: boolean;
  }): Promise<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }> {
    return this.request<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }>('POST', `/api/v1/server/backups`, undefined, body, false);
  }

  /** Set or remove the offsite target (folder, SFTP, S3); an enabled target is tested first. Browser session only, with the password (and authenticator code) again
   *
   * Requires permission `system.admin`. */
  putServerBackupsOffsite(body: {
    /** authenticator code (two-factor sign-in) */
    code?: string;
    config: {
      /** s3 */
      access_key?: string;
      /** s3 */
      bucket?: string;
      enabled?: boolean;
      /** s3: https URL */
      endpoint?: string;
      /** sftp */
      host?: string;
      /** sftp: pinned SHA256:… fingerprint (required) */
      host_key?: string;
      /** folder, sftp or s3 (blank removes the target) */
      kind?: string;
      /** sftp, write-only */
      password?: string;
      /** folder: absolute path (a mounted SMB/NFS share or a UNC path); sftp: remote directory; s3: key prefix */
      path?: string;
      /** s3: https://endpoint/bucket/key */
      path_style?: boolean;
      /** sftp, default 22 */
      port?: number;
      /** sftp, unencrypted OpenSSH/PEM key, write-only */
      private_key?: string;
      /** s3 */
      region?: string;
      /** s3, write-only */
      secret_key?: string;
      /** sftp */
      user?: string;
    };
    /** the caller's password */
    password: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }> {
    return this.request<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }>('PUT', `/api/v1/server/backups/offsite`, undefined, body, false);
  }

  /** Copy the backups the offsite target misses and apply the retention there (in the background)
   *
   * Requires permission `system.admin`. */
  postServerBackupsOffsiteSync(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }> {
    return this.request<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }>('POST', `/api/v1/server/backups/offsite/sync`, undefined, body, false);
  }

  /** Test an offsite target: a probe file is written, read back and deleted (stored secrets fill blank ones for the same place)
   *
   * Requires permission `system.admin`. */
  postServerBackupsOffsiteTest(body: {
    config: {
      /** s3 */
      access_key?: string;
      /** s3 */
      bucket?: string;
      enabled?: boolean;
      /** s3: https URL */
      endpoint?: string;
      /** sftp */
      host?: string;
      /** sftp: pinned SHA256:… fingerprint (required) */
      host_key?: string;
      /** folder, sftp or s3 (blank removes the target) */
      kind?: string;
      /** sftp, write-only */
      password?: string;
      /** folder: absolute path (a mounted SMB/NFS share or a UNC path); sftp: remote directory; s3: key prefix */
      path?: string;
      /** s3: https://endpoint/bucket/key */
      path_style?: boolean;
      /** sftp, default 22 */
      port?: number;
      /** sftp, unencrypted OpenSSH/PEM key, write-only */
      private_key?: string;
      /** s3 */
      region?: string;
      /** s3, write-only */
      secret_key?: string;
      /** sftp */
      user?: string;
    };
  }): Promise<{
    detail?: string;
    /** SFTP: the fingerprint the server presented (pin it after comparing) */
    host_key?: string;
    ok?: boolean;
  }> {
    return this.request<{
    detail?: string;
    /** SFTP: the fingerprint the server presented (pin it after comparing) */
    host_key?: string;
    ok?: boolean;
  }>('POST', `/api/v1/server/backups/offsite/test`, undefined, body, false);
  }

  /** Run a restore test now: the newest backup into a temporary database, compared with its manifest (in the background)
   *
   * Requires permission `system.admin`. */
  postServerBackupsRestoreTest(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** offsite or local */
    source?: string;
  }): Promise<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }> {
    return this.request<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }>('POST', `/api/v1/server/backups/restore-test`, undefined, body, false);
  }

  /** Schedule the automatic restore test (a day of the month)
   *
   * Requires permission `system.admin`. */
  putServerBackupsRestoreTest(body: {
    /** 1–28 */
    day: number;
    enabled?: boolean;
    hour: number;
    minute: number;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** offsite or local (blank: offsite when a target is enabled) */
    source?: string;
  }): Promise<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }> {
    return this.request<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }>('PUT', `/api/v1/server/backups/restore-test`, undefined, body, false);
  }

  /** Set the backup schedule and the retention (grandfather–father–son)
   *
   * Requires permission `system.admin`. */
  putServerBackupsSchedule(body: {
    /** remove the passphrase (refused while an offsite target is enabled) */
    clear_passphrase?: boolean;
    enabled?: boolean;
    hour?: number;
    /** daily: the newest N backups (1–365) */
    keep?: number;
    /** plus the newest of each of the last N months (0–120) */
    keep_monthly?: number;
    /** plus the newest of each of the last N weeks (0–104) */
    keep_weekly?: number;
    minute?: number;
    /** encrypt backups (blank keeps the stored one) */
    passphrase?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
  }> {
    return this.request<{
    /** docker, native, unknown or simulated */
    db_mode?: string;
    dir?: string;
    files?: ({
      created_at?: string;
      encrypted?: boolean;
      name?: string;
      /** present at the offsite target */
      offsite?: boolean;
      /** from the checksum file */
      sha256?: string;
      size_bytes?: number;
    })[];
    history?: ({
      actor?: string;
      /** written into the audit trail */
      audited?: boolean;
      error?: string;
      file?: string;
      finished_at?: string;
      id?: string;
      /** backup, offsite_sync or restore_test */
      kind?: string;
      ok?: boolean;
      running?: boolean;
      size_bytes?: number;
      /** restore test: offsite or local */
      source?: string;
      started_at?: string;
      steps?: ({
        detail?: string;
        ms?: number;
        /** dump, bundle, encrypt, checksum, upload, verify, sync, prune, prune_offsite, connect, select, fetch, manifest, restore, checks, cleanup */
        name?: string;
        ok?: boolean;
      })[];
      /** offsite target */
      target?: string;
      /** schedule, manual or update */
      trigger?: string;
    })[];
    offsite?: {
      /** the target without secrets (has_password, has_private_key, has_secret_key) */
      config?: Record<string, unknown>;
      describe?: string;
      files?: ({
        modified?: string;
        name?: string;
        size?: number;
      })[];
      list_error?: string;
      listed_at?: string;
    };
    restore_running?: boolean;
    restore_test?: {
      day?: number;
      enabled?: boolean;
      hour?: number;
      last_ok?: string;
      last_run?: string;
      minute?: number;
      source?: string;
    };
    running?: boolean;
    schedule?: {
      enabled?: boolean;
      has_passphrase?: boolean;
      hour?: number;
      keep?: number;
      keep_monthly?: number;
      keep_weekly?: number;
      last_error?: string;
      last_file?: string;
      last_ok?: string;
      last_run?: string;
      minute?: number;
    };
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

  /** Logs of the server
   *
   * The tail of one log of a closed list, through the server agent: `app` (ctrl32 telemetry), `agent`, `caddy` (reverse proxy container), `postgresql` (database container), `wireguard`, `cloudflared` (Cloudflare Tunnel), `kernel` (this boot). Linux (journal, Docker) only; lines longer than 2000 characters are cut.
   * 
   * Requires permission `system.admin`. */
  getServerLogs(query: { unit: string; /** app, agent, caddy, postgresql, wireguard, cloudflared, kernel */ lines?: number; /** 1–2000, default 200 */ }): Promise<{
    error?: string;
    label?: string;
    lines?: string[];
    unit?: string;
  }> {
    return this.request<{
    error?: string;
    label?: string;
    lines?: string[];
    unit?: string;
  }>('GET', `/api/v1/server/logs`, query, undefined, false);
  }

  /** Operating system: updates, reboot, jobs
   *
   * Through the server agent, Linux servers with apt (Debian, Ubuntu) only (ADR-0025): the package upgrades a full upgrade would install (from the package lists of the last check; computed again every 5 minutes or with `refresh=1`), how many are security updates, whether a reboot is required and for which packages, a scheduled reboot, the running job with the tail of its log, the newest finished job with its log and the history. Elsewhere `supported` is false with the reason (Windows: Windows Update). Finished jobs are written to the audit trail (server.os_job_result) once.
   * 
   * Requires permission `system.admin`. */
  getServerOs(query?: { refresh?: string; /** 1: list the upgrades again now */ }): Promise<OSState> {
    return this.request<OSState>('GET', `/api/v1/server/os`, query, undefined, false);
  }

  /** Check for operating system updates
   *
   * Downloads the package lists (apt-get update) and lists the upgrades again, as a job in the background (202); nothing is installed. 409 `busy` while a job, an update or a backup runs. Audited as server.os_check.
   * 
   * Requires permission `system.admin`. */
  postServerOsCheck(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<OSJob> {
    return this.request<OSJob>('POST', `/api/v1/server/os/check`, undefined, body, false);
  }

  /** Reboot the server
   *
   * A reboot in `delay_s` seconds (30–3600, default 60) through a systemd timer; it can be cancelled until then. Refused (409 `busy`) while an operating system job, an update of ctrl32 telemetry, a backup or a restore test runs, or another reboot is scheduled. Audited as server.reboot.
   * 
   * Requires permission `system.admin`. */
  postServerOsReboot(body: {
    /** 30–3600, default 60 */
    delay_s?: number;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<OSReboot> {
    return this.request<OSReboot>('POST', `/api/v1/server/os/reboot`, undefined, body, false);
  }

  /** Cancel the scheduled reboot
   *
   * 404 `not_found` when none is scheduled. Audited as server.reboot_cancel.
   * 
   * Requires permission `system.admin`. */
  postServerOsRebootCancel(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/server/os/reboot/cancel`, undefined, body, false);
  }

  /** Install the operating system updates
   *
   * apt-get update and apt-get full-upgrade without questions (changed configuration files are kept) in its own systemd unit, so it finishes when services — also the server agent — restart; the log is in the state. Services using upgraded libraries may restart; a new kernel needs a reboot (`reboot_required`). 409 `busy` while a job, an update, a backup or a scheduled reboot is pending; 501 `unsupported` off Linux/apt. Audited as server.os_upgrade (with the number of pending upgrades), the result as server.os_job_result; a failure is mailed to the server administrators.
   * 
   * Requires permission `system.admin`. */
  postServerOsUpgrade(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<OSJob> {
    return this.request<OSJob>('POST', `/api/v1/server/os/upgrade`, undefined, body, false);
  }

  /** Remote management settings (MeshCentral)
   *
   * The address of the MeshCentral server used as the emergency remote desktop and the node of this server in it (ADR-0025).
   * 
   * Requires permission `system.admin`. */
  getServerRemote(): Promise<{
    mesh_node?: string;
    mesh_url?: string;
  }> {
    return this.request<{
    mesh_node?: string;
    mesh_url?: string;
  }>('GET', `/api/v1/server/remote`, undefined, undefined, false);
  }

  /** Set the remote management settings
   *
   * `mesh_url`: https address of the MeshCentral server (empty: none); `mesh_node`: the node id of this server in MeshCentral (from the device's details or `gotonode=` in its address; a `node/<domain>/` prefix is removed). Audited as server.remote_settings.
   * 
   * Requires permission `system.admin`. */
  putServerRemote(body: {
    mesh_node?: string;
    /** https://mesh.example.com */
    mesh_url?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    mesh_node?: string;
    mesh_url?: string;
  }> {
    return this.request<{
    mesh_node?: string;
    mesh_url?: string;
  }>('PUT', `/api/v1/server/remote`, undefined, body, false);
  }

  /** Open the emergency remote desktop of this server
   *
   * The MeshCentral link to the remote desktop of this server (`viewmode=11`); MeshCentral asks for its own sign-in, the link carries no credential. 409 `not_configured` without the address or the node. Audited as remote.mesh_open with the reason.
   * 
   * Requires permission `system.admin`. */
  postServerRemoteOpen(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    url?: string;
  }> {
    return this.request<{
    url?: string;
  }>('POST', `/api/v1/server/remote/open`, undefined, body, false);
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

  /** Outgoing e-mail (SMTP)
   *
   * The settings in use and their source: `ui` (saved on this page), `config` ([smtp] of config.toml) or `none`. Settings saved here take precedence over config.toml unless [smtp] override = true. The password is never returned (`has_password`).
   * 
   * Requires permission `system.admin`. */
  getServerSmtp(): Promise<{
    /** e-mail can be sent */
    available?: boolean;
    /** [smtp] of config.toml without the password */
    config?: Record<string, unknown>;
    config_has_password?: boolean;
    has_password?: boolean;
    /** config.toml wins; the page is read-only */
    override?: boolean;
    saved?: boolean;
    /** host, port, security, user, from, from_name, reply_to, to */
    settings?: Record<string, unknown>;
    source?: "ui" | "config" | "none";
  }> {
    return this.request<{
    /** e-mail can be sent */
    available?: boolean;
    /** [smtp] of config.toml without the password */
    config?: Record<string, unknown>;
    config_has_password?: boolean;
    has_password?: boolean;
    /** config.toml wins; the page is read-only */
    override?: boolean;
    saved?: boolean;
    /** host, port, security, user, from, from_name, reply_to, to */
    settings?: Record<string, unknown>;
    source?: "ui" | "config" | "none";
  }>('GET', `/api/v1/server/smtp`, undefined, undefined, false);
  }

  /** Set the outgoing e-mail
   *
   * Security: `starttls` (port 587), `tls` (implicit TLS, port 465) or `none` — only for a relay on this server or in the local network (localhost, a private address or a name without a public domain). The password is write-only and stored encrypted with the server's secret key; an empty password keeps the stored one, clear_password removes it. An AI assistant cannot set the password. Applies without a restart; audited as server.smtp (without the password).
   * 
   * Requires permission `system.admin`. */
  putServerSmtp(body: {
    clear_password?: boolean;
    /** sender address */
    from: string;
    /** sender name */
    from_name?: string;
    host: string;
    /** write-only */
    password?: string;
    /** default 587, 465 with tls */
    port?: number;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    reply_to?: string;
    security: "starttls" | "tls" | "none";
    to?: string[];
    user?: string;
  }): Promise<{
    ok?: boolean;
    warnings?: string[];
  }> {
    return this.request<{
    ok?: boolean;
    warnings?: string[];
  }>('PUT', `/api/v1/server/smtp`, undefined, body, false);
  }

  /** Use config.toml for the outgoing e-mail
   *
   * Removes the settings saved on the page; [smtp] of config.toml applies again. Audited as server.smtp_reset.
   * 
   * Requires permission `system.admin`. */
  deleteServerSmtp(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    ok?: boolean;
  }> {
    return this.request<{
    ok?: boolean;
  }>('DELETE', `/api/v1/server/smtp`, undefined, body, false);
  }

  /** Send a test e-mail
   *
   * Sends a test message to `to` with the settings of the form (`settings`, unsaved; an empty password takes the stored one) or the ones in use, and returns the SMTP conversation (authentication redacted) also when it fails. Logged in the delivery log and audited as server.smtp_test.
   * 
   * Requires permission `system.admin`. */
  postServerSmtpTest(body: {
    /** optional: the form values as in PUT */
    settings?: Record<string, unknown>;
    /** recipient of the test message */
    to: string;
  }): Promise<{
    error?: string;
    ok?: boolean;
    /** the SMTP conversation */
    transcript?: string;
  }> {
    return this.request<{
    error?: string;
    ok?: boolean;
    /** the SMTP conversation */
    transcript?: string;
  }>('POST', `/api/v1/server/smtp/test`, undefined, body, false);
  }

  /** Download, verify and install a release
   *
   * The agent installs a release only when its manifest (`release.json`) carries a valid Ed25519 signature of a key built into this version and the binary matches the size and SHA-256 the manifest lists (ADR-0024). From the update URL (the portal or a mirror) or, with `source` package, from the offline package staged by an upload. Optionally a backup first; the running binary is kept, the migrations run, and when they fail the previous binary returns and the database is restored from that backup. After the restart the previous version returns automatically unless the new one answers healthy within the rollback time of the update settings. With a license whose included updates end (`updates_until`), a release dated later (signed release date) is refused with 402 `updates_ended` and the running system stays as it is; servers without a license always update. Refusals: 409 `version` (the distribution point offers another version), 409 `busy`, 502 `signature` / `checksum` / `unsigned`, 500 `rolled_back`.
   * 
   * Since 0.86.0 the answer comes as soon as the release is verified and its binary staged: 202 with the installation `id` and `phase` installing; the backup, the migrations and the restart follow in the background, so a proxy that cuts long requests (Cloudflare: 100 s) cannot break an update with a backup. Follow the record with this id in the history of `GET /api/v1/server/updates` (installing → restarting → done, rolled_back or failed); the result is also written to the audit trail (server.update_result). `wait: true` keeps the earlier behaviour: 200 when everything is done (or 500 `rolled_back`).
   * 
   * Requires permission `system.admin`. */
  postServerUpdate(body: {
    /** take a backup first (database, configuration) */
    backup?: boolean;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    source?: "" | "package";
    /** release version, e.g. 0.76.0 (optional with source package) */
    version?: string;
    /** answer only when the installation is done (before 0.86.0 always) */
    wait?: boolean;
  }): Promise<{
    /** backup file taken first (wait only) */
    backup?: string;
    /** installation id in the history */
    id?: string;
    /** trusted key that verified the release */
    key_id?: string;
    message?: string;
    /** installing (without wait) */
    phase?: string;
    sha256?: string;
    version?: string;
  }> {
    return this.request<{
    /** backup file taken first (wait only) */
    backup?: string;
    /** installation id in the history */
    id?: string;
    /** trusted key that verified the release */
    key_id?: string;
    message?: string;
    /** installing (without wait) */
    phase?: string;
    sha256?: string;
    version?: string;
  }>('POST', `/api/v1/server/update`, undefined, body, false);
  }

  /** Upload an offline update package
   *
   * multipart/form-data, the fields before the file. A package of deploy/publish.sh (`ctrl32-telemetry-<version>-offline.zip`: the signed manifest, the binaries of every platform, optionally the parts of the object counting detector) is streamed to the agent, which verifies the signature and every member (no member outside the manifest, sizes and SHA-256) before anything is kept, stages it and — unless install=0 — installs it exactly like an online release (license check, backup, migrations, rollback). The detector installation then takes its parts from the staged package. At most [agent] max_package_mb (default 2048 MB). Asks for the password (and authenticator code) again, from a browser session only (not an API token or an AI assistant); audited as server.update_package (also refusals).
   * 
   * Requires permission `system.admin`. */
  postServerUpdatePackage(body: FormData): Promise<{
    package?: {
      bytes?: number;
      detector?: string[];
      files?: number;
      /** the server binary of this platform */
      has_binary?: boolean;
      key_id?: string;
      platform?: string;
      released?: string;
      staged_at?: string;
      version?: string;
    };
    /** the installation, as POST /api/v1/server/update; absent when nothing was installed */
    update?: Record<string, unknown>;
  }> {
    return this.request<{
    package?: {
      bytes?: number;
      detector?: string[];
      files?: number;
      /** the server binary of this platform */
      has_binary?: boolean;
      key_id?: string;
      platform?: string;
      released?: string;
      staged_at?: string;
      version?: string;
    };
    /** the installation, as POST /api/v1/server/update; absent when nothing was installed */
    update?: Record<string, unknown>;
  }>('POST', `/api/v1/server/update/package`, undefined, body, false);
  }

  /** Update mode, available release and history
   *
   * The update mode (manual, notify — the default — or auto with a weekly maintenance window), the latest release of the distribution point (`available`: version, signed release date, the key that signed it), the kind of step (patch, minor, major), whether the automatic mode may install it and the license covers it, the next window and check, and the agent's history of installations (with rollbacks), the staged offline package and the trusted keys.
   * 
   * Requires permission `system.admin`. */
  getServerUpdates(): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/server/updates`, undefined, undefined, false);
  }

  /** Set the update mode
   *
   * manual: nothing happens by itself. notify (default): a daily check (with jitter), a banner for system administrators and one e-mail per version to the server administrators (when SMTP works). auto: in the weekly window (weekday 0 = Sunday, time HH:MM in `tz`, empty = the server's time zone; the window stays open one hour) a patch version — with `include_minor` also a minor one, never a major one — is installed with a backup first and the automatic rollback; a version that failed is not retried automatically. Audited as server.update_settings.
   * 
   * Requires permission `system.admin`. */
  putServerUpdates(body: {
    include_minor?: boolean;
    mode: "manual" | "notify" | "auto";
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** 2–60, default 10 */
    rollback_minutes?: number;
    /** HH:MM */
    time?: string;
    /** IANA time zone, empty = server time */
    tz?: string;
    /** 0 = Sunday … 6 = Saturday */
    weekday?: number;
  }): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('PUT', `/api/v1/server/updates`, undefined, body, false);
  }

  /** Check the distribution point now
   *
   * Reads VERSION and the signed manifest of the update URL now (otherwise once a day).
   * 
   * Requires permission `system.admin`. */
  postServerUpdatesCheck(body: Record<string, unknown>): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/server/updates/check`, undefined, body, false);
  }

  /** Update banner of the web UI
   *
   * A newer version for the banner of system administrators (empty when the mode is manual or the server is up to date). From the last check, no network access.
   * 
   * Requires permission `system.admin`. */
  getServerUpdatesNotice(): Promise<{
    current?: string;
    mode?: string;
    signed?: boolean;
    version?: string;
    /** automatic mode: when it will be installed */
    window?: string;
  }> {
    return this.request<{
    current?: string;
    mode?: string;
    signed?: boolean;
    version?: string;
    /** automatic mode: when it will be installed */
    window?: string;
  }>('GET', `/api/v1/server/updates/notice`, undefined, undefined, false);
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

  /** Set the four-eyes policies and Stay signed in
   *
   * `four_eyes_commands`: commands wait for approval. `four_eyes_limits`: alarm rule changes become change requests. `four_eyes_assignments`: device ↔ datastream assignments become change requests. `four_eyes_vpn`: VPN access of technicians needs a grant approved by a second person. `remember_device_days`: 0 (default) or up to 90 — the sign-in page offers *Stay signed in on this device*, such a session lasts this many days with a persistent cookie; lowering it applies to open sessions at once. At least one field.
   * 
   * Requires permission `user.admin`. */
  putSettingsPolicy(body: {
    four_eyes_assignments?: boolean;
    four_eyes_commands?: boolean;
    four_eyes_limits?: boolean;
    four_eyes_vpn?: boolean;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** 0–90 */
    remember_device_days?: number;
  }): Promise<{
    four_eyes_assignments?: boolean;
    four_eyes_commands?: boolean;
    four_eyes_limits?: boolean;
    four_eyes_vpn?: boolean;
    remember_device_days?: number;
  }> {
    return this.request<{
    four_eyes_assignments?: boolean;
    four_eyes_commands?: boolean;
    four_eyes_limits?: boolean;
    four_eyes_vpn?: boolean;
    remember_device_days?: number;
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

  /** A site with its assets, devices and alarms
   *
   * The site (with its map position), every asset of it as in the assets list, the devices that feed its datastreams now and the active alarms of its datastreams.
   * 
   * Requires permission `data.read`. */
  getSitesId(id: string): Promise<{
    alarms?: ActiveAlarm[];
    assets?: Asset[];
    datastreams?: number;
    devices?: AssetDevice[];
    site?: {
      address?: string;
      attributes?: Record<string, unknown>;
      created_at?: string;
      id?: string;
      location?: {
        lat?: number;
        lon?: number;
      } | null;
      name?: string;
      tz?: string;
    };
  }> {
    return this.request<{
    alarms?: ActiveAlarm[];
    assets?: Asset[];
    datastreams?: number;
    devices?: AssetDevice[];
    site?: {
      address?: string;
      attributes?: Record<string, unknown>;
      created_at?: string;
      id?: string;
      location?: {
        lat?: number;
        lon?: number;
      } | null;
      name?: string;
      tz?: string;
    };
  }>('GET', `/api/v1/sites/${encodeURIComponent(id)}`, undefined, undefined, false);
  }

  /** Update a site
   *
   * `location` sets the map position of the site (kept in its attributes as lat, lon); `clear_location` removes it.
   * 
   * Requires permission `config.write`. */
  patchSitesId(id: string, body: {
    address?: string;
    clear_location?: boolean;
    location?: {
      lat?: number;
      lon?: number;
    };
    name?: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    tz?: string;
  }): Promise<Site> {
    return this.request<Site>('PATCH', `/api/v1/sites/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Live events (Server-Sent Events)
   *
   * Long-lived `text/event-stream`. Events: `hello`, `observation` {datastream_id, key, value, measured_at}, `device` {device_id, connected}, `alarm` {alarm_id, datastream_id, state, severity, value}, `command` {device_id, command_id, method, status, detail}, `device_data` {device_id, state: telemetry|status, detail: {values, faults}} (with `device_data=1`: the keys of a telemetry message that have no datastream, every key of a status message, and the keys the device flagged as faulty); `ping` every 25 s with the count of dropped events. Filters: `datastreams` (comma-separated ids), `devices` (`all` or ids), `alarms=1`, `device_data=1`.
   * 
   * Requires permission `data.read`. */
  getStream(query?: { datastreams?: string; /** comma-separated datastream ids */ devices?: string; /** all, or comma-separated device ids */ alarms?: string; /** 1 to include alarm events */ device_data?: string; /** 1 to include device_data events of the filtered devices */ }): Promise<Response> {
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
    /** free note ≤ 500 (mounting, cabling, recorder channel); spec 028 */
    note?: string;
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
    /** free note ≤ 500 (mounting, cabling, recorder channel); spec 028 */
    note?: string;
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
   * One MP4 file (fragmented) of at most 24 hours; a change of resolution or codec ends it early. The recorded sound is the second track only for people with video.audio. format=h264 converts an H.265 recording to H.264 + AAC (at most 1080 lines, at most one hour, one conversion at a time per server: 409 while another runs, 503 without ffmpeg); H.264 recordings are sent unchanged. Only the original is the unchanged recording. Written to the audit trail (video.export).
   * 
   * Requires permission `video.playback`. */
  getVideoCamerasIdExport(id: string, query?: { from?: string; /** RFC 3339, required */ to?: string; /** RFC 3339, required */ format?: string; /** original (default) or h264 */ }): Promise<Response> {
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

  /** Camera inventory (export)
   *
   * Every setting of the cameras of the organization the caller may see, without passwords (spec 028): Excel (sheets Cameras and Read me), CSV (UTF-8 with BOM, semicolons) or PDF (a card per camera, stored as a report). The file imports back unchanged. Audited as export.read.
   * 
   * Requires permission `video.manage`. */
  getVideoInventory(query?: { format?: string; /** xlsx (default), csv or pdf */ }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/video/inventory`, query, undefined, true);
  }

  /** Camera inventory with passwords
   *
   * As the export, with the camera passwords and relay secrets; the file is marked CONFIDENTIAL and a PDF is not stored. Needs video.manage too, the user's password (and authenticator code with two-factor sign-in) again (403 `step_up_required` / `step_up_failed`), a reason; at most 3 at once, then one a minute (429). Refused for API tokens and AI assistants. Audited as video.credentials_export.
   * 
   * Requires permission `video.credentials_export`. */
  postVideoInventoryCredentials(body: {
    /** authenticator code (two-factor sign-in) */
    code?: string;
    format?: "xlsx" | "csv" | "pdf";
    /** the user's password */
    password: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<Response> {
    return this.request<Response>('POST', `/api/v1/video/inventory/credentials`, undefined, body, true);
  }

  /** Import cameras
   *
   * multipart/form-data. An inventory (Excel or CSV, at most 2 MB and 1000 rows) is checked row by row: matched by id, else by name (and ip); create, update, restore (a removed camera by its id), unchanged, skip or error with the messages. Without apply the plan is returned and nothing changes; with apply=1 and a reason it is stored in one transaction, all rows or none (422 `rows_invalid` with the plan when a row has an error). Never deletes. An empty cell keeps the value, an empty password keeps the stored one; make and ip fill in the addresses; a different ip moves the addresses. Without a license the cameras beyond 4 enabled are imported disabled. Audited: video.import and camera.create/update/restore per camera.
   * 
   * Requires permission `video.manage`. */
  postVideoInventoryImport(body: FormData): Promise<{
    applied?: boolean;
    columns?: string[];
    /** rows per action; disabled_by_licence */
    counts?: Record<string, unknown>;
    /** enabled cameras of the server after the import */
    enabled_after?: number;
    /** xlsx or csv */
    format?: string;
    /** 4 */
    free_limit?: number;
    ignored?: string[];
    licensed?: boolean;
    /** rows that set a password or relay secret */
    passwords?: number;
    rows?: ({
      action?: "create" | "update" | "restore" | "unchanged" | "skip" | "error";
      changes?: string[];
      /** after the import */
      enabled?: boolean;
      errors?: string[];
      id?: string;
      ip?: string;
      line?: number;
      name?: string;
      warnings?: string[];
    })[];
    sheet?: string;
  }> {
    return this.request<{
    applied?: boolean;
    columns?: string[];
    /** rows per action; disabled_by_licence */
    counts?: Record<string, unknown>;
    /** enabled cameras of the server after the import */
    enabled_after?: number;
    /** xlsx or csv */
    format?: string;
    /** 4 */
    free_limit?: number;
    ignored?: string[];
    licensed?: boolean;
    /** rows that set a password or relay secret */
    passwords?: number;
    rows?: ({
      action?: "create" | "update" | "restore" | "unchanged" | "skip" | "error";
      changes?: string[];
      /** after the import */
      enabled?: boolean;
      errors?: string[];
      id?: string;
      ip?: string;
      line?: number;
      name?: string;
      warnings?: string[];
    })[];
    sheet?: string;
  }>('POST', `/api/v1/video/inventory/import`, undefined, body, false);
  }

  /** Camera inventory template
   *
   * The columns of the inventory with two example rows (their id starts with #, so they are not imported).
   * 
   * Requires permission `video.manage`. */
  getVideoInventoryTemplate(query?: { format?: string; /** xlsx (default) or csv */ }): Promise<Response> {
    return this.request<Response>('GET', `/api/v1/video/inventory/template`, query, undefined, true);
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
    /** a click on a tile enlarges the camera on every wall; with this the enlarged camera offers a Recordings button that opens its player with the recordings, for users with video.playback (playing back is audited); default false, omitted keeps the value */
    playback_on_click?: boolean;
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
   * Fields left out keep their value. Allowing playback from the wall (`playback_on_click`) is audited with the reason like every change.
   * 
   * Requires permission `video.manage`. */
  putVideoWallsId(id: string, body: {
    /** ≤ 80 */
    name?: string;
    /** a click on a tile enlarges the camera on every wall; with this the enlarged camera offers a Recordings button that opens its player with the recordings, for users with video.playback (playing back is audited); default false, omitted keeps the value */
    playback_on_click?: boolean;
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
    uplink?: VPNUplink;
    /** uplink tokens issued by this server carry the HTTPS transport (it has an https base URL) */
    uplink_https?: boolean;
    uplink_rules?: (Record<string, unknown>)[];
    uplink_services?: (Record<string, unknown>)[];
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
    uplink?: VPNUplink;
    /** uplink tokens issued by this server carry the HTTPS transport (it has an https base URL) */
    uplink_https?: boolean;
    uplink_rules?: (Record<string, unknown>)[];
    uplink_services?: (Record<string, unknown>)[];
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
    /** site only: another telemetry server (uplink); the answer carries `uplink_token` instead of `config`; subnets optional */
    uplink?: boolean;
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
    /** uplink: the token carries the HTTPS transport */
    https_transport?: boolean;
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
    /** uplink: the token to paste on the remote server (holds its private key; shown once) */
    uplink_token?: string;
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
    /** uplink: the token carries the HTTPS transport */
    https_transport?: boolean;
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
    /** uplink: the token to paste on the remote server (holds its private key; shown once) */
    uplink_token?: string;
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

  /** Connect this server to a hub (uplink)
   *
   * VPN phase 3. Pastes the uplink token issued by a hub (another telemetry server with a public address: on the hub, *Add peer → Server (uplink)*). The agent brings up an outbound-only WireGuard interface (`wg1` on Linux; inside the agent on Windows) with a keepalive of 25 s, and applies this server's own uplink rules: everything that arrives from the hub is dropped unless an uplink rule allows it. `transport`: `udp` (default) or `https` (WireGuard over a WebSocket through the hub's HTTPS port, when UDP is blocked; the token must carry it). With an uplink already connected the configuration is replaced (after *Rotate key* on the hub). The hub's range must not overlap this server's networks or own VPN (409). Needs the caller's `password` again (and `code` with two-factor sign-in).
   * 
   * Requires permission `system.admin`. */
  postVpnUplink(body: {
    /** authenticator code when two-factor sign-in is on */
    code?: string;
    /** your password (step-up) */
    password: string;
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    /** the uplink token from the hub (ctrl32-uplink-v1.…) */
    token: string;
    transport?: "udp" | "https";
  }): Promise<{
    ok?: boolean;
    rules_applied?: number;
    uplink?: VPNUplink;
  }> {
    return this.request<{
    ok?: boolean;
    rules_applied?: number;
    uplink?: VPNUplink;
  }>('POST', `/api/v1/vpn/uplink`, undefined, body, false);
  }

  /** Disconnect this server from its hub
   *
   * Stops the uplink interface, removes its firewall table and deletes its keys. The uplink rules are kept for the next connection.
   * 
   * Requires permission `system.admin`. */
  postVpnUplinkDisconnect(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    ok?: boolean;
    uplink?: VPNUplink;
  }> {
    return this.request<{
    ok?: boolean;
    uplink?: VPNUplink;
  }>('POST', `/api/v1/vpn/uplink/disconnect`, undefined, body, false);
  }

  /** Add an uplink access rule
   *
   * What the hub may reach on this server (default deny): from the hub's VPN range or an address/subnet inside it (`src_cidr`, empty = the whole range) to `server` (this server's uplink address, e.g. the web interface) or `lan` (a host or subnet of the networks offered to the hub, `dest_cidr`; empty = all of them; forwarded with masquerade, so LAN devices see this server). The hub's own access rules must allow the technician too. Without a connected uplink the rule is stored and applied on connecting.
   * 
   * Requires permission `system.admin`. */
  postVpnUplinkRules(body: VPNUplinkRuleInput): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('POST', `/api/v1/vpn/uplink/rules`, undefined, body, false);
  }

  /** Change an uplink access rule
   *
   * Requires permission `system.admin`. */
  putVpnUplinkRulesId(id: string, body: VPNUplinkRuleInput): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('PUT', `/api/v1/vpn/uplink/rules/${encodeURIComponent(id)}`, undefined, body, false);
  }

  /** Remove an uplink access rule
   *
   * Requires permission `system.admin`. */
  postVpnUplinkRulesIdRemove(id: string, body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
  }): Promise<{
    /** true on success */
    ok?: boolean;
  }> {
    return this.request<{
    /** true on success */
    ok?: boolean;
  }>('POST', `/api/v1/vpn/uplink/rules/${encodeURIComponent(id)}/remove`, undefined, body, false);
  }

  /** Switch the uplink's transport
   *
   * `udp` or `https` (only when the token carries the HTTPS transport).
   * 
   * Requires permission `system.admin`. */
  postVpnUplinkTransport(body: {
    /** Reason for the change, written to the audit trail (required, 3–200 characters). */
    reason: string;
    transport: "udp" | "https";
  }): Promise<{
    ok?: boolean;
    uplink?: VPNUplink;
  }> {
    return this.request<{
    ok?: boolean;
    uplink?: VPNUplink;
  }>('POST', `/api/v1/vpn/uplink/transport`, undefined, body, false);
  }

  /** HTTPS transport of an uplink (WebSocket)
   *
   * The hub's end of the HTTPS transport: a customer server's agent opens a WebSocket with `Authorization: Bearer <tunnel key>` from its uplink token; each binary message is one WireGuard datagram, relayed to this server's WireGuard port on the loopback address. Only enabled, unexpired uplink peers; the key is stored as SHA-256. Not called by users.
   * 
   * Requires permission `uplink`. */
  getVpnUplinkTunnel(): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('GET', `/api/v1/vpn/uplink/tunnel`, undefined, undefined, false);
  }
}
