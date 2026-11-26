#!/usr/bin/env bash
# End-to-end check of the S7 tag import (spec 037, 0.91.0): exports of TIA Portal and STEP 7 are uploaded, the
# proposed tags are checked, and — the proof that the calculated data block offsets are right — the proposed
# addresses are read from the bundled S7 simulator, whose DB1 holds known values.
# Needs Go (builds tools/s7sim) and Node (JSON extraction) — a developer machine, not the demo server.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/s7_import.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
PORT=${SIM_PORT:-10112}
command -v go >/dev/null 2>&1 || export PATH="/d/tools/go/bin:$PATH"
TMP=$(mktemp -d); trap 'rm -rf "$TMP"; [[ -n "${SIM:-}" ]] && kill $SIM 2>/dev/null; true' EXIT
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
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
# imp FILE [form fields…] — upload an export
imp()  { local f=$1; shift; local a=(); for x in "$@"; do a+=(--form-string "$x"); done
  curl -s -b "$JAR" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' -F "file=@$f" "${a[@]}" "$BASE/api/v1/connectors/s7/import" > "$TMP/s"; }
tag()  { jx "(j.tags.find(t => t.name === '$1') || {}).$2"; }

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jget csrf_token < "$TMP/b"); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== a data block source of TIA Portal that mirrors DB1 of the simulator"
cat > "$TMP/sim.db" <<'EOF'
TYPE "udtSpare"
VERSION : 0.1
   STRUCT
      A : Int;
      B : Int;
   END_STRUCT;
END_TYPE

DATA_BLOCK "DB_Sim"
{ S7_Optimized_Access := 'FALSE' }
VERSION : 0.1
NON_RETAIN
   STRUCT
      Sinus : Real;   // a sine wave
      Counter : Int;   // counts up
      Toggle : Bool;   // toggles every second
      Setpoint : Real;   // 21.5 at start
      Spare : Array[0..1] of "udtSpare";
      Text : String[20];   // hello
   END_STRUCT;

BEGIN
   Setpoint := 21.5;

END_DATA_BLOCK
EOF
imp "$TMP/sim.db"; check "source without a block number is read" "$(st)" 200
check "format" "$(jx j.format)" "data block source"
check "addresses carry the placeholder" "$(tag Setpoint address):$(tag Setpoint needs_db)" "DB{db}.DBD8:true"
jx "j.warnings.join(' ')" | grep -q "data block number" && ok "the missing block number is explained" || bad "warnings: $(jx 'j.warnings')"
imp "$TMP/sim.db" "db=1" "prefix=sim_" "existing=sim_counter"; check "source with db=1" "$(st)" 200
check "Sinus" "$(tag Sinus address) $(tag Sinus type) $(tag Sinus key)" "DB1.DBD0 real sim_sinus"
check "Counter (key avoids the existing one)" "$(tag Counter address) $(tag Counter type) $(tag Counter key)" "DB1.DBW4 int sim_counter_2"
check "Toggle" "$(tag Toggle address) $(tag Toggle type)" "DB1.DBX6.0 bool"
check "Setpoint" "$(tag Setpoint address) $(tag Setpoint comment)" "DB1.DBD8 21.5 at start"
check "array of a data type" "$(tag 'Spare[0].A' address) $(tag 'Spare[1].B' address)" "DB1.DBW12 DB1.DBW18"
check "Text" "$(tag Text address) $(tag Text type)" "DB1.DBS20.20 string"
check "block" "$(jx 'j.blocks[0].name + " " + j.blocks[0].number + " " + j.blocks[0].tags + " " + j.blocks[0].size')" "DB_Sim 1 9 42"
check "nothing is writable" "$(jx 'j.tags.filter(t => t.writable).length')" 0
jx "JSON.stringify(j.tags.filter(t => ['Sinus','Counter','Toggle','Setpoint','Text'].includes(t.name)).map(t => ({key: t.key, address: t.address, type: t.type})))" > "$TMP/dbtags"

echo "== a STEP 7 symbol table in Windows-1250"
printf '"Hladina n\xe1dr\x9ee          ","MW     10","INT       ","hladina v mm"\r\n"Start                   ","E       0.0","BOOL      ",""\r\n"Analog                  ","PEW   256","WORD      ",""\r\n"DB_Sim                  ","DB      1","DB      1",""\r\n' > "$TMP/sym.sdf"
imp "$TMP/sym.sdf" "encoding=windows-1250"; check "symbol table" "$(st)" 200
check "diacritics decoded and removed from the key" "$(jx 'j.tags[0].name + "|" + j.tags[0].key + "|" + j.tags[0].address + "|" + j.tags[0].type + "|" + j.tags[0].comment')" "Hladina nádrže|hladina_nadrze|MW10|int|hladina v mm"
check "German mnemonics" "$(tag Start address) $(tag Start area)" "I0.0 i"
check "peripheral input and the block symbol are left out with reasons" "$(jx 'j.skipped.length + " " + j.skipped.every(s => s.reason.length > 10)')" "2 true"
jx "JSON.stringify(j.tags.filter(t => t.address === 'MW10').map(t => ({key: t.key, address: t.address, type: t.type})))" > "$TMP/mtags"

