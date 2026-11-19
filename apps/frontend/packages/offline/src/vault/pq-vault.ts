/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Bóveda post-cuántica directa (ADR 0086).
 *
 * Sustituye a Synapse/Matrix para el almacenamiento cifrado de extremo a extremo
 * de las apps de la suite (todo salvo Rooms, que mantiene Matrix para chat en
 * tiempo real):
 *
 * - Criptografía híbrida post-cuántica (`age-MLKEM768-X25519-v1`, ML-KEM-768 +
 *   X25519 + ChaCha20-Poly1305) ejecutada en el núcleo Go/Wasm (`pqc-wasm`), sin
 *   implementar primitivas en JS ni cargar `matrix-js-sdk` ni RustCrypto WASM.
 * - La identidad PQ y la clave de propietario (`owner_key`) se envuelven en el
 *   puente de Sytue ID (`id.<dominio>`) con la clave de desbloqueo de la cuenta
 *   (ADR 0084/0085), con separación de dominio (`sytue-pq-vault-wrap-v1`).
 * - Los registros cifrados y los envoltorios públicos viven en `services/drive`
 *   (`/api/pq-vault/*`) bajo aislamiento RLS por `(tenant, owner_sub, product)`.
 */
import {
  encodeStorageScopeHeader,
  storageScopeHeader,
  type MatrixStorageProduct,
  type MatrixStorageScope,
} from '@sytue/config/matrix';
import { EncryptedOfflineValues, type EncryptedOfflinePort } from '../encrypted-storage.js';
import { PqcRuntime, type PqcIdentityMaterial } from '../pqc/runtime.js';
import { StorageJobs } from '../matrix/storage-jobs.js';
import { StoragePq, type StoragePqKeySource } from '../matrix/storage-pq.js';
import { StorageRepository, type StorageRecordTransport } from '../matrix/storage-repository.js';
import type { StorageBlobPort } from '../matrix/storage-files.js';
import {
  decodeStorageRecord,
  StorageRecordError,
  type StorageRecord,
  type StorageRecordEntry,
} from '../matrix/storage.js';
import { MatrixSecurityError, type MatrixReference } from '../matrix/types.js';
import {
  pqVaultProfile,
  unwrapVaultInBridge,
  wrapVaultInBridge,
  type PqVaultSecret,
  type PqVaultWrapper,
  type UnlockBridgeOptions,
} from '../matrix/unlock-bridge.js';

export type PqVaultReason =
  'signin' | 'wrong' | 'identity' | 'closed' | 'account' | 'busy' | 'unavailable';

export class PqVaultError extends Error {
  constructor(
    readonly reason: PqVaultReason,
    message?: string,
  ) {
    super(message ?? `pq_vault_${reason}`);
    this.name = 'PqVaultError';
  }
}

/** Señal de despliegue con `SYTUE_CIFRADO_E2E=desactivado` (sin cifrado E2E). */
export class PqVaultDisabledError extends Error {
  constructor() {
    super('pq_vault_disabled');
    this.name = 'PqVaultDisabledError';
  }
}

export interface PqVaultBootstrap {
  actor: string;
  tenant: string;
  product: MatrixStorageProduct;
  id_origin: string;
  encryption: typeof pqVaultProfile;
}

export interface PqVaultStateResponse {
  vault_id: string;
  active_key_id: string;
  keys: PqVaultWrapper[];
}

interface PqVaultRecordResponse {
  event_id: string;
  vault_id: string;
  seq: number;
  payload: Record<string, unknown>;
}

interface PqVaultRecordPageResponse {
  entries: PqVaultRecordResponse[];
  next_cursor?: string;
}

interface PqOwnerProof {
  v: 1;
  alg: 'hmac-sha256';
  key_id: string;
  mac: string;
}

export interface PqVaultBridgePort {
  unwrap: typeof unwrapVaultInBridge;
  wrap: typeof wrapVaultInBridge;
}

export interface PqVaultOpenOptions {
  actor: string;
  tenant?: string | undefined;
  product: MatrixStorageProduct;
  /** Prefijo HTTP de la app (p. ej. `""` o `"/drive"`). Por defecto `""`. */
  basePath?: string | undefined;
  signal?: AbortSignal | undefined;
  fetchImpl?: typeof fetch | undefined;
  bridge?: PqVaultBridgePort | undefined;
  bridgeOptions?: Partial<UnlockBridgeOptions> | undefined;
  /** Sustituible en pruebas para evitar instanciar un Web Worker real. */
  createCore?: ((lease: { signal: AbortSignal; assertOpen: () => void }) => PqcRuntime) | undefined;
  offlinePort?: EncryptedOfflinePort | undefined;
}

