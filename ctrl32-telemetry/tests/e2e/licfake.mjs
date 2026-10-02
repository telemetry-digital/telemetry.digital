// A stand-in for the licence worker's POST /telemetry/activate (lic.ctrl32.com) for the e2e suites: it signs 30-day
// licenses with the test ONLINE key (tests/keys/license-online-test.key, trusted by test builds as "build-online",
// max-days 31) and ones without expiry with the test master key (tests/keys/license-test.key, "build"). Answers are set per
// order through POST /control {order, mode, days}:
//   starter (default)  200, a license of `days` days (default 30) from the online key
//   permanent          200, a license without expiry from the master key
//   rogue              200, a license WITHOUT EXPIRY from the ONLINE key (the server must refuse it)
// An optional updates_until (YYYY-MM-DD) in the control body is written into the signed license.
//   refunded | claimed-by-editor | host-cap | unknown-order | unpaid   the worker's refusals (410/409/409/404/402)
//   pending            503 pending (temporary)
// GET /requests lists the activation requests received.
//
//   LICFAKE_PORT=18790 node tests/e2e/licfake.mjs
import { createServer } from 'node:http';
import { readFileSync } from 'node:fs';
import { createPrivateKey, sign } from 'node:crypto';

const dir = new URL('../keys/', import.meta.url);
const key = (name) => {
  const seed = Buffer.from(readFileSync(new URL(name, dir), 'utf8').trim(), 'base64');
  const der = Buffer.concat([Buffer.from('302e020100300506032b657004220420', 'hex'), seed]);
  return createPrivateKey({ key: der, format: 'der', type: 'pkcs8' });
};
const online = key('license-online-test.key');
const master = key('license-test.key');
const b64url = (b) => Buffer.from(b).toString('base64url');
const rfc = (ms) => new Date(Math.floor(ms / 1000) * 1000).toISOString().replace('.000Z', 'Z');
function token(k, payload) {
  const body = 'c32l1.' + b64url(JSON.stringify(payload));
  return body + '.' + b64url(sign(null, Buffer.from(body), k));
}

const modes = new Map();
const requests = [];
const refusals = { refunded: 410, 'claimed-by-editor': 409, 'host-cap': 409, 'unknown-order': 404, unpaid: 402, pending: 503 };
const send = (res, st, obj) => { res.writeHead(st, { 'content-type': 'application/json' }); res.end(JSON.stringify(obj)); };

createServer((req, res) => {
  let body = '';
  req.on('data', (c) => { body += c; });
  req.on('end', () => {
    let j = {};
    try { j = body ? JSON.parse(body) : {}; } catch { return send(res, 400, { error: 'bad-request' }); }
    if (req.method === 'POST' && req.url === '/control') { modes.set(j.order, j); return send(res, 200, { ok: true }); }
    if (req.method === 'GET' && req.url === '/requests') return send(res, 200, requests);
    if (req.method !== 'POST' || req.url !== '/telemetry/activate') return send(res, 404, { error: 'not-found' });
    requests.push(j);
    if (typeof j.order !== 'string' || !j.order || typeof j.host !== 'string' || !j.host) return send(res, 400, { error: 'bad-request' });
    const m = modes.get(j.order) || { mode: 'starter' };
    if (refusals[m.mode]) return send(res, refusals[m.mode], { error: m.mode });
    const now = Date.now();
    const p = { id: 'E2E-' + m.mode + '-' + now, licensee: 'E2E Online', email: 'e2e@example.com', issued: rfc(now),
                features: ['white_label'], domains: [j.host], notes: 'order ' + j.order };
    if (m.updates_until) p.updates_until = m.updates_until;
    if (m.mode === 'permanent') return send(res, 200, { license: token(master, p), kind: 'permanent', expires: null });
    if (m.mode === 'rogue') return send(res, 200, { license: token(online, p), kind: 'permanent', expires: null });
    p.expires = rfc(now + (m.days || 30) * 86400_000);
    return send(res, 200, { license: token(online, p), kind: 'starter', expires: p.expires });
  });
}).listen(Number(process.env.LICFAKE_PORT || 18790), '127.0.0.1');
