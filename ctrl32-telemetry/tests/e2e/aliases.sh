#!/usr/bin/env bash
# End-to-end checks for entity aliases and key filters (0.73.0): every alias kind (one entity, chosen entities, by
# type, by name, an asset hierarchy N levels deep, the current entity of a dashboard state, a query), key filters on
# latest values, attributes, quality, staleness and alarms with constant and dynamic values (a value filter of the
# dashboard, an attribute of the entity, of the current entity or of the organization), highlighting, the answer as
# entities, datastreams or devices, the dashboard filters, dashboards of format 1.2 and public links that never read
# outside the dashboard's scope. Telemetry goes through the device HTTP API; JSON extraction with Node.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/aliases.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
# eq PATH key=value…: GET with URL-encoded parameters (JSON of aliases and key filters)
eq()   { local p=$1; shift; local args=(); for a in "$@"; do args+=(--data-urlencode "$a"); done
  curl -s -G -b "$JAR" -o "$TMP/b" -w '%{http_code}' "${args[@]}" "$BASE$p" > "$TMP/s"; }
anon() { local p=$1; shift; local args=(); for a in "$@"; do args+=(--data-urlencode "$a"); done
  curl -s -G -o "$TMP/b" -w '%{http_code}' "${args[@]}" "$BASE$p" > "$TMP/s"; }
