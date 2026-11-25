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
import { vaultCoreModule } from '@sytue/config/client-core';
import { PqcRuntime, prewarmClientCore, type PqcIdentityMaterial } from '../pqc/runtime.js';
import { StorageJobs } from '../matrix/storage-jobs.js';
import { StoragePq, type StoragePqKeySource } from '../matrix/storage-pq.js';
import { StorageRepository, type StorageRecordTransport } from '../matrix/storage-repository.js';
import type { StorageBlobPort } from '../matrix/storage-files.js';
import {
  createStorageRecord,
  decodeStorageRecord,
  storageJSONBytes,
  StorageRecordError,
  type StorageRecord,
  type StorageRecordEntry,
  type StoragePublicationTarget,
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

/**
 * `copy`: otro dispositivo está pasando el Drive de Matrix a la bóveda (tiene su
 * marca de copia en curso) y el servidor no deja publicar ni vaciar.
 * `changed`: se pidió vaciar la bóveda tal como se vio y ya no es esa; no se vació.
 */
export type PqVaultReason =
  | 'signin'
  /**
   * Sytue ID pide una identificación reciente (`vault_key_reauth`, ADR 0087): la
   * clave de firma de esta bóveda sustituye a otra y eso solo vale tras volver a
   * teclear la clave. Un inicio de sesión normal no basta: el relevo entrega el
   * código con la sesión de antes y Sytue ID vuelve a contestar 403.
   */
  | 'reauth'
  | 'wrong'
  | 'identity'
  | 'closed'
  | 'account'
  | 'busy'
  | 'unavailable'
  | 'copy'
  | 'changed'
  /** Con `existing`: la bóveda no existe todavía, y quien abre no puede crearla. */
  | 'missing';

/** Lo que se vio de la bóveda: vaciarla solo vale si sigue así. */
export interface PqVaultExpect {
  vault_id: string;
  version: number;
}

/**
 * La marca «copia en curso» de una bóveda (ADR 0086, «pasa Matrix a ella»): la
 * pone un cliente nativo mientras copia en ella el Drive de Matrix. Mientras está
 * activa, solo él publica o vacía; caduca si no la renueva.
 */
export interface PqVaultCopyMarker {
  id: string;
  holder: string;
  kind: string;
  epoch: string;
  started_at: string;
  expires_at: string;
  active: boolean;
}

export class PqVaultError extends Error {
  /**
   * Qué petición falló y cómo (`signing_put_403_vault_key_proof`), para «Más
   * información» y la consola: sin él, todo fallo de red de la bóveda era el mismo
   * `vault.unavailable`. Solo lleva lo fijo del código: la petición, el estado HTTP
   * y el código de error del servicio; nunca datos de la persona.
   */
  check?: string;
  constructor(
    readonly reason: PqVaultReason,
    message?: string,
    /** Con `wrong`: la bóveda tal como se vio, para vaciar solo esa. */
    readonly expect?: PqVaultExpect,
  ) {
    super(message ?? `pq_vault_${reason}`);
    this.name = 'PqVaultError';
  }
}

const checkToken = /[^a-z0-9_]+/gu;

/** `unavailable` con el detalle de la petición que falló (ver `PqVaultError.check`). */
export function pqVaultUnavailable(...parts: Array<string | number | undefined>): PqVaultError {
  const error = new PqVaultError('unavailable');
  const check = parts
    .filter((p) => p !== undefined && p !== '')
    .map((p) => String(p).toLowerCase().replace(checkToken, '_'))
    .join('_')
    .slice(0, 64);
  if (check) error.check = check;
  return error;
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
  /** Sube con cada registro y cada vaciado (servidores de antes: ausente). */
  version?: number;
  copy?: PqVaultCopyMarker;
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
  /**
   * Traspaso de la autoridad del correo de Matrix a esta bóveda (ADR 0087): la
   * clave de firma se registra con la marca `handover`, que Sytue ID solo acepta
   * con identificación reciente. Solo para `mail`, y solo cuando la persona lo
   * confirma en la puerta de Matrix («vaciar y pasar a la bóveda»).
   */
  handover?: boolean | undefined;
  /**
   * Solo con `reset`: lo que se vio de la bóveda al decidir vaciarla (el `expect`
   * de un PqVaultError `wrong`). Si ya no es esa, `changed` y no se vacía nada.
   */
  expect?: PqVaultExpect | undefined;
  /**
   * Abrir solo una bóveda que ya existe (ADR 0086, «Photos sube a la bóveda de
   * Drive»): la usa una app que consume la bóveda de OTRO producto. Nunca la crea,
   * nunca la reinicia (ni en silencio con la clave equivocada) y nunca vuelve a
   * sellar ni registra su clave de firma: eso es cosa de la app dueña. Sin bóveda,
   * `PqVaultError('missing')`.
   */
  existing?: boolean | undefined;
}

/** Lo que hace falta para hablar con la bóveda sin abrirla. */
export type PqVaultCopyOptions = Pick<
  PqVaultOpenOptions,
  'actor' | 'tenant' | 'product' | 'basePath' | 'signal' | 'fetchImpl'
>;

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
  // Sin regex: `/\/+$/` es cuadrático con muchas barras seguidas.
  let end = basePath.length;
  while (end > 0 && basePath.charCodeAt(end - 1) === 47) end--;
  return basePath.slice(0, end);
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
  if (!response.ok)
    throw pqVaultUnavailable('bootstrap', response.status, await errorCode(response));
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

function seenState(state: PqVaultStateResponse): PqVaultExpect {
  return {
    vault_id: typeof state.vault_id === 'string' ? state.vault_id : '',
    version: typeof state.version === 'number' ? state.version : 0,
  };
}

function copyMarker(state: PqVaultStateResponse): PqVaultCopyMarker | null {
  const copy = state.copy;
  if (!copy || typeof copy !== 'object' || typeof copy.id !== 'string' || !copy.id) return null;
  return copy;
}

/**
 * El código de error de un servicio en el sobre de `platform/pkg/httpx`
 * (`{"error":{"code":…}}`: `services/drive` y Sytue ID), o "". Un `{"code"}` en la
 * raíz (lo que contesta el propio BFF) vale igual.
 */
async function errorCode(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: unknown; code?: unknown } | null;
    const inner = body?.error;
    if (inner && typeof inner === 'object') {
      const code = (inner as { code?: unknown }).code;
      return typeof code === 'string' ? code : '';
    }
    if (typeof inner === 'string') return inner;
    return typeof body?.code === 'string' ? body.code : '';
  } catch {
    return '';
  }
}

