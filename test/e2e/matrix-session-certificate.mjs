// SPDX-License-Identifier: Apache-2.0
// Opt-in conformance probe: actual Rust SDK signs a Go/Wasm canonical public
// certificate; the SAME Go verifier checks device/root/resource/session bindings.
// All identities are disposable in-memory fixtures. No HTTP or product sessions.
import assert from "node:assert/strict";
import { webcrypto } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";

assert.equal(process.env.SYTUE_MATRIX_SESSION_CERTIFICATE_TEST, "1");
const core = process.env.SYTUE_MATRIX_SESSION_CERTIFICATE_CORE;
const runtime = process.env.SYTUE_MATRIX_SESSION_CERTIFICATE_RUNTIME;
assert.ok(core && runtime);
const sdkRequire = createRequire(
  new URL("../../apps/frontend/packages/offline/package.json", import.meta.url),
);
const rustRequire = createRequire(sdkRequire.resolve("matrix-js-sdk"));
const Rust = rustRequire("@matrix-org/matrix-sdk-crypto-wasm");
await Rust.initAsync();
globalThis.crypto ??= webcrypto;
const require = createRequire(import.meta.url);
require(runtime);
const go = new globalThis.Go();
go.env = { GOMAXPROCS: "1" };
const { instance } = await WebAssembly.instantiate(
  await readFile(core),
  go.importObject,
);
void go.run(instance);
const rpc = (operation, params) => {
  const result = globalThis.sytuePqc.invoke({ operation, ...params });
  assert.equal(result.ok, true, result.code);
  return result.value;
};
const jsonBytes = (value) => new TextEncoder().encode(JSON.stringify(value));
const decode = (value) => JSON.parse(new TextDecoder().decode(value));
const user = "@sender:matrix.test",
  device = "RUST_SENDER",
  room = "!resource:matrix.test";
