/* SPDX-License-Identifier: Apache-2.0 */
/** Synthetic metadata only; actual Go/Wasm capability normalization and binding. */
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
const [wasmPath, runtimePath] = process.argv.slice(2);
await import(pathToFileURL(runtimePath).href);
const scope = { tenant: 'tenant', subject: 'owner', product: 'mail' },
  port = 'a'.repeat(64);
let current = {
  v: 1,
  scope,
  binding: { matrix_user_id: '@sytue_storage_mail_tenant_owner:matrix.test', device_id: 'DEVICE' },
};
let maximum = 0,
  shared = false,
  ready = true,
  calls = 0;
const encoder = new TextEncoder();
globalThis.sytueCoreHostCall = async (request) => {
  assert.equal(request.port, port);
  if (request.method === 'current') return encoder.encode(JSON.stringify(current));
  assert.equal(request.method, 'source.capabilities');
  assert.deepEqual(request.scope, scope);
  calls++;
  return encoder.encode(
    JSON.stringify({
      v: 1,
      scope,
      mode: 'legacy',
      rows: 0,
      blobs: 0,
      shared,
      max_body_bytes: maximum,
      private_ingress_ready: ready,
    }),
  );
};
const go = new Go();
go.env = { ...go.env, GOMAXPROCS: '1' };
const module = await WebAssembly.instantiate(await readFile(wasmPath), go.importObject);
void go.run(module.instance);
for (let count = 0; !globalThis.sytuePqc && count < 100; count++)
  await new Promise((resolve) => setTimeout(resolve, 5));
assert(globalThis.sytuePqc);
let request = 0;
async function call(requestedScope = scope) {
  return await sytuePqc.invoke({
    operation: 'mail.capabilities',
    _request_id: ++request,
    port,
    scope: requestedScope,
  });
}
try {
  let result = await call();
  assert.equal(result.ok, true);
  assert.equal(result.value.max_body_bytes, 50 << 20);
  assert.deepEqual(result.value.scope, scope);
  maximum = 100 << 20;
  result = await call();
  assert.equal(result.ok, true);
  assert.equal(result.value.max_body_bytes, maximum);
  const before = calls;
  result = await call({ ...scope, subject: 'foreign' });
  assert.equal(result.ok, false);
  assert.equal(calls, before);
  current = { ...current, binding: { ...current.binding, matrix_user_id: '@owner:matrix.test' } };
  result = await call();
  assert.equal(result.ok, false);
  assert.equal(calls, before);
  current = {
    ...current,
    binding: { ...current.binding, matrix_user_id: '@sytue_storage_mail_tenant_owner:matrix.test' },
  };
  for (const state of [
    { maximum: -1, shared: false, ready: true },
    { maximum: 2 ** 48, shared: false, ready: true },
    { maximum: 50 << 20, shared: true, ready: true },
    { maximum: 50 << 20, shared: false, ready: false },
  ]) {
    ({ maximum, shared, ready } = state);
    result = await call();
    assert.equal(result.ok, false);
  }
  console.log(
    'MAIL_CAPABILITIES_SHARED_NATIVE_LIMIT_WASM_SCOPE_ZERO_DEFAULT_100M_NO_SILENT_REDUCTION_GREEN',
  );
  process.exit(0);
} catch (error) {
  console.error(error);
  process.exit(1);
}
