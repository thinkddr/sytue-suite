/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Drive sobre la bóveda post-cuántica (ADR 0086), compartido por las apps web que
 * lo usan: la propia Drive y las que guardan ficheros en el Drive de la persona
 * (Photos sube ahí sus originales). Aparte de `./vault` para que las demás apps de
 * la bóveda no carguen el motor de Drive.
 *
 * Antes vivía en `apps/drive/src/client/private-drive.ts`; Drive lo reexporta tal
 * cual. Lo nuevo es `importar: false`: una app que solo CONSUME el Drive de la
 * bóveda nunca importa ni congela la fuente del servidor. Abre el repositorio solo
 * si la bóveda ya tiene el recibo de la migración (`initialize`, que no lee nada
 * en claro); si no, `PrivateDriveError('not_ready')` y la migración la hace Drive.
 *
 * `openConsumerPrivateDrive` y los ayudantes de carpetas y nombres de abajo son lo
 * que comparten esas apps consumidoras (Photos y la ofimática): abrir la bóveda
 * solo si existe, encontrar o crear su carpeta en la raíz y no pisar un nombre.
 */
import type { DriveClient } from '@sytue/api-clients';
import {
  MatrixSecurityError,
  PrivateDriveError,
  PrivateDriveRepository,
  type StorageFileSink,
  type StorageRepository,
} from '../matrix/index.js';
import { PqVault, PqVaultError, type PqVaultOpenOptions } from './pq-vault.js';

/** One private presenter per opened post-quantum vault; never rebound to another owner. */
export interface PrivateDriveSession {
  readonly client: { readonly isClosed: boolean };
  readonly vault?: PqVault;
  readonly storage: StorageRepository;
  readonly repository: PrivateDriveRepository;
  readonly signal: AbortSignal;
  readonly events: EventTarget;
  /** A retained upload needs attention. Provider messages and names stay out of notices. */
  uploadFailed: boolean;
  resume: () => Promise<void>;
}

interface Entry {
  value: PrivateDriveSession;
  ready: boolean;
  started: boolean;
}

export interface PrivateDriveVaultHost {
  prepare: (vault: PqVault, signal: AbortSignal) => Promise<void>;
  activate: (vault: PqVault) => void;
  close: (vault: PqVault) => Promise<void>;
}

/** Lo que el puente usa del cliente de Drive: bloques cifrados y el estado de la fuente. */
export type PrivateDriveSource = Pick<
  DriveClient,
  | 'readCipherBlob'
  | 'putCipherBlob'
  | 'privateStorageState'
  | 'beginPrivateStorageMigration'
  | 'privateStorageSourcePage'
  | 'openPrivateStorageSource'
>;