const machine = await Rust.OlmMachine.initialize(
  new Rust.UserId(user),
  new Rust.DeviceId(device),
);
try {
  const bootstrap = await machine.bootstrapCrossSigning(true);
  const signingKeys = JSON.parse(bootstrap.uploadSigningKeysRequest.body);
  const uploaded = JSON.parse(bootstrap.uploadKeysRequest.body).device_keys;
  const signedDevices = JSON.parse(bootstrap.uploadSignaturesRequest.body);
  const signed = signedDevices[user]?.[device];
  assert.ok(signed, "Rust bootstrap did not cross-sign its device");
  const deviceKeys = {
    ...uploaded,
    ...signed,
    signatures: {
      [user]: { ...uploaded.signatures?.[user], ...signed.signatures?.[user] },
    },
  };
  const root = Object.values(signingKeys.master_key.keys)[0];
  const settings = new Rust.EncryptionSettings();
  await machine.shareRoomKey(new Rust.RoomId(room), [], settings);
  const encrypted = JSON.parse(
    await machine.encryptRoomEvent(
      new Rust.RoomId(room),
      "m.room.message",
      JSON.stringify({
        msgtype: "m.text",
        body: "synthetic certificate fixture",
      }),
    ),
  );
  const expectation = {
    scope: { tenant: "fixture", subject: "sender", product: "rooms" },
    resource: {
      tenant: "fixture",
      product: "rooms",
      kind: "channel",
      id: "fixture-channel",
    },
    epoch: "epoch1",
    generation: "9223372036854775807",
    policy_sha256: "1".repeat(64),
    session: {
      room_id: room,
      session_id: encrypted.session_id,
      sender_key: encrypted.sender_key,
      sender_user: user,
      signing_key: deviceKeys.keys["ed25519:" + device],
    },
    device_id: device,
    root_key: root,
  };
  const encoded = rpc("matrix.session.certificate.encode", {
    expectation: jsonBytes(expectation),
  });
  const unsigned = decode(encoded.payload);
  const signatures = await machine.sign(
    new TextDecoder().decode(encoded.payload),
  );
  const certificate = {
    ...unsigned,
    signatures: JSON.parse(signatures.asJSON()),
  };
  const origin = {
    scope: expectation.scope,
    server_name: "matrix.test",
    user_id: user,
    device_id: device,
    account_active: true,
    device_active: true,
    keys: {
      master_keys: { [user]: signingKeys.master_key },
      self_signing_keys: { [user]: signingKeys.self_signing_key },
      device_keys: { [user]: { [device]: deviceKeys } },
      failures: {},
    },
  };
  const params = {
    expectation: jsonBytes(expectation),
    certificate: jsonBytes(certificate),
    origin: jsonBytes(origin),
  };
  assert.equal(rpc("matrix.session.certificate.verify", params).id, encoded.id);
  let rejected = 0;
  for (const change of [
    (e) => {
      e.resource.product = "mail";
      e.scope.product = "mail";
    },
    (e) => {
      e.resource.tenant = "another";
      e.scope.tenant = "another";
    },
    (e) => {
      e.resource.id = "other-resource";
    },
    (e) => {
      e.scope.subject = "other-subject";
    },
    (e) => {
      e.epoch = "epoch2";
    },
    (e) => {
      e.generation = "1";
    },
    (e) => {
      e.generation = 1;
    },
    (e) => {
      e.policy_sha256 = "2".repeat(64);
    },
    (e) => {
      e.session.room_id = "!other:matrix.test";
    },
    (e) => {
      e.session.sender_user = "@other:matrix.test";
    },
    (e) => {
      e.session.session_id = e.session.sender_key;
    },
    (e) => {
      e.session.sender_key = e.session.session_id;
    },
    (e) => {
      e.session.signing_key = e.session.sender_key;
    },
    (e) => {
      e.device_id = "OTHER_DEVICE";
    },
    (e) => {
      e.root_key = e.session.signing_key;
    },
  ]) {
    const changed = structuredClone(expectation);
    change(changed);
    assert.equal(
      globalThis.sytuePqc.invoke({
        operation: "matrix.session.certificate.verify",
        ...params,
        expectation: jsonBytes(changed),
      }).ok,
      false,
    );
    rejected++;
  }
  const inactive = structuredClone(origin);
  inactive.device_active = false;
  assert.equal(
    globalThis.sytuePqc.invoke({
      operation: "matrix.session.certificate.verify",
      ...params,
      origin: jsonBytes(inactive),
    }).ok,
    false,
  );
  rejected++;
  const unsignedCertificate = { ...certificate };
  delete unsignedCertificate.signatures;
  assert.equal(
    globalThis.sytuePqc.invoke({
      operation: "matrix.session.certificate.verify",
      ...params,
      certificate: jsonBytes(unsignedCertificate),
    }).ok,
    false,
  );
  rejected++;
  const anotherMachine = await Rust.OlmMachine.initialize(
    new Rust.UserId(user),
    new Rust.DeviceId(device),
  );
  try {
    const forged = {
      ...unsigned,
      signatures: JSON.parse(
        (
          await anotherMachine.sign(new TextDecoder().decode(encoded.payload))
        ).asJSON(),
      ),
    };
    assert.equal(
      globalThis.sytuePqc.invoke({
        operation: "matrix.session.certificate.verify",
        ...params,
        certificate: jsonBytes(forged),
      }).ok,
      false,
    );
    rejected++;
  } finally {
    anotherMachine.close();
  }
  if (process.env.SYTUE_RESOURCE_BOOTSTRAP_VECTOR) {
    const identity = rpc("identity.create", {});
    identity.identity.fill(0);
    const context = {
      v: 1,
      domain: "org.sytue.resource.bootstrap.v1",
      kind: "baseline",
      resource: expectation.resource,
      room_id: room,
      epoch: "epoch1",
      recipient_id: identity.key_id,
      source_cutover: { seq: "9223372036854775807", head_cipher_sha256: "" },
      creator_scope: expectation.scope,
      creator: { user_id: user, device_id: device, root_key: root },
      snapshot_kind: "history_archive",
      snapshot_version: 1,
    };
    const encodedContext = rpc("resource.bootstrap.context", { context: jsonBytes(context) });
    const canonicalContext = decode(encodedContext.payload);
    assert.equal(canonicalContext.source_cutover.seq, "9223372036854775807");
    assert.equal(typeof canonicalContext.source_cutover.seq, "string");
    const contextSignatures = await machine.sign(new TextDecoder().decode(encodedContext.payload));
    await writeFile(process.env.SYTUE_RESOURCE_BOOTSTRAP_VECTOR, JSON.stringify({
      context: canonicalContext,
      signed_context: { ...canonicalContext, signatures: JSON.parse(contextSignatures.asJSON()) },
      origin,
      context_sha256: encodedContext.sha256,
    }), { mode: 0o600 });
  }
  if (process.env.SYTUE_MATRIX_SESSION_CERTIFICATE_VECTOR) {
    await writeFile(
      process.env.SYTUE_MATRIX_SESSION_CERTIFICATE_VECTOR,
      JSON.stringify({ expectation, certificate, origin }),
      { mode: 0o600 },
    );
  }
  if (process.env.SYTUE_MATRIX_SESSION_CERTIFICATE_GO_VECTOR) {
    const native = JSON.parse(
      await readFile(
        process.env.SYTUE_MATRIX_SESSION_CERTIFICATE_GO_VECTOR,
        "utf8",
      ),
    );
    const result = rpc("matrix.session.certificate.verify", {
      expectation: jsonBytes(native.expectation),
      certificate: jsonBytes(native.certificate),
      origin: jsonBytes(native.origin),
    });
    assert.ok(result.id.startsWith("sessioncert1_"));
  }
  console.log(
    JSON.stringify({
      sdk: "maintained Rust OlmMachine",
      core: "same Go/Wasm",
      accepted: 1,
      rejected,
    }),
  );
} finally {
  machine.close();
}
process.exit(0);
