#!/usr/bin/env bash
# End-to-end checks for screens/dashboards (URS J-02), aggregated observations, latest values and device locations.
# Feeds telemetry through MQTT with tools/devfill (needs Go); JSON extraction with Node — a developer machine.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/dashboards.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
command -v go >/dev/null 2>&1 || export PATH="/d/tools/go/bin:$PATH"
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$1" "$2" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  STATUS=$(cat "$TMP/s"); cat "$TMP/b"; }
SUF=$RANDOM

echo "== pages and login"
for p in /dashboards /dashboards/x /dashboards/x/edit; do req GET $p >/dev/null; check "page $p served" "$(st)" 200; done
req GET /api/v1/dashboards >/dev/null; [[ "$(st)" == "401" || "$(st)" == "403" ]] && ok "dashboards need auth ($(st))" || bad "dashboards need auth (got $(st))"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== data: site, asset, datastream, device, telemetry over MQTT"
SITE=$(req POST /api/v1/sites "{\"name\":\"Dash site $SUF\",\"reason\":\"e2e\"}" | jget id); [[ -n "$SITE" ]] && ok "site" || bad "site: $(head -c 120 "$TMP/b")"
ASSET=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"machine\",\"name\":\"Dash asset $SUF\",\"reason\":\"e2e\"}" | jget id); [[ -n "$ASSET" ]] && ok "asset" || bad "asset: $(head -c 120 "$TMP/b")"
DS=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t_c\",\"quantity\":\"temperature\",\"unit\":\"\u00b0C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id); [[ -n "$DS" ]] && ok "datastream" || bad "datastream: $(head -c 160 "$TMP/b")"
DEV=$(req POST /api/v1/devices "{\"external_id\":\"dash-$SUF\",\"name\":\"dash test\",\"reason\":\"e2e\"}" | jget id); [[ -n "$DEV" ]] && ok "device" || bad "device"
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' | jget token); [[ -n "$TOK" ]] && ok "token" || bad "token"
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e\"}" >/dev/null; check "assign t_c" "$(st)" 201
go run ./tools/devfill -addr "$MQTT" -user "dash-$SUF" -token "$TOK" -key t_c -start 20 -step 0.5 -n 12 -every 300 -lat 48.7164 -lon 21.2611 > "$TMP/pub.log" 2>&1 && ok "published 12 samples with a position" || bad "publish: $(tail -3 "$TMP/pub.log")"
sleep 2

echo "== aggregated observations and latest values"
FROM=$(date -u -d '-2 hours' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-2H +%Y-%m-%dT%H:%M:%SZ)
req GET "/api/v1/observations?datastream_id=$DS&from=$FROM&bucket=600" >/dev/null; check "bucketed observations" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.bucket_s')" == "600" ]] && ok "bucket echoed" || bad "bucket"
[[ "$(jx "$TMP/b" 'j.points.length > 0 && typeof j.points[0].avg')" == "number" ]] && ok "avg/min/max points" || bad "points: $(head -c 200 "$TMP/b")"
req GET "/api/v1/observations?datastream_id=$DS&from=$FROM&bucket=0" >/dev/null; check "bucket validated" "$(st)" 400
req GET "/api/v1/datastreams/latest?ids=$DS" >/dev/null; check "latest values" "$(st)" 200
[[ "$(jx "$TMP/b" "typeof j['$DS'].value")" == "number" ]] && ok "latest value present" || bad "latest: $(head -c 200 "$TMP/b")"
req GET "/api/v1/datastreams/latest?ids=nonsense" >/dev/null; check "latest ids validated" "$(st)" 400

