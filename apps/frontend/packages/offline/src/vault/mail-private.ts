/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Sytue Mail sobre la bóveda post-cuántica directa (ADR 0087, entrega 4).
 *
 * Monta para el módulo de correo (`mail-wasm`) los mismos puertos que
 * `SuiteMatrix.openMailPrivate`, servidos por la bóveda en vez de por Matrix:
 *
 * - almacén: `vault.storage()` (registros `age-MLKEM768-X25519-v1` en Drive);
 * - diario, trabajo y servicio: los mismos puertos, con la sesión de la app;
 * - secretos de entrada y control: `VaultIngressSecretsPort`/`VaultIngressControlPort`;
 * - `identity.root`: la raíz de la bóveda (la mitad Ed25519 de su clave de firma).
 *
 * Lo que NO hace: migrar un correo de Matrix. Una cuenta cuya autoridad de correo
 * es de Matrix no se abre aquí (`identity`); la puerta la manda por Matrix.
 *
 * El módulo de correo abre en modo de bóveda (`store: 'vault'`, lo elige este
 * anfitrión): sin el corte de identidad de Matrix ni su origen histórico, y con la
 * importación inicial del correo del servidor directa a la bóveda (ADR 0087).
 */
import { DriveClient, HttpClient } from '@sytue/api-clients';
import { withBase } from '@sytue/config/base-path';
import { encodeStorageScopeHeader, storageScopeHeader } from '@sytue/config/matrix';
import { openSessionOwner, verifySessionOwner, type OpaqueSessionOwner } from '../session-owner.js';
import { PqcRuntime, type ClientCoreModule, type PqcLease } from '../pqc/runtime.js';
import { PqcPortGroup } from '../pqc/port-lifetime.js';
import { PqcEncryptedJournal } from '../pqc/encrypted-journal-port.js';
import { PqcSourcePort } from '../pqc/source-port.js';
import { PqcWorkPort } from '../pqc/work-port.js';
import { PqcServicePort, privateServiceRoute } from '../pqc/service-port.js';
import { PqcStoragePort } from '../pqc/storage-port.js';
import { encodeHostJSON } from '../pqc/host-codec.js';
import { eraseOwnedBytes } from '../pqc/owned-bytes.js';
import type { HostBinding } from '../pqc/host-port.js';
import { StorageJobs } from '../matrix/storage-jobs.js';
import type { StorageBlobPort } from '../matrix/storage-files.js';
import { MatrixSecurityError, type MatrixStorageScope } from '../matrix/types.js';
import { PqVaultError, type PqVault } from './pq-vault.js';
import { VaultIngressControlPort, VaultIngressSecretsPort } from './mail-ports.js';

/** Lo que recibe `PrivateMailClient`: la misma forma que `MailPrivatePorts` de Matrix. */
export interface VaultMailPrivatePorts {
  readonly runtime: PqcRuntime;
  readonly port: string;
  readonly signal: AbortSignal;
  readonly scope: Readonly<MatrixStorageScope>;
  readonly session: string;
  /**
   * Lo mismo que el `destructionGuard` de Matrix: pausa el motor y exige «trabajo
   * pendiente = 0» antes de borrar nada local. Rechaza con `locked` si queda algo.
   */
  guardDestruction: () => Promise<void>;
  closeAndDrain: () => Promise<void>;
}

export interface VaultMailOptions {
  basePath: string;
  signal?: AbortSignal | undefined;
  /** Sustituibles en pruebas: no hay Web Worker ni sesión real. */
  openOwner?: (
    expected: { actor: string; tenant: string },
    basePath: string,
    lifetime: AbortSignal,
  ) => Promise<OpaqueSessionOwner>;
  createRuntime?: (lease: PqcLease, module: ClientCoreModule) => PqcRuntime;
}

interface VaultAuthority {
  v: number;
  scope: MatrixStorageScope;
  user_id: string;
  root_key: string;
  generation: number;
  status: string;
}

const decoder = new TextDecoder('utf-8', { fatal: true });

/**
 * La autoridad que Rooms da para el correo tiene que ser la de ESTA bóveda: su
 * identidad, su raíz y activa. Si es la de Matrix, este no es el camino.
 */
