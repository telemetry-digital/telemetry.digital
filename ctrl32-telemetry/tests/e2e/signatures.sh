#!/usr/bin/env bash
# End-to-end checks of electronic signatures of stored reports (report.sign, URS H-13) and of signed reviews of the
# audit trail (audit.review, URS I-05); 0.67.0:
#   - a quality reviewer (built-in role qa, two-factor sign-in) signs a stored PDF report: the password and the
#     authenticator code are asked again, the meaning and the manifestation (name, user name, roles, time with the
#     time zone) are stored with the hash of the PDF, the server signature verifies; the same meaning twice and
#     signing without report.sign are refused; an API token cannot sign;
#   - a changed PDF invalidates the signature (needs REPORTS_DIR, the server's report directory);
#   - the audit trail list (audit.read) and reviews of a period or one entry: outcome, comment for findings,
#     the covered entries and the hash of the last one, signed with the meaning "reviewed"; audited.
# Needs curl, Node, Go (tools/totpcode) and Typst on the server (PDF reports; without it the report part is skipped).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 [REPORTS_DIR=…] bash tests/e2e/signatures.sh
set -u
cd "$(dirname "$0")/../.."
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
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
go build -o "$TMP/totpcode" ./tools/totpcode || { echo "totpcode build failed"; exit 1; }
code() { "$TMP/totpcode" -secret "$SECRET"; }
nextstep() { sleep $((30 - $(date +%s) % 30 + 1)); } # a TOTP code is accepted once: wait for the next one
signin() { # jar user password new-password
  JAR="$1"; CSRF=""
  req POST /api/v1/auth/login "{\"username\":\"$2\",\"password\":\"$3\"}"
  req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
  req POST /api/v1/auth/password "{\"current\":\"$3\",\"new\":\"$4\"}"
  req POST /api/v1/auth/login "{\"username\":\"$2\",\"password\":\"$4\"}"
  req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
}
admin() { JAR="$TMP/jar"; CSRF=$ADMIN_CSRF; }
SUF=$RANDOM
iso() { date -u -d "@$1" +%Y-%m-%dT%H:%M:%SZ; }
NOW=$(date -u +%s)

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token); ADMIN_CSRF=$CSRF
req POST /api/v1/users "{\"username\":\"qa$SUF\",\"full_name\":\"Quality Reviewer $SUF\",\"roles\":[\"qa\"],\"reason\":\"e2e\"}"; check "quality reviewer created" "$(st)" 201
QPW=$(jx j.initial_password); QNEW="Qa-e2e-$SUF-pass!"
req POST /api/v1/roles "{\"name\":\"signer$SUF\",\"permissions\":[\"data.read\",\"data.export\",\"report.sign\",\"token.manage\"],\"reason\":\"e2e\"}"; check "role with report.sign and token.manage" "$(st)" 201
req POST /api/v1/users "{\"username\":\"sg$SUF\",\"full_name\":\"Token Signer\",\"roles\":[\"signer$SUF\"],\"reason\":\"e2e\"}"; SPW=$(jx j.initial_password)
# a stored PDF report of some observations
req POST /api/v1/sites "{\"name\":\"Sign site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"room\",\"name\":\"Sign room $SUF\",\"reason\":\"e2e\"}"; ASSET=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"C\",\"reason\":\"e2e\"}"; DS=$(jx j.id)
req POST /api/v1/devices "{\"external_id\":\"sg-$SUF\",\"name\":\"sign test\",\"transport\":\"http\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; TOK=$(jx j.token)
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e\"}"
curl -s -X POST -H "Authorization: Bearer $TOK" -H "Content-Type: application/json" --data "[{\"ts\":$((NOW - 600)),\"v\":{\"temp\":4.1}},{\"ts\":$((NOW - 300)),\"v\":{\"temp\":4.4}}]" "$BASE/api/v1/d/telemetry" > /dev/null
curl -s -b "$JAR" -D "$TMP/h" -o "$TMP/r.pdf" "$BASE/api/v1/export/observations?datastream_id=$DS&from=$(iso $((NOW - 3600)))&to=$(iso $((NOW + 60)))&format=pdf"
RID=$(grep -i '^x-report-id:' "$TMP/h" | tr -d '\r' | awk '{print $2}')
HAVE_PDF=0; [[ -n "$RID" ]] && HAVE_PDF=1
[[ $HAVE_PDF == 1 ]] && ok "stored PDF report $RID" || echo "  (no PDF report — Typst missing on the server? the report part is skipped)"

echo "== the quality reviewer signs in with two-factor sign-in"
signin "$TMP/jarq" "qa$SUF" "$QPW" "$QNEW"
req POST /api/v1/auth/totp/setup '{}'; SECRET=$(jx j.secret)
req POST /api/v1/auth/totp/confirm "{\"code\":\"$(code)\"}"; check "authenticator enrolled" "$(st)" 200
req POST /api/v1/auth/login "{\"username\":\"qa$SUF\",\"password\":\"$QNEW\"}"; PRE=$(jx j.pre_auth_token)
nextstep
req POST /api/v1/auth/mfa "{\"pre_auth_token\":\"$PRE\",\"code\":\"$(code)\"}"; check "signed in with a code" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token); QA_CSRF=$CSRF
qa() { JAR="$TMP/jarq"; CSRF=$QA_CSRF; }