/** Los dos rechazos de la bóveda que el cliente distingue; si no, `unavailable`. */
async function pqVaultFailure(res: Response, where = 'keys_put'): Promise<PqVaultError> {
  const code = await errorCode(res);
  if (res.status === 409) {
    if (code === 'pq_vault_copy_in_progress') return new PqVaultError('copy');
    if (code === 'pq_vault_changed') return new PqVaultError('changed');
  }
  return pqVaultUnavailable(where, res.status, code);
}

async function copyRequest(
  options: PqVaultCopyOptions,
): Promise<{ f: typeof fetch; base: string; scopeHeader: string }> {
  const f = options.fetchImpl ?? fetch;
  const base = normalizeBase(options.basePath);
  const bootstrap = await fetchPqVaultBootstrap({ ...options, basePath: base, fetchImpl: f });
  const scopeHeader = encodeStorageScopeHeader({
    tenant: bootstrap.tenant,
    subject: bootstrap.actor,
    product: bootstrap.product,
  });
  return { f, base, scopeHeader };
}

async function readCopy(
  f: typeof fetch,
  base: string,
  scopeHeader: string,
  signal: AbortSignal | undefined,
): Promise<PqVaultCopyMarker | null> {
  const res = await f(`${base}/api/pq-vault/keys`, {
    method: 'GET',
    credentials: 'same-origin',
    headers: { [storageScopeHeader]: scopeHeader },
    ...(signal ? { signal } : {}),
  });
  if (!res.ok) throw pqVaultUnavailable('copy_keys_get', res.status, await errorCode(res));
  return copyMarker((await res.json()) as PqVaultStateResponse);
}

async function releaseCopy(
  f: typeof fetch,
  base: string,
  scopeHeader: string,
  id: string,
  signal: AbortSignal | undefined,
): Promise<void> {
  const params = new URLSearchParams({ id });
  const res = await f(`${base}/api/pq-vault/copy?${params.toString()}`, {
    method: 'DELETE',
    credentials: 'same-origin',
    headers: { [storageScopeHeader]: scopeHeader },
    ...(signal ? { signal } : {}),
  });
  // Otra marca distinta (`changed`) es que la que se quería retirar ya no está.
  if (!res.ok) throw await pqVaultFailure(res, 'copy_delete');
}