echo "== device location"
req GET /api/v1/devices >/dev/null; [[ "$(jx "$TMP/b" "(j.find(d=>d.id==='$DEV')||{}).location.source")" == "reported" ]] && ok "reported position stored from telemetry" || bad "reported location: $(jx "$TMP/b" "(j.find(d=>d.id==='$DEV')||{}).location")"
req PATCH "/api/v1/devices/$DEV" '{"location":{"lat":48.1486,"lon":17.1077},"reason":"fixed position"}' >/dev/null; check "set fixed position" "$(st)" 200
req PATCH "/api/v1/devices/$DEV" '{"location":{"lat":99,"lon":0},"reason":"x"}' >/dev/null; check "position validated" "$(st)" 400
go run ./tools/devfill -addr "$MQTT" -user "dash-$SUF" -token "$TOK" -key t_c -start 25 -step 0 -n 1 -every 1 -lat 50.0 -lon 14.0 > "$TMP/pub2.log" 2>&1; sleep 1
req GET /api/v1/devices >/dev/null; [[ "$(jx "$TMP/b" "(j.find(d=>d.id==='$DEV')||{}).location.lat")" == "48.1486" ]] && ok "fixed position wins over reported" || bad "fixed vs reported: $(jx "$TMP/b" "(j.find(d=>d.id==='$DEV')||{}).location")"
req PATCH "/api/v1/devices/$DEV" '{"clear_location":true,"reason":"back to reported"}' >/dev/null; check "clear fixed position" "$(st)" 200

