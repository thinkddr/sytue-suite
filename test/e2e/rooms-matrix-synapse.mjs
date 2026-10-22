/* SPDX-License-Identifier: Apache-2.0 */
// Opt-in transport smoke against a disposable, loopback-only Synapse. No SSO or UI claim.
import assert from 'node:assert/strict';
import { createHmac, randomBytes, randomUUID } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';

const require = createRequire(
  new URL('../../apps/frontend/apps/rooms/package.json', import.meta.url),
);
const { createClient, EventType, MatrixEvent } = await import(
  pathToFileURL(require.resolve('matrix-js-sdk')).href
);
const quietLogger = {
  trace() {},
  debug() {},
  info() {},
  warn() {},
  error() {},
  getChild() {
    return this;
  },
};
const {
  AllDevicesIsolationMode,
  CryptoEvent,
  VerificationPhase,
  VerificationRequestEvent,
  VerifierEvent,
} = await import(pathToFileURL(require.resolve('matrix-js-sdk/lib/crypto-api')).href);

const base = process.env.SYTUE_MATRIX_SMOKE_HOME;
if (!base || !['127.0.0.1', 'localhost', '[::1]'].includes(new URL(base).hostname))
  throw new Error('A disposable loopback-only Synapse is required');
const adminFile = process.env.SYTUE_MATRIX_SMOKE_ADMIN_FILE;
const configFile = process.env.SYTUE_MATRIX_SMOKE_CONFIG_FILE;
const nativePath = process.env.SYTUE_MATRIX_SMOKE_NATIVE;
if (!adminFile || !configFile || !nativePath)
  throw new Error('Missing smoke fixture configuration');
const admin = (await readFile(adminFile, 'utf8')).trim();
const registrationSecret = JSON.parse(
  await readFile(configFile, 'utf8'),
).registration_shared_secret;
if (typeof registrationSecret !== 'string' || !registrationSecret)
  throw new Error('Fixture registration is unavailable');