/** La marca de copia en curso de la bóveda (o `null`), sin abrirla. */
export async function readPqVaultCopy(
  options: PqVaultCopyOptions,
): Promise<PqVaultCopyMarker | null> {
  const { f, base, scopeHeader } = await copyRequest(options);
  return readCopy(f, base, scopeHeader, options.signal);
}

/**
 * Cancela la marca de copia `id` («Cancelar la copia»). No borra nada: la copia
 * del otro dispositivo se para en su siguiente escritura, lo que ya copió se queda
 * en la bóveda sin recibo y Matrix sigue entero; desde ese dispositivo se repite.
 */
export async function cancelPqVaultCopy(options: PqVaultCopyOptions, id: string): Promise<void> {
  const { f, base, scopeHeader } = await copyRequest(options);
  await releaseCopy(f, base, scopeHeader, id, options.signal);
}

/**
 * ¿La bóveda no guarda ni un registro? Ante cualquier duda —error de red, respuesta
 * rara— contesta que no: un reinicio silencioso solo es aceptable si no hay nada que
 * perder.
 */
async function vaultIsEmpty(
  f: typeof fetch,
  base: string,
  vaultId: string,
  scopeHeader: string,
  signal: AbortSignal | undefined,
): Promise<boolean> {
  try {
    const params = new URLSearchParams({ vault_id: vaultId, limit: '1' });
    const res = await f(`${base}/api/pq-vault/records?${params.toString()}`, {
      method: 'GET',
      credentials: 'same-origin',
      headers: { [storageScopeHeader]: scopeHeader },
      ...(signal ? { signal } : {}),
    });
    if (!res.ok) return false;
    const page = (await res.json()) as PqVaultRecordPageResponse;
    return Array.isArray(page.entries) && page.entries.length === 0 && !page.next_cursor;
  } catch {
    return false;
  }
}

/**
 * La identidad con la que una bóveda aparece ante el correo (ADR 0087): el prefijo
 * aislado del ámbito, como una cuenta de Matrix, y un servidor que no existe en
 * Matrix. Es exactamente Go `vaultproof.UserID`.
 */
export function pqVaultUserID(scope: MatrixStorageScope): string {
  return `@sytue_storage_${scope.product}_${scope.tenant}_${scope.subject}:sytue-vault`;
}

/** Los secretos de entrada del correo (Go `keyring.SecretName`). */
const ingressSecretName = /^org\.sytue\.ingress\.key\.v1\.([0-9a-f]{64})$/u;
const maxIngressSecretBytes = 2048;

function ingressSecretHex(name: string): string {
  const hex = ingressSecretName.exec(name)?.[1];
  if (!hex) throw new StorageRecordError();
  return hex;
}

function sameBytes(left: Uint8Array, right: Uint8Array): boolean {
  if (left.byteLength !== right.byteLength) return false;
  let diff = 0;
  for (let i = 0; i < left.byteLength; i++) diff |= left[i]! ^ right[i]!;
  return diff === 0;
}

export type PqVaultSigningKeyStatus = 'registered' | 'reauth' | 'missing' | 'unavailable';

/** El resultado de registrar la clave de firma, con el porqué si no quedó registrada. */
interface SigningKeyOutcome {
  status: PqVaultSigningKeyStatus;
  /** Ver `PqVaultError.check`: `signing_put_409_vault_key_stale`, `signing_get_502`… */
  check?: string | undefined;
}

type CoreInvoker = Pick<PqcRuntime, 'invoke'>;

function bytesToText(value: unknown): string {
  if (value instanceof Uint8Array) {
    try {
      return decoder.decode(value);
    } finally {
      value.fill(0);
    }
  }
  if (typeof value === 'string') return value;
  throw new PqVaultError('unavailable');
}

/** Una clave de firma nueva del núcleo, en la forma en que va dentro del secreto. */
async function createSigningKey(core: CoreInvoker): Promise<string> {
  const created = await core.invoke<{ signing_key: unknown }>({
    operation: 'vault.signing.create',
  });
  const key = bytesToText(created.signing_key);
  if (!/^[A-Za-z0-9+/]{86}==$/.test(key)) throw new PqVaultError('unavailable');
  return key;
}