export function createPrivateDriveGate(options: {
  actor: string;
  tenant?: string | undefined;
  /** Lazy: SSR never touches window, fetch or the browser store. */
  source: () => PrivateDriveSource;
  /**
   * `true` (Drive): importa la fuente del servidor a la bóveda si aún no lo está.
   * `false` (las apps que solo consumen el Drive): nunca la importa ni la congela;
   * sin recibo en la bóveda, `PrivateDriveError('not_ready')`.
   */
  importar?: boolean | undefined;
}) {
  const owner = Object.freeze({ actor: options.actor, tenant: options.tenant });
  const newSource = options.source;
  const importar = options.importar ?? true;
  const entries = new Map<PqVault, Entry>();

  const current = (vault: PqVault): PrivateDriveSession => {
    const entry = entries.get(vault);
    if (!entry?.ready || vault.isClosed || entry.value.signal.aborted)
      throw new MatrixSecurityError('device');
    return entry.value;
  };

  const host: PrivateDriveVaultHost = {
    prepare: async (vault, signal) => {
      const scope = vault.scope;
      if (
        scope.subject !== owner.actor ||
        (owner.tenant !== undefined && scope.tenant !== owner.tenant) ||
        scope.product !== 'drive'
      )
        throw new MatrixSecurityError('identity');
      signal.throwIfAborted();
      const source = newSource();
      const boundScope = Object.freeze({
        tenant: scope.tenant,
        subject: owner.actor,
        product: 'drive' as const,
      });
      const storage = vault.storage({
        read: (id, child) => source.readCipherBlob(boundScope, id, child),
        write: (id, bytes, child) => source.putCipherBlob(boundScope, id, bytes, child),
      });
      if (
        storage.scope.subject !== owner.actor ||
        storage.scope.tenant !== scope.tenant ||
        storage.scope.product !== 'drive'
      )
        throw new MatrixSecurityError('identity');
      const repository = new PrivateDriveRepository(storage);
      const value: PrivateDriveSession = {
        client: vault,
        vault,
        storage,
        repository,
        signal,
        events: new EventTarget(),
        uploadFailed: false,
        resume: async () => {
          signal.throwIfAborted();
          if (vault.isClosed || entries.get(vault)?.value !== value)
            throw new MatrixSecurityError('device');
          try {
            await repository.resumeUploads(signal);
            value.uploadFailed = false;
          } catch (error) {
            value.uploadFailed = true;
            throw error;
          } finally {
            if (!signal.aborted && !vault.isClosed && entries.get(vault)?.value === value)
              value.events.dispatchEvent(new Event('change'));
          }
        },
      };
      const previous = entries.get(vault);
      if (previous) {
        entries.delete(vault);
        await previous.value.repository.close();
        signal.throwIfAborted();
      }
      const entry: Entry = { value, ready: false, started: false };
      entries.set(vault, entry);
      try {
        if (importar) {
          // Otro dispositivo está pasando su Drive de Matrix a esta bóveda (ADR
          // 0086): mientras su marca valga, no se importa encima. La puerta lo
          // enseña y deja esperar o cancelar esa copia; el servidor tampoco dejaría
          // publicar (409 → `copy`). Por eso la marca es un aviso y no una puerta:
          // si no se puede leer, se sigue. Antes un fallo al releerla dejaba Drive en
          // «no se pudieron abrir los datos cifrados» (`prepare/vault.unavailable`)
          // con la bóveda ya abierta, y ni reintentar ni vaciar lo cambiaban.
          const marker = await vault.copyMarker(signal).catch((error: unknown) => {
            if (error instanceof PqVaultError && error.reason === 'unavailable') return null;
            throw error;
          });
          if (marker?.active) throw new PqVaultError('copy');
          signal.throwIfAborted();
          // migrate prepares the vault before it can read the retained source.
          await repository.migrate(source, signal);
        } else {
          // Solo el estado (sin contenido): la bóveda abre si ya tiene su recibo.
          await repository.initialize(await source.privateStorageState(boundScope, signal), signal);
        }
        signal.throwIfAborted();
        if (entries.get(vault) !== entry || vault.isClosed) throw new MatrixSecurityError('device');
        entry.ready = true;
        // Con el recibo ya leído, una marca caducada no dice nada: se retira.
        if (importar) void vault.clearStaleCopy(signal).catch(() => {});
      } catch (error) {
        if (entries.get(vault) === entry) entries.delete(vault);
        await repository.close();
        throw error;
      }
    },
    activate: (vault) => {
      const entry = entries.get(vault);
      if (!entry?.ready || entry.started) return;
      entry.started = true;
      // Replays run after the verified presenter can mount; closing drains them.
      void current(vault)
        .resume()
        .catch(() => {});
    },
    close: async (vault) => {
      const entry = entries.get(vault);
      entries.delete(vault);
      await entry?.value.repository.close();
    },
  };

  return { host, current };
}

/**
 * Junta en memoria un fichero de Drive descifrado, con el tamaño que dice su fila:
 * los bytes solo salen tras el compromiso autenticado completo y se borran al
 * terminar, salga bien o mal. `run` hace la descarga con el sumidero que recibe.
 */
export async function collectPrivateDriveFile(options: {
  size: unknown;
  maximum: number;
  type: string;
  run: (sink: StorageFileSink) => Promise<unknown>;
  check?: (() => void) | undefined;
}): Promise<Blob> {
  const { size, maximum } = options;
  if (typeof size !== 'number' || !Number.isSafeInteger(size) || size < 0 || size > maximum)
    throw new MatrixSecurityError('unsupported');
  const chunks: Uint8Array<ArrayBuffer>[] = [];
  let bytes = 0,
    committed = false;
  const clear = () => {
    for (const chunk of chunks) chunk.fill(0);
    chunks.length = 0;
  };
  try {
    const sink: StorageFileSink = {
      write: (chunk) => {
        options.check?.();
        if (bytes + chunk.byteLength > size) throw new MatrixSecurityError('unsupported');
        bytes += chunk.byteLength;
        chunks.push(chunk.slice());
        return Promise.resolve();
      },
      commit: () => {
        if (bytes !== size) throw new MatrixSecurityError('reference');
        committed = true;
        return Promise.resolve();
      },
      abort: () => {
        clear();
        return Promise.resolve();
      },
    };
    await options.run(sink);
    options.check?.();
    if (!committed) throw new MatrixSecurityError('reference');
    return new Blob(chunks, { type: options.type });
  } finally {
    clear();
  }
}

/**
 * Por qué una app consumidora no pudo usar el Drive de la bóveda: `signin`, volver a
 * entrar para desbloquearlo; `pending`, Drive aún no ha terminado de pasar a la
 * bóveda (o no la tiene); `failed`, cualquier otra cosa.
 */
export type ConsumerPrivateDriveFailure = 'signin' | 'pending' | 'failed';