const encoder = new TextEncoder();
const decoder = new TextDecoder('utf-8', { fatal: true });
const ownerProofDomain = 'org.sytue.storage.auth.v1';
const vaultProofPrefix = 'sytue-pq-vault-proof-v1\0';

function toBase64(bytes: Uint8Array): string {
  let out = '';
  for (let i = 0; i < bytes.length; i++) out += String.fromCharCode(bytes[i]!);
  return btoa(out);
}

function fromBase64(value: string): Uint8Array<ArrayBuffer> {
  const bin = atob(value);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function normalizeBase(basePath: string | undefined): string {
  if (!basePath || basePath === '/') return '';
  return basePath.replace(/\/+$/, '');
}

async function computeRecordDigest(
  vaultId: string,
  input: StorageRecord,
  scope: MatrixStorageScope,
): Promise<string> {
  const record = decodeStorageRecord(input, scope);
  const tuple = [
    ownerProofDomain,
    ...[
      vaultId,
      record.scope.tenant,
      record.scope.subject,
      record.scope.product,
      record.object_id,
      record.revision_id,
      record.role,
    ].map((value) => toBase64(encoder.encode(value))),
    record.payload,
    ...(record.previous ?? []).map((value) => toBase64(encoder.encode(value))),
  ];
  const digest = new Uint8Array(
    await crypto.subtle.digest('SHA-256', encoder.encode(JSON.stringify(tuple))),
  );
  return Array.from(digest, (byte) => byte.toString(16).padStart(2, '0')).join('');
}

async function importHmacKey(ownerKeyB64: string): Promise<CryptoKey> {
  const raw = fromBase64(ownerKeyB64);
  try {
    if (raw.byteLength !== 32) throw new PqVaultError('identity');
    return await crypto.subtle.importKey('raw', raw, { name: 'HMAC', hash: 'SHA-256' }, false, [
      'sign',
      'verify',
    ]);
  } finally {
    raw.fill(0);
  }
}

/** Pide los metadatos de sesión + origen de Sytue ID al BFF de la app. */
export async function fetchPqVaultBootstrap(options: {
  basePath?: string | undefined;
  product: MatrixStorageProduct;
  actor: string;
  tenant?: string | undefined;
  signal?: AbortSignal | undefined;
  fetchImpl?: typeof fetch | undefined;
}): Promise<PqVaultBootstrap> {
  const f = options.fetchImpl ?? fetch;
  const base = normalizeBase(options.basePath);
  const response = await f(`${base}/api/pq-vault/bootstrap`, {
    method: 'GET',
    credentials: 'same-origin',
    ...(options.signal ? { signal: options.signal } : {}),
  });
  if (!response.ok) throw new PqVaultError('unavailable');
  const data = (await response.json()) as Record<string, unknown>;
  if (data.cifrado === 'desactivado') throw new PqVaultDisabledError();
  if (
    typeof data.actor !== 'string' ||
    typeof data.tenant !== 'string' ||
    data.actor !== options.actor ||
    (options.tenant !== undefined && data.tenant !== options.tenant) ||
    data.product !== options.product ||
    typeof data.id_origin !== 'string' ||
    data.encryption !== pqVaultProfile
  ) {
    throw new PqVaultError('identity');
  }
  return {
    actor: data.actor,
    tenant: data.tenant,
    product: options.product,
    id_origin: data.id_origin,
    encryption: pqVaultProfile,
  };
}

/**
 * Sesión abierta de bóveda post-cuántica directa para `(tenant, subject, product)`.
 *
 * Mantiene en memoria la identidad híbrida `age-MLKEM768-X25519-v1` y la clave
 * HMAC `ownerKey` (no extraíble) durante la vida de la pestaña, y borra todo el
 * material privado al cerrarse (`close()` / `closeAndDrain()`).
 */
export class PqVault {
  readonly scope: Readonly<MatrixStorageScope>;
  readonly profile = pqVaultProfile;
  readonly idOrigin: string;
  readonly vaultId: string;
  readonly keyId: string;
  readonly recipient: string;
  private readonly base: string;
  private readonly fetchImpl: typeof fetch;
  private readonly core: PqcRuntime;
  private readonly lifetime: AbortController;
  private readonly identityBytes: Uint8Array;
  private readonly hmacKey: CryptoKey;
  private readonly jobs = new StorageJobs();
  private readonly storagePq: StoragePq;
  private readonly uploadCache: EncryptedOfflineValues;
  private exclusiveQueue: Promise<void> = Promise.resolve();
  private closed = false;

  private constructor(params: {
    scope: MatrixStorageScope;
    idOrigin: string;
    vaultId: string;
    keyId: string;
    recipient: string;
    base: string;
    fetchImpl: typeof fetch;
    core: PqcRuntime;
    lifetime: AbortController;
    identityBytes: Uint8Array;
    hmacKey: CryptoKey;
    offlinePort?: EncryptedOfflinePort | undefined;
  }) {
    this.scope = Object.freeze({ ...params.scope });
    this.idOrigin = params.idOrigin;
    this.vaultId = params.vaultId;
    this.keyId = params.keyId;
    this.recipient = params.recipient;
    this.base = params.base;
    this.fetchImpl = params.fetchImpl;
    this.core = params.core;
    this.lifetime = params.lifetime;
    this.identityBytes = params.identityBytes;
    this.hmacKey = params.hmacKey;
    const assertOpen = (): void => {
      if (this.closed || this.lifetime.signal.aborted) throw new MatrixSecurityError('device');
    };
    const keySource: StoragePqKeySource = {
      prepare: () => {
        assertOpen();
        return Promise.resolve();
      },
      key: (requested = ''): Promise<PqcIdentityMaterial> => {
        assertOpen();
        if (requested && requested !== this.keyId)
          return Promise.reject(new MatrixSecurityError('recovery'));
        return Promise.resolve({
          profile: this.profile,
          key_id: this.keyId,
          recipient: this.recipient,
          identity: this.identityBytes.slice(),
        });
      },
      close: () => {
        this.identityBytes.fill(0);
      },
      destroy: () => {
        this.identityBytes.fill(0);
        return Promise.resolve();
      },
    };
    this.storagePq = new StoragePq(this.core, keySource, this.scope, assertOpen);
    this.uploadCache = new EncryptedOfflineValues(
      JSON.stringify([this.scope.tenant, this.scope.subject, this.scope.product, this.vaultId]),
      'pq-vault-upload-v1',
      () => !this.closed,
      params.offlinePort,
    );
  }

  get isClosed(): boolean {
    return this.closed;
  }

  /** Abre la bóveda PQ del usuario, creando e inscribiendo su identidad en el primer uso. */
  static async open(options: PqVaultOpenOptions): Promise<PqVault> {
    return PqVault.openInternal(options, false);
  }

  /** Genera una identidad PQ nueva y reinicia la bóveda cuando se perdió la AUK anterior. */
  static async reset(options: PqVaultOpenOptions): Promise<PqVault> {
    return PqVault.openInternal(options, true);
  }

  private static async openInternal(
    options: PqVaultOpenOptions,
    forceReset: boolean,
  ): Promise<PqVault> {
    const f = options.fetchImpl ?? fetch;
    const base = normalizeBase(options.basePath);
    const bridge: PqVaultBridgePort = options.bridge ?? {
      unwrap: unwrapVaultInBridge,
      wrap: wrapVaultInBridge,
    };
    options.signal?.throwIfAborted();
    const bootstrap = await fetchPqVaultBootstrap({
      basePath: base,
      product: options.product,
      actor: options.actor,
      tenant: options.tenant,
      signal: options.signal,
      fetchImpl: f,
    });
    options.signal?.throwIfAborted();

    const scope: MatrixStorageScope = {
      tenant: bootstrap.tenant,
      subject: bootstrap.actor,
      product: bootstrap.product,
    };
    const scopeHeader = encodeStorageScopeHeader(scope);
    const lifetime = new AbortController();
    const core = options.createCore
      ? options.createCore({
          signal: lifetime.signal,
          assertOpen: () => {
            if (lifetime.signal.aborted) throw new PqVaultError('closed');
          },
        })
      : new PqcRuntime({
          signal: lifetime.signal,
          assertOpen: () => {
            if (lifetime.signal.aborted) throw new PqVaultError('closed');
          },
        });

    let identityBytes: Uint8Array | undefined;
    try {
      const bridgeOpts: UnlockBridgeOptions = {
        ...options.bridgeOptions,
        idOrigin: bootstrap.id_origin,
        subject: scope.subject,
      };
      const keysRes = await f(`${base}/api/pq-vault/keys`, {
        method: 'GET',
        credentials: 'same-origin',
        headers: { [storageScopeHeader]: scopeHeader },
        ...(options.signal ? { signal: options.signal } : {}),
      });
      if (!keysRes.ok) throw new PqVaultError('unavailable');
      const state = (await keysRes.json()) as PqVaultStateResponse;
      options.signal?.throwIfAborted();

      let vaultId = state.vault_id;
      let keyId: string;
      let recipient: string;
      let secret: PqVaultSecret;

      if (!forceReset && Array.isArray(state.keys) && state.keys.length > 0 && state.vault_id) {
        // Priorizar la clave activa si aparece en la lista.
        const ordered = [...state.keys].sort((a, b) =>
          a.key_id === state.active_key_id ? -1 : b.key_id === state.active_key_id ? 1 : 0,
        );
        const opened = await bridge.unwrap(bridgeOpts, {
          tenant: scope.tenant,
          product: scope.product,
          wrappers: ordered,
        });
        options.signal?.throwIfAborted();
        if (!opened.ok) throw new PqVaultError(opened.error);
        secret = opened.secret;
        identityBytes = encoder.encode(secret.identity);
        // Verificar en el núcleo Go/Wasm que la identidad privada coincide con key_id y recipient.
        const pub = await core.invoke<{ profile: string; recipient: string; key_id: string }>({
          operation: 'identity.public',
          identity: identityBytes.slice(),
        });
        if (
          pub.profile !== pqVaultProfile ||
          pub.key_id !== opened.keyID ||
          pub.recipient !== opened.recipient
        ) {
          throw new PqVaultError('identity');
        }
        keyId = pub.key_id;
        recipient = pub.recipient;
      } else {
        // Primer uso o reinicio explícito: generar identidad ML-KEM-768 + X25519 en Go/Wasm.
        const created = await core.invoke<{
          profile: string;
          recipient: string;
          key_id: string;
          identity: Uint8Array;
        }>({ operation: 'identity.create' });
        options.signal?.throwIfAborted();
        if (created.profile !== pqVaultProfile || !(created.identity instanceof Uint8Array)) {
          throw new PqVaultError('unavailable');
        }
        identityBytes = created.identity.slice();
        created.identity.fill(0);
        const ownerRaw = crypto.getRandomValues(new Uint8Array(32));
        const ownerB64 = toBase64(ownerRaw);
        ownerRaw.fill(0);
        secret = {
          v: 1,
          profile: pqVaultProfile,
          identity: decoder.decode(identityBytes),
          owner_key: ownerB64,
        };
        const wrapped = await bridge.wrap(bridgeOpts, {
          tenant: scope.tenant,
          product: scope.product,
          profile: pqVaultProfile,
          keyID: created.key_id,
          recipient: created.recipient,
          secret,
        });
        options.signal?.throwIfAborted();
        if (!wrapped.ok) throw new PqVaultError(wrapped.error);
        const putRes = await f(`${base}/api/pq-vault/keys`, {
          method: 'PUT',
          credentials: 'same-origin',
          headers: {
            'content-type': 'application/json',
            [storageScopeHeader]: scopeHeader,
          },
          body: JSON.stringify({
            wrapper: wrapped.wrapper,
            set_active: true,
            reset: forceReset,
          }),
          ...(options.signal ? { signal: options.signal } : {}),
        });
        if (!putRes.ok) throw new PqVaultError('unavailable');
        const saved = (await putRes.json()) as PqVaultStateResponse;
        if (!saved.vault_id) throw new PqVaultError('unavailable');
        vaultId = saved.vault_id;
        keyId = created.key_id;
        recipient = created.recipient;
      }

      const hmacKey = await importHmacKey(secret.owner_key);
      options.signal?.throwIfAborted();
      const keptIdentity = identityBytes;
      identityBytes = undefined;
      return new PqVault({
        scope,
        idOrigin: bootstrap.id_origin,
        vaultId,
        keyId,
        recipient,
        base,
        fetchImpl: f,
        core,
        lifetime,
        identityBytes: keptIdentity,
        hmacKey,
        offlinePort: options.offlinePort,
      });
    } catch (error) {
      identityBytes?.fill(0);
      lifetime.abort();
      core.close();
      throw error;
    }
  }

  /**
   * Repositorio completo (`StorageRepository`) respaldado por `age-MLKEM768-X25519-v1`
   * directo sobre `/api/pq-vault/records`, sin pasar por Synapse/Matrix.
   */
  storage(blobs?: StorageBlobPort): StorageRepository {
    const assertOpen = (): void => {
      if (this.closed || this.lifetime.signal.aborted) throw new MatrixSecurityError('device');
    };
    assertOpen();
    const scopeHeader = encodeStorageScopeHeader(this.scope);

    const signProof = async (room: string, record: StorageRecord): Promise<Uint8Array> => {
      const digest = await computeRecordDigest(room, record, this.scope);
      const mac = new Uint8Array(
        await crypto.subtle.sign('HMAC', this.hmacKey, encoder.encode(vaultProofPrefix + digest)),
      );
      const proof: PqOwnerProof = {
        v: 1,
        alg: 'hmac-sha256',
        key_id: this.keyId,
        mac: toBase64(mac),
      };
      return encoder.encode(JSON.stringify(proof));
    };

    const verifyProof = async (
      room: string,
      record: StorageRecord,
      raw: unknown,
    ): Promise<void> => {
      if (!raw || typeof raw !== 'object' || Array.isArray(raw))
        throw new MatrixSecurityError('unverified');
      const proof = raw as Record<string, unknown>;
      if (
        proof.v !== 1 ||
        proof.alg !== 'hmac-sha256' ||
        typeof proof.key_id !== 'string' ||
        typeof proof.mac !== 'string'
      )
        throw new MatrixSecurityError('unverified');
      const digest = await computeRecordDigest(room, record, this.scope);
      const macBytes = fromBase64(proof.mac);
      const ok = await crypto.subtle.verify(
        'HMAC',
        this.hmacKey,
        macBytes,
        encoder.encode(vaultProofPrefix + digest),
      );
      if (!ok) throw new MatrixSecurityError('unverified');
    };

    const sendRaw = async (
      room: string,
      transaction: string,
      content: Record<string, unknown>,
      signal?: AbortSignal,
    ): Promise<MatrixReference> => {
      assertOpen();
      const res = await this.fetchImpl(`${this.base}/api/pq-vault/records`, {
        method: 'POST',
        credentials: 'same-origin',
        headers: {
          'content-type': 'application/json',
          [storageScopeHeader]: scopeHeader,
        },
        body: JSON.stringify({
          vault_id: room,
          transaction_id: transaction,
          payload: content,
        }),
        ...(signal ? { signal } : {}),
      });
      if (!res.ok) throw new StorageRecordError();
      const saved = (await res.json()) as PqVaultRecordResponse;
      if (saved.vault_id !== room || typeof saved.event_id !== 'string')
        throw new MatrixSecurityError('reference');
      return { room_id: saved.vault_id, event_id: saved.event_id };
    };

    const readRaw = async (reference: MatrixReference): Promise<Record<string, unknown>> => {
      assertOpen();
      const params = new URLSearchParams({ vault_id: reference.room_id });
      const res = await this.fetchImpl(
        `${this.base}/api/pq-vault/records/${encodeURIComponent(reference.event_id)}?${params.toString()}`,
        {
          method: 'GET',
          credentials: 'same-origin',
          headers: { [storageScopeHeader]: scopeHeader },
        },
      );
      if (!res.ok) throw new StorageRecordError();
      const rec = (await res.json()) as PqVaultRecordResponse;
      if (
        rec.vault_id !== reference.room_id ||
        rec.event_id !== reference.event_id ||
        !rec.payload ||
        typeof rec.payload !== 'object'
      )
        throw new MatrixSecurityError('reference');
      return rec.payload;
    };

    const publication = Object.freeze({
      room_id: this.vaultId,
      device_id: `pq-${this.keyId.slice(0, 24)}`,
    });

    const runExclusive = <T>(
      action: (signal: AbortSignal) => Promise<T>,
      parent?: AbortSignal,
    ): Promise<T> => {
      assertOpen();
      parent?.throwIfAborted();
      const prev = this.exclusiveQueue;
      let release!: () => void;
      this.exclusiveQueue = new Promise<void>((resolve) => {
        release = resolve;
      });
      return this.jobs.run(
        'exclusive',
        async (child) => {
          try {
            await prev;
            child.throwIfAborted();
            assertOpen();
            return await action(child);
          } finally {
            release();
          }
        },
        parent,
      );
    };

    const transport: StorageRecordTransport = {
      rooms: [this.vaultId],
      publication,
      assertOpen,
      prepare: (signal) => {
        signal.throwIfAborted();
        assertOpen();
        return Promise.resolve();
      },
      recoverable: (target, signal) => {
        signal.throwIfAborted();
        assertOpen();
        if (target.room_id !== this.vaultId || target.device_id !== publication.device_id)
          return Promise.reject(new MatrixSecurityError('room'));
        return Promise.resolve();
      },
      exclusive: runExclusive,
      run: (name, task, parent) => this.jobs.run(name, task, parent),
      put: async (transaction, record, target, signal) => {
        assertOpen();
        const room = target?.room_id ?? this.vaultId;
        if (room !== this.vaultId) throw new MatrixSecurityError('room');
        const proof = await signProof(room, record);
        try {
          return await this.storagePq.write(
            room,
            transaction,
            record,
            proof,
            (tx, content) => sendRaw(room, tx, content, signal),
            readRaw,
          );
        } finally {
          proof.fill(0);
        }
      },
      putAuthorized: async (transaction, record, target, policy, signal) => {
        assertOpen();
        if (target.room_id !== this.vaultId || target.device_id !== publication.device_id)
          throw new MatrixSecurityError('room');
        const proof = await signProof(this.vaultId, record);
        try {
          return await this.storagePq.write(
            this.vaultId,
            transaction,
            record,
            proof,
            async (tx, content) => {
              const current = signal ?? policy.signal;
              policy.assertOpen();
              await policy.authorize(tx, target, current);
              return sendRaw(this.vaultId, tx, content, current);
            },
            readRaw,
          );
        } finally {
          proof.fill(0);
        }
      },
      read: async (reference) => {
        assertOpen();
        if (reference.room_id !== this.vaultId) throw new MatrixSecurityError('room');
        const payload = await readRaw(reference);
        const opened = await this.storagePq.open(payload, reference, readRaw);
        if (!opened || 'fragment' in opened) throw new StorageRecordError();
        await verifyProof(reference.room_id, opened.record, opened.proof);
        return opened.record;
      },
      history: async (room, cursor, limit) => {
        assertOpen();
        if (room !== this.vaultId) throw new MatrixSecurityError('room');
        const params = new URLSearchParams({
          vault_id: room,
          limit: String(limit),
        });
        if (cursor) params.set('cursor', cursor);
        const res = await this.fetchImpl(`${this.base}/api/pq-vault/records?${params.toString()}`, {
          method: 'GET',
          credentials: 'same-origin',
          headers: { [storageScopeHeader]: scopeHeader },
        });
        if (!res.ok) throw new StorageRecordError();
        const page = (await res.json()) as PqVaultRecordPageResponse;
        const rawEntries = Array.isArray(page.entries) ? page.entries : [];
        const entries: StorageRecordEntry[] = [];
        for (const item of rawEntries) {
          const reference: MatrixReference = {
            room_id: item.vault_id,
            event_id: item.event_id,
          };
          const opened = await this.storagePq.open(item.payload, reference, readRaw);
          if (!opened || 'fragment' in opened) continue;
          await verifyProof(reference.room_id, opened.record, opened.proof);
          entries.push({ reference, record: opened.record });
        }
        return {
          entries,
          scanned: rawEntries.length,
          next: page.next_cursor || null,
        };
      },
    };

    return new StorageRepository(this.scope, transport, blobs, this.uploadCache);
  }

  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.lifetime.abort();
    void this.jobs.close();
    this.identityBytes.fill(0);
    this.storagePq.close();
    this.uploadCache.close();
    this.core.close();
  }

  async closeAndDrain(): Promise<void> {
    this.close();
    await Promise.all([this.jobs.close(), this.core.closeAndDrain()]);
  }
}