/** Añade la clave de firma a un secreto que no la tiene y guarda el envoltorio nuevo. */
async function addSigningKey(o: {
  core: CoreInvoker;
  bridge: PqVaultBridgePort;
  bridgeOpts: UnlockBridgeOptions;
  scope: MatrixStorageScope;
  keyId: string;
  recipient: string;
  secret: PqVaultSecret;
  put: (wrapper: PqVaultWrapper) => Promise<Response>;
}): Promise<PqVaultSecret> {
  const next: PqVaultSecret = { ...o.secret, signing_key: await createSigningKey(o.core) };
  const wrapped = await o.bridge.wrap(o.bridgeOpts, {
    tenant: o.scope.tenant,
    product: o.scope.product,
    profile: pqVaultProfile,
    keyID: o.keyId,
    recipient: o.recipient,
    secret: next,
  });
  if (!wrapped.ok) throw new PqVaultError(wrapped.error);
  const res = await o.put(wrapped.wrapper);
  if (!res.ok) throw pqVaultUnavailable('signing_keys_put', res.status, await errorCode(res));
  return next;
}

interface SigningKeyView {
  generation: number;
  key_id: string;
  retired_at?: string;
}

/**
 * Registra en Sytue ID la clave de firma de la bóveda si no es ya la activa
 * (ADR 0087). Sin clave activa es un alta; con otra activa, que esta bóveda ya
 * no puede aprobar porque no la tiene, es un vaciado, y Sytue ID pide entonces
 * una identificación reciente (`reauth`).
 *
 * Los rechazos de Sytue ID llegan en el sobre de `platform/pkg/httpx`
 * (`{"error":{"code":…}}`). Aquí se leía `code` en la raíz, así que el `403
 * vault_key_reauth` de un vaciado salía como `unavailable`: el correo enseñaba
 * «no se pudieron abrir los datos cifrados» (`prepare/vault.unavailable`) en vez
 * de llevar a identificarse otra vez, y ni volver a entrar ni vaciar lo arreglaban.
 * Si otro dispositivo registró entre la lectura y el registro (`409
 * vault_key_stale`), se vuelve a leer la activa y se repite una vez.
 */
async function registerSigningKey(o: {
  core: CoreInvoker;
  scope: MatrixStorageScope;
  signingKey: string;
  handover?: boolean | undefined;
  call: (method: 'GET' | 'PUT', body?: unknown) => Promise<Response>;
}): Promise<SigningKeyOutcome> {
  if (o.handover && o.scope.product !== 'mail')
    return { status: 'unavailable', check: 'signing_handover' };
  for (let attempt = 0; ; attempt++) {
    const outcome = await registerSigningKeyOnce(o);
    if (outcome !== 'stale') return outcome;
    if (attempt > 0) return { status: 'unavailable', check: 'signing_put_409_vault_key_stale' };
  }
}