if [[ $HAVE_PDF == 1 ]]; then
  echo "== signing a stored report"
  req GET /api/v1/reports?limit=20; check "reports list carries signatures" "$(jx 'JSON.stringify(j.find(r=>r.id==="'"$RID"'").signatures)')" "[]"
  req POST "/api/v1/reports/$RID/signatures" '{"meaning":"approved"}'; check "without the password: asked for it" "$(st) $(jx j.code)" "403 step_up_required"
  req POST "/api/v1/reports/$RID/signatures" "{\"meaning\":\"approved\",\"password\":\"wrong-$SUF\",\"code\":\"000000\"}"; check "wrong password refused" "$(st) $(jx j.code)" "403 step_up_failed"
  nextstep
  req POST "/api/v1/reports/$RID/signatures" "{\"meaning\":\"approved\",\"password\":\"$QNEW\",\"code\":\"000000\"}"; check "wrong authenticator code refused" "$(st) $(jx j.code)" "403 step_up_failed"
  req POST "/api/v1/reports/$RID/signatures" "{\"meaning\":\"blessed\",\"password\":\"$QNEW\",\"code\":\"$(code)\"}"; check "unknown meaning refused" "$(st)" 400
  req POST "/api/v1/reports/$RID/signatures" "{\"meaning\":\"approved\",\"comment\":\"Monthly check\",\"password\":\"$QNEW\",\"code\":\"$(code)\"}"; check "signed" "$(st)" 201
  check "manifestation" "$(jx '[j.signer, j.signer_name, j.roles.join("+"), j.meaning, j.method, j.valid].join("|")')" "qa$SUF|Quality Reviewer $SUF|qa|approved|password_totp|true"
  check "time with the time zone" "$(jx 'j.signed_at_local.includes(" (") && j.signed_at_local.endsWith(")") && j.tz.length > 0')" "true"
  FH=$(curl -s -b "$JAR" "$BASE/api/v1/reports/$RID" | node -e 'let s="";process.stdin.on("data",c=>s+=c).on("end",()=>console.log(JSON.parse(s).file_hash))')
  check "the signed hash is the PDF's hash" "$(jx j.record_hash)" "$FH"
  nextstep
  req POST "/api/v1/reports/$RID/signatures" "{\"meaning\":\"approved\",\"password\":\"$QNEW\",\"code\":\"$(code)\"}"; check "the same meaning twice refused" "$(st)" 409
  nextstep
  req POST "/api/v1/reports/$RID/signatures" "{\"meaning\":\"reviewed\",\"password\":\"$QNEW\",\"code\":\"$(code)\"}"; check "a second meaning" "$(st)" 201
  req GET "/api/v1/reports/$RID/signatures"; check "both valid" "$(st) $(jx 'j.file_ok+" "+j.signatures.map(s=>s.meaning+":"+s.valid).join(",")')" "200 true approved:true,reviewed:true"
  req GET /api/v1/reports?limit=20; check "list shows them" "$(jx 'j.find(r=>r.id==="'"$RID"'").signatures.map(s=>s.meaning+":"+s.hash_matches).join(",")')" "approved:true,reviewed:true"
  admin
  req POST "/api/v1/reports/$RID/signatures" "{\"meaning\":\"approved\",\"password\":\"$PASS\"}"; check "without report.sign refused" "$(st)" 403
  req GET "/api/v1/export/audit?from=$(iso $((NOW - 300)))&format=jsonl"; check "signatures audited" "$(grep 'esignature.create' "$TMP/b" | grep -c "$RID")" "2"
  # an API token cannot sign, even with report.sign
  signin "$TMP/jars" "sg$SUF" "$SPW" "Signer-e2e-$SUF-pass!"
  req POST /api/v1/tokens '{"name":"sign","scopes":["report.sign","data.export"]}'; TK=$(jx j.token)
  printf '%s' "{\"meaning\":\"verified\",\"password\":\"Signer-e2e-$SUF-pass!\"}" > "$TMP/req"
  curl -s -o "$TMP/b" -w '%{http_code}' -X POST -H "Authorization: Bearer $TK" -H "Content-Type: application/json" --data-binary @"$TMP/req" "$BASE/api/v1/reports/$RID/signatures" > "$TMP/s"
  check "API token refused" "$(st) $(jx j.code)" "403 session_required"
  req POST "/api/v1/reports/$RID/signatures" "{\"meaning\":\"verified\",\"password\":\"Signer-e2e-$SUF-pass!\"}"; check "same user in the browser: password only (no two-factor)" "$(st) $(jx j.method)" "201 password"
  # a changed file invalidates the signatures
  if [[ -n "${REPORTS_DIR:-}" ]] && FILE=$(find "$REPORTS_DIR" -name "$RID.pdf" 2>/dev/null | head -1) && [[ -n "$FILE" ]]; then
    cp "$FILE" "$TMP/orig.pdf"; printf ' ' >> "$FILE"
    qa; req GET "/api/v1/reports/$RID/signatures"; check "changed PDF: signatures no longer valid" "$(jx 'j.file_ok+" "+j.signatures.every(s=>!s.valid)+" "+j.signatures[0].problem')" "false true the record changed after it was signed"
    nextstep; req POST "/api/v1/reports/$RID/signatures" "{\"meaning\":\"responsible\",\"password\":\"$QNEW\",\"code\":\"$(code)\"}"; check "a changed PDF cannot be signed" "$(st)" 409
    cp "$TMP/orig.pdf" "$FILE"
    req GET "/api/v1/reports/$RID/signatures"; check "restored: valid again" "$(jx 'j.signatures.filter(s=>s.valid).length')" "3"
  else echo "  (REPORTS_DIR not set or the file not found: the tamper check is skipped)"; fi