const users = [];
let client;
let room;
let relay;
const deadline = setTimeout(() => {
  process.exitCode = 1;
  void cleanup().finally(() => process.exit(1));
}, 120000);
const summary = {
  artificial: true,
  sdk: 'matrix-js-sdk 43.0.0 + mautrix-go 0.32.0/goolm',
  transport: false,
  sas: false,
  js_to_go: false,
  go_to_js: false,
  revoked_recipient: false,
  opaque_server_events: false,
};
async function http(method, path, body, token = admin) {
  const response = await fetch(new URL(path, base), {
    method,
    headers: {
      'content-type': 'application/json',
      ...(token ? { authorization: `Bearer ${token}` } : {}),
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) throw new Error(`Fixture HTTP ${response.status} ${path.split('?')[0]}`);
  return response.status === 204 ? {} : response.json();
}
async function register() {
  const username = `sytue_${randomUUID()}`;
  const password = randomBytes(32).toString('base64url');
  const { nonce } = await http('GET', '/_synapse/admin/v1/register', undefined, '');
  const mac = createHmac('sha1', registrationSecret)
    .update(`${nonce}\0${username}\0${password}\0notadmin`)
    .digest('hex');
  const session = await http(
    'POST',
    '/_synapse/admin/v1/register',
    { nonce, username, password, admin: false, mac },
    '',
  );
  assert.ok(session.access_token && session.device_id && session.user_id);
  users.push(session.user_id);
  return { ...session, password };
}
function until(check, label, ms = 20000) {
  const start = Date.now();
  return new Promise((resolve, reject) => {
    const poll = async () => {
      try {
        const result = await check();
        if (result) {
          resolve(result);
          return;
        }
      } catch (error) {
        reject(error);
        return;
      }
      if (Date.now() - start >= ms) {
        reject(new Error(`Timed out: ${label}`));
        return;
      }
      setTimeout(poll, 100);
    };
    void poll();
  });
}
function native() {
  const child = spawn(nativePath, ['-test.run', '^TestMatrixSynapseRelay$', '-test.v=false'], {
    stdio: ['pipe', 'pipe', 'ignore'],
  });
  const messages = [];
  const stream = createInterface({ input: child.stdout });
  stream.on('line', (line) => {
    if (line.startsWith('{')) messages.push(JSON.parse(line));
  });
  let exited = false;
  child.on('exit', () => {
    exited = true;
  });
  const send = (message) => child.stdin.write(JSON.stringify(message) + '\n');
  const wait = (kind) =>
    until(() => {
      const failure = messages.find((item) => item.type === 'error');
      if (failure)
        throw new Error(
          `Native relay failed: ${String(failure.stage ?? 'unknown') + ' (' + String(failure.error_type ?? 'unknown') + ')'}`,
        );
      const index = messages.findIndex((item) => item.type === kind);
      if (index >= 0) return messages.splice(index, 1)[0];
      if (exited) throw new Error('Native relay exited before completing the smoke');
      return null;
    }, `native ${kind}`);
  return {
    send,
    wait,
    close: () => {
      send({ action: 'close' });
      child.stdin.end();
      child.kill();
      stream.close();
    },
  };
}
async function cleanup() {
  clearTimeout(deadline);
  relay?.close();
  if (client) {
    client.stopClient();
    await client.logout().catch(() => undefined);
  }
  if (room)
    await http('DELETE', `/_synapse/admin/v2/rooms/${encodeURIComponent(room)}`, {
      purge: true,
      block: false,
    }).catch(() => undefined);
  for (const user of users)
    await http('POST', `/_synapse/admin/v1/deactivate/${encodeURIComponent(user)}`, {
      erase: true,
    }).catch(() => undefined);
}
try {
  const js = await register();
  const go = await register();
  const who = await http('GET', '/_matrix/client/v3/account/whoami');
  const controller = who.user_id;
  assert.equal(controller, '@rooms_controller:matrix.sytue.test');
  const created = await http('POST', '/_matrix/client/v3/createRoom', {
    room_version: '12',
    preset: 'private_chat',
    power_level_content_override: {
      users: {},
      users_default: 0,
      events_default: 100,
      state_default: 100,
      invite: 100,
      kick: 100,
      ban: 100,
      redact: 100,
      events: { 'm.room.encrypted': 0 },
    },
    invite: [js.user_id, go.user_id],
    creation_content: { 'm.federate': false },
    initial_state: [
      { type: 'm.room.encryption', state_key: '', content: { algorithm: 'm.megolm.v1.aes-sha2' } },
      {
        type: 'm.room.history_visibility',
        state_key: '',
        content: { history_visibility: 'joined' },
      },
    ],
  });
  room = created.room_id;
  assert.match(room, /^![A-Za-z0-9_-]{43}$/);
  const secretKeys = new Map();
  client = createClient({
    logger: quietLogger,
    baseUrl: base,
    userId: js.user_id,
    deviceId: js.device_id,
    accessToken: js.access_token,
    cryptoCallbacks: {
      getSecretStorageKey: async ({ keys }) => {
        for (const id of Object.keys(keys)) if (secretKeys.has(id)) return [id, secretKeys.get(id)];
        return null;
      },
      cacheSecretStorageKey: (id, _description, key) => {
        secretKeys.set(id, key);
      },
    },
  });
  await client.initRustCrypto({ useIndexedDB: false });
  const crypto = client.getCrypto();
  crypto.setDeviceIsolationMode(new AllDevicesIsolationMode(true));
  crypto.globalBlacklistUnverifiedDevices = true;
  crypto.setTrustCrossSignedDevices(true);
  await client.startClient({ initialSyncLimit: 20, lazyLoadMembers: false });
  await until(() => client.isInitialSyncComplete(), 'JS initial sync');
  await crypto.bootstrapSecretStorage({
    createSecretStorageKey: () => crypto.createRecoveryKeyFromPassphrase(),
    setupNewKeyBackup: false,
  });
  await crypto.bootstrapCrossSigning({ authUploadDeviceSigningKeys: (request) => request(null) });
  await crypto.resetKeyBackup();
  assert.equal((await crypto.checkKeyBackupAndEnable())?.trustInfo.trusted, true);
  await client.joinRoom(room);
  await until(
    () =>
      client.getRoom(room)?.getMyMembership() === 'join' && crypto.isEncryptionEnabledInRoom(room),
    'JS joined encrypted room',
  );
  const cleartextAttempt = await fetch(
    new URL(
      `/_matrix/client/v3/rooms/${encodeURIComponent(room)}/send/m.room.message/${randomUUID()}`,
      base,
    ),
    {
      method: 'PUT',
      headers: { authorization: `Bearer ${js.access_token}`, 'content-type': 'application/json' },
      body: JSON.stringify({
        msgtype: 'm.text',
        body: 'ARTIFICIAL CLEAR MESSAGE MUST BE REJECTED',
      }),
    },
  );
  assert.equal(cleartextAttempt.status, 403);
  relay = native();
  relay.send({
    action: 'init',
    homeserver_url: base,
    user_id: go.user_id,
    access_token: go.access_token,
    device_id: go.device_id,
    room_id: room,
    controller_user_id: controller,
    password: go.password,
  });
  const nativeReady = await relay.wait('ready');
  assert.equal(nativeReady.device_id, go.device_id);
  summary.transport = true;
  let verificationFinished;
  const verification = new Promise((resolve, reject) => {
    verificationFinished = { resolve, reject };
  });
  client.on(CryptoEvent.VerificationRequestReceived, (request) => {
    if (request.otherUserId !== go.user_id) {
      void request.cancel();
      return;
    }
    let started = false;
    let accepting = false;
    let compared = false;
    const changed = async () => {
      try {
        if (request.phase === VerificationPhase.Requested) {
          if (accepting) return;
          accepting = true;
          await request.accept();
          return;
        }
        if (
          started ||
          (request.phase !== VerificationPhase.Ready && request.phase !== VerificationPhase.Started)
        )
          return;
        started = true;
        const verifier = request.verifier ?? (await request.startVerification('m.sas.v1'));
        const compare = (sas) => {
          if (compared) return;
          compared = true;
          void (async () => {
            const remote = await relay.wait('sas');
            assert.deepEqual(sas.sas.decimal, remote.decimal);
            relay.send({ action: 'confirm', transaction_id: remote.transaction_id });
            await sas.confirm();
          })().catch(verificationFinished.reject);
        };
        verifier.on(VerifierEvent.ShowSas, compare);
        const current = verifier.getShowSasCallbacks();
        if (current) compare(current);
        await verifier.verify();
        verificationFinished.resolve();
      } catch (error) {
        verificationFinished.reject(error);
      }
    };
    request.on(VerificationRequestEvent.Change, () => {
      void changed();
    });
    void changed();
  });
  relay.send({ action: 'verify', user_id: js.user_id });
  await verification;
  await relay.wait('verified');
  assert.equal((await crypto.getUserVerificationStatus(go.user_id)).isCrossSigningVerified(), true);
  summary.sas = true;
  const sent = await client.sendEvent(room, EventType.RoomMessage, {
    msgtype: 'm.text',
    body: 'ARTIFICIAL JS TO GO PRIVATE MESSAGE',
  });
  const nativeMessage = await relay.wait('message');
  assert.equal(nativeMessage.event_id, sent.event_id);
  assert.equal(nativeMessage.text, 'ARTIFICIAL JS TO GO PRIVATE MESSAGE');
  summary.js_to_go = true;
  const raw = await client.fetchRoomEvent(room, sent.event_id);
  assert.equal(raw.type, EventType.RoomMessageEncrypted);
  assert.equal(JSON.stringify(raw).includes('ARTIFICIAL JS TO GO PRIVATE MESSAGE'), false);
  const firstSession = raw.content.session_id;
  relay.send({ action: 'send', room_id: room, text: 'ARTIFICIAL GO TO JS PRIVATE MESSAGE' });
  const goSent = await relay.wait('sent');
  const goEvent = new MatrixEvent(await client.fetchRoomEvent(room, goSent.event_id));
  await until(async () => {
    await client.decryptEventIfNeeded(goEvent);
    return goEvent.getContent().body === 'ARTIFICIAL GO TO JS PRIVATE MESSAGE';
  }, 'JS decrypt Go message');
  summary.go_to_js = true;
  await http('POST', `/_matrix/client/v3/rooms/${encodeURIComponent(room)}/kick`, {
    user_id: go.user_id,
  });
  await until(
    () => client.getRoom(room)?.getMember(go.user_id)?.membership === 'leave',
    'JS revoked membership sync',
  );
  const later = await client.sendEvent(room, EventType.RoomMessage, {
    msgtype: 'm.text',
    body: 'ARTIFICIAL POST REVOCATION PRIVATE MESSAGE',
  });
  const laterRaw = await client.fetchRoomEvent(room, later.event_id);
  assert.equal(laterRaw.type, EventType.RoomMessageEncrypted);
  assert.notEqual(laterRaw.content.session_id, firstSession);
  assert.equal(
    JSON.stringify(laterRaw).includes('ARTIFICIAL POST REVOCATION PRIVATE MESSAGE'),
    false,
  );
  relay.send({ action: 'decrypt', room_id: room, event: laterRaw });
  await relay.wait('undecryptable');
  summary.revoked_recipient = true;
  summary.opaque_server_events = true;
  process.stdout.write(JSON.stringify(summary) + '\n');
} catch (error) {
  process.stderr.write(
    `Matrix transport smoke failed: ${error instanceof Error ? error.message : 'unknown'}\n`,
  );
  process.exitCode = 1;
} finally {
  await cleanup();
  process.exit(process.exitCode ?? 0);
}
