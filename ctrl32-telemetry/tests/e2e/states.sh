#!/usr/bin/env bash
# End-to-end checks for dashboard states and widget actions (0.73.0): saving dashboards with states (the first is the
# main view), widgets in states, actions (row, marker, chart and widget clicks, buttons) that open a state, the device
# page or another dashboard, value filters for key filters, the entity selector and the entities table in the showcase,
# and a public link whose detail state shows only entities within the dashboard's scope.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/states.sh
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
anon() { local p=$1; shift; local args=(); for a in "$@"; do args+=(--data-urlencode "$a"); done
  curl -s -G -o "$TMP/b" -w '%{http_code}' "${args[@]}" "$BASE$p" > "$TMP/s"; }
# save NAME SPEC → status of creating the dashboard (slug in $SLUG)
save() { req POST /api/v1/dashboards "{\"name\":\"$1 $SUF\",\"spec\":$2,\"reason\":\"e2e\"}" >/dev/null; SLUG=$(jx 'j.slug'); }
SUF=$RANDOM

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== data: two devices on two assets"
S1=$(req POST /api/v1/sites "{\"name\":\"Sta site $SUF\",\"reason\":\"e2e\"}" | jget id)
A1=$(req POST /api/v1/assets "{\"site_id\":\"$S1\",\"type\":\"pump\",\"name\":\"Sta pump 1 $SUF\",\"reason\":\"e2e\"}" | jget id)
A2=$(req POST /api/v1/assets "{\"site_id\":\"$S1\",\"type\":\"pump\",\"name\":\"Sta pump 2 $SUF\",\"reason\":\"e2e\"}" | jget id)
A3=$(req POST /api/v1/assets "{\"site_id\":\"$S1\",\"type\":\"tank\",\"name\":\"Sta tank $SUF\",\"reason\":\"e2e\"}" | jget id)
mk() { req POST /api/v1/datastreams "{\"asset_id\":\"$1\",\"key\":\"$2\",\"quantity\":\"$3\",\"unit\":\"$4\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id; }
P1=$(mk "$A1" pressure pressure bar); P2=$(mk "$A2" pressure pressure bar); L3=$(mk "$A3" level level %)
V1=$(req POST /api/v1/devices "{\"external_id\":\"sta-1-$SUF\",\"name\":\"Sta controller 1 $SUF\",\"transport\":\"http\",\"reason\":\"e2e\"}" | jget id)
V2=$(req POST /api/v1/devices "{\"external_id\":\"sta-2-$SUF\",\"name\":\"Sta controller 2 $SUF\",\"transport\":\"http\",\"reason\":\"e2e\"}" | jget id)
V3=$(req POST /api/v1/devices "{\"external_id\":\"sta-3-$SUF\",\"name\":\"Sta tank gauge $SUF\",\"transport\":\"http\",\"reason\":\"e2e\"}" | jget id)
for x in "$V1 $P1" "$V2 $P2" "$V3 $L3"; do set -- $x; req POST "/api/v1/devices/$1/datastreams" "{\"datastream_id\":\"$2\",\"reason\":\"e2e\"}" >/dev/null; done
check "devices with datastreams" "$(st)" 201

echo "== states and actions"
SPEC="{\"version\":1,\"filters\":[{\"kind\":\"value\",\"key\":\"limit\",\"label\":\"Limit\",\"default\":\"5\",\"public\":true}],
 \"aliases\":[{\"id\":\"pumps\",\"name\":\"Pumps\",\"kind\":\"type\",\"entity_type\":\"asset\",\"types\":[\"pump\"]},{\"id\":\"dev\",\"name\":\"Selected device\",\"kind\":\"current\",\"entity_type\":\"device\"},{\"id\":\"ds\",\"name\":\"Its datastreams\",\"kind\":\"current\",\"entity_type\":\"datastream\"}],
 \"key_filters\":[{\"id\":\"high\",\"name\":\"Above the limit\",\"rules\":[{\"source\":\"value\",\"key\":\"pressure\",\"op\":\"gt\",\"dynamic\":{\"from\":\"filter\",\"key\":\"limit\"}}]}],
 \"states\":[{\"id\":\"main\",\"name\":\"Pumps\"},{\"id\":\"device\",\"name\":\"Device detail\"}],
 \"widgets\":[
  {\"id\":\"sel\",\"type\":\"entity_selector\",\"geometry\":{\"x\":0,\"y\":0,\"w\":4,\"h\":1},\"alias\":{\"id\":\"pumps\"},\"params\":{\"style\":\"list\"}},
  {\"id\":\"tab\",\"type\":\"entities_table\",\"state\":\"main\",\"geometry\":{\"x\":0,\"y\":1,\"w\":12,\"h\":4},\"alias\":{\"id\":\"pumps\"},\"params\":{\"columns\":[\"name\",\"t:pressure=Pressure\"],\"highlight_filter\":\"high\"},
   \"actions\":[{\"event\":\"row\",\"action\":\"state\",\"target\":\"device\",\"pass\":\"device\"},{\"event\":\"row_button\",\"action\":\"device\",\"label\":\"Device page\"},{\"event\":\"button\",\"action\":\"dashboard\",\"target\":\"overview\",\"label\":\"Overview\"}]},
  {\"id\":\"map\",\"type\":\"map\",\"geometry\":{\"x\":0,\"y\":5,\"w\":6,\"h\":3},\"alias\":{\"id\":\"pumps\"},\"bindings\":{\"devices\":[]},\"actions\":[{\"event\":\"marker\",\"action\":\"state\",\"target\":\"device\"}]},
  {\"id\":\"trend\",\"type\":\"chart\",\"state\":\"device\",\"geometry\":{\"x\":0,\"y\":0,\"w\":8,\"h\":3},\"alias\":{\"id\":\"ds\"},\"bindings\":{\"datastreams\":[]},\"actions\":[{\"event\":\"chart\",\"action\":\"device\"}]},
  {\"id\":\"devs\",\"type\":\"devices\",\"state\":\"device\",\"geometry\":{\"x\":8,\"y\":0,\"w\":4,\"h\":3},\"alias\":{\"id\":\"dev\"},\"bindings\":{\"devices\":[]},\"actions\":[{\"event\":\"row\",\"action\":\"device\"}]},
  {\"id\":\"note\",\"type\":\"text\",\"state\":\"device\",\"geometry\":{\"x\":0,\"y\":3,\"w\":12,\"h\":1},\"params\":{\"text\":\"Detail\"},\"actions\":[{\"event\":\"click\",\"action\":\"state\",\"target\":\"main\"}]}]}"
save "Sta drill-down" "$SPEC"; check "a dashboard with two states" "$(st)" 201
check "widgets of the main state carry no state" "$(jx 'j.spec.widgets.filter(w=>!w.state).map(w=>w.id).join()')" "sel,tab,map"
check "widgets of the detail state" "$(jx 'j.spec.widgets.filter(w=>w.state==="device").map(w=>w.id).join()')" "trend,devs,note"
check "actions kept" "$(jx 'j.spec.widgets[1].actions.map(a=>a.event+":"+a.action+":"+(a.target||"")+":"+(a.pass||"")).join()')" "row:state:device:device,row_button:device::,button:dashboard:overview:"
check "the value filter" "$(jx 'JSON.stringify(j.spec.filters[0])')" '{"default":"5","key":"limit","kind":"value","label":"Limit","public":true}'
for BAD in '"target\":\"device\",\"pass\":\"device\"|"target\":\"device\",\"pass\":\"site\"|pass must be' \
           '"event\":\"marker\"|"event\":\"hover\"|event must be' \
           '"target\":\"overview\"|"target\":\"Over view\"|slug' \
           '"label\":\"Device page\"|"label\":\"\"|needs a label' \
           '"state\":\"device\",\"geometry\":{\"x\":8|"state\":\"nowhere\",\"geometry\":{\"x\":8|no state' \
           '{\"id\":\"main\",\"name\":\"Pumps\"},|{\"id\":\"device\",\"name\":\"Pumps\"},|unique'; do
  IFS='|' read -r from to want <<< "$BAD"; from=${from//\\\"/\"}; to=${to//\\\"/\"}
  req POST /api/v1/dashboards "{\"name\":\"Sta bad $SUF $RANDOM\",\"spec\":${SPEC/"$from"/"$to"},\"reason\":\"e2e\"}" >/dev/null
  [[ "$(st)" == 400 && "$(cat "$TMP/b")" == *"$want"* ]] && ok "refused: $want" || bad "refused: $want (got $(st) $(head -c 160 "$TMP/b"))"
done
req POST /api/v1/dashboards "{\"name\":\"Sta canvas $SUF\",\"spec\":{\"version\":1,\"kind\":\"mimic\",\"states\":[{\"id\":\"a\"}],\"widgets\":[]},\"reason\":\"e2e\"}" >/dev/null; check "a process picture has no states" "$(st)" 400
req PUT "/api/v1/dashboards/$SLUG" "{\"spec\":${SPEC/\"states\":[{\"id\":\"main\",\"name\":\"Pumps\"},/\"states\":[},\"reason\":\"e2e\"}" >/dev/null; check "removing the main state while widgets use it is refused" "$(st)" 400
req GET "/api/v1/dashboards/$SLUG" >/dev/null; check "the stored version is unchanged" "$(jx 'j.version')" 1

echo "== the entity selector, the table and the detail state (server side)"
req GET "/api/v1/entities/query?alias=$(node -pe 'encodeURIComponent(JSON.stringify({kind:"type",entity_type:"asset",types:["pump"]}))')&limit=500" >/dev/null
check "the selector lists the pumps" "$(jx "j.items.filter(x=>x.name.endsWith('$SUF')).length")" 2
req GET "/api/v1/entities/query?alias=$(node -pe 'encodeURIComponent(JSON.stringify({kind:"current",entity_type:"device"}))')&current=asset:$A1" >/dev/null
check "a pump passed with pass=device: its controller" "$(jx 'j.items.map(x=>x.name).join()')" "Sta controller 1 $SUF"
req GET "/api/v1/entities/query?alias=$(node -pe 'encodeURIComponent(JSON.stringify({kind:"current",entity_type:"datastream"}))')&current=device:$V2&want=datastreams" >/dev/null
check "the detail trend of controller 2" "$(jx 'j.items.map(x=>x.id).join()')" "$P2"

echo "== public link of a dashboard with states"
TOKEN=$(req POST "/api/v1/dashboards/$SLUG/share" '{"reason":"e2e"}' | jget token); [[ -n "$TOKEN" ]] && ok "public link" || bad "public link"
curl -s -o "$TMP/b" -w '%{http_code}' "$BASE/p/$TOKEN" > "$TMP/s"; check "the public page" "$(st)" 200
grep -q 'dashfilters.js' "$TMP/b" && grep -q 'pStates' "$TMP/b" && ok "the public page carries the state navigation" || bad "the public page carries the state navigation"
anon "/api/v1/public/$TOKEN" ; check "spec with states for the public page" "$(jx 'j.spec.states.map(s=>s.id).join()')" "main,device"
anon "/api/v1/public/$TOKEN/entities/query" "widget=sel"; check "public selector: the pumps" "$(jx "j.items.filter(x=>x.name.endsWith('$SUF')).length")" 2
anon "/api/v1/public/$TOKEN/entities/query" "widget=devs" "current=asset:$A1"; check "public detail: the pump's controller" "$(jx 'j.items.map(x=>x.id).join()')" "$V1"
anon "/api/v1/public/$TOKEN/entities/query" "widget=devs" "current=asset:$A3"; check "public detail: the tank is outside the scope" "$(jx 'j.total')" 0
anon "/api/v1/public/$TOKEN/entities/query" "widget=trend" "current=device:$V3"; check "public detail: the tank gauge is outside the scope" "$(jx 'j.total')" 0
anon "/api/v1/public/$TOKEN/entities/query" "widget=trend" "current=device:$V2" "latest=1"; check "public detail trend" "$(jx 'j.items.map(x=>x.id).join()')" "$P2"
anon "/api/v1/public/$TOKEN/devices"; check "public devices: a map of an alias does not open all devices" "$(jx "j.some(d=>d.id==='$V3')")" false
anon "/api/v1/public/$TOKEN/entities/query" "widget=tab" "f_value=limit=-1"; check "a public value filter reaches the key filter" "$(jx "j.items.filter(x=>x.name.endsWith('$SUF')).length")" 2
anon "/api/v1/public/$TOKEN/entities/query" "widget=note"; check "a widget without an alias" "$(st)" 404
req DELETE "/api/v1/dashboards/$SLUG/share?reason=e2e" >/dev/null; check "link withdrawn" "$(st)" 200

echo "== showcase: entities table and entity selector"
req GET "/api/v1/dashboards/starter?kind=showcase" >/dev/null; check "showcase starter" "$(st)" 200
check "the showcase has an alias of all devices" "$(jx 'j.spec.aliases[0].kind+":"+j.spec.aliases[0].types.join()')" "type:*"
check "the showcase shows both new widgets" "$(jx '["entities_table","entity_selector"].every(t=>j.spec.widgets.some(w=>w.type===t&&w.alias))')" true
req DELETE "/api/v1/dashboards/$SLUG?reason=e2e" >/dev/null; check "dashboard retired" "$(st)" 200

echo
echo "states: $pass passed, $fail failed"
[[ $fail == 0 ]]