echo "== the proposed addresses read the right values from the simulator"
EXE=bin/s7sim; [[ "$(uname -s)" == MINGW* || "$(uname -s)" == MSYS* ]] && EXE=bin/s7sim.exe
go build -o "$EXE" ./tools/s7sim && ok "simulator built" || { bad "simulator build"; exit 1; }
"$EXE" -port "$PORT" > "$TMP/sim.log" 2>&1 & SIM=$!
for i in $(seq 1 20); do grep -q "listening" "$TMP/sim.log" && break; sleep 0.5; done
grep -q "listening" "$TMP/sim.log" && ok "simulator listening on $PORT" || { bad "simulator: $(cat "$TMP/sim.log")"; exit 1; }
TAGS=$(node -e 'const fs=require("fs"); console.log(JSON.stringify(JSON.parse(fs.readFileSync(process.argv[1],"utf8")).concat(JSON.parse(fs.readFileSync(process.argv[2],"utf8")))))' "$TMP/dbtags" "$TMP/mtags")
req POST /api/v1/connectors/test "{\"kind\":\"s7\",\"settings\":{\"host\":\"127.0.0.1\",\"port\":$PORT,\"plc\":\"s7-1200\",\"tags\":$TAGS}}"; check "test read with the imported tags" "$(st)" 200
check "no tag is unreadable" "$(jx '(j.bad_nodes || []).length')" 0
check "Setpoint (REAL at the calculated offset 8)" "$(jx 'j.values.sim_setpoint')" 21.5
check "Text (STRING[20] at the calculated offset 20)" "$(jx 'j.values.sim_text')" hello
check "MW10 from the symbol table" "$(jx 'j.values.hladina_nadrze')" 1234
check "Sinus is a number in -100..100" "$(jx 'typeof j.values.sim_sinus === "number" && Math.abs(j.values.sim_sinus) <= 100')" true
check "Counter is an integer" "$(jx 'Number.isInteger(j.values.sim_counter_2)')" true
check "Toggle is a bool" "$(jx 'typeof j.values.sim_sinus === "number" && typeof j.values.sim_toggle')" boolean

echo "== other formats"
printf 'Name;Address;Data Type;Comment\nLevel;%%MD100;Real;m\nPump;%%Q0.0;Bool;\nStamp;%%MB20;DTL;\n' > "$TMP/tags.csv"
imp "$TMP/tags.csv"; check "CSV with a header" "$(jx 'j.tags.map(t => t.address).join(",") + " " + j.skipped.length')" "MD100,Q0.0 1"
cat > "$TMP/tags.xml" <<'EOF'
<?xml version="1.0" encoding="utf-8"?>
<Document><SW.Tags.PlcTagTable ID="0"><ObjectList>
<SW.Tags.PlcTag ID="1" CompositionName="Tags"><AttributeList><DataTypeName>Bool</DataTypeName><LogicalAddress>%Q0.3</LogicalAddress><Name>Valve_Open</Name></AttributeList></SW.Tags.PlcTag>
<SW.Tags.PlcTag ID="2" CompositionName="Tags"><AttributeList><DataTypeName>Int</DataTypeName><LogicalAddress>%MW12</LogicalAddress><Name>Speed</Name></AttributeList></SW.Tags.PlcTag>
</ObjectList></SW.Tags.PlcTagTable></Document>
EOF
imp "$TMP/tags.xml"; check "Openness XML tag table" "$(jx 'j.format + ": " + j.tags.map(t => t.key + "=" + t.address).join(",")')" "TIA Portal PLC tag table (XML): valve_open=Q0.3,speed=MW12"
sed "s/'FALSE'/'TRUE'/" "$TMP/sim.db" > "$TMP/opt.db"
imp "$TMP/opt.db" "db=1"; check "optimized block: no tags, the reason says what to change" "$(st):$(jx 'j.tags.length'):$(jx '/Optimized block access/.test(j.blocks[0].note)')" "200:0:true"

echo "== refusals"
printf 'hello world\n' > "$TMP/x.txt"; imp "$TMP/x.txt"; check "a file without variables" "$(st)" 400
imp "$TMP/sim.db" "db=70000"; check "block number out of range" "$(st)" 400
imp "$TMP/sim.db" "prefix=Bad Prefix"; check "invalid prefix" "$(st)" 400
curl -s -b "$JAR" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' -X POST "$BASE/api/v1/connectors/s7/import" > "$TMP/s"; check "no file" "$(st)" 400
req GET /api/v1/connectors; check "the import stored nothing" "$(jx 'j.filter(c => c.kind === "s7" && /sim_/.test(JSON.stringify(c.settings))).length')" 0

echo; echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]
