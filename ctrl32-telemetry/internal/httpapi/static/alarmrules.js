// Alarm rule types (internal/alarm) and the escalation policies a rule can use: shared by the alarm rules screen and
// the asset page (0.74.0).
(function () {
  const esc = (s) => ctrl32.esc(s);
  const RULE_TYPES = [['high', 'high (value above limits)', 'Warning and action limits: the value at or above them.'],
    ['low', 'low (value below limits)', 'Warning and action limits: the value at or below them.'],
    ['low_battery', 'low battery (battery level at or below the limits)', 'Warning and action limits in the unit of the datastream (%, V).'],
    ['power_loss', 'power loss (value 0 = no power)', 'No limits: a value of 0 (false) raises the alarm, any other value clears it.'],
    ['door_open', 'door open (value other than 0)', 'No limits: a value other than 0 (true) raises the alarm after the delay (how long the door may stay open).'],
    ['comm_loss', 'communication loss (no new value)', 'Limits in seconds without a new value; empty = 1.5 × the expected interval of the datastream. A new value clears it.'],
    ['sensor_fault', 'sensor fault (value marked as faulty)', 'No limits: a value the device flags as faulty, or outside the physical range of the datastream, raises the alarm; the next good value clears it.']];
  let POLS = [];
  async function loadPols() { if (ctrl32.can('config.write')) { try { POLS = (await ctrl32.api('GET', '/api/v1/escalation-policies')).filter((p) => !p.revoked_at); } catch (_) { POLS = []; } } return POLS; }
  function ruleTypeOpts(cur) { return RULE_TYPES.map(([v, l]) => `<option value="${v}" ${v === cur ? 'selected' : ''}>${esc(l)}</option>`).join(''); }
  function policyOpts(cur) { return `<option value="">(none: global e-mail)</option>` + POLS.map((p) => `<option value="${p.id}" ${p.id === cur ? 'selected' : ''}>${esc(p.name)}</option>`).join(''); }
  function ruleHint(f) { const t = RULE_TYPES.find((x) => x[0] === f.type.value); const h = f.querySelector('.rule-hint'); if (h) h.textContent = t ? ctrl32.t(t[2]) : ''; const lim = !['power_loss', 'door_open', 'sensor_fault'].includes(f.type.value); f.warning.disabled = f.action.disabled = !lim; f.hysteresis.disabled = !lim || f.type.value === 'comm_loss'; if (!lim) { f.warning.value = f.action.value = ''; } if (f.hysteresis.disabled) f.hysteresis.value = '0'; }
  // the request body of a new rule version from a rule form
  function ruleBody(f, datastreamId) { const body = { datastream_id: datastreamId, type: f.type.value, delay_seconds: parseFloat(f.delay.value || '0'), hysteresis: parseFloat(f.hysteresis.value || '0'), reason: f.reason.value }; if (f.warning.value !== '') body.warning = parseFloat(f.warning.value); if (f.action.value !== '') body.action = parseFloat(f.action.value); if (f.policy && f.policy.value) body.escalation_policy_id = f.policy.value; return body; }
  function ruleSaved(r) { ctrl32.toast(r.status === 'pending' ? 'change request recorded — waiting for approval by a second person' : `rule version ${r.version} active`, r.status === 'pending' ? 'bad' : 'ok'); }
  window.ctrl32Rules = { RULE_TYPES, loadPols, ruleTypeOpts, policyOpts, ruleHint, ruleBody, ruleSaved, get pols() { return POLS; } };
})();
