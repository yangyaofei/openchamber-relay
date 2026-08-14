#!/usr/bin/env node
// Smoke test for the OpenChamber relay.
//
// Verifies the core lifecycle without touching the end-to-end crypto layer:
//   1. host-control connects and receives a "sync" message
//   2. a client connects; host-control receives "connected" with its id
//   3. host opens the matching host-data socket (pair succeeds)
//   4. client disconnects; host-control receives "disconnected"
//
// Real deployments keep the host permanently online; this test follows the
// same ordering (host ready before a client connects) to avoid racing the
// relay's "host unavailable" path.
//
// Usage:
//   node test/smoke.mjs [wsUrl]
//   RELAY_URL=ws://127.0.0.1:8080/ws node test/smoke.mjs
//
// Start the relay first: go run .   (from the relay/ directory)

import { WebSocket } from 'ws';

const url = process.argv[2] || process.env.RELAY_URL || 'ws://127.0.0.1:8080/ws';
const serverId = 'smoke-' + Math.random().toString(36).slice(2, 10);
const PROTO = '1';

const fail = (msg) => {
  console.error(`FAIL: ${msg}`);
  process.exit(1);
};
const ok = (msg) => console.log(`ok: ${msg}`);

const step = (description, fn) =>
  new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`timeout: ${description}`)), 10_000);
    fn((err) => {
      clearTimeout(timer);
      if (err) reject(new Error(`${description}: ${err}`));
      else resolve();
    });
  });

function open(params) {
  const ws = new WebSocket(`${url}?v=${PROTO}&${params}`);
  ws.on('error', (e) => fail(`websocket error (${params}): ${e.message}`));
  return ws;
}

const onMsg = (ws, fn) => ws.on('message', (raw) => fn(JSON.parse(raw.toString())));

/** Open a host-control socket and resolve once its initial "sync" arrives. */
function openHost() {
  const host = open(`role=host-control&serverId=${serverId}`);
  return new Promise((resolve) => {
    onMsg(host, function onSync(msg) {
      if (msg.type !== 'sync') return;
      host.removeListener('message', onSync);
      resolve(host);
    });
  });
}

try {
  await step('host-control connects and receives sync', async (done) => {
    const host = await openHost();
    ok('host-control got sync');
    host.close();
    done();
  });

  await step('client connects and host is notified', async (done) => {
    const host = await openHost();
    const client = open(`role=client&serverId=${serverId}`);
    client.on('open', () => ok('client socket open'));
    onMsg(host, (msg) => {
      if (msg.type === 'connected') {
        ok(`host-control saw client ${msg.connectionId}`);
        client.close();
        host.close();
        done();
      }
    });
  });

  await step('host-data pairs with the waiting client', async (done) => {
    const host = await openHost();
    const client = open(`role=client&serverId=${serverId}`);
    client.on('open', () => ok('client socket open'));
    onMsg(host, (msg) => {
      if (msg.type !== 'connected') return;
      const hostData = open(
        `role=host-data&serverId=${serverId}&connectionId=${msg.connectionId}`,
      );
      hostData.on('open', () => {
        ok(`host-data paired with ${msg.connectionId}`);
        client.close();
        hostData.close();
        host.close();
        done();
      });
    });
  });

  await step('host-control sees disconnected after paired client closes', async (done) => {
    const host = await openHost();
    const client = open(`role=client&serverId=${serverId}`);
    client.on('open', () => ok('client socket open'));
    onMsg(host, (msg) => {
      if (msg.type === 'connected') {
        // Real hosts open the data socket as soon as they see "connected".
        const hostData = open(
          `role=host-data&serverId=${serverId}&connectionId=${msg.connectionId}`,
        );
        hostData.on('open', () => {
          ok('pair established, dropping client side');
          setTimeout(() => client.close(1000), 100);
        });
      }
      if (msg.type === 'disconnected') {
        ok('host-control saw disconnected');
        host.close();
        done();
      }
    });
  });

  console.log('\nAll smoke checks passed.');
  process.exit(0);
} catch (e) {
  fail(e.message);
}