dev()  { curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $1" -H 'Content-Type: application/json' -d "$2" "$BASE/api/v1/d/telemetry"; }
names(){ jx 'j.items.map(x=>x.name).sort().join("|")'; }
SUF=$RANDOM

echo "== login"
eq /api/v1/entities/query 'alias={"kind":"current"}'; [[ "$(st)" == "401" || "$(st)" == "403" ]] && ok "entities need a session ($(st))" || bad "entities need a session (got $(st))"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== data: a building with a room, two fridges and a freezer; a meter at another site"
S1=$(req POST /api/v1/sites "{\"name\":\"Ali pharmacy $SUF\",\"reason\":\"e2e\"}" | jget id)
S2=$(req POST /api/v1/sites "{\"name\":\"Ali plant $SUF\",\"reason\":\"e2e\"}" | jget id)
[[ -n "$S1" && -n "$S2" ]] && ok "sites" || bad "sites"
B1=$(req POST /api/v1/assets "{\"site_id\":\"$S1\",\"type\":\"building\",\"name\":\"Ali building $SUF\",\"reason\":\"e2e\"}" | jget id)
R1=$(req POST /api/v1/assets "{\"site_id\":\"$S1\",\"parent_id\":\"$B1\",\"type\":\"room\",\"name\":\"Ali store room $SUF\",\"reason\":\"e2e\"}" | jget id)
F1=$(req POST /api/v1/assets "{\"site_id\":\"$S1\",\"parent_id\":\"$R1\",\"type\":\"fridge\",\"name\":\"Ali fridge 1 $SUF\",\"attributes\":{\"max_temp\":8,\"storage_class\":\"2-8C\"},\"reason\":\"e2e\"}" | jget id)
F2=$(req POST /api/v1/assets "{\"site_id\":\"$S1\",\"parent_id\":\"$R1\",\"type\":\"fridge\",\"name\":\"Ali fridge 2 $SUF\",\"attributes\":{\"max_temp\":6,\"storage_class\":\"2-8C\"},\"reason\":\"e2e\"}" | jget id)
FZ=$(req POST /api/v1/assets "{\"site_id\":\"$S1\",\"parent_id\":\"$R1\",\"type\":\"freezer\",\"name\":\"Ali freezer $SUF\",\"reason\":\"e2e\"}" | jget id)
MA=$(req POST /api/v1/assets "{\"site_id\":\"$S2\",\"type\":\"meter\",\"name\":\"Ali meter $SUF\",\"reason\":\"e2e\"}" | jget id)
[[ -n "$B1" && -n "$R1" && -n "$F1" && -n "$F2" && -n "$FZ" && -n "$MA" ]] && ok "asset tree" || bad "asset tree: $(head -c 200 "$TMP/b")"
mkds() { req POST /api/v1/datastreams "{\"asset_id\":\"$1\",\"key\":\"$2\",\"quantity\":\"$3\",\"unit\":\"$4\",\"kind\":\"$5\",\"expected_interval\":\"15 minutes\",\"reason\":\"e2e\"}" | jget id; }
D1=$(mkds "$F1" temp temperature °C gauge); D2=$(mkds "$F2" temp temperature °C gauge); DZ=$(mkds "$FZ" temp temperature °C gauge); DM=$(mkds "$MA" energy energy_active_import kWh counter)
D1D=$(mkds "$F1" door door "" gauge)
[[ -n "$D1" && -n "$D2" && -n "$DZ" && -n "$DM" && -n "$D1D" ]] && ok "datastreams" || bad "datastreams: $(head -c 200 "$TMP/b")"
mkdev() { req POST /api/v1/devices "{\"external_id\":\"ali-$1-$SUF\",\"name\":\"Ali $1 logger $SUF\",\"transport\":\"http\",\"reason\":\"e2e\"}" | jget id; }
V1=$(mkdev f1); V2=$(mkdev f2); V3=$(mkdev fz); V4=$(mkdev meter)
for x in "$V1 $D1" "$V1 $D1D" "$V2 $D2" "$V3 $DZ" "$V4 $DM"; do set -- $x; req POST "/api/v1/devices/$1/datastreams" "{\"datastream_id\":\"$2\",\"reason\":\"e2e\"}" >/dev/null; done
check "datastreams assigned" "$(st)" 201
tok() { req POST "/api/v1/devices/$1/token" '{"reason":"e2e"}' | jget token; }
T1=$(tok "$V1"); T2=$(tok "$V2"); T3=$(tok "$V3"); T4=$(tok "$V4")
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$D2\",\"type\":\"high\",\"warning\":7.5,\"action\":8,\"reason\":\"e2e\"}" >/dev/null; [[ "$(st)" == "201" || "$(st)" == "202" ]] && ok "alarm rule of fridge 2" || bad "alarm rule: $(st)"
NOW=$(date +%s)
check "fridge 1: 7.5 °C, door closed" "$(dev "$T1" "{\"ts\":$NOW,\"v\":{\"temp\":7.5,\"door\":0}}")" 202
check "fridge 2: 8.6 °C" "$(dev "$T2" "{\"ts\":$NOW,\"v\":{\"temp\":8.6}}")" 202
check "freezer: -18 °C" "$(dev "$T3" "{\"ts\":$NOW,\"v\":{\"temp\":-18}}")" 202
check "meter" "$(dev "$T4" "{\"ts\":$NOW,\"v\":{\"energy\":1200}}")" 202
sleep 2

echo "== alias kinds"
eq /api/v1/entities/query "alias={\"kind\":\"type\",\"entity_type\":\"asset\",\"types\":[\"fridge\"]}" "limit=500"; check "by type: fridges" "$(st)" 200
check "both fridges" "$(jx "j.items.filter(x=>x.name.endsWith('$SUF')).map(x=>x.name).sort().join('|')")" "Ali fridge 1 $SUF|Ali fridge 2 $SUF"
check "entity fields" "$(jx "(r=>[r.type,r.site,r.asset_type,r.datastreams].join('|'))(j.items.find(x=>x.id==='$F1'))")" "asset|Ali pharmacy $SUF|fridge|2"
eq /api/v1/entities/query "alias={\"kind\":\"name\",\"entity_type\":\"device\",\"match\":\"starts\",\"text\":\"Ali f\"}" "limit=500"
check "by name: starts with" "$(jx "j.items.filter(x=>x.name.endsWith('$SUF')).length")" 3
eq /api/v1/entities/query "alias={\"kind\":\"name\",\"entity_type\":\"asset\",\"match\":\"ends\",\"text\":\"freezer $SUF\"}"
check "by name: ends with" "$(names)" "Ali freezer $SUF"
eq /api/v1/entities/query "alias={\"kind\":\"single\",\"entity\":{\"type\":\"site\",\"id\":\"$S2\"}}"
check "one entity (a site)" "$(jx '[j.type, j.items.length, j.items[0].name].join("|")')" "site|1|Ali plant $SUF"
eq /api/v1/entities/query "alias={\"kind\":\"list\",\"entity_type\":\"device\",\"ids\":[\"$V4\",\"$V1\"]}"
check "chosen entities, sorted by name" "$(jx 'j.items.map(x=>x.id).join()')" "$V1,$V4"
eq /api/v1/entities/query "alias={\"kind\":\"hierarchy\",\"root\":{\"type\":\"asset\",\"id\":\"$B1\"},\"entity_type\":\"datastream\"}" "limit=500"
check "hierarchy: every datastream under the building" "$(jx 'j.items.length')" 4
eq /api/v1/entities/query "alias={\"kind\":\"hierarchy\",\"root\":{\"type\":\"asset\",\"id\":\"$B1\"},\"depth\":1,\"entity_type\":\"asset\"}"
check "hierarchy one level deep: the room only" "$(names)" "Ali store room $SUF"
eq /api/v1/entities/query "alias={\"kind\":\"hierarchy\",\"root\":{\"type\":\"asset\",\"id\":\"$B1\"},\"depth\":1,\"entity_type\":\"datastream\"}"
check "hierarchy one level: the room carries no datastream" "$(jx 'j.items.length')" 0
eq /api/v1/entities/query "alias={\"kind\":\"hierarchy\",\"root\":{\"type\":\"asset\",\"id\":\"$B1\"},\"depth\":2,\"entity_type\":\"datastream\"}"
check "hierarchy two levels: the fridges and the freezer (level 2)" "$(jx 'j.items.length')" 4
eq /api/v1/entities/query "alias={\"kind\":\"hierarchy\",\"root\":{\"type\":\"site\",\"id\":\"$S1\"},\"entity_type\":\"device\",\"asset_type\":[\"fridge\"]}"
check "devices of fridges under a site" "$(names)" "Ali f1 logger $SUF|Ali f2 logger $SUF"
eq /api/v1/entities/query "alias={\"kind\":\"hierarchy\",\"root_current\":true,\"entity_type\":\"device\"}" "current=asset:$R1"
check "devices under the current entity" "$(jx 'j.items.length')" 3
eq /api/v1/entities/query "alias={\"kind\":\"hierarchy\",\"root_current\":true,\"entity_type\":\"device\"}"
check "under the current entity, none chosen: empty" "$(jx 'j.total')" 0
eq /api/v1/entities/query "alias={\"kind\":\"current\",\"entity_type\":\"datastream\"}" "current=device:$V1"
check "current device → its datastreams" "$(jx 'j.items.map(x=>x.key).sort().join()')" "door,temp"
eq /api/v1/entities/query "alias={\"kind\":\"current\",\"default\":{\"type\":\"asset\",\"id\":\"$F2\"}}"
check "current entity: the default when none is chosen" "$(names)" "Ali fridge 2 $SUF"
eq /api/v1/entities/query "alias={\"kind\":\"current\",\"entity_type\":\"device\"}" "current=datastream:$D2"
check "current datastream → its device" "$(names)" "Ali f2 logger $SUF"
eq /api/v1/entities/query "alias={\"kind\":\"query\",\"source\":{\"quantity\":[\"energy*\"],\"site\":[\"$S2\"]}}"
check "query alias" "$(jx '[j.type, j.items.map(x=>x.id).join()].join("|")')" "datastream|$DM"

echo "== key filters"
FR="alias={\"kind\":\"hierarchy\",\"root\":{\"type\":\"asset\",\"id\":\"$R1\"},\"entity_type\":\"asset\",\"asset_type\":[\"fridge\"]}"
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"value","key":"temp","op":"gt","value":8}]}'; check "fridges above 8 °C" "$(names)" "Ali fridge 2 $SUF"
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"value","key":"temp","op":"between","value":7,"value2":8}]}'; check "between" "$(names)" "Ali fridge 1 $SUF"
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"value","key":"temp","op":"gt","dynamic":{"from":"self","key":"max_temp"}}]}'; check "above its own limit (attribute)" "$(names)" "Ali fridge 2 $SUF"
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"value","key":"temp","op":"gt","dynamic":{"from":"filter","key":"limit"},"value":100}]}' "f_value=limit=7"; check "above a value filter" "$(jx 'j.items.length')" 2
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"value","key":"temp","op":"gt","dynamic":{"from":"filter","key":"limit"},"value":8}]}'; check "the constant when the value filter is empty" "$(names)" "Ali fridge 2 $SUF"
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"value","key":"temp","op":"lt","dynamic":{"from":"current","key":"max_temp"}}]}' "current=asset:$F1"; check "below an attribute of the current entity" "$(names)" "Ali fridge 1 $SUF"
req PUT /api/v1/organization/attributes '{"attributes":{"limit":8.2,"note":"cold chain"},"reason":"e2e"}' >/dev/null; check "organization attributes set" "$(st)" 200
req GET /api/v1/organization/attributes >/dev/null; check "organization attributes read" "$(jx 'j.attributes.limit')" 8.2
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"value","key":"temp","op":"gt","dynamic":{"from":"org","key":"limit"}}]}'; check "above an attribute of the organization" "$(names)" "Ali fridge 2 $SUF"
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"attribute","key":"storage_class","type":"string","op":"in","value":["15-25C","2-8c"]}]}'; check "attribute in a list (case-insensitive)" "$(jx 'j.items.length')" 2
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"alarm","op":"eq","value":"any"}]}'; check "in alarm" "$(names)" "Ali fridge 2 $SUF"
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"stale","key":"temp"}]}'; check "no stale fridge" "$(jx 'j.items.length')" 0
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"stale","key":"humidity"}]}'; check "a missing key counts as stale" "$(jx 'j.items.length')" 2
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"quality","key":"temp","op":"eq","value":"good"}]}'; check "quality good" "$(jx 'j.items.length')" 2
eq /api/v1/entities/query "$FR" 'filter={"match":"any","rules":[{"source":"value","key":"temp","op":"lt","value":0},{"match":"all","rules":[{"source":"value","key":"door","op":"eq","value":0},{"source":"value","key":"temp","op":"gt","value":7}]}]}'; check "any of (…, all of (…)) — a group" "$(names)" "Ali fridge 1 $SUF"
eq /api/v1/entities/query "$FR" 'highlight={"rules":[{"source":"value","key":"temp","op":"gt","value":8}]}' "values=temp,door" "attrs=max_temp"
check "highlight marks, never hides" "$(jx "j.items.map(x=>x.name.split(' ')[2]+':'+x.match).join()")" "1:false,2:true"
check "latest values and attributes per entity" "$(jx "(r=>[r.values.temp.value, r.values.temp.unit, r.values.door.value, r.attributes.max_temp].join('|'))(j.items.find(x=>x.id==='$F1'))")" "7.5|°C|0|8"
check "alarm of the entity" "$(jx "j.items.find(x=>x.id==='$F2').alarm.severity")" "action"