echo "== dashboards"
req GET /api/v1/dashboards/catalog >/dev/null; check "catalog" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.widgets.slice(0,20).map(w=>w.type).join(",")')" == "chart,bars,donut,heatmap,value,gauge,status,table,alarms,devices,map,text,tank,pump,valve,pipe,instrument,setpoint,switch,button" ]] && ok "20 original widget types first" || bad "catalog types: $(jx "$TMP/b" 'j.widgets.map(w=>w.type)')"
[[ "$(jx "$TMP/b" 'j.widgets.length >= 44')" == "true" ]] && ok "at least 44 widget types (spec 002 SC-001)" || bad "types: $(jx "$TMP/b" 'j.widgets.length')"
[[ "$(jx "$TMP/b" 'j.widgets.every(w=>w.params.length>=15)')" == "true" ]] && ok "every type has at least 15 settings (SC-002)" || bad "settings: $(jx "$TMP/b" 'j.widgets.filter(w=>w.params.length<15).map(w=>w.type)')"
[[ "$(jx "$TMP/b" '["data","appearance","series","time","actions"].every(g=>j.widgets.find(w=>w.type==="chart").params.some(p=>(p.group||"data")===g))')" == "true" ]] && ok "chart settings grouped in five tabs" || bad "groups"
SPEC="{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"range\":\"6h\",\"refresh_s\":30,\"widgets\":[{\"id\":\"a\",\"type\":\"chart\",\"geometry\":{\"x\":0,\"y\":0,\"w\":8,\"h\":3},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{\"title\":\"Temp\",\"chart\":\"area\",\"warn\":24,\"decimals\":1}},{\"id\":\"b\",\"type\":\"value\",\"geometry\":{\"x\":8,\"y\":0,\"w\":4,\"h\":2},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{\"warn\":22}},{\"id\":\"c\",\"type\":\"gauge\",\"geometry\":{\"x\":8,\"y\":2,\"w\":4,\"h\":2},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{\"y_min\":0,\"y_max\":40}},{\"id\":\"d\",\"type\":\"status\",\"geometry\":{\"x\":0,\"y\":3,\"w\":4,\"h\":2},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{\"states\":[\"25.5=Warm:#d97706\"]}},{\"id\":\"e\",\"type\":\"map\",\"geometry\":{\"x\":4,\"y\":3,\"w\":8,\"h\":4},\"bindings\":{\"devices\":[\"$DEV\"]},\"params\":{}},{\"id\":\"f\",\"type\":\"text\",\"geometry\":{\"x\":0,\"y\":5,\"w\":4,\"h\":2},\"bindings\":{},\"params\":{\"text\":\"Hello {{org}}\",\"style\":\"note\"}}]}"
req POST /api/v1/dashboards "{\"name\":\"Hala $SUF\",\"spec\":$SPEC,\"is_default\":true,\"reason\":\"e2e\"}" >/dev/null; check "create dashboard" "$(st)" 201
SLUG=$(jget slug < "$TMP/b"); [[ "$SLUG" == "hala-$SUF" ]] && ok "slug from name" || bad "slug: $SLUG"
[[ "$(jx "$TMP/b" 'j.spec.widgets.length')" == "6" ]] && ok "widgets stored" || bad "widgets"
[[ "$(jx "$TMP/b" 'j.spec.widgets[0].params.chart')" == "area" ]] && ok "chart style stored" || bad "style"
req GET /api/v1/dashboards/default >/dev/null; check "default dashboard" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.slug')" == "$SLUG" ]] && ok "default resolves" || bad "default slug"
req POST /api/v1/dashboards "{\"name\":\"Bad $SUF\",\"spec\":{\"widgets\":[{\"type\":\"chart\",\"geometry\":{\"x\":10,\"y\":0,\"w\":6,\"h\":2},\"bindings\":{\"datastreams\":[\"$DS\"]}}]},\"reason\":\"e2e\"}" >/dev/null; check "geometry validated" "$(st)" 400
req POST /api/v1/dashboards "{\"name\":\"Bad $SUF\",\"spec\":{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"range\":\"6h\",\"refresh_s\":30,\"widgets\":[{\"id\":\"a\",\"type\":\"chart\",\"geometry\":{\"x\":0,\"y\":0,\"w\":6,\"h\":3},\"bindings\":{\"datastreams\":[\"nonsense\"]},\"params\":{}}]},\"reason\":\"e2e\"}" >/dev/null; check "binding ids validated" "$(st)" 400
req POST /api/v1/dashboards "{\"name\":\"Empty $SUF\",\"spec\":{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"range\":\"6h\",\"refresh_s\":30,\"widgets\":[{\"id\":\"a\",\"type\":\"chart\",\"geometry\":{\"x\":0,\"y\":0,\"w\":6,\"h\":3},\"bindings\":{\"datastreams\":[]},\"params\":{}}]},\"reason\":\"e2e\"}" >/dev/null; check "widget without a datastream is saved (chosen later)" "$(st)" 201
req DELETE "/api/v1/dashboards/empty-$SUF?reason=e2e" >/dev/null; check "empty dashboard retired" "$(st)" 200
EMPTY="{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"range\":\"24h\",\"refresh_s\":30,\"widgets\":[]}"
req POST /api/v1/dashboards "{\"name\":\"Blank $SUF\",\"spec\":$EMPTY,\"reason\":\"e2e\"}" >/dev/null; check "\"Dashboard — empty\" can be created" "$(st)" 201
req PUT "/api/v1/dashboards/blank-$SUF" "{\"spec\":{\"version\":1,\"widgets\":[{\"id\":\"a\",\"type\":\"text\",\"geometry\":{\"x\":0,\"y\":0,\"w\":4,\"h\":2},\"bindings\":{},\"params\":{}}]},\"reason\":\"e2e\"}" >/dev/null; check "a widget added" "$(st)" 200
req PUT "/api/v1/dashboards/blank-$SUF" "{\"spec\":$EMPTY,\"reason\":\"e2e: every widget removed\"}" >/dev/null; check "saved after removing every widget" "$(st)" 200
req DELETE "/api/v1/dashboards/blank-$SUF?reason=e2e" >/dev/null; check "blank dashboard retired" "$(st)" 200
req POST /api/v1/dashboards "{\"name\":\"Bad $SUF\",\"spec\":{\"widgets\":[{\"type\":\"blink\"}]},\"reason\":\"e2e\"}" >/dev/null; check "unknown widget rejected" "$(st)" 400
req POST /api/v1/dashboards "{\"name\":\"Hala $SUF\",\"spec\":$SPEC,\"reason\":\"e2e\"}" >/dev/null; check "duplicate slug rejected" "$(st)" 409
SPEC2=$(echo "$SPEC" | sed 's/"range":"6h"/"range":"24h"/')
req PUT "/api/v1/dashboards/$SLUG" "{\"spec\":$SPEC2,\"reason\":\"range change\"}" >/dev/null; check "new version" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.version')" == "2" ]] && ok "version 2" || bad "version"
req GET /api/v1/dashboards >/dev/null; [[ "$(jx "$TMP/b" "(j.find(d=>d.slug==='$SLUG')||{}).version")" == "2" ]] && ok "list shows current version" || bad "list"
echo "== widget settings (spec 002)"
req GET "/api/v1/dashboards/$SLUG" >/dev/null
[[ "$(jx "$TMP/b" '"show_title" in j.spec.widgets[1].params || "fullscreen" in j.spec.widgets[1].params')" == "false" ]] && ok "older widgets keep their stored form" || bad "defaults added: $(jx "$TMP/b" 'j.spec.widgets[1].params')"
RICH="{\"version\":1,\"widgets\":[{\"id\":\"t\",\"type\":\"chart\",\"geometry\":{\"x\":0,\"y\":0,\"w\":8,\"h\":3},\"bindings\":{\"datastreams\":[\"$DS\",\"$DS\"]},\"params\":{\"title\":\"Rich\",\"series\":[{\"label\":\"Air\",\"color\":\"#FF0000\",\"unit\":\"C\",\"decimals\":1,\"style\":\"step\",\"points\":true},{\"axis\":\"right\",\"hidden\":true}],\"time_mode\":\"fixed\",\"from\":\"2026-09-01T10:00:00+02:00\",\"to\":\"2026-09-02T10:00:00+02:00\",\"agg\":\"max\",\"agg_interval\":\"1h\",\"legend_pos\":\"right\",\"legend_values\":[\"min\",\"max\",\"avg\"],\"filters\":[\"range\",\"series\"],\"bg_color\":\"#0F172A\",\"title_icon\":\"gauge\",\"click\":\"trend\",\"legacy_setting\":1}},{\"id\":\"v\",\"type\":\"value_card\",\"geometry\":{\"x\":8,\"y\":0,\"w\":4,\"h\":2},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{\"ranges\":[{\"op\":\"between\",\"v\":2,\"v2\":8,\"target\":\"background\",\"color\":\"#16a34a\"}]}}]}"
req POST /api/v1/dashboards "{\"name\":\"Rich $SUF\",\"spec\":$RICH,\"reason\":\"e2e\"}" >/dev/null; check "rich settings stored" "$(st)" 201
[[ "$(jx "$TMP/b" 'JSON.stringify(j.spec.widgets[0].params.series)')" == '[{"color":"#ff0000","decimals":1,"label":"Air","points":true,"style":"step","unit":"C"},{"axis":"right","hidden":true}]' ]] && ok "series read back canonical" || bad "series: $(jx "$TMP/b" 'JSON.stringify(j.spec.widgets[0].params.series)')"
[[ "$(jx "$TMP/b" 'j.spec.widgets[0].params.from')" == "2026-09-01T08:00:00Z" ]] && ok "fixed window in UTC" || bad "from: $(jx "$TMP/b" 'j.spec.widgets[0].params.from')"
[[ "$(jx "$TMP/b" 'j.spec.widgets[0].params.bg_color+" "+j.spec.widgets[0].params.agg+" "+j.spec.widgets[0].params.filters.join("+")')" == "#0f172a max range+series" ]] && ok "appearance, aggregation, viewer filters" || bad "params: $(jx "$TMP/b" 'JSON.stringify(j.spec.widgets[0].params)')"
[[ "$(jx "$TMP/b" '("legacy_setting" in j.spec.widgets[0].params) + " " + (j.warnings||[]).join("|")')" == "false widget 1: unknown setting legacy_setting dropped" ]] && ok "unknown setting dropped with a warning" || bad "warnings: $(jx "$TMP/b" 'JSON.stringify(j.warnings)')"
[[ "$(jx "$TMP/b" 'j.spec.widgets[1].params.ranges[0].v2')" == "8" ]] && ok "conditional ranges stored" || bad "ranges"
req DELETE "/api/v1/dashboards/rich-$SUF?reason=e2e" >/dev/null; check "rich dashboard retired" "$(st)" 200
for BADP in '"bg_color":"red"' '"bg_color":"expression(x)"' '"series":[{"axis":"middle"}]' '"series":[{"color":"url(x)"}]' '"agg":"median"' '"legend_values":["p99"]' '"title_icon":"<svg>"' '"title_size":200' '"from":"yesterday"' '"filters":["script"]'; do
  req POST /api/v1/dashboards "{\"name\":\"Bad $SUF\",\"spec\":{\"version\":1,\"widgets\":[{\"id\":\"t\",\"type\":\"chart\",\"geometry\":{\"x\":0,\"y\":0,\"w\":8,\"h\":3},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{$BADP}}]},\"reason\":\"e2e\"}" >/dev/null; check "refused: $BADP" "$(st)" 400
