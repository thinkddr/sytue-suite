/* SPDX-License-Identifier: Apache-2.0 */
// Real Go/Wasm RPC, disposable native fixtures. Implements no JavaScript cipher.
const fs = require('node:fs');
const crypto = require('node:crypto');
const assert = require('node:assert/strict');
globalThis.crypto ??= crypto.webcrypto;
const [bundle, runtime, input, output] = process.argv.slice(2);
require(runtime);
const bytes = (value) => new Uint8Array(Buffer.from(value, 'base64'));
const jsonBytes = (value) => new TextEncoder().encode(JSON.stringify(value));
(async () => {
  const go = new Go();
  go.env = { GOMAXPROCS: '1' };
  const { instance } = await WebAssembly.instantiate(fs.readFileSync(bundle), go.importObject);
  void go.run(instance);
  const core = globalThis.sytuePqc;
  // El mismo sobre se abre en el núcleo completo (`core`) y en el de la bóveda (`vault`).
  assert.ok(core.module_id === 'core' || core.module_id === 'vault');
  const invoke = (request) => {
    const result = core.invoke(request);
    assert.equal(result.ok, true);
    return result.value;
  };
  const fixtures = JSON.parse(fs.readFileSync(input, 'utf8'));
  const result = [];
  for (let i = 0; i < fixtures.length; i++) {
    const fixture = fixtures[i];
    const identity = bytes(fixture.identity);
    const envelope = { ...fixture.envelope, ciphertext: bytes(fixture.envelope.ciphertext) };
    const request = {
      operation: 'storage.open',
      identity,
      scope: fixture.scope,
      room: fixture.room,
      envelope,
    };
    const opened = invoke(request);
    assert.equal(Buffer.from(opened.record).toString('base64'), fixture.record);
    assert.equal(Buffer.from(opened.proof).toString('base64'), fixture.proof);
    for (const fault of ['tenant', 'subject', 'product', 'room', 'key', 'profile']) {
      let changed = { ...request };
      if (fault === 'tenant') changed.scope = { ...fixture.scope, tenant: 'other' };
      if (fault === 'subject') changed.scope = { ...fixture.scope, subject: 'other' };
      if (fault === 'product')
        changed.scope = {
          ...fixture.scope,
          product: fixture.scope.product === 'mail' ? 'drive' : 'mail',
        };
      if (fault === 'room') changed.room = '!other:matrix.test';
      if (fault === 'key') changed.identity = bytes(fixtures[(i + 1) % fixtures.length].identity);
      if (fault === 'profile') changed.envelope = { ...envelope, profile: 'age-X25519-v1' };
      const denied = core.invoke(changed);
      assert.equal(denied.ok, false);
      assert.equal(denied.value, undefined);
    }
    const sealed = invoke({
      operation: 'storage.seal',
      scope: fixture.scope,
      room: fixture.room,
      profile: 'age-MLKEM768-X25519-v1',
      recipient: fixture.recipient,
      record: bytes(fixture.record),
      proof: bytes(fixture.proof),
    });
    assert.equal(sealed.profile, 'age-MLKEM768-X25519-v1');
    result.push({ ...sealed, ciphertext: Buffer.from(sealed.ciphertext).toString('base64') });
    identity.fill(0);
    envelope.ciphertext.fill(0);
    opened.record.fill(0);
    opened.proof.fill(0);
    sealed.ciphertext.fill(0);
  }
  fs.writeFileSync(output, JSON.stringify(result), { mode: 0o600 });
  process.exit(0);
})().catch(() => {
  process.stderr.write('native/Wasm interoperability assertion failed\n');
  process.exit(1);
});