export function checkVaultAuthority(
  raw: Uint8Array,
  scope: MatrixStorageScope,
  userID: string,
  rootKey: string,
): VaultAuthority {
  let value: VaultAuthority;
  try {
    value = JSON.parse(decoder.decode(raw)) as VaultAuthority;
  } catch {
    throw new MatrixSecurityError('identity');
  }
  if (
    !value ||
    value.v !== 1 ||
    value.scope?.tenant !== scope.tenant ||
    value.scope?.subject !== scope.subject ||
    value.scope?.product !== scope.product ||
    value.user_id !== userID ||
    value.root_key !== rootKey ||
    value.status !== 'active' ||
    !Number.isSafeInteger(value.generation) ||
    value.generation < 1
  )
    throw new MatrixSecurityError('identity');
  return value;
}

/**
 * Pausa el motor de correo y comprueba que no le queda trabajo pendiente (envíos
 * en cola, ACK por confirmar): solo entonces se puede borrar lo local. Es la
 * comprobación del `destructionGuard` de `SuiteMatrix.openMailPrivate`.
 */
export async function guardMailDestruction(
  domain: Pick<PqcRuntime, 'invoke'>,
  port: string,
  scope: Readonly<MatrixStorageScope>,
  session: string,
  signal: AbortSignal,
): Promise<void> {
  signal.throwIfAborted();
  const input = encodeHostJSON({ session });
  try {
    await domain.invoke({ operation: 'mail.pause', port, scope, input }, signal);
    const pending = await domain.invoke<{ scope: MatrixStorageScope; count: number }>(
      { operation: 'mail.pending', port, scope, input },
      signal,
    );
    if (
      !pending ||
      pending.scope?.tenant !== scope.tenant ||
      pending.scope?.subject !== scope.subject ||
      pending.scope?.product !== scope.product ||
      !Number.isSafeInteger(pending.count) ||
      pending.count !== 0
    )
      throw new MatrixSecurityError('locked');
  } finally {
    input.fill(0);
  }
}

/**
 * Abre el motor de correo en modo de bóveda. El modo lo elige el anfitrión, aquí y
 * de forma explícita (`store: 'vault'`): el módulo lo comprueba contra la autoridad
 * de Rooms y nunca lo deduce. Sin puertos de Matrix: no hay nada que migrar.
 */
export async function openVaultMailEngine(
  domain: Pick<PqcRuntime, 'invoke'>,
  port: string,
  scope: Readonly<MatrixStorageScope>,
  signal: AbortSignal,
): Promise<string> {
  const input = encodeHostJSON({ store: 'vault' });
  let opened: { session: string; scope: MatrixStorageScope; ready: boolean };
  try {
    opened = await domain.invoke({ operation: 'mail.open', port, scope, input }, signal);
  } finally {
    input.fill(0);
  }
  if (
    !opened ||
    typeof opened.session !== 'string' ||
    !/^[0-9a-f]{64}$/.test(opened.session) ||
    opened.ready !== true ||
    opened.scope?.tenant !== scope.tenant ||
    opened.scope?.subject !== scope.subject ||
    opened.scope?.product !== scope.product
  )
    throw new MatrixSecurityError('identity');
  return opened.session;
}