done
req POST /api/v1/dashboards "{\"name\":\"Bad $SUF\",\"spec\":{\"version\":1,\"widgets\":[{\"id\":\"v\",\"type\":\"value_card\",\"geometry\":{\"x\":0,\"y\":0,\"w\":4,\"h\":2},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{\"ranges\":[{\"op\":\"~\",\"v\":1,\"color\":\"#fff\"}]}}]},\"reason\":\"e2e\"}" >/dev/null; check "refused: range operator" "$(st)" 400
echo "== screen format 1.1: filters and query sources (0.68.0)"
req GET "/api/v1/dashboards/$SLUG" >/dev/null
check "an older dashboard keeps its form (no filters, source or follow)" "$(jx "$TMP/b" '["filters" in j.spec, j.spec.widgets.some(w=>"source" in w || "follow" in w)].join()')" "false,false"
SPEC3=$(echo "$SPEC2" | sed "s/\"range\":\"24h\"/\"range\":\"24h\",\"filters\":[{\"kind\":\"site\",\"public\":true},{\"kind\":\"search\"}]/; s/\"id\":\"a\",\"type\":\"chart\"/\"id\":\"a\",\"follow\":true,\"source\":{\"quantity\":[\"temperature\"],\"limit\":6},\"type\":\"chart\"/")
req PUT "/api/v1/dashboards/$SLUG" "{\"spec\":$SPEC3,\"reason\":\"filters added\"}" >/dev/null; check "filters and a query source added" "$(st)" 200
check "version 3 with the filter bar" "$(jx "$TMP/b" '[j.version, j.spec.filters.map(f=>f.kind).join("+"), j.spec.widgets[0].source.limit, j.spec.widgets[0].follow, "follow" in j.spec.widgets[1]].join()')" "3,site+search,6,true,false"
req PUT "/api/v1/dashboards/$SLUG" "{\"spec\":$SPEC2,\"reason\":\"filters removed again\"}" >/dev/null; check "filters removed again" "$(jx "$TMP/b" '[j.version, "filters" in j.spec, "source" in j.spec.widgets[0]].join()')" "4,false,false"
echo "== public link"
anon(){ curl -s -o "$TMP/b" -w '%{http_code}' "$BASE$1" > "$TMP/s"; cat "$TMP/b"; }
TOKEN=$(req POST "/api/v1/dashboards/$SLUG/share" '{"reason":"publish"}' | jget token); check "public link created" "$(st)" 201; [[ ${#TOKEN} -ge 32 ]] && ok "token" || bad "token: $TOKEN"
req GET "/api/v1/dashboards/$SLUG/share" | grep -q '"enabled":true' && ok "share status" || bad "share status"
anon "/p/$TOKEN" >/dev/null; check "public page served" "$(st)" 200
anon "/api/v1/public/$TOKEN" | grep -q "\"slug\":\"$SLUG\"" && ok "public dashboard (anonymous)" || bad "public dashboard: $(head -c 120 "$TMP/b")"
anon "/api/v1/public/$TOKEN/datastreams" | grep -q "\"id\":\"$DS\"" && ok "public datastreams limited to bound" || bad "public datastreams: $(head -c 120 "$TMP/b")"
anon "/api/v1/public/$TOKEN/datastreams/latest?ids=$DS" >/dev/null; check "public latest" "$(st)" 200
anon "/api/v1/public/$TOKEN/observations?datastream_id=$DS&from=$FROM&bucket=600" >/dev/null; check "public observations" "$(st)" 200
anon "/api/v1/public/$TOKEN/observations?datastream_id=00000000-0000-0000-0000-000000000000&from=$FROM" >/dev/null; check "unbound datastream refused" "$(st)" 403
anon "/api/v1/public/$TOKEN/devices" >/dev/null; check "public devices" "$(st)" 200
anon "/api/v1/public/$TOKEN/alarms" >/dev/null; check "public alarms" "$(st)" 200
anon "/api/v1/public/${TOKEN}x/datastreams" >/dev/null; check "bad token 404" "$(st)" 404
req DELETE "/api/v1/dashboards/$SLUG/share?reason=unpublish" >/dev/null; check "public link disabled" "$(st)" 200
anon "/api/v1/public/$TOKEN" >/dev/null; check "revoked link 404" "$(st)" 404
req DELETE "/api/v1/dashboards/$SLUG?reason=e2e" >/dev/null; check "retire" "$(st)" 200
req GET "/api/v1/dashboards/$SLUG" >/dev/null; check "retired is gone" "$(st)" 404
req GET "/api/v1/export/audit" >/dev/null; grep -q "dashboard.create" "$TMP/b" && grep -q "dashboard.retire" "$TMP/b" && ok "audited" || bad "audit"

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]
