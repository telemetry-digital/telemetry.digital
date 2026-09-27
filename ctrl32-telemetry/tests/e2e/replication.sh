#!/usr/bin/env bash
# End-to-end checks for the redundant database (spec 003, URS K-15…K-20) with two real PostgreSQL clusters on one
# machine: the e2e cluster is the primary's, a second cluster (created with initdb) becomes the replica. The suite
# runs its OWN primary server (port 8092, database ctrl32_repl_e2e) because the standby scenario fences the primary.
#   PG_ADMIN_URL=postgres://ctrl32:…@127.0.0.1:5433/postgres BIN=… CONFIG=… bash tests/e2e/replication.sh
# Needs root (the replica cluster runs as the postgres user), the PostgreSQL server binaries of the same major version
# as the primary (PGBIN, default /usr/lib/postgresql/<n>/bin) and Node for JSON. Skips itself when these are missing.
set -u
cd "$(dirname "$0")/../.."
PG_ADMIN_URL=${PG_ADMIN_URL:-postgres://ctrl32:ctrl32-dev-only@127.0.0.1:5433/postgres?sslmode=disable}
PGBIN=${PGBIN:-$(ls -d /usr/lib/postgresql/*/bin 2>/dev/null | sort -V | tail -1)}
REPDB=ctrl32_repl_e2e
P_PORT=8092; R_PORT=8093; S_PORT=8094; R_PG=5434
pass=0; fail=0
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
skip() { echo "  ok   skipped: $1"; exit 0; }

[[ $(id -u) == 0 ]] || skip "needs root to run the replica cluster as postgres"
id postgres >/dev/null 2>&1 || skip "no postgres user"
[[ -x "$PGBIN/pg_basebackup" && -x "$PGBIN/initdb" ]] || skip "no PostgreSQL server binaries (set PGBIN)"
command -v node >/dev/null || skip "no node"
url_user() { local r=${1#*://}; r=${r%%@*}; printf '%s' "${r%%:*}"; }
url_pass() { local r=${1#*://}; r=${r%%@*}; [[ "$r" == *:* ]] && printf '%s' "${r#*:}"; }
url_db()   { # url database → the same URL with another database
  local rest=${1#*://} creds=${1#*://}; creds=${creds%%@*}; rest=${rest#*@}
  local hostport=${rest%%/*} query=""; [[ "$rest" == *\?* ]] && query="?${rest#*\?}"
  printf 'postgres://%s@%s/%s%s' "$creds" "$hostport" "$2" "$query"; }
ADMIN_DB=$(url_db "$PG_ADMIN_URL" "$REPDB")
SERVER_MAJOR=$(psql -Atq "$PG_ADMIN_URL" -c "SELECT current_setting('server_version_num')::int / 10000")
[[ "$SERVER_MAJOR" == "$("$PGBIN/pg_basebackup" --version | grep -o '[0-9]*' | head -1)" ]] || skip "PGBIN major differs from the server ($SERVER_MAJOR)"

W=$(mktemp -d /tmp/c32repl.XXXX); chmod 1777 "$W"
PIDS=()
cleanup() {
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null; done; wait 2>/dev/null
  for d in "$W"/rdata*; do [[ -f "$d/postmaster.pid" ]] && su postgres -s /bin/bash -c "$PGBIN/pg_ctl -D $d -m immediate stop" >/dev/null 2>&1; done
  # slots, roles and pg_hba rules of this run (the standby is never detached)
  psql -q "$PG_ADMIN_URL" -c "SELECT pg_drop_replication_slot(slot_name) FROM pg_replication_slots WHERE slot_name LIKE 'ctrl32_replica_%' AND NOT active" >/dev/null 2>&1
  for r in $(psql -Atq "$PG_ADMIN_URL" -c "SELECT rolname FROM pg_roles WHERE rolname LIKE 'ctrl32\_repl\_%'" 2>/dev/null); do psql -q "$PG_ADMIN_URL" -c "DROP ROLE $r" >/dev/null 2>&1; done
  HBA=$(psql -Atq "$PG_ADMIN_URL" -c "SHOW hba_file" 2>/dev/null); [[ -f "$HBA" ]] && sed -i '/# ctrl32-replica /d' "$HBA" && psql -q "$PG_ADMIN_URL" -c "SELECT pg_reload_conf()" >/dev/null
  psql -q "$PG_ADMIN_URL" -c "ALTER SYSTEM RESET max_slot_wal_keep_size" -c "SELECT pg_reload_conf()" >/dev/null 2>&1
  psql -q "$PG_ADMIN_URL" -c "DROP DATABASE IF EXISTS $REPDB WITH (FORCE)" >/dev/null 2>&1
  [[ "${KEEP:-}" == 1 ]] || rm -rf "$W"
}
trap cleanup EXIT
if [[ -z "${BIN:-}" ]]; then BIN=$W/c32; go build -o "$BIN" ./cmd/ctrl32-telemetry || exit 2; fi
cp "$BIN" "$W/c32bin"; chmod 755 "$W/c32bin"; BIN=$W/c32bin
CONFIG=${CONFIG:-config.example.toml}

echo "== primary: own database and server (port $P_PORT)"
psql -q "$PG_ADMIN_URL" -c "DROP DATABASE IF EXISTS $REPDB WITH (FORCE)" -c "CREATE DATABASE $REPDB OWNER $(url_user "$PG_ADMIN_URL")" || exit 2
if [[ "$SERVER_MAJOR" -lt 18 ]]; then psql -q "$ADMIN_DB" <<'SQL'
CREATE FUNCTION public.uuidv7() RETURNS uuid LANGUAGE sql VOLATILE AS $$
  SELECT (lpad(to_hex(floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint), 12, '0') || '7' || substr(h, 1, 3)
          || substr('89ab', 1 + (get_byte(decode(substr(h, 4, 2), 'hex'), 0) & 3), 1) || substr(h, 6, 15))::uuid
  FROM (SELECT md5(random()::text || clock_timestamp()::text) h) r $$;
SQL
fi
# ctrl32_app is one role for the whole cluster: keep the password the other databases' servers use.
APP_PW=$(sed -n 's/^app_password = "\(.*\)".*/\1/p' "$CONFIG" | head -1)
mkcfg() { # dir http-port mqtt-port pg-port
  mkdir -p "$1"; chmod 755 "$1"
  local app admin
  app=$(url_db "$PG_ADMIN_URL" "$REPDB" | sed -e "s|://[^@]*@|://ctrl32_app:$APP_PW@|" -e "s|127.0.0.1:[0-9]*|127.0.0.1:$4|")
  admin=$(printf '%s' "$ADMIN_DB" | sed -e "s|127.0.0.1:[0-9]*|127.0.0.1:$4|")
  sed -e "s|^url = .*|url = \"$app\"|" -e "s|^admin_url = .*|admin_url = \"$admin\"|" -e "s|^app_password = .*|app_password = \"$APP_PW\"|" \
      -e "s|^listen = \":8080\"|listen = \"127.0.0.1:$2\"|" -e "s|^base_url = .*|base_url = \"http://127.0.0.1:$2\"|" \
      -e "s|^listen = \":1883\".*|listen = \"127.0.0.1:$3\"|" -e "s|^ws_listen = .*|ws_listen = \"127.0.0.1:$(( $3 + 1 ))\"|" \
      -e "s|^socket = .*|# no agent|" -e "s|REPLACE-WITH-RANDOM-32-CHARS|repl-$RANDOM$RANDOM-not-a-secret-0123456789|" \
      -e "/^storage_dir = /d" -e "/^pmtiles = /d" -e "/^attribution = /d" -e "s|^\[report\]$|[report]\nstorage_dir = \"$1/reports\"|" -e "s|^\[fota\]$|[fota]\nstorage_dir = \"$1/firmware\"|" \
      "$CONFIG" > "$1/config.toml"
  chmod 644 "$1/config.toml"
}
PG_MAIN_PORT=$(printf '%s' "$PG_ADMIN_URL" | grep -o '127.0.0.1:[0-9]*' | cut -d: -f2)
mkcfg "$W/primary" $P_PORT 18830 "$PG_MAIN_PORT"
"$BIN" migrate -config "$W/primary/config.toml" > "$W/migrate.log" 2>&1 || { tail -5 "$W/migrate.log"; exit 2; }
"$BIN" admin bootstrap -config "$W/primary/config.toml" > /dev/null 2>&1
"$BIN" admin reset-password -username admin -password admin1234 -permanent -allow-weak -config "$W/primary/config.toml" > /dev/null || exit 2
psql -q "$ADMIN_DB" -c "UPDATE core.organization SET settings = jsonb_set(coalesce(settings,'{}'), '{security}', coalesce(settings->'security','{}') || '{\"mfa_optional\": true}')"
start() { # dir port name
  "$BIN" serve -config "$1/config.toml" >> "$1/server.log" 2>&1 &
  PIDS+=($!); eval "PID_$3=$!"
  for _ in $(seq 80); do curl -s -o /dev/null --max-time 1 "http://127.0.0.1:$2/healthz" && return 0; sleep 0.25; done
  echo "server $3 did not start"; tail -5 "$1/server.log"; return 1; }
start "$W/primary" $P_PORT primary || exit 2
BASE=http://127.0.0.1:$P_PORT
JAR=$W/jar; CSRF=""
req() { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$W/b" -w '%{http_code}' --data-binary "$d" "$BASE$p" > "$W/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$W/b" -w '%{http_code}' "$BASE$p" > "$W/s"; fi; cat "$W/b"; }
st() { cat "$W/s"; }
jx() { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$W/b" "$1" 2>/dev/null; }
req POST /api/v1/auth/login '{"username":"admin","password":"admin1234"}' >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me >/dev/null; jx j.csrf_token)
req GET /api/v1/replication >/dev/null; check "replication status" "$(st)" 200
[[ "$(jx 'j.node.role')" == primary ]] && ok "node role primary" || bad "role: $(jx j.node)"
PRIMARY_NODE=$(jx 'j.node.node_id')

echo "== data on the primary"
SITE=$(req POST /api/v1/sites '{"name":"Repl site","reason":"e2e"}' >/dev/null; jx j.id)
ASSET=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"machine\",\"name\":\"Repl asset\",\"reason\":\"e2e\"}" >/dev/null; jx j.id)
DS=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" >/dev/null; jx j.id)
DEV=$(req POST /api/v1/devices '{"external_id":"repl-dev","name":"repl","transport":"http","reason":"e2e"}' >/dev/null; jx j.id)
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' >/dev/null; jx j.token)
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e\"}" >/dev/null
publish() { curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -H "Authorization: Bearer $TOK" -d "{\"v\":{\"t\":$1}}" "$BASE/api/v1/d/telemetry"; }
check "telemetry stored" "$(publish 20.5)" 202

echo "== pairing"
req POST /api/v1/replication/pairings '{"mode":"continuous","name":"Backup PC","lag_limit_s":30}' >/dev/null; check "reason required" "$(st)" 400
req POST /api/v1/replication/pairings '{"mode":"continuous","name":"Backup PC","lag_limit_s":30,"reason":"e2e attach"}' >/dev/null; check "pairing code" "$(st)" 201
CODE=$(jx j.code); [[ "$CODE" =~ ^[A-Z2-9]{4}-[A-Z2-9]{4}-[A-Z2-9]{4}$ ]] && ok "code format" || bad "code: $CODE"
curl -s -o "$W/b" -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d "{\"code\":\"AAAA-BBBB-CCCC\",\"pg_major\":$SERVER_MAJOR,\"ctrl32_version\":\"dev\"}" "$BASE/api/v1/replication/pair" > "$W/s"
check "wrong code refused" "$(st)" 403
psql -Atq "$ADMIN_DB" -c "SELECT count(*) FROM core.audit_trail WHERE action = 'replica.pair_refused'" | grep -qx 1 && ok "refused pairing audited" || bad "refused pairing not audited"
curl -s -o "$W/b" -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d "{\"code\":\"$CODE\",\"pg_major\":$((SERVER_MAJOR - 1)),\"ctrl32_version\":\"x\"}" "$BASE/api/v1/replication/pair" > "$W/s"
check "PostgreSQL major mismatch refused" "$(st)" 400

new_replica_cluster() { # dir
  rm -rf "$1"; mkdir -p "$1"; chown postgres: "$1"
  su postgres -s /bin/bash -c "$PGBIN/initdb -D $1 -A trust -U postgres >/dev/null && $PGBIN/pg_ctl -D $1 -l $1/server.log -w -o '-p $R_PG -k /tmp' start >/dev/null" || return 1
}
attach() { # config-dir code name [extra]
  su postgres -s /bin/bash -c "$BIN replica attach -config $1/config.toml -primary $BASE -code $2 -name '$3' -pgdata $W/rdata -pgbin $PGBIN -pgport $R_PG -pguser postgres -tunnel-port 54391 -no-service -yes ${4:-}" > "$1/attach.log" 2>&1
}

echo "== attach a continuous replica (second cluster on port $R_PG)"
new_replica_cluster "$W/rdata" && ok "second cluster running" || bad "initdb"
mkcfg "$W/replica" $R_PORT 18840 $R_PG; chown -R postgres: "$W/replica"
attach "$W/replica" "$CODE" "Backup PC" && ok "attach (base copy through the HTTPS tunnel)" || { bad "attach: $(tail -5 "$W/replica/attach.log")"; }
[[ -f "$W/replica/replica.json" ]] && ok "replica state file" || bad "no replica.json"
ls -d "$W"/rdata.before-replica-* >/dev/null 2>&1 && ok "previous data directory kept aside" || bad "no aside copy"
start "$W/replica" $R_PORT replica && ok "replica server (replica mode)" || bad "replica server"
curl -s "http://127.0.0.1:$R_PORT/api/v1/replication/role" > "$W/b"; [[ "$(jx j.role)" == replica ]] && ok "replica role endpoint" || bad "role: $(cat "$W/b")"
check "writes refused on the replica" "$(curl -s -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:$R_PORT/api/v1/auth/login" -d '{}')" 503
state() { req GET /api/v1/replication >/dev/null; jx "(j.replicas.find(r => r.name === '$1') || {}).$2"; }
for _ in $(seq 60); do [[ "$(state 'Backup PC' state)" == in_sync ]] && break; sleep 1; done
check "replica in sync" "$(state 'Backup PC' state)" in_sync
check "published during streaming" "$(publish 21.5)" 202
sleep 2
NPRIM=$(psql -Atq "$ADMIN_DB" -c "SELECT count(*) FROM core.observation")
NREP=$(psql -Atq -h /tmp -p $R_PG -U postgres -d $REPDB -c "SELECT count(*) FROM core.observation")
check "observations streamed to the replica" "$NREP" "$NPRIM"
check "replica is read-only" "$(psql -Atq -h /tmp -p $R_PG -U postgres -d $REPDB -c 'SELECT pg_is_in_recovery()')" t
RID=$(state 'Backup PC' id)
req PATCH "/api/v1/replication/replicas/$RID" '{"lag_limit_s":45,"reason":"e2e limit"}' >/dev/null; check "change lag limit" "$(st)" 200
psql -Atq "$ADMIN_DB" -c "SELECT old_value->>'lag_limit_s' || '>' || (new_value->>'lag_limit_s') FROM core.audit_trail WHERE action = 'replica.update' ORDER BY seq DESC LIMIT 1" | grep -qx '30>45' && ok "change audited (old/new)" || bad "change audit"

echo "== safe detach"
req POST "/api/v1/replication/replicas/$RID/detach" '{"reason":"move the copy to the archive"}' >/dev/null; check "detach started" "$(st)" 202
check "primary keeps collecting during the detach" "$(publish 22.5)" 202
for _ in $(seq 90); do s=$(state 'Backup PC' state); [[ "$s" == detached || "$s" == failed ]] && break; sleep 1; done
check "replica detached" "$(state 'Backup PC' state)" detached
check "verification ok" "$(state 'Backup PC' 'detach.verification.ok')" true
DSEQ=$(state 'Backup PC' 'detach.seq'); DHASH=$(state 'Backup PC' 'detach.hash')
psql -Atq "$ADMIN_DB" -c "SELECT count(*) FROM core.audit_trail WHERE action IN ('replica.detach', 'replica.detach_done')" | grep -qx 2 && ok "detach and result in the primary's audit trail" || bad "primary audit"
check "replication slot removed" "$(psql -Atq "$ADMIN_DB" -c "SELECT count(*) FROM pg_replication_slots WHERE slot_name LIKE 'ctrl32_replica_%'")" 0
for _ in $(seq 40); do kill -0 "$PID_replica" 2>/dev/null || break; sleep 0.5; done
kill -0 "$PID_replica" 2>/dev/null && bad "replica server did not exit for the restart" || ok "replica server exited to restart as a normal server"
check "copy promoted (independent)" "$(psql -Atq -h /tmp -p $R_PG -U postgres -d $REPDB -c 'SELECT pg_is_in_recovery()')" f
check "copy has the detach entry with the same hash" "$(psql -Atq -h /tmp -p $R_PG -U postgres -d $REPDB -c "SELECT hash FROM core.audit_trail WHERE seq = $DSEQ")" "$DHASH"
psql -Atq -h /tmp -p $R_PG -U postgres -d $REPDB -c "SELECT new_value->>'verified' FROM core.audit_trail WHERE action = 'replica.detached'" | grep -qx true && ok "detachment recorded on the copy" || bad "copy audit"
check "copy got a new identity" "$(psql -Atq -h /tmp -p $R_PG -U postgres -d $REPDB -c "SELECT promoted_from::text = '$PRIMARY_NODE' AND epoch = 2 FROM core.node")" t
"$BIN" audit verify -config "$W/replica/config.toml" > "$W/verify.log" 2>&1 && ok "detached copy verifies its audit chain" || bad "audit verify: $(tail -2 "$W/verify.log")"
start "$W/replica" $R_PORT replica2 && ok "detached copy starts as its own server" || bad "detached copy server"
curl -s "http://127.0.0.1:$R_PORT/api/v1/replication/role" > "$W/b"; check "detached copy is a primary" "$(jx j.role)" primary
kill "$PID_replica2" 2>/dev/null; su postgres -s /bin/bash -c "$PGBIN/pg_ctl -D $W/rdata -m fast stop" >/dev/null 2>&1

echo "== hot standby, failover, fencing"
req POST /api/v1/replication/pairings '{"mode":"standby","name":"Standby","reason":"e2e standby"}' >/dev/null; CODE2=$(jx j.code)
new_replica_cluster "$W/rdata" >/dev/null 2>&1 || true
rm -f "$W/standby/replica.json"; mkcfg "$W/standby" $S_PORT 18850 $R_PG; chown -R postgres: "$W/standby"
attach "$W/standby" "$CODE2" "Standby" "-standby-url http://127.0.0.1:$S_PORT" && ok "standby attached" || bad "standby attach: $(tail -3 "$W/standby/attach.log")"
start "$W/standby" $S_PORT standby || bad "standby server"
for _ in $(seq 60); do [[ "$(state Standby state)" == in_sync ]] && break; sleep 1; done
check "standby in sync" "$(state Standby state)" in_sync
kill "$PID_primary"; wait "$PID_primary" 2>/dev/null
"$BIN" replica promote -config "$W/standby/config.toml" -reason "primary failed (e2e)" -pgdata "$W/rdata" -pgbin "$PGBIN" -pgport $R_PG -pguser postgres -no-service -yes > "$W/promote.log" 2>&1 \
  && ok "standby promoted (manual, with reason)" || bad "promote: $(tail -3 "$W/promote.log")"
kill "$PID_standby" 2>/dev/null; wait "$PID_standby" 2>/dev/null
start "$W/standby" $S_PORT standby2 && ok "promoted standby runs as primary" || bad "promoted standby server"
curl -s "http://127.0.0.1:$S_PORT/api/v1/replication/role" > "$W/b"
check "new primary reports its origin" "$(jx "j.role + ' ' + (j.promoted_from === '$PRIMARY_NODE')")" "primary true"
check "new primary accepts telemetry" "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -H "Authorization: Bearer $TOK" -d '{"v":{"t":23.5}}' "http://127.0.0.1:$S_PORT/api/v1/d/telemetry")" 202
start "$W/primary" $P_PORT primary2 >/dev/null 2>&1
for _ in $(seq 30); do [[ -f "$W/primary/fenced.json" ]] && break; sleep 0.5; done
[[ -f "$W/primary/fenced.json" ]] && ok "returning primary fenced itself" || bad "no fence file"
for _ in $(seq 40); do curl -s -o /dev/null --max-time 1 "$BASE/healthz" && break; sleep 0.25; done
curl -s "$BASE/api/v1/replication/role" > "$W/b"; check "old primary reports fenced" "$(jx j.role)" fenced
check "old primary refuses telemetry" "$(publish 24.5)" 503

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]
