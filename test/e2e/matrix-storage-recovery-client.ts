import { verifyCoreIngressPort } from './core-ingress-port-client.js';
import { textoAleatorio } from './lib/texto-aleatorio.js';
// SPDX-License-Identifier: Apache-2.0
import { verifyCoreStoragePort } from './storage-core-port-client.js';
import {
  SuiteMatrix,
  type MatrixRecoveryUI,
  type MatrixSessionAdapter,
} from '../../apps/frontend/packages/offline/src/matrix/client';
import {
  createStorageRecord,
  storageJSONBytes,
  encodeStorageJSON,
} from '../../apps/frontend/packages/offline/src/matrix/storage';
import { MatrixEvent, EventType, type RoomMessageEventContent } from 'matrix-js-sdk';
import { EventShieldReason } from 'matrix-js-sdk/lib/crypto-api';
import { matrixStorePrefix } from '../../apps/frontend/packages/offline/src/matrix/types';
import type {
  SignedStorageControlAction,
  StorageRootEnrollment,
} from '../../apps/frontend/packages/config/src/matrix';
import { storageControlIntent } from '../../apps/frontend/packages/offline/src/matrix/storage-authority';
let rootProof: SignedStorageControlAction | undefined;
let rootRequest: StorageRootEnrollment | undefined;
let rootApprovals = 0;
const state = { stage: 'fixture', ok: false, reason: '', diagnostic: {} };
Object.assign(window, { storageProbe: state });
let suite: SuiteMatrix | undefined;
let recoveryKey = 'invalid artificial recovery key';
const resume = new URL(location.href).searchParams.has('resume');
void (async () => {
  const fixture = await (await fetch('/fixture')).json();
  if (fixture.artificial !== true) throw new Error('artificial fixture required');
  state.stage = 'open';
  let finishErase!: () => void, enteredErase!: () => void;
  const erasing = new Promise<void>((resolve) => {
    enteredErase = resolve;
  });
  const eraseGate = new Promise<void>((resolve) => {
    finishErase = resolve;
  });
  const recoveryUI: MatrixRecoveryUI = {
    confirmStorageRoot: async () => {
      rootApprovals++;
    },
    saveRecoveryKey: async () => {
      throw new Error('must recover existing identity');
    },
    readRecoveryKey: async () => {
      if (resume) throw new Error('reload must use encrypted owner cache');
      return recoveryKey;
    },
    authenticate: async () => {
      throw new Error('existing identity should not be reset');
    },
  };
  const sessionAdapter: MatrixSessionAdapter = {
    readSession: async () => fixture.session,
    forgetSession: async () => {
      if (resume) {
        enteredErase();
        await eraseGate;
      }
    },
    refreshBootstrap: async () => fixture.bootstrap,
  };
  suite = await SuiteMatrix.open(fixture.bootstrap, recoveryUI, sessionAdapter);
  const records = suite.storage({
    read: async (id) => {
      const response = await fetch('/blob/' + id);
      if (!response.ok) throw new Error('missing artificial ciphertext');
      return new Uint8Array(await response.arrayBuffer());
    },
    write: async (id, ciphertext) => {
      const response = await fetch('/blob/' + id, {
        method: 'PUT',
        body: ciphertext,
      });
      if (!response.ok) throw new Error('artificial blob write failed');
      return response.json();
    },
  });
  const fileRequest = {
    object_id: 'browser-file',
    revision_id: 'browser-file-revision',
    size_bytes: 73,
    metadata: encodeStorageJSON(
      new TextEncoder().encode(
        '{"title":"private browser upload","integer":900719925474099512345}',
      ),
    ),
  };
  const originalFile = new Blob([new Uint8Array(73).fill(9)]);

  if (resume) {
    state.stage = 'reloaded-encrypted-cache';
    const restored = await records.readRecord(
      fixture.reference,
      fixture.record.object_id,
      'content',
    );
    if (restored.payload !== fixture.record.payload)
      throw new Error('reload changed owner-attested bytes');
    const db = await new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open('sytue-offline');
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(new Error('offline inspection'));
    });
    try {
      const rows = await new Promise<Record<string, unknown>[]>((resolve, reject) => {
        const request = db.transaction('records', 'readonly').objectStore('records').getAll();
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(new Error('offline rows'));
      });
      const root = rows.find((row) => row.namespace === 'local-crypto-key-v1')?.value as
        { key?: CryptoKey } | undefined;
      const encrypted = rows.find((row) => row.namespace === 'local-crypto-value-v1')?.value as
        { iv?: Uint8Array; ciphertext?: Uint8Array } | undefined;
      if (
        !(root?.key instanceof CryptoKey) ||
        root.key.extractable ||
        encrypted?.iv?.length !== 12 ||
        !(encrypted.ciphertext instanceof Uint8Array)
      )
        throw new Error('owner cache is not encrypted with a non-extractable IDB key');
      let exportDenied = false;
      try {
        await crypto.subtle.exportKey('raw', root.key);
      } catch {
        exportDenied = true;
      }
      if (!exportDenied) throw new Error('device cache key was exportable');
      if (JSON.stringify(rows).includes(fixture.recovery))
        throw new Error('clear recovery key in local storage');
      state.diagnostic = {
        encryptedCache: true,
        nonExtractable: true,
        recoveryPrompted: false,
      };
    } finally {
      db.close();
    }
    state.stage = 'reloaded-file-journal';
    const resumedFile = await records
      .uploads()
      .upload('browser-file-job', fileRequest, originalFile);
    const publishedFile = await (await fetch('/uploaded')).json();
    if (
      resumedFile.event_id !== publishedFile.event_id ||
      resumedFile.room_id !== publishedFile.room_id
    )
      throw new Error('reload duplicated file publication');
    state.stage = 'forget-encrypted-cache';
    const deviceLock = matrixStorePrefix(fixture.bootstrap, fixture.session.device_id);
    if (!(await navigator.locks.query()).held?.some((lock) => lock.name === deviceLock))
      throw new Error('missing device lease');
    const forgetting = suite.forget();
    await erasing;
    let protectedErase = false;
    try {
      await navigator.locks.request(deviceLock, { ifAvailable: true }, (lock) => {
        protectedErase = lock === null;
      });
    } finally {
      finishErase();
      await forgetting;
    }
    if (!protectedErase) throw new Error('device reopened while credential erasure was pending');
    await navigator.locks.request(deviceLock, { signal: AbortSignal.timeout(5000) }, () => {});
    state.diagnostic = {
      ...state.diagnostic,
      leaseHeldDuringErase: true,
      leaseReleasedAfterErase: true,
    };
    const after = await new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open('sytue-offline');
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(new Error('offline cleanup inspection'));
    });
    try {
      const count = await new Promise<number>((resolve, reject) => {
        const request = after.transaction('records', 'readonly').objectStore('records').count();
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(new Error('offline cleanup count'));
      });
      if (count !== 0) throw new Error('owner cache retained after forget');
    } finally {
      after.close();
    }
    state.stage = 'complete-reloaded';
    state.ok = true;
    return;
  }
  state.stage = 'token-only';
  let denied = false;
  try {
    await records.readRecord(fixture.reference, fixture.record.object_id, 'content');
  } catch {
    denied = true;
  }
  if (!denied) throw new Error('token-only recovery bypass');
  state.stage = 'invalid-recovery';
  let invalidDenied = false;
  try {
    await suite.recover();
  } catch {
    invalidDenied = true;
  }
  if (!invalidDenied || (await suite.trusted()))
    throw new Error('invalid key unlocked private storage');
  recoveryKey = fixture.recovery;
  state.stage = 'recover';
  try {
    await suite.recover();
  } catch (failure) {
    const crypto = suite.client.getCrypto()!;
    const backup = await crypto.checkKeyBackupAndEnable();
    state.diagnostic = {
      device: await crypto.getDeviceVerificationStatus(
        fixture.session.user_id,
        fixture.session.device_id,
      ),
      crossSigning: await crypto.getCrossSigningStatus(),
      crossSigningReady: await crypto.isCrossSigningReady(),
      secretStorageReady: await crypto.isSecretStorageReady(),
      backupTrusted: backup?.trustInfo.trusted,
    };
    throw failure;
  }
  state.stage = 'trusted';
  if (!(await suite.trusted())) throw new Error('recovered device remains untrusted');
  if (fixture.bootstrap.storage_identity_version === 2) {
    state.stage = 'signed-root-consent';
    const absent = new Error('synthetic missing authority');
    await suite.ensureStorageAuthority(
      {
        read: async () => {
          throw absent;
        },
        enrollmentRequired: (error) => error === absent,
        challenge: async (epoch, kind, request) => {
          rootRequest = request;
          const intent = await storageControlIntent(request, epoch, kind),
            now = Date.now();
          return {
            request,
            action: {
              v: 1,
              ...intent,
              root_key: request.root_key,
              nonce: 'browser_fixture_consent',
              issued_at: now,
              expires_at: now + 60_000,
            },
          };
        },
        confirm: async (proof) => {
          rootProof = proof;
          return {
            v: 1,
            scope: proof.scope,
            user_id: fixture.session.user_id,
            root_key: proof.root_key,
            generation: proof.authority_generation,
            status: 'active',
          };
        },
      },
      new AbortController().signal,
    );
    if (rootApprovals !== 1 || !rootProof || !rootRequest)
      throw new Error('browser root consent was not signed');
  }
  state.stage = 'diagnostic';
  const raw = await suite.client.fetchRoomEvent(
    fixture.reference.room_id,
    fixture.reference.event_id,
  );
  const event = new MatrixEvent({ ...raw, room_id: fixture.reference.room_id });
  await suite.client.decryptEventIfNeeded(event);
  state.diagnostic = {
    decryptionFailure: event.isDecryptionFailure(),
    shield: await suite.client.getCrypto()!.getEncryptionInfoForEvent(event),
  };
  const nativeContent = event.getContent();
  if (
    !nativeContent['org.sytue.storage_pq'] ||
    nativeContent['org.sytue.storage'] !== undefined ||
    JSON.stringify(nativeContent).includes(fixture.record.payload)
  )
    throw new Error('native transport exposed the inner private record');
  state.stage = 'backup-trust-boundary';
  const shield = await suite.client.getCrypto()!.getEncryptionInfoForEvent(event);
  if (!shield || shield.shieldReason !== EventShieldReason.AUTHENTICITY_NOT_GUARANTEED)
    throw new Error('unexpected backup trust boundary; review maintained SDK policy');
  state.stage = 'owner-attested-recovery';
  const recovered = await records.readRecord(
    fixture.reference,
    fixture.record.object_id,
    'content',
  );
  if (recovered.payload !== fixture.record.payload)
    throw new Error('owner-attested native bytes changed');
  const catalog = await records.catalog();
  if (catalog.length !== 1 || catalog[0]?.reference.event_id !== fixture.catalog.reference.event_id)
    throw new Error('authenticated catalog changed');
  state.stage = 'browser-write';
  // High entropy forces the shared fragment format near the historical limit;
  // domain JSON, including its integer above 2^53, remains exact original bytes.
  // Muestreo por rechazo (sin sesgo de módulo): ver lib/texto-aleatorio.ts.
  const noise = textoAleatorio(
    29000,
    'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789',
  );
  const payload = new TextEncoder().encode(
    '{"text":"artificial private browser to native","integer":9007199254740995,"noise":"' +
      noise +
      '"}',
  );
  const created = createStorageRecord(
    records.scope,
    fixture.record.object_id,
    'browser-revision',
    'content',
    payload,
  );
  const policyController = new AbortController();
  const policyTransactions = new Set<string>();
  const authorizedRecords = records.withPublicationPolicy({
    target: records.publicationTarget(),
    signal: policyController.signal,
    assertOpen: () => policyController.signal.throwIfAborted(),
    authorize: async (transaction, target, signal) => {
      signal.throwIfAborted();
      policyController.signal.throwIfAborted();
      if (target.room_id !== records.publicationTarget().room_id)
        throw new Error('publication escaped original target');
      policyTransactions.add(transaction);
    },
  });
  const reference = await authorizedRecords.putRecord('artificial-browser-content', created);
  if (policyTransactions.size < 3)
    throw new Error('fragment/root policy was not checked for each SDK HTTP transaction');
  policyController.abort();
  let retiredPolicyDenied = false;
  try {
    await authorizedRecords.putRecord('retired-policy', created);
  } catch {
    retiredPolicyDenied = true;
  }
  if (!retiredPolicyDenied) throw new Error('retired publication view remained writable');
  state.stage = 'browser-readback';
  const readback = await records.readRecord(reference, fixture.record.object_id, 'content');
  if (
    new TextDecoder().decode(storageJSONBytes(readback.payload)) !==
    new TextDecoder().decode(payload)
  )
    throw new Error('browser bytes changed');
  const acceptedPacket = new MatrixEvent({
    ...(await suite.client.fetchRoomEvent(reference.room_id, reference.event_id)),
    room_id: reference.room_id,
  });
  await suite.client.decryptEventIfNeeded(acceptedPacket);
  if (acceptedPacket.getContent()['org.sytue.storage_pq']?.kind !== 'fragment-root')
    throw new Error('large browser record did not exercise the common fragment format');
  state.stage = 'browser-file-upload';
  const fileReference = await records
    .uploads()
    .upload('browser-file-job', fileRequest, originalFile);
  const fileBytes = await records
    .files()
    .readRange(fileReference, fileRequest.object_id, fileRequest.revision_id, 0, 73);
  if (fileBytes.length !== 73 || fileBytes.some((byte) => byte !== 9))
    throw new Error('browser upload bytes changed');
  fileBytes.fill(0);
  state.stage = 'shared-go-storage-port';
  const coreCopy = await verifyCoreStoragePort(
    records,
    suite.clientCoreRuntime(),
    fileReference,
    fileRequest,
    {
      reference: fixture.reference,
      object: fixture.record.object_id,
      payload: fixture.record.payload,
    },
  );
  const browserIngressPlan = fixture.core_ingress
    ? await verifyCoreIngressPort(records, suite.clientCoreRuntime(), suite, fixture.core_ingress)
    : undefined;
  state.stage = 'anonymous-public-backup-forgery-denied';
  const anonymous = await (await fetch('/inject/forged', { method: 'POST' })).json();
  const forgedReference = {
    room_id: fixture.reference.room_id,
    event_id: anonymous.event_id,
  };
  const forgedEvent = new MatrixEvent({
    ...(await suite.client.fetchRoomEvent(forgedReference.room_id, forgedReference.event_id)),
    room_id: forgedReference.room_id,
  });
  await suite.client.decryptEventIfNeeded(forgedEvent);
  if (
    forgedEvent.isDecryptionFailure() ||
    forgedEvent.getContent()['org.sytue.storage']?.payload !== fixture.forged_record.payload
  )
    throw new Error('public-only backup fixture was not decryptable; review SDK trust boundary');
  let anonymousDenied = false;
  try {
    await records.readRecord(forgedReference, fixture.record.object_id, 'content');
  } catch (error) {
    anonymousDenied = error instanceof Error && error.message === 'matrix_unverified';
  }
  if (!anonymousDenied) throw new Error('anonymous backup writer forged owner attestation');
  state.stage = 'unsealed-backup-denied';
  const unsealed = await (await fetch('/inject/legacy', { method: 'POST' })).json();
  const legacyReference = {
    room_id: fixture.reference.room_id,
    event_id: unsealed.event_id,
  };
  let historyDenied = false;
  try {
    await records.readRecord(legacyReference, fixture.record.object_id, 'content');
  } catch (error) {
    historyDenied = error instanceof Error && error.message === 'matrix_unverified';
  }
  if (!historyDenied)
    throw new Error('unsealed legacy backup was accepted as owner-authenticated history');
  let catalogDenied = false;
  try {
    await records.catalog();
  } catch (error) {
    catalogDenied = error instanceof Error && error.message === 'matrix_unverified';
  }
  if (!catalogDenied) throw new Error('untrusted legacy history silently accepted by catalog');
  state.stage = 'invalid-owner-proof-denied';
  const forged = { ...created, payload: btoa('{"forged":true}') };
  const invalid = await suite.client.sendEvent(
    fixture.reference.room_id,
    EventType.RoomMessage,
    {
      msgtype: 'm.text',
      body: '',
      'org.sytue.storage': forged,
      'org.sytue.storage_auth': forgedEvent.getContent()['org.sytue.storage_auth'],
    } as RoomMessageEventContent,
    'artificial-invalid-owner-proof',
  );
  const invalidReference = {
    room_id: fixture.reference.room_id,
    event_id: invalid.event_id,
  };
  let invalidProofDenied = false;
  try {
    await records.readRecord(invalidReference, fixture.record.object_id, 'content');
  } catch (error) {
    invalidProofDenied = error instanceof Error && error.message === 'matrix_unverified';
  }
  if (!invalidProofDenied) throw new Error('verified SDK sender bypassed invalid owner proof');
  await fetch('/result', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      core_ingress_plan: browserIngressPlan,
      root_proof: rootProof,
      root_request: rootRequest,
      reference,
      payload: created.payload,
      legacy_reference: legacyReference,
      invalid_reference: invalidReference,
      forged_reference: forgedReference,
      core_copy_reference: coreCopy.reference,
      core_copy_request: coreCopy.request,
      file_reference: fileReference,
      file_request: fileRequest,
    }),
  });
  state.stage = 'complete';
  state.ok = true;
})()
  .catch((error: unknown) => {
    state.reason =
      error instanceof Error
        ? error.name + ('reason' in error ? ':' + String(error.reason) : ':' + error.message)
        : 'unknown';
    state.stage = 'failed:' + state.stage;
  })
  .finally(async () => {
    await suite?.close();
  });