echo "== answers: datastreams, devices, dashboard filters, limits"
eq /api/v1/entities/query "$FR" 'filter={"rules":[{"source":"value","key":"temp","op":"gt","value":8}]}' "want=datastreams" "latest=1"
check "datastreams of the filtered fridges" "$(jx 'j.items.map(x=>x.id).join()')" "$D2"
check "with the latest value and the limits" "$(jx 'j.items[0].latest.value+"|"+j.items[0].limits.high_action')" "8.6|8"
eq /api/v1/entities/query "$FR" "want=datastreams" "quantity=temperature"; check "narrowed to a quantity" "$(jx 'j.items.length')" 2
eq /api/v1/entities/query "$FR" "want=datastreams" "key=door"; check "narrowed to a key" "$(jx 'j.items.map(x=>x.id).join()')" "$D1D"
eq /api/v1/entities/query "$FR" "want=devices"; check "devices of the fridges" "$(jx 'j.items.map(x=>x.name).sort().join("|")')" "Ali f1 logger $SUF|Ali f2 logger $SUF"
eq /api/v1/entities/query "$FR" "want=datastreams" 'highlight={"rules":[{"source":"value","op":"gt","value":8}]}' "key=temp"; check "highlight of datastreams" "$(jx "j.items.map(x=>x.match).join()")" "false,true"
eq /api/v1/entities/query "$FR" "follow=1" "f_site=$S2"; check "dashboard filters narrow (other site: nothing)" "$(jx 'j.total')" 0
eq /api/v1/entities/query "$FR" "follow=1" "f_site=$S1" "f_q=fridge 2"; check "dashboard filters narrow (search)" "$(names)" "Ali fridge 2 $SUF"
eq /api/v1/entities/query "$FR" "limit=1"; check "limit and truncation" "$(jx '[j.items.length, j.total, j.truncated].join()')" "1,2,true"
for BADQ in 'alias=nope' 'alias={"kind":"magic"}' 'alias={"kind":"current"}|current=device' 'alias={"kind":"current"}|want=all' 'alias={"kind":"current"}|limit=501' 'alias={"kind":"current"}|filter={"rules":[]}' 'alias={"kind":"current"}|values=Bad' 'alias={"kind":"current"}|f_value=bad key=1'; do
  IFS='|' read -r -a parts <<< "$BADQ"; eq /api/v1/entities/query "${parts[@]}"; check "refused: ${BADQ:0:42}" "$(st)" 400; done