async function registerSigningKeyOnce(o: {
  core: CoreInvoker;
  scope: MatrixStorageScope;
  signingKey: string;
  handover?: boolean | undefined;
  call: (method: 'GET' | 'PUT', body?: unknown) => Promise<Response>;
}): Promise<SigningKeyOutcome | 'stale'> {
  const listed = await o.call('GET');
  if (!listed.ok)
    return {
      status: 'unavailable',
      check: pqVaultUnavailable('signing_get', listed.status, await errorCode(listed)).check,
    };
  const { keys } = (await listed.json()) as { keys?: SigningKeyView[] };
  const active = (Array.isArray(keys) ? keys : []).find((k) => !k.retired_at);
  const raw = encoder.encode(o.signingKey);
  try {
    const enroll = await o.core.invoke<{
      profile: string;
      key_id: string;
      ed25519: Uint8Array;
      mldsa65: Uint8Array;
      proof: Uint8Array;
      previous_key_id: string;
    }>({
      operation: 'vault.signing.enroll',
      signing_key: raw.slice(),
      subject: o.scope.subject,
      tenant: o.scope.tenant,
      product: o.scope.product,
      generation: active ? active.generation + 1 : 1,
      ...(active ? { previous_key_id: active.key_id } : {}),
    });
    // Un traspaso se pide siempre: Sytue ID dice si esa clave ya lo tiene.
    if (active && active.key_id === enroll.key_id && !o.handover) return { status: 'registered' };
    const res = await o.call('PUT', {
      public_key: {
        profile: enroll.profile,
        ed25519: toBase64(enroll.ed25519),
        mldsa65: toBase64(enroll.mldsa65),
      },
      previous_key_id: enroll.previous_key_id,
      proof: toBase64(enroll.proof),
      ...(o.handover ? { handover: true } : {}),
    });
    if (res.ok) return { status: 'registered' };
    const code = await errorCode(res);
    if (res.status === 403 && code === 'vault_key_reauth') return { status: 'reauth' };
    if (res.status === 409 && code === 'vault_key_stale') return 'stale';
    return {
      status: 'unavailable',
      check: pqVaultUnavailable('signing_put', res.status, code).check,
    };
  } finally {
    raw.fill(0);
  }
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
  /**
   * Si la clave de firma de esta bóveda es la activa en Sytue ID (ADR 0087).
   * `reauth`: hay otra registrada y sustituirla pide identificarse otra vez; la
   * bóveda funciona igual, solo lo que firma (el correo entrante) espera.
   */
  readonly signingKeyStatus: PqVaultSigningKeyStatus;
  /** Si no quedó registrada, por qué (ver `PqVaultError.check`). */
  readonly signingKeyCheck: string | undefined;
  private readonly base: string;
  private readonly fetchImpl: typeof fetch;
  private readonly core: PqcRuntime;
  private readonly lifetime: AbortController;
  private readonly identityBytes: Uint8Array;
  private readonly hmacKey: CryptoKey;
  private readonly jobs = new StorageJobs();
  private readonly storagePq: StoragePq;
  private readonly uploadCache: EncryptedOfflineValues;
  /** La clave de firma (ADR 0087), en la forma del secreto; se borra al cerrar. */
  private readonly signingKey: Uint8Array<ArrayBuffer> | undefined;
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
    signingKeyStatus?: PqVaultSigningKeyStatus | undefined;
    signingKeyCheck?: string | undefined;
    signingKey?: string | undefined;
  }) {
    this.signingKeyStatus = params.signingKeyStatus ?? 'missing';
    this.signingKeyCheck =
      this.signingKeyStatus === 'registered' ? undefined : params.signingKeyCheck;
    this.signingKey = params.signingKey ? encoder.encode(params.signingKey) : undefined;
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

  /**
   * El núcleo (`vault`) empieza a bajar YA, en paralelo con el bootstrap, las claves y el
   * puente, y no con el primer `invoke`, tras los tres. Lo hacía solo la puerta
   * (`PqVaultGate`); aquí lo tiene también quien abre la bóveda por su cuenta (Photos y
   * Docs con el Drive de la bóveda). El worker nace sin cuenta: solo sabe la URL pública
   * de los artefactos y sus resúmenes. El `PqcRuntime` de esta apertura lo adopta, una
   * sola vez; si la apertura acaba sin adoptarlo (falla, o no llegó a usar el núcleo),
   * se termina. Con un núcleo propio (`createCore`, las pruebas) no se arranca nada.
   */
  private static async openInternal(
    options: PqVaultOpenOptions,
    forceReset: boolean,
  ): Promise<PqVault> {
    if (forceReset && options.existing) throw new PqVaultError('missing');
    const soltarNucleo = options.createCore ? undefined : prewarmClientCore(vaultCoreModule);
    try {
      return await PqVault.abrir(options, forceReset);
    } finally {
      soltarNucleo?.();
    }
  }

  private static async abrir(options: PqVaultOpenOptions, forceReset: boolean): Promise<PqVault> {
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
      if (!keysRes.ok)
        throw pqVaultUnavailable('keys_get', keysRes.status, await errorCode(keysRes));
      const state = (await keysRes.json()) as PqVaultStateResponse;
      options.signal?.throwIfAborted();
      // Lo que se vacía, si se vacía: lo que vio quien decidió vaciar, que tiene
      // que seguir siendo lo que hay, o lo que se acaba de leer.
      const seen = seenState(state);
      if (
        forceReset &&
        options.expect &&
        (options.expect.vault_id !== seen.vault_id || options.expect.version !== seen.version)
      )
        throw new PqVaultError('changed');

      let vaultId = state.vault_id;
      let keyId: string;
      let recipient: string;
      let secret: PqVaultSecret;

      let reset = forceReset;
      let opened: Awaited<ReturnType<PqVaultBridgePort['unwrap']>> | null = null;
      if (!reset && Array.isArray(state.keys) && state.keys.length > 0 && state.vault_id) {
        // Priorizar la clave activa si aparece en la lista.
        const ordered = [...state.keys].sort((a, b) =>
          a.key_id === state.active_key_id ? -1 : b.key_id === state.active_key_id ? 1 : 0,
        );
        opened = await bridge.unwrap(bridgeOpts, {
          tenant: scope.tenant,
          product: scope.product,
          wrappers: ordered,
        });
        options.signal?.throwIfAborted();
        // La clave se envolvió con otra clave de desbloqueo (la de antes de ADR 0086 o
        // una que ya no existe). Si la bóveda no guarda ni un registro, no hay nada que
        // perder: se reinicia en silencio en vez de dejar la app bloqueada. Es el caso
        // de las doce apps que pasan por la puerta sin guardar nada en la bóveda.
        if (!opened.ok && opened.error === 'wrong' && !options.existing) {
          const empty = await vaultIsEmpty(f, base, state.vault_id, scopeHeader, options.signal);
          options.signal?.throwIfAborted();
          if (empty) reset = true;
        }
      }

      if (opened && !reset) {
        if (!opened.ok)
          throw new PqVaultError(
            opened.error,
            undefined,
            opened.error === 'wrong' ? seen : undefined,
          );
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
      } else if (options.existing) {
        throw new PqVaultError('missing');
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
        // La clave de firma nace con la bóveda (ADR 0087). Si el núcleo no la da,
        // la bóveda se crea igual: se añadirá en la siguiente apertura.
        const signing = await createSigningKey(core).catch(() => undefined);
        if (signing) secret.signing_key = signing;
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
          // Vaciar va con lo visto: si alguien publicó algo después (el recibo de
          // una importación, por ejemplo), el servidor no vacía nada.
          body: JSON.stringify({
            wrapper: wrapped.wrapper,
            set_active: true,
            reset,
            ...(reset ? { expect: seen } : {}),
          }),
          ...(options.signal ? { signal: options.signal } : {}),
        });
        if (!putRes.ok) throw await pqVaultFailure(putRes);
        const saved = (await putRes.json()) as PqVaultStateResponse;
        if (!saved.vault_id) throw pqVaultUnavailable('keys_put_no_vault');
        vaultId = saved.vault_id;
        keyId = created.key_id;
        recipient = created.recipient;
      }

      // Una bóveda de antes de la ADR 0087 gana su clave de firma: se vuelve a sellar
      // el mismo secreto con ella y se guarda el envoltorio. Nada de esto impide
      // abrir la bóveda: si falla, se intenta en la siguiente apertura.
      if (!secret.signing_key && !options.existing) {
        secret = await addSigningKey({
          core,
          bridge,
          bridgeOpts,
          scope,
          keyId,
          recipient,
          secret,
          put: (wrapper) =>
            f(`${base}/api/pq-vault/keys`, {
              method: 'PUT',
              credentials: 'same-origin',
              headers: { 'content-type': 'application/json', [storageScopeHeader]: scopeHeader },
              body: JSON.stringify({ wrapper, set_active: true, reset: false }),
              ...(options.signal ? { signal: options.signal } : {}),
            }),
        }).catch(() => secret);
        options.signal?.throwIfAborted();
      }
      const signing: SigningKeyOutcome = options.existing
        ? { status: 'unavailable', check: 'signing_existing' }
        : secret.signing_key
          ? await registerSigningKey({
              core,
              scope,
              signingKey: secret.signing_key,
              handover: options.handover,
              call: (method, body) =>
                f(`${base}/api/pq-vault/signing-key`, {
                  method,
                  credentials: 'same-origin',
                  ...(body
                    ? {
                        headers: { 'content-type': 'application/json' },
                        body: JSON.stringify(body),
                      }
                    : {}),
                  ...(options.signal ? { signal: options.signal } : {}),
                }),
            }).catch((error: unknown): SigningKeyOutcome => ({
              status: 'unavailable',
              check: error instanceof PqVaultError && error.check ? error.check : 'signing_failed',
            }))
          : { status: 'missing', check: 'signing_missing' };
      options.signal?.throwIfAborted();

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
        signingKeyStatus: signing.status,
        signingKeyCheck: signing.check,
        signingKey: secret.signing_key,
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
    return new StorageRepository(
      this.scope,
      this.recordTransport().transport,
      blobs,
      this.uploadCache,
    );
  }

  /** El destino de publicación de la bóveda: su sala y su «dispositivo». */
  get publicationTarget(): Readonly<StoragePublicationTarget> {
    return Object.freeze({ room_id: this.vaultId, device_id: `pq-${this.keyId.slice(0, 24)}` });
  }

  /** La marca de copia en curso de esta bóveda, o `null` (ADR 0086). */
  async copyMarker(signal?: AbortSignal): Promise<PqVaultCopyMarker | null> {
    if (this.closed || this.lifetime.signal.aborted) throw new MatrixSecurityError('device');
    return readCopy(this.fetchImpl, this.base, encodeStorageScopeHeader(this.scope), signal);
  }

  /**
   * Retira una marca de copia ya caducada. Solo después de abrir la bóveda con
   * su recibo: la copia que la puso terminó o se paró, y la marca ya no dice nada.
   */
  async clearStaleCopy(signal?: AbortSignal): Promise<void> {
    const marker = await this.copyMarker(signal);
    if (!marker || marker.active) return;
    await releaseCopy(
      this.fetchImpl,
      this.base,
      encodeStorageScopeHeader(this.scope),
      marker.id,
      signal,
    );
  }

  /** Se aborta al cerrar la bóveda: lo que dependa de ella se retira con ella. */
  get signal(): AbortSignal {
    return this.lifetime.signal;
  }

  /**
   * La raíz con la que la bóveda aparece ante el correo (Go `vaultproof.RootKey`):
   * la mitad Ed25519 de su clave de firma, en base64 sin relleno.
   */
  async ingressRootKey(signal?: AbortSignal): Promise<string> {
    if (this.closed || this.lifetime.signal.aborted) throw new MatrixSecurityError('device');
    if (!this.signingKey) throw new MatrixSecurityError('recovery');
    const out = await this.core.invoke<{ ed25519: unknown }>(
      { operation: 'vault.signing.public', signing_key: this.signingKey.slice() },
      signal,
    );
    if (!(out?.ed25519 instanceof Uint8Array) || out.ed25519.byteLength !== 32)
      throw new MatrixSecurityError('unverified');
    return toBase64(out.ed25519).replace(/=+$/u, '');
  }

  /** La identidad de la bóveda ante el correo (Go `vaultproof.UserID`). */
  get ingressUserID(): string {
    return pqVaultUserID(this.scope);
  }

  /**
   * Firma una acción de entrada del correo (ADR 0087) con la clave de la bóveda, en
   * el núcleo: `request` son los bytes `{expected, action}` del puerto
   * `ingress.action.sign`, tal cual. Sin clave registrada en Sytue ID no firma: el
   * servidor no la aceptaría.
   */
  async signIngressAction(
    request: Uint8Array,
    signal?: AbortSignal,
  ): Promise<Uint8Array<ArrayBuffer>> {
    if (this.closed || this.lifetime.signal.aborted) throw new MatrixSecurityError('device');
    if (!this.signingKey || this.signingKeyStatus !== 'registered')
      throw new MatrixSecurityError('recovery');
    const out = await this.core.invoke<{ proof: unknown }>(
      {
        operation: 'vault.ingress.sign',
        signing_key: this.signingKey.slice(),
        request: request.slice(),
      },
      signal,
    );
    if (!(out?.proof instanceof Uint8Array)) throw new MatrixSecurityError('unverified');
    const proof = new Uint8Array(out.proof);
    out.proof.fill(0);
    if (this.closed) {
      proof.fill(0);
      throw new MatrixSecurityError('device');
    }
    return proof;
  }

  /**
   * Lee un secreto de entrada del correo guardado en la bóveda (ADR 0087): un
   * registro `content` cifrado como los demás, escrito una sola vez con el
   * identificador `ingress_secret_<id>`. `null` si no existe.
   */
  async readIngressSecret(
    name: string,
    signal?: AbortSignal,
  ): Promise<Uint8Array<ArrayBuffer> | null> {
    const hex = ingressSecretHex(name);
    const record = await this.recordTransport().find(`ingress_secret_${hex}`, signal);
    if (!record) return null;
    if (
      record.object_id !== `ingress_${hex}` ||
      record.revision_id !== 'v1' ||
      record.role !== 'content' ||
      (record.previous ?? []).length
    )
      throw new MatrixSecurityError('unverified');
    return storageJSONBytes(record.payload, maxIngressSecretBytes);
  }

  /**
   * Guarda un secreto de entrada del correo. Es de una sola escritura: repetirla
   * con los mismos bytes no hace nada; con otros, falla (nunca se sustituye una
   * clave con la que ya se ha podido cifrar correo).
   */
  async writeIngressSecret(name: string, value: Uint8Array, signal?: AbortSignal): Promise<void> {
    const hex = ingressSecretHex(name);
    if (value.byteLength > maxIngressSecretBytes) throw new StorageRecordError();
    const records = this.recordTransport();
    const record = createStorageRecord(this.scope, `ingress_${hex}`, 'v1', 'content', value);
    try {
      await records.transport.put(`ingress_secret_${hex}`, record, this.publicationTarget, signal);
    } catch (error) {
      // Un reintento tras una escritura que sí llegó choca con su propio cifrado:
      // vale si lo guardado es lo mismo.
      const saved = await this.readIngressSecret(name, signal).catch(() => null);
      if (!saved) throw error;
      const same = sameBytes(saved, value);
      saved.fill(0);
      if (!same) throw error;
    }
  }

  private recordTransport(): {
    transport: StorageRecordTransport;
    find: (transaction: string, signal?: AbortSignal) => Promise<StorageRecord | null>;
  } {
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
      if (!res.ok) {
        // Otro dispositivo copia en la bóveda: nada se publicó; se dice tal cual.
        if (res.status === 409 && (await errorCode(res)) === 'pq_vault_copy_in_progress')
          throw new PqVaultError('copy');
        throw new StorageRecordError();
      }
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

    const publication = this.publicationTarget;

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
      // Cada turno es un trabajo propio. Con un nombre fijo, el segundo que llegaba
      // mientras corría el primero se rechazaba (`locked`) sin soltar su turno, y
      // todo lo exclusivo que viniera después esperaba para siempre.
      const turn = this.jobs.run(
        crypto.randomUUID(),
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
      // Si ni siquiera llegó a empezar (bóveda cerrada, señal ya abortada), el turno
      // se suelta igual cuando le toque.
      void turn.catch(() => {
        void prev.then(release, release);
      });
      return turn;
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

    const find = async (
      transaction: string,
      signal?: AbortSignal,
    ): Promise<StorageRecord | null> => {
      assertOpen();
      const params = new URLSearchParams({ vault_id: this.vaultId, transaction_id: transaction });
      const res = await this.fetchImpl(`${this.base}/api/pq-vault/records?${params.toString()}`, {
        method: 'GET',
        credentials: 'same-origin',
        headers: { [storageScopeHeader]: scopeHeader },
        ...(signal ? { signal } : {}),
      });
      if (!res.ok) throw new StorageRecordError();
      const page = (await res.json()) as PqVaultRecordPageResponse;
      const entries = Array.isArray(page.entries) ? page.entries : [];
      if (entries.length === 0) return null;
      const item = entries[0]!;
      if (entries.length !== 1 || item.vault_id !== this.vaultId)
        throw new MatrixSecurityError('reference');
      const reference: MatrixReference = { room_id: item.vault_id, event_id: item.event_id };
      const opened = await this.storagePq.open(item.payload, reference, readRaw);
      if (!opened || 'fragment' in opened) throw new StorageRecordError();
      await verifyProof(reference.room_id, opened.record, opened.proof);
      return opened.record;
    };

    return { transport, find };
  }

  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.lifetime.abort();
    void this.jobs.close();
    this.identityBytes.fill(0);
    this.signingKey?.fill(0);
    this.storagePq.close();
    this.uploadCache.close();
    this.core.close();
  }

  async closeAndDrain(): Promise<void> {
    this.close();
    await Promise.all([this.jobs.close(), this.core.closeAndDrain()]);
  }
}