export function consumerPrivateDriveFailure(error: unknown): ConsumerPrivateDriveFailure {
  if (error instanceof PqVaultError) {
    if (error.reason === 'signin') return 'signin';
    if (error.reason === 'missing') return 'pending';
  }
  if (error instanceof PrivateDriveError && error.reason === 'not_ready') return 'pending';
  return 'failed';
}

/** El Drive de la bóveda abierto por una app consumidora. `close` lo suelta todo. */
export interface ConsumerPrivateDrive {
  readonly repository: PrivateDriveRepository;
  readonly signal: AbortSignal;
  close: () => Promise<void>;
}

/**
 * Abre el Drive de la bóveda para una app que solo lo CONSUME: la bóveda solo si
 * ya existe (`existing`: nunca se crea, se vacía ni se vuelve a sellar desde aquí)
 * y el repositorio solo con el recibo de la migración (`importar: false`). Lanza lo
 * que haya que contar (`consumerPrivateDriveFailure`).
 */
export async function openConsumerPrivateDrive(options: {
  actor: string;
  tenant?: string | undefined;
  /** Las rutas del BFF de la app para la bóveda de `drive`. */
  basePath: string;
  source: () => PrivateDriveSource;
  signal?: AbortSignal | undefined;
  /** Sustituible en pruebas. */
  open?: ((options: PqVaultOpenOptions) => Promise<PqVault>) | undefined;
}): Promise<ConsumerPrivateDrive> {
  const open = options.open ?? ((o: PqVaultOpenOptions) => PqVault.open(o));
  const vault = await open({
    actor: options.actor,
    tenant: options.tenant,
    product: 'drive',
    basePath: options.basePath,
    signal: options.signal,
    existing: true,
  });
  const life = new AbortController();
  const gate = createPrivateDriveGate({
    actor: options.actor,
    tenant: options.tenant,
    source: options.source,
    importar: false,
  });
  const close = async () => {
    life.abort();
    try {
      await gate.host.close(vault);
    } finally {
      vault.close();
      await vault.closeAndDrain();
    }
  };
  try {
    if (vault.scope.product !== 'drive') throw new PqVaultError('identity');
    const signal = options.signal ? AbortSignal.any([life.signal, options.signal]) : life.signal;
    await gate.host.prepare(vault, signal);
    // Lo que quedó a medias de otra sesión (en el diario de la bóveda) se reanuda.
    gate.host.activate(vault);
    return { repository: gate.current(vault).repository, signal, close };
  } catch (error) {
    await close().catch(() => {});
    throw error;
  }
}

/** `name`, o «name (2).ext» y siguientes si ya está cogido (como `nombreLibre` en Go). */
export function freePrivateDriveName(name: string, taken: ReadonlySet<string>): string {
  if (!taken.has(name)) return name;
  const dot = name.lastIndexOf('.');
  const [base, ext] = dot > 0 ? [name.slice(0, dot), name.slice(dot)] : [name, ''];
  for (let n = 2; ; n++) {
    const candidate = `${base} (${n})${ext}`;
    if (!taken.has(candidate)) return candidate;
  }
}

/** Lo que los ayudantes de carpetas usan del repositorio. */
export type PrivateDriveFolders = Pick<
  PrivateDriveRepository,
  'root' | 'children' | 'createFolder'
>;

const field = (row: Record<string, unknown>, key: string): string => {
  const value = row[key];
  return typeof value === 'string' ? value : '';
};

/** Todos los hijos de una carpeta (las carpetas vienen en cada página: sin repetir). */
export async function privateDriveFolderEntries(
  repository: Pick<PrivateDriveRepository, 'children'>,
  id: string,
  signal?: AbortSignal,
): Promise<{ folders: Record<string, unknown>[]; files: Record<string, unknown>[] }> {
  const folders = new Map<string, Record<string, unknown>>();
  const files: Record<string, unknown>[] = [];
  let cursor = '';
  do {
    const page = await repository.children(id, cursor, signal);
    for (const f of page.folders) folders.set(field(f, 'id'), f);
    files.push(...page.files);
    cursor = page.next_cursor;
  } while (cursor);
  return { folders: [...folders.values()], files };
}

/** La carpeta `name` de la raíz: su id, o `null` si no está y no se pide crearla. */
export async function privateDriveRootFolder(
  repository: PrivateDriveFolders,
  name: string,
  create: boolean,
  signal?: AbortSignal,
): Promise<string | null> {
  const root = field(await repository.root(signal), 'id');
  const { folders } = await privateDriveFolderEntries(repository, root, signal);
  const found = folders.find((f) => field(f, 'name') === name);
  if (found) return field(found, 'id');
  if (!create) return null;
  return field(await repository.createFolder(root, name, signal), 'id');
}