fi

echo "== the audit trail and its reviews"
qa
req GET "/api/v1/audit?from=$(iso $((NOW - 3600)))&limit=5"; check "audit trail (audit.read)" "$(st) $(jx 'j.entries.length')" "200 5"
check "newest first" "$(jx 'j.entries[0].seq > j.entries[4].seq')" "true"
BEFORE=$(jx j.next_before)
req GET "/api/v1/audit?from=$(iso $((NOW - 3600)))&limit=5&before=$BEFORE"; check "next page" "$(jx 'j.entries[0].seq < '"$BEFORE")" "true"
req GET "/api/v1/audit?from=$(iso $((NOW - 3600)))&action=datastream.create"; check "filter by action" "$(jx 'j.entries.length>0 && j.entries.every(e=>e.action==="datastream.create")')" "true"
FROM=$(iso $((NOW - 3600))); TO=$(date -u +%Y-%m-%dT%H:%M:%SZ)
req POST /api/v1/audit/reviews "{\"from\":\"$FROM\",\"to\":\"$TO\",\"outcome\":\"no_findings\"}"; check "without the password: asked for it" "$(st) $(jx j.code)" "403 step_up_required"
req POST /api/v1/audit/reviews "{\"from\":\"$FROM\",\"to\":\"$TO\",\"outcome\":\"findings\",\"password\":\"$QNEW\"}"; check "findings without a comment refused" "$(st)" 400
req POST /api/v1/audit/reviews "{\"from\":\"$FROM\",\"to\":\"$(iso $((NOW + 86400)))\",\"outcome\":\"no_findings\",\"password\":\"$QNEW\"}"; check "a period that has not ended refused" "$(st)" 400
nextstep
req POST /api/v1/audit/reviews "{\"from\":\"$FROM\",\"to\":\"$TO\",\"outcome\":\"no_findings\",\"comment\":\"Weekly review\",\"password\":\"$QNEW\",\"code\":\"$(code)\"}"; check "period reviewed" "$(st)" 201
check "covered entries" "$(jx '(j.entries > 0) + " " + (j.to_seq >= j.from_seq) + " " + (j.last_hash.length === 64)')" "true true true"
check "signed: reviewed" "$(jx 'j.signature.meaning+" "+j.signature.method+" "+j.signature.signer')" "reviewed password_totp qa$SUF"
TOSEQ=$(jx j.to_seq)
req GET "/api/v1/audit?from=$FROM&limit=500"; check "entries show the review" "$(jx 'j.entries.find(e=>e.seq==='"$TOSEQ"').review.by')" "qa$SUF"
ONE=$(jx 'j.entries[0].seq')
nextstep
req POST /api/v1/audit/reviews "{\"seq\":$ONE,\"outcome\":\"findings\",\"comment\":\"Reason too short for this change\",\"password\":\"$QNEW\",\"code\":\"$(code)\"}"; check "one entry reviewed with findings" "$(st) $(jx 'j.scope+" "+j.entries+" "+(j.from_seq===j.to_seq)')" "201 entry 1 true"
req POST /api/v1/audit/reviews "{\"seq\":999999999,\"outcome\":\"no_findings\",\"password\":\"$QNEW\"}"; check "unknown entry" "$(st)" 404
req GET /api/v1/audit/reviews; check "reviews with valid signatures" "$(jx 'j.slice(0,2).map(r=>r.scope+":"+r.signatures[0].valid).join(",")')" "entry:true,period:true"
admin
req GET /api/v1/audit/reviews; check "administrator reads the reviews (audit.read)" "$(st)" 200
req POST /api/v1/audit/reviews "{\"seq\":$ONE,\"outcome\":\"no_findings\",\"password\":\"$PASS\"}"; check "administrator cannot review (no audit.review)" "$(st)" 403
req GET "/api/v1/export/audit?from=$(iso $((NOW - 300)))&format=jsonl"; check "reviews audited" "$(grep 'audit_review.create' "$TMP/b" | grep -c "qa$SUF")" "2"

echo
echo "signatures: $pass passed, $fail failed"
[[ $fail -eq 0 ]]
