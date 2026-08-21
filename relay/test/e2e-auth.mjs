// E2E: connect host-control with a FRESH upstream-format vector against a
// relay running with RELAY_VERIFY_AUTH=true, expect upgrade success ("sync").
// Usage: node e2e-auth.mjs <fresh-vector.json> [ws-url]
import WebSocket from 'ws';
import { readFileSync } from 'node:fs';

const vec = JSON.parse(readFileSync(process.argv[2], 'utf8'));
const base = process.argv[3] ?? 'ws://127.0.0.1:23098/ws';
const url = new URL(base);
url.searchParams.set('v', '1');
url.searchParams.set('role', vec.role);
url.searchParams.set('serverId', vec.serverId);
url.searchParams.set('ts', vec.ts);
url.searchParams.set('sig', vec.sig);
url.searchParams.set('pk', vec.pk);

const ws = new WebSocket(url);
const done = (ok, msg) => { console.log(ok ? 'PASS' : 'FAIL', msg); ws.close(); process.exit(ok ? 0 : 1); };
ws.on('open', () => ws.on('message', (d) => {
  const m = JSON.parse(d);
  if (m.type === 'sync') done(true, 'upgraded + sync received (auth accepted)');
}));
ws.on('close', (code, reason) => done(false, `closed ${code} ${reason}`));
ws.on('error', (e) => done(false, `error ${e.message}`));
setTimeout(() => done(false, 'timeout'), 5000);