/** Abre el correo privado sobre una bóveda ya abierta de `mail`. */
export async function openVaultMailPrivate(
  vault: PqVault,
  options: VaultMailOptions,
): Promise<VaultMailPrivatePorts> {
  const scope = Object.freeze({ ...vault.scope });
  if (vault.isClosed || scope.product !== 'mail') throw new MatrixSecurityError('identity');
  // Sin la clave de firma activa en Sytue ID el servidor no acepta nada de lo que
  // la bóveda firme: tras un vaciado hay que identificarse otra vez, de verdad
  // (`reauth`: con `max_age=0`, no con la sesión que ya hay).
  if (vault.signingKeyStatus === 'reauth') throw new PqVaultError('reauth');
  if (vault.signingKeyStatus !== 'registered') {
    // Con el porqué (`signing_put_400_vault_key_proof`…): sin él, «Más información»
    // decía `vault.unavailable` y no había forma de saber qué contestó Sytue ID.
    const error = new PqVaultError('unavailable');
    error.check = vault.signingKeyCheck ?? 'signing_' + vault.signingKeyStatus;
    throw error;
  }
  options.signal?.throwIfAborted();
  const basePath = options.basePath || '/';
  const openOwner =
    options.openOwner ?? ((expected, base, lifetime) => openSessionOwner(expected, base, lifetime));
  const createRuntime = options.createRuntime ?? ((lease, module) => new PqcRuntime(lease, module));
  const expected = { actor: scope.subject, tenant: scope.tenant };
  const owner = await openOwner(expected, basePath, vault.signal);
  verifySessionOwner(owner, expected);

  const group = new PqcPortGroup([
    owner.signal,
    vault.signal,
    ...(options.signal ? [options.signal] : []),
  ]);
  const target = vault.publicationTarget;
  const binding: HostBinding = Object.freeze({
    scope,
    matrixUserID: vault.ingressUserID,
    deviceID: target.device_id,
  });
  const lease: PqcLease = {
    binding,
    signal: group.signal,
    assertOpen: () => group.assertOpen(),
  };
  // Un solo núcleo: el de Mail sirve también `mail.capabilities`. Antes se bajaba
  // además el núcleo completo (11 MB) solo para esa pregunta.
  const domain = createRuntime(lease, 'mail');
  const jobs = new StorageJobs();
  let closing: Promise<void> | undefined;
  const closeAndDrain = (): Promise<void> => {
    if (closing) return closing;
    closing = (async () => {
      const results = await Promise.allSettled([
        group.closeAndDrain(),
        owner.closeAndDrain(),
        jobs.close(),
        domain.closeAndDrain(),
      ]);
      const errors: unknown[] = results.flatMap((r): unknown[] =>
        r.status === 'rejected' ? [r.reason as unknown] : [],
      );
      if (errors.length) throw new AggregateError(errors, 'vault_private_mail_close');
    })();
    void closing.catch(() => {});
    return closing;
  };
  try {
    group.assertOpen();
    const http = new HttpClient({
      baseUrl: new URL(withBase(basePath, '/api'), globalThis.location.origin).toString(),
      fetchImpl: owner.fetch,
      defaultHeaders: { [storageScopeHeader]: encodeStorageScopeHeader(scope) },
    });
    const drive = new DriveClient({
      baseUrl: new URL(withBase(basePath, '/'), globalThis.location.origin)
        .toString()
        .replace(/\/$/, ''),
      fetchImpl: owner.fetch,
    });
    const blobs: StorageBlobPort = Object.freeze({
      read: (id: string, signal?: AbortSignal) => drive.readCipherBlob(scope, id, signal),
      write: (id: string, bytes: Uint8Array<ArrayBuffer>, signal?: AbortSignal) =>
        drive.putCipherBlob(scope, id, bytes, signal),
    });
    const revalidate = async (active: AbortSignal): Promise<void> => {
      group.assertOpen();
      active.throwIfAborted();
      if (vault.isClosed) throw new MatrixSecurityError('device');
      verifySessionOwner(owner, expected);
      await owner.revalidate(active);
      group.assertOpen();
      active.throwIfAborted();
    };
    const run = <T>(action: (active: AbortSignal) => Promise<T>, parent: AbortSignal) =>
      jobs.run(
        crypto.randomUUID(),
        async (active) => {
          await revalidate(active);
          const value = await action(active);
          try {
            await revalidate(active);
            return value;
          } catch (error) {
            eraseOwnedBytes(value);
            throw error;
          }
        },
        parent,
      );

    // 1. La autoridad del correo es la de esta bóveda (Rooms, ADR 0087).
    const rootKey = await vault.ingressRootKey(group.signal);
    const [authorityMethod, authorityPath] = privateServiceRoute('rooms.authority');
    const authorityRaw = await http.bytes({
      method: authorityMethod,
      path: authorityPath,
      maxResponseBytes: 16384,
      signal: group.signal,
    });
    try {
      checkVaultAuthority(authorityRaw, scope, vault.ingressUserID, rootKey);
    } finally {
      authorityRaw.fill(0);
    }
    await revalidate(group.signal);

    // 2. El corte de identidad de Matrix no es de este camino: el módulo de correo
    //    abre en modo de bóveda (`store: 'vault'`), sin origen histórico, e importa
    //    el correo retenido del servidor directo en la bóveda.

    // 3. Capacidades del módulo con un puerto de origen temporal (solo lectura).
    const unavailable = (): Promise<never> =>
      Promise.reject(new MatrixSecurityError('unsupported'));
    const sourceAdapter = new PqcSourcePort(scope, group, {
      state: async (active) => {
        await revalidate(active);
        const [method, path] = privateServiceRoute('mail.source.state');
        return http.bytes({ method, path, maxResponseBytes: 64 << 10, signal: active });
      },
      begin: unavailable,
      page: unavailable,
      open: unavailable,
    });
    const temporary = group.add(
      domain.registerHostPort({
        scope,
        lease: group,
        run,
        handlers: { 'source.capabilities': sourceAdapter.handlers['source.capabilities']! },
        cleanup: () => sourceAdapter.close(),
      }),
    );
    let maximum: number;
    try {
      const capabilities = await domain.invoke<{
        v: number;
        scope: MatrixStorageScope;
        max_body_bytes: number;
      }>({ operation: 'mail.capabilities', port: temporary.id, scope }, group.signal);
      if (
        capabilities.v !== 1 ||
        capabilities.scope.tenant !== scope.tenant ||
        capabilities.scope.subject !== scope.subject ||
        capabilities.scope.product !== scope.product ||
        !Number.isSafeInteger(capabilities.max_body_bytes) ||
        capabilities.max_body_bytes <= 0 ||
        capabilities.max_body_bytes > 2 ** 47
      )
        throw new MatrixSecurityError('reference');
      maximum = capabilities.max_body_bytes;
    } finally {
      temporary.revoke();
      await temporary.drain();
    }

    // 4. Los puertos del correo sobre la bóveda.
    const records = vault.storage(blobs);
    await records.prepare(group.signal);
    await revalidate(group.signal);
    let canonicalID = '';
    const storageAdapter = new PqcStoragePort(records, domain, () => canonicalID);
    const journal = new PqcEncryptedJournal(scope, () => !group.signal.aborted),
      work = new PqcWorkPort(scope, group, {
        revalidate,
        acquire: async (active) => {
          await revalidate(active);
          const releases: Array<() => void | Promise<void>> = [];
          try {
            releases.push(await owner.acquire(active));
            releases.push(await jobs.retain(active));
            await revalidate(active);
            let release: Promise<void> | undefined;
            return () => {
              release ??= Promise.allSettled(
                releases.map((done) => Promise.resolve().then(done)),
              ).then((results) => {
                if (results.some((result) => result.status === 'rejected'))
                  throw new MatrixSecurityError('device');
              });
              return release;
            };
          } catch (error) {
            await Promise.allSettled(releases.map((done) => Promise.resolve().then(done)));
            throw error;
          }
        },
      }),
      secrets = new VaultIngressSecretsPort(vault, records),
      control = new VaultIngressControlPort(vault, records);
    const service = new PqcServicePort(
      scope,
      {
        signal: group.signal,
        assertOpen: () => group.assertOpen(),
        revalidate,
        fetch: owner.fetch,
      },
      domain,
      () => canonicalID,
      basePath,
      maximum,
    );
    const canonical = group.add(
      domain.registerHostPort({
        scope,
        lease: group,
        run,
        handlers: {
          ...storageAdapter.handlers,
          ...journal.handlers,
          ...work.handlers,
          ...secrets.handlers,
          ...control.handlers,
          ...service.handlers,
          'identity.root': async (_raw, ctx) => {
            await revalidate(ctx.signal);
            return encodeHostJSON({ root_key: rootKey });
          },
        },
        cleanup: async () => {
          const cleaned = await Promise.allSettled([
            Promise.resolve(storageAdapter.close()),
            journal.close(),
            work.close(),
            secrets.close(),
            control.close(),
            service.close(),
          ]);
          if (cleaned.some((result) => result.status === 'rejected'))
            throw new MatrixSecurityError('device');
        },
      }),
    );
    canonicalID = canonical.id;
    await revalidate(group.signal);
    const engine = await openVaultMailEngine(domain, canonicalID, scope, group.signal);
    await revalidate(group.signal);
    return Object.freeze({
      runtime: domain,
      port: canonicalID,
      scope,
      signal: group.signal,
      session: engine,
      guardDestruction: async () => {
        group.assertOpen();
        await revalidate(group.signal);
        await guardMailDestruction(domain, canonicalID, scope, engine, group.signal);
        await revalidate(group.signal);
      },
      closeAndDrain,
    });
  } catch (error) {
    try {
      await closeAndDrain();
    } catch {
      /* Se conserva el fallo original: todo lo abierto intentó cerrarse. */
    }
    throw error;
  }
}
