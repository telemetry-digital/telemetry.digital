#!/usr/bin/env bash
# End-to-end checks for overviews (0.68.0): datastreams selected by query, filter choices (facets), the export of an
# overview table, dashboards with a filter bar and query sources, the ready-made overviews, and public links that
# never read outside the dashboard's scope. Telemetry goes through the device HTTP API; JSON extraction with Node.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/filters.sh
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
anon() { curl -s -o "$TMP/b" -w '%{http_code}' "$BASE$1" > "$TMP/s"; cat "$TMP/b"; }
dev()  { curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $1" -H 'Content-Type: application/json' -d "$2" "$BASE/api/v1/d/telemetry"; }
SUF=$RANDOM

echo "== login"
req GET "/api/v1/datastreams/query" >/dev/null; [[ "$(st)" == "401" || "$(st)" == "403" ]] && ok "query needs auth ($(st))" || bad "query needs auth (got $(st))"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== data: two sites, fridges with a storage class, a meter, a device per site"
S1=$(req POST /api/v1/sites "{\"name\":\"Flt pharmacy $SUF\",\"reason\":\"e2e\"}" | jget id); S2=$(req POST /api/v1/sites "{\"name\":\"Flt plant $SUF\",\"reason\":\"e2e\"}" | jget id)
[[ -n "$S1" && -n "$S2" ]] && ok "sites" || bad "sites"
A1=$(req POST /api/v1/assets "{\"site_id\":\"$S1\",\"type\":\"fridge\",\"name\":\"Flt fridge 1 $SUF\",\"attributes\":{\"storage_class\":\"2-8C\"},\"reason\":\"e2e\"}" | jget id)
A2=$(req POST /api/v1/assets "{\"site_id\":\"$S1\",\"type\":\"freezer\",\"name\":\"Flt freezer $SUF\",\"attributes\":{\"storage_class\":\"-25..-15C\"},\"reason\":\"e2e\"}" | jget id)
A3=$(req POST /api/v1/assets "{\"site_id\":\"$S2\",\"type\":\"meter\",\"name\":\"Flt meter $SUF\",\"reason\":\"e2e\"}" | jget id)
A4=$(req POST /api/v1/assets "{\"site_id\":\"$S2\",\"type\":\"meter\",\"name\":\"Out meter $SUF\",\"reason\":\"e2e\"}" | jget id)
[[ -n "$A1" && -n "$A2" && -n "$A3" && -n "$A4" ]] && ok "assets" || bad "assets: $(head -c 160 "$TMP/b")"
D1=$(req POST /api/v1/datastreams "{\"asset_id\":\"$A1\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"°C\",\"kind\":\"gauge\",\"expected_interval\":\"15 minutes\",\"reason\":\"e2e\"}" | jget id)
D2=$(req POST /api/v1/datastreams "{\"asset_id\":\"$A2\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"°C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
D3=$(req POST /api/v1/datastreams "{\"asset_id\":\"$A3\",\"key\":\"energy\",\"quantity\":\"energy_active_import\",\"unit\":\"kWh\",\"kind\":\"counter\",\"reason\":\"e2e\"}" | jget id)
D4=$(req POST /api/v1/datastreams "{\"asset_id\":\"$A4\",\"key\":\"energy\",\"quantity\":\"energy_active_import\",\"unit\":\"kWh\",\"kind\":\"counter\",\"reason\":\"e2e\"}" | jget id)
[[ -n "$D1" && -n "$D2" && -n "$D3" && -n "$D4" ]] && ok "datastreams" || bad "datastreams: $(head -c 160 "$TMP/b")"
V1=$(req POST /api/v1/devices "{\"external_id\":\"flt-a-$SUF\",\"name\":\"Flt logger $SUF\",\"transport\":\"http\",\"reason\":\"e2e\"}" | jget id)
V2=$(req POST /api/v1/devices "{\"external_id\":\"flt-b-$SUF\",\"name\":\"Flt meter dev $SUF\",\"transport\":\"http\",\"reason\":\"e2e\"}" | jget id)
V3=$(req POST /api/v1/devices "{\"external_id\":\"flt-c-$SUF\",\"name\":\"Flt freezer logger $SUF\",\"transport\":\"http\",\"reason\":\"e2e\"}" | jget id)
T1=$(req POST "/api/v1/devices/$V1/token" '{"reason":"e2e"}' | jget token); T2=$(req POST "/api/v1/devices/$V2/token" '{"reason":"e2e"}' | jget token); T3=$(req POST "/api/v1/devices/$V3/token" '{"reason":"e2e"}' | jget token)
for x in "$V1 $D1" "$V3 $D2" "$V2 $D3"; do set -- $x; req POST "/api/v1/devices/$1/datastreams" "{\"datastream_id\":\"$2\",\"reason\":\"e2e\"}" >/dev/null; done
check "datastreams assigned" "$(st)" 201
NOW=$(date +%s)
B1="["; B2="["; for i in 5 4 3 2 1 0; do t=$((NOW - i * 600)); B1="$B1{\"ts\":$t,\"v\":{\"temp\":$((4 + i)).5}},"; B2="$B2{\"ts\":$t,\"v\":{\"energy\":$((1000 + (5 - i) * 10))}},"; done
B1="${B1%,}]"; B2="${B2%,}]"
check "telemetry of the fridge logger" "$(dev "$T1" "$B1")" 202
check "telemetry of the meter" "$(dev "$T2" "$B2")" 202
check "freezer reading" "$(dev "$T3" "{\"ts\":$NOW,\"v\":{\"temp\":-18}}")" 202
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$D1\",\"type\":\"high\",\"warning\":7.5,\"action\":8,\"reason\":\"e2e\"}" >/dev/null; [[ "$(st)" == "201" || "$(st)" == "202" ]] && ok "alarm rule with limits ($(st))" || bad "alarm rule: $(st) $(head -c 160 "$TMP/b")"
sleep 1

echo "== query"
req GET "/api/v1/datastreams/query?site=$S1&latest=1" >/dev/null; check "by site" "$(st)" 200
check "both fridges of the pharmacy" "$(jx 'j.items.map(x=>x.asset).sort().join("|")')" "Flt freezer $SUF|Flt fridge 1 $SUF"
check "latest value, device and site" "$(jx "(r=>[r.latest.value, r.device, r.site, r.interval_s].join('|'))(j.items.find(x=>x.id==='$D1'))")" "4.5|Flt logger $SUF|Flt pharmacy $SUF|900"
check "limits of the alarm rule" "$(jx "JSON.stringify(j.items.find(x=>x.id==='$D1').limits)")" '{"high_action":8,"high_warn":7.5}'
req GET "/api/v1/datastreams/query?attr=storage_class=2-8C&quantity=temperature" >/dev/null; check "by attribute and quantity" "$(jx "j.items.filter(x=>x.asset.endsWith('$SUF')).map(x=>x.id).join()")" "$D1"
req GET "/api/v1/datastreams/query?quantity=energy*&site=$S2" >/dev/null; check "quantity with a wildcard" "$(jx 'j.items.map(x=>x.id).join()')" "$D3,$D4"
req GET "/api/v1/datastreams/query?name=Flt%20fr*%20$SUF" >/dev/null; check "name pattern" "$(jx 'j.items.length')" 2
req GET "/api/v1/datastreams/query?q=flt%20logger%20$SUF" >/dev/null; check "free text matches the device" "$(jx 'j.items.map(x=>x.id).join()')" "$D1"
req GET "/api/v1/datastreams/query?device=$V2" >/dev/null; check "by device" "$(jx 'j.items.map(x=>x.id).join()')" "$D3"
req GET "/api/v1/datastreams/query?site=$S1&f_site=$S2" >/dev/null; check "a filter only narrows the source" "$(jx 'j.total')" 0
req GET "/api/v1/datastreams/query?ids=$D1,$D3&f_quantity=temperature" >/dev/null; check "fixed list narrowed by a filter" "$(jx 'j.items.map(x=>x.id).join()')" "$D1"
req GET "/api/v1/datastreams/query?site=$S1&limit=1" >/dev/null; check "limit and truncation" "$(jx '[j.items.length, j.total, j.truncated].join()')" "1,2,true"
FROM=$(date -u -d '-2 hours' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-2H +%Y-%m-%dT%H:%M:%SZ)
req GET "/api/v1/datastreams/query?ids=$D1,$D3&from=$FROM" >/dev/null; check "window: change of the meter" "$(jx "j.items.find(x=>x.id==='$D3').window.change")" 50
check "window: minimum and maximum" "$(jx "(w=>w.min+'..'+w.max)(j.items.find(x=>x.id==='$D1').window)")" "4.5..9.5"
for BADQ in "site=nope" "limit=501" "attr=bad%20key=1" "from=yesterday" "ids=1,2" "q=$(printf 'x%.0s' $(seq 90))"; do req GET "/api/v1/datastreams/query?$BADQ" >/dev/null; check "refused: ${BADQ:0:20}" "$(st)" 400; done

echo "== facets"
req GET "/api/v1/datastreams/facets?kinds=site,asset_type,quantity,device,attr:storage_class,attr_keys&f_site=$S1" >/dev/null; check "facets" "$(st)" 200
check "the site filter does not narrow its own choices" "$(jx "['$S1','$S2'].every(id=>j.site.some(x=>x.value===id))")" true
check "other choices follow the site" "$(jx "j.asset_type.some(x=>x.value==='meter')")" false
check "attribute values with counts" "$(jx "j['attr:storage_class'].filter(x=>x.value==='2-8C'||x.value==='-25..-15C').length")" 2
check "attribute names" "$(jx 'j.attr_keys.includes("storage_class")')" true
req GET "/api/v1/datastreams/facets?kinds=site,nope" >/dev/null; check "unknown facet refused" "$(st)" 400

echo "== export of an overview table"
curl -s -b "$JAR" -X POST -H "X-CSRF-Token: $CSRF" -H 'Content-Type: application/json' -d "{\"ids\":[\"$D3\",\"$D1\"],\"columns\":[\"site\",\"asset\",\"value\",\"limits\",\"change\"],\"format\":\"csv\",\"title\":\"Flt $SUF\",\"from\":\"$FROM\"}" -o "$TMP/x.csv" -w '%{http_code}' "$BASE/api/v1/datastreams/export" > "$TMP/s"
check "csv export" "$(st)" 200
check "csv header with the unit column" "$(sed -n 1p "$TMP/x.csv" | tr -d '\r\357\273\277')" "Site,Asset,Value,Unit,Limits,Change"
check "rows in the order of the table" "$(sed -n 2p "$TMP/x.csv" | cut -d, -f2),$(sed -n 3p "$TMP/x.csv" | cut -d, -f2)" "Flt meter $SUF,Flt fridge 1 $SUF"
check "limits and change" "$(sed -n 3p "$TMP/x.csv" | cut -d, -f5),$(sed -n 2p "$TMP/x.csv" | cut -d, -f6)" "≤ 8,50.00"
curl -s -b "$JAR" -X POST -H "X-CSRF-Token: $CSRF" -H 'Content-Type: application/json' -d "{\"ids\":[\"$D1\"],\"format\":\"xlsx\"}" -o "$TMP/x.xlsx" -w '%{http_code} %{content_type}' "$BASE/api/v1/datastreams/export" > "$TMP/s"
check "xlsx export" "$(st)" "200 application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
check "xlsx is a zip" "$(head -c 2 "$TMP/x.xlsx")" "PK"
req POST /api/v1/datastreams/export "{\"ids\":[\"$D1\"],\"columns\":[\"secret\"]}" >/dev/null; check "unknown column refused" "$(st)" 400
req POST /api/v1/datastreams/export '{"ids":[]}' >/dev/null; check "no ids refused" "$(st)" 400
req GET "/api/v1/export/audit" >/dev/null; grep -q '"overview_table"' "$TMP/b" && ok "export audited" || bad "export not audited"

echo "== dashboard with filters and query sources (format 1.1)"
SPEC="{\"version\":1,\"range\":\"24h\",\"filters\":[{\"kind\":\"site\",\"multi\":true,\"public\":true},{\"kind\":\"attribute\",\"key\":\"storage_class\",\"label\":\"Storage class\",\"public\":true},{\"kind\":\"quantity\",\"default\":[\"temperature\"]},{\"kind\":\"search\"},{\"kind\":\"range\",\"default\":\"7d\",\"public\":true}],\"widgets\":[{\"id\":\"t\",\"type\":\"overview_table\",\"geometry\":{\"x\":0,\"y\":0,\"w\":12,\"h\":5},\"bindings\":{\"datastreams\":[]},\"params\":{\"group_by\":\"site\"},\"source\":{\"name\":\"Flt * $SUF\"},\"follow\":true},{\"id\":\"c\",\"type\":\"chart\",\"geometry\":{\"x\":0,\"y\":5,\"w\":8,\"h\":3},\"bindings\":{\"datastreams\":[]},\"params\":{},\"source\":{\"quantity\":[\"temperature\"],\"site\":[\"$S1\"],\"limit\":12}},{\"id\":\"a\",\"type\":\"alarms\",\"geometry\":{\"x\":8,\"y\":5,\"w\":4,\"h\":3},\"bindings\":{},\"params\":{},\"follow\":true}]}"
req POST /api/v1/dashboards "{\"name\":\"Flt overview $SUF\",\"spec\":$SPEC,\"reason\":\"e2e\"}" >/dev/null; check "dashboard with filters saved" "$(st)" 201
FS=$(jget slug < "$TMP/b")
check "filters canonical" "$(jx 'j.spec.filters.map(f=>f.kind+(f.public?"+":"")).join()')" "site+,attribute+,quantity,search,range+"
check "source and follow stored" "$(jx '[j.spec.widgets[0].source.name, j.spec.widgets[0].source.limit, j.spec.widgets[0].follow, j.spec.widgets[1].source.limit, j.spec.widgets[2].follow].join()')" "Flt * $SUF,500,true,12,true"
req GET /api/v1/dashboards/catalog >/dev/null; check "catalog announces format 1.2 (0.73.0) and the filter kinds" "$(jx '[j.format, j.format_minor, j.filter_kinds.length, j.widgets.some(w=>w.type==="overview_table")].join()')" "screen/v1,2,8,true"
for BADS in '"filters":[{"kind":"customer"}]' '"filters":[{"kind":"attribute"}]' '"filters":[{"kind":"site","default":["x"]}]' '"filters":[{"kind":"range","default":"2h"}]'; do
  req POST /api/v1/dashboards "{\"name\":\"Bad $SUF\",\"spec\":{\"version\":1,$BADS,\"widgets\":[]},\"reason\":\"e2e\"}" >/dev/null; check "refused: ${BADS:11:30}" "$(st)" 400; done
req POST /api/v1/dashboards "{\"name\":\"Bad $SUF\",\"spec\":{\"version\":1,\"widgets\":[{\"type\":\"text\",\"params\":{},\"source\":{}}]},\"reason\":\"e2e\"}" >/dev/null; check "refused: a source on a text widget" "$(st)" 400
req POST /api/v1/dashboards "{\"name\":\"Bad $SUF\",\"spec\":{\"version\":1,\"widgets\":[{\"type\":\"chart\",\"bindings\":{},\"params\":{},\"source\":{\"limit\":25}}]},\"reason\":\"e2e\"}" >/dev/null; check "refused: a chart with 25 series" "$(st)" 400

echo "== ready-made overviews"
for K in sensors energy coldchain; do
  req GET "/api/v1/dashboards/starter?kind=$K" >/dev/null; check "starter $K" "$(st)" 200
  [[ "$(jx '[j.spec.filters.length >= 3, j.spec.widgets.some(w=>w.source), j.spec.widgets.every(w=>w.follow)].join()')" == "true,true,true" ]] && ok "$K: filters, a query source, every widget follows" || bad "$K spec: $(head -c 200 "$TMP/b")"
done
req GET "/api/v1/dashboards/starter?kind=nope" >/dev/null; check "unknown starter refused" "$(st)" 400

echo "== public link: only the dashboard's scope, only public filters"
TOKEN=$(req POST "/api/v1/dashboards/$FS/share" '{"reason":"e2e"}' | jget token); check "public link" "$(st)" 201
anon "/api/v1/public/$TOKEN/datastreams/query?widget=t&latest=1" >/dev/null; check "public query" "$(st)" 200
check "the quantity filter (not public) keeps its default" "$(jx 'j.items.map(x=>x.quantity).filter((v,i,a)=>a.indexOf(v)===i).join()')" "temperature"
anon "/api/v1/public/$TOKEN/datastreams/query?widget=t&f_quantity=energy_active_import" >/dev/null; check "a viewer cannot change a filter that is not public" "$(jx 'j.items.some(x=>x.id===`'"$D3"'`)')" false
anon "/api/v1/public/$TOKEN/datastreams/query?widget=t&f_attr=storage_class=2-8C" >/dev/null; check "a public filter narrows" "$(jx 'j.items.map(x=>x.id).join()')" "$D1"
anon "/api/v1/public/$TOKEN/datastreams/query?widget=c&f_site=$S2" >/dev/null; check "a widget that does not follow ignores filters" "$(jx 'j.items.length')" 2
anon "/api/v1/public/$TOKEN/datastreams/query?widget=nope" >/dev/null; check "unknown widget 404" "$(st)" 404
anon "/api/v1/public/$TOKEN/datastreams/facets?kinds=site,quantity,attr:storage_class" >/dev/null; check "public facets" "$(st)" 200
check "only public filters offer choices" "$(jx '[!!j.site, !!j.quantity, !!j["attr:storage_class"]].join()')" "true,false,true"
check "choices only from the dashboard's scope" "$(jx "j.site.map(x=>x.value).join()")" "$S1"
anon "/api/v1/public/$TOKEN/datastreams" >/dev/null; check "datastreams of the query sources are readable" "$(jx "['$D1','$D2'].every(id=>j.some(x=>x.id===id))")" true
check "nothing outside the sources" "$(jx "j.some(x=>x.id==='$D4')")" false
anon "/api/v1/public/$TOKEN/datastreams/latest?ids=$D4" >/dev/null; check "latest of a datastream outside the scope refused" "$(st)" 403
anon "/api/v1/public/$TOKEN/observations?datastream_id=$D1&from=$FROM" >/dev/null; check "observations of a resolved datastream" "$(st)" 200
anon "/api/v1/public/$TOKEN/alarms" >/dev/null; check "public alarms carry the datastream" "$(jx 'j.every(a=>"datastream_id" in a)')" true
req DELETE "/api/v1/dashboards/$FS/share?reason=e2e" >/dev/null; check "public link withdrawn" "$(st)" 200
anon "/api/v1/public/$TOKEN/datastreams/query?widget=t" >/dev/null; check "withdrawn link answers 404" "$(st)" 404

echo "== permissions"
TOK=$(req POST /api/v1/tokens '{"name":"flt e2e","scopes":["data.read"]}' | jget token); check "token with data.read only" "$(st)" 201
curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOK" "$BASE/api/v1/datastreams/query?site=$S1" > "$TMP/s"; check "data.read may query" "$(st)" 200
curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d "{\"ids\":[\"$D1\"]}" "$BASE/api/v1/datastreams/export" > "$TMP/s"; check "export needs data.export" "$(st)" 403
req GET /api/v1/tokens >/dev/null; TID=$(jx "j.find(t => t.name === 'flt e2e' && !t.revoked_at).id"); req DELETE "/api/v1/tokens/$TID" >/dev/null
req DELETE "/api/v1/dashboards/$FS?reason=e2e" >/dev/null; check "dashboard retired" "$(st)" 200

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]
