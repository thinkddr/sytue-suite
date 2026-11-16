/* SPDX-License-Identifier: Apache-2.0 */
/** Metadata-only proof of the real shared Go/Wasm module boundary. */
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
const [wasmPath, runtimePath, expected] = process.argv.slice(2);
assert(['core', 'mail'].includes(expected));
await import(pathToFileURL(runtimePath).href);
const scope = { tenant: 'tenant', subject: 'owner', product: 'mail' },
  port = 'a'.repeat(64);
const encoder = new TextEncoder();
let calls = 0;
globalThis.sytueCoreHostCall = async (request) => {
  assert.equal(request.port, port);
  calls++;
  assert.equal(request.method, 'current');
  return encoder.encode(
    JSON.stringify({
      v: 1,
      scope,
      binding: { matrix_user_id: '@legacy:matrix.test', device_id: 'DEVICE' },
    }),
  );
};
const go = new Go();
go.env = { ...go.env, GOMAXPROCS: '1' };
const loaded = await WebAssembly.instantiate(await readFile(wasmPath), go.importObject);
void go.run(loaded.instance);
for (let i = 0; !globalThis.sytuePqc && i < 100; i++)
  await new Promise((resolve) => setTimeout(resolve, 5));
assert.equal(sytuePqc.module_id, expected);
const input = encoder.encode('{}');
const opened = sytuePqc.invoke({ operation: 'mail.open', _request_id: 1, port, scope, input });
assert.equal(opened instanceof Promise, expected === 'mail');
assert.equal((await opened).ok, false);
assert.equal(calls > 0, expected === 'mail');
const crypto = sytuePqc.invoke({ operation: 'identity.create' });
assert.equal(crypto.ok, expected === 'core');
if (crypto.ok) crypto.value.identity.fill(0);
if (expected === 'mail') {
  const before = calls;
  assert.equal(await sytuePqc.revokePort(port), true);
  const denied = await sytuePqc.invoke({
    operation: 'mail.open',
    _request_id: 2,
    port,
    scope,
    input,
  });
  assert.equal(denied.ok, false);
  assert.equal(calls, before);
  const unknown = await sytuePqc.invoke({
    operation: 'mail.pending',
    _request_id: 3,
    port: 'b'.repeat(64),
    scope,
    input: encoder.encode(JSON.stringify({ session: 'c'.repeat(64) })),
  });
  assert.equal(unknown.ok, false);
}
console.log(
  'MAIL_LAZY_LITERAL_MODULE_' + expected.toUpperCase() + '_NO_CROSS_RUNTIME_HANDLES_GREEN',
);
process.exit(0);