eq /api/v1/entities/query; check "refused: no alias" "$(st)" 400
req PUT /api/v1/organization/attributes '{"attributes":{"bad name":1},"reason":"e2e"}' >/dev/null; check "organization attributes: bad name refused" "$(st)" 400
req PUT /api/v1/organization/attributes '{"attributes":{"x":{"a":1}},"reason":"e2e"}' >/dev/null; check "organization attributes: objects refused" "$(st)" 400
req PUT /api/v1/organization/attributes '{"attributes":{"x":1}}' >/dev/null; check "organization attributes: a reason is required" "$(st)" 400
req GET "/api/v1/audit?limit=100" >/dev/null; [[ "$(jx 'JSON.stringify(j).includes("organization.attributes")')" == "true" ]] && ok "organization attributes audited" || bad "organization attributes audited: $(head -c 200 "$TMP/b")"
req PUT /api/v1/organization/attributes '{"attributes":{},"reason":"e2e cleanup"}' >/dev/null

echo "== dashboards of format 1.2"
req GET /api/v1/dashboards/catalog >/dev/null; check "catalog: format 1.2" "$(jx '[j.format_minor, j.alias_kinds.length, j.widgets.some(w=>w.type==="entities_table"&&w.entities), j.widgets.some(w=>w.type==="entity_selector")].join()')" "2,7,true,true"
SPEC="{\"version\":1,\"aliases\":[{\"id\":\"fridges\",\"name\":\"Fridges\",\"kind\":\"hierarchy\",\"root\":{\"type\":\"asset\",\"id\":\"$R1\"},\"entity_type\":\"asset\",\"asset_type\":[\"fridge\"]},{\"id\":\"sel\",\"name\":\"Selected\",\"kind\":\"current\",\"entity_type\":\"datastream\"}],
 \"key_filters\":[{\"id\":\"warm\",\"name\":\"Above 8\",\"rules\":[{\"source\":\"value\",\"key\":\"temp\",\"op\":\"gt\",\"value\":8}]}],
 \"states\":[{\"id\":\"main\",\"name\":\"Overview\"},{\"id\":\"detail\",\"name\":\"Fridge detail\"}],
 \"widgets\":[{\"id\":\"table\",\"type\":\"entities_table\",\"geometry\":{\"x\":0,\"y\":0,\"w\":12,\"h\":4},\"alias\":{\"id\":\"fridges\"},\"params\":{\"columns\":[\"name\",\"t:temp\",\"a:max_temp\"],\"highlight_filter\":\"warm\"},\"actions\":[{\"event\":\"row\",\"action\":\"state\",\"target\":\"detail\"}]},
  {\"id\":\"warm\",\"type\":\"overview_table\",\"geometry\":{\"x\":0,\"y\":4,\"w\":12,\"h\":4},\"alias\":{\"id\":\"fridges\",\"filter\":\"warm\",\"quantity\":[\"temperature\"]},\"bindings\":{\"datastreams\":[]}},
  {\"id\":\"trend\",\"type\":\"chart\",\"state\":\"detail\",\"geometry\":{\"x\":0,\"y\":0,\"w\":12,\"h\":3},\"alias\":{\"id\":\"sel\"},\"bindings\":{\"datastreams\":[]}}]}"
req POST /api/v1/dashboards "{\"name\":\"Ali drill-down $SUF\",\"spec\":$SPEC,\"reason\":\"e2e\"}" >/dev/null; check "dashboard with aliases, key filters and states" "$(st)" 201
SLUG=$(jx 'j.slug')
check "stored: root state left out, detail kept" "$(jx '[j.spec.widgets[0].state||"", j.spec.widgets[2].state, j.spec.states.length, j.spec.aliases.length].join()')" ",detail,2,2"
req PUT "/api/v1/dashboards/$SLUG" "{\"spec\":${SPEC/\"target\":\"detail\"/\"target\":\"gone\"},\"reason\":\"e2e\"}" >/dev/null; check "an action to a missing state is refused" "$(st)" 400
req PUT "/api/v1/dashboards/$SLUG" "{\"spec\":${SPEC/\"alias\":{\"id\":\"sel\"}/\"alias\":{\"id\":\"gone\"}},\"reason\":\"e2e\"}" >/dev/null; check "a missing alias is refused" "$(st)" 400

echo "== public link: only within the dashboard's scope"
TOKEN=$(req POST "/api/v1/dashboards/$SLUG/share" '{"reason":"e2e"}' | jget token); [[ -n "$TOKEN" ]] && ok "public link" || bad "public link"
anon "/api/v1/public/$TOKEN/entities/query" "widget=table"; check "entities table of the link" "$(st)" 200
check "the fridges, highlighted by the key filter" "$(jx "j.items.map(x=>x.name.split(' ')[2]+':'+x.match).join()")" "1:false,2:true"
check "columns from the stored widget" "$(jx "(r=>r.values.temp.value+'|'+r.attributes.max_temp)(j.items.find(x=>x.name.startsWith('Ali fridge 1')))")" "7.5|8"
anon "/api/v1/public/$TOKEN/entities/query" "widget=warm" "latest=1"; check "overview table with the key filter" "$(jx 'j.items.map(x=>x.id).join()')" "$D2"
anon "/api/v1/public/$TOKEN/entities/query" "widget=table" 'alias={"kind":"type","entity_type":"device","types":["*"]}'; check "an alias in the request is ignored" "$(jx 'j.items.every(x=>x.type==="asset")')" true
anon "/api/v1/public/$TOKEN/entities/query" "widget=trend" "current=datastream:$D1"; check "detail state: the current entity in scope" "$(jx 'j.items.map(x=>x.id).join()')" "$D1"
anon "/api/v1/public/$TOKEN/entities/query" "widget=trend" "current=datastream:$DM"; check "detail state: a current entity outside the scope is ignored" "$(jx 'j.total')" 0
anon "/api/v1/public/$TOKEN/entities/query" "widget=trend" "current=device:$V4"; check "a device outside the scope is ignored" "$(jx 'j.total')" 0
anon "/api/v1/public/$TOKEN/datastreams/latest" "ids=$D1"; check "latest value of a datastream the aliases select" "$(st)" 200
anon "/api/v1/public/$TOKEN/datastreams/latest" "ids=$DM"; check "the meter is outside the scope" "$(st)" 403
anon "/api/v1/public/$TOKEN/observations" "datastream_id=$DZ"; check "the freezer is outside the scope (the alias takes fridges only)" "$(st)" 403
anon "/api/v1/public/$TOKEN/entities/query" "widget=nope"; check "unknown widget" "$(st)" 404
anon "/api/v1/public/notatoken0000000000000000000000000000/entities/query" "widget=table"; check "unknown link" "$(st)" 404
req DELETE "/api/v1/dashboards/$SLUG/share?reason=e2e" >/dev/null
anon "/api/v1/public/$TOKEN/entities/query" "widget=table"; check "withdrawn link" "$(st)" 404
req DELETE "/api/v1/dashboards/$SLUG?reason=e2e" >/dev/null; check "dashboard retired" "$(st)" 200

echo
echo "aliases: $pass passed, $fail failed"
[[ $fail == 0 ]]
