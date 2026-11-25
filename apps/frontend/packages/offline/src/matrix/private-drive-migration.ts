/* SPDX-License-Identifier: Apache-2.0 */
/** Frozen legacy source to the same private records consumed by verified native devices. */
import { createSHA256 } from './sha256.js';
import {
  decodeLegacyMigrationState,
  decodeLegacySourcePage,
  legacySourceBytes,
  isMigrationEpoch,
  type MatrixStorageScope,
  type LegacyMigrationState,
  type LegacySourceCursor,
  type LegacySourcePage,
  type LegacySourceRow,
} from '@sytue/config/matrix';
import {
  storageJSONBytes,
  encodeStorageJSON,
  storageReference,
  storageScope,
  storageIdentifier,
  createStorageRecord,
  storageObjectKind,
  StorageRecordError,
  type StoragePublicationTarget,
  type StorageObjectVersion,
} from './storage.js';
import { fields, digest, invalid, hash } from './storage-file-codec.js';
import { storageStagedFile, type StorageStagedFile } from './storage-staged-files.js';
import {
  sourceKind,
  summarizeDriveRow,
  summary,
  driveObject,
  driveItemKind,
  driveBodyKind,
  driveMigrationKind,
  json,
  text,
  binary,
  bodyPointer,
  type DriveSummary,
  type DriveBodyPointer,
} from './private-drive-codec.js';
import type { MatrixReference } from './types.js';
import {
  publishDriveEntry,
  publishDriveBody,
  saveDriveWriteIntent,
  decodeDriveBodyWork,
  type DriveWriteIntent,
  type DriveEntryHost,
} from './private-drive-metadata.js';
export interface DriveMigrationSource {
  privateStorageState: (
    scope: MatrixStorageScope,
    signal?: AbortSignal,
  ) => Promise<LegacyMigrationState>;
  beginPrivateStorageMigration: (
    scope: MatrixStorageScope,
    epoch: string,
    signal?: AbortSignal,
  ) => Promise<LegacyMigrationState>;
  privateStorageSourcePage: (
    scope: MatrixStorageScope,
    epoch: string,
    after?: LegacySourceCursor,
    signal?: AbortSignal,
  ) => Promise<LegacySourcePage>;
  openPrivateStorageSource: (
    scope: MatrixStorageScope,
    epoch: string,
    kind: 'file' | 'version',
    id: string,
    size: number,
    signal?: AbortSignal,
  ) => Promise<Response>;
}
export interface DriveMigrationHost extends DriveEntryHost {
  completed: (
    state: LegacyMigrationState,
    inventory: string | undefined,
    signal: AbortSignal,
  ) => Promise<boolean>;
  verifyBinary: (
    item: DriveSummary,
    pointer: DriveBodyPointer,
    signal: AbortSignal,
  ) => Promise<void>;
  /** Tras `completed`: ¿lo cargado tiene alguna fila o recibo con más de una cabeza? */
  duplicated: () => boolean;
}
interface RowIntent extends DriveWriteIntent {
  v: 1;
  operation: 'import';
  epoch: string;
  staged?: StorageStagedFile;
}

/**
 * Por qué la importación no siguió: la comprobación exacta que lo rechazó, con un
 * código corto y fijo (`row_heads`, `inventory`…) que la puerta enseña en «Más
 * información» para saber en producción qué pasó sin datos de la persona. Es un
 * `StorageRecordError` como el de siempre: quien ya lo trataba no cambia.
 */
export class DriveImportError extends StorageRecordError {
  constructor(readonly check: DriveImportCheck) {
    super('invalid');
    this.name = 'DriveImportError';
  }
}
export type DriveImportCheck =
  /** La fuente está en `legacy` y la bóveda ya tiene datos de Drive (la fuente se soltó sin vaciarla). */
  | 'begin_items'
  /** El servidor no congeló la fuente con la época pedida. */
  | 'begin_state'
  /** La fuente no está congelada y la bóveda no tiene su recibo (p. ej. `private` sin recibo). */
  | 'source_mode'
  /** Una fila tiene en la bóveda varias revisiones que no son copias idénticas de la importación. */
  | 'row_heads'
  /** Lo que la bóveda guarda de una fila no es la fila congelada. */
  | 'row_mismatch'
  /** El original de un fichero no llegó como se esperaba (tipo, longitud, codificación). */
  | 'row_body'
  /** Las filas o los ficheros leídos no son los que dice el estado congelado. */
  | 'inventory'
  /** La fuente no trae exactamente una carpeta raíz viva. */
  | 'roots'
  /** Otra importación publicó a la vez una copia distinta de una fila. */
  | 'duplicate_items'
  /** El recibo publicado no se pudo leer de vuelta como el de esta importación. */
  | 'receipt';
function refuse(check: DriveImportCheck): never {
  throw new DriveImportError(check);
}
const encoder = new TextEncoder(),
  equal = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
async function prefix(host: DriveEntryHost) {
  return (
    'drive_import_' + (await digest(encoder.encode(JSON.stringify(host.repository.scope)))) + '_'
  );
}
async function rowIntent(
  bytes: Uint8Array<ArrayBuffer>,
  host: DriveMigrationHost,
  item: DriveSummary,
  payload: string,
): Promise<RowIntent> {
  if (bytes.length > 256 << 10) invalid();
  const value = fields(
      JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)) as unknown,
    ),
    scope = storageScope(value.scope);
  if (
    value.v !== 1 ||
    value.operation !== 'import' ||
    value.epoch !== item.epoch ||
    !equal(scope, host.repository.scope) ||
    !storageIdentifier(value.revision) ||
    !equal(value.item, item) ||
    value.raw !== payload ||
    value.base != null
  )
    invalid();
  const intent: RowIntent = {
    v: 1,
    operation: 'import',
    epoch: item.epoch,
    scope,
    item,
    raw: payload,
    revision: value.revision,
    target: host.repository.publicationTarget(value.target as StoragePublicationTarget),
  };
  if (value.staged != null) {
    intent.staged = storageStagedFile(value.staged, scope);
    if (
      !binary(item.source_kind) ||
      intent.staged.size_bytes !== item.size_bytes ||
      (item.checksum !== '' && intent.staged.sha256 !== item.checksum)
    )
      invalid();
  }
  if (value.metadata != null) intent.metadata = decodeDriveBodyWork(value.metadata);
  if (value.binary_work != null) intent.binary_work = decodeDriveBodyWork(value.binary_work);
  if (value.binary != null) {
    intent.binary = await bodyPointer(value.binary, item, 'binary');
    if (
      intent.binary.origin ||
      !intent.binary_work?.catalog ||
      !equal(intent.binary.reference, intent.binary_work.catalog)
    )
      invalid();
  }
  if (value.content != null) intent.content = storageReference(value.content);
  if (value.catalog != null) intent.catalog = storageReference(value.catalog);
  if (
    (intent.catalog && !intent.content) ||
    (intent.content && binary(item.source_kind) && !intent.binary) ||
    (!binary(item.source_kind) && (intent.binary || intent.binary_work || intent.staged))
  )
    invalid();
  return intent;
}
async function confirm(
  host: DriveMigrationHost,
  item: DriveSummary,
  payload: string,
  version: StorageObjectVersion,
  signal: AbortSignal,
) {
  const read = await host.read(version, signal);
  try {
    if (read.item.epoch !== item.epoch || encodeStorageJSON(read.raw) !== payload)
      refuse('row_mismatch');
    if (binary(item.source_kind)) {
      if (!read.entry.binary) refuse('row_mismatch');
      await host.verifyBinary(read.item, read.entry.binary, signal);
    }
  } finally {
    read.raw.fill(0);
  }
}

function groupHeads(catalog: readonly StorageObjectVersion[]) {
  const heads = new Map<string, StorageObjectVersion[]>();
  for (const version of catalog) {
    const list = heads.get(version.object_id) ?? [];
    list.push(version);
    heads.set(version.object_id, list);
  }
  return heads;
}
/** ¿Algún objeto de Drive (fila o recibo) con más de una cabeza? Solo mira el catálogo. */
function duplicated(heads: ReadonlyMap<string, readonly StorageObjectVersion[]>): boolean {
  for (const versions of heads.values()) {
    if (versions.length < 2) continue;
    const kind = json(storageJSONBytes(versions[0]!.commit.metadata, 8 << 10)).kind;
    if (kind === driveItemKind || kind === driveMigrationKind) return true;
  }
  return false;
}

/** Lo que identifica una copia importada de una fila: su resumen y lo que trae. */
async function importCopy(
  host: DriveMigrationHost,
  epoch: string,
  version: StorageObjectVersion,
  signal: AbortSignal,
): Promise<{ raw: string; body: string; binary: DriveBodyPointer | undefined } | null> {
  const item = await summary(version);
  // Solo una importación deja `seed` (la línea base es la propia fila): cualquier
  // cambio hecho después (renombrar, mover, subir otra versión, favorito) lo quita.
  if (!item.seed || item.epoch !== epoch || item.baseline_sha256 !== item.raw_sha256) return null;
  const read = await host.read(version, signal);
  try {
    let body = '';
    if (binary(item.source_kind)) {
      const pointer = read.entry.binary;
      if (!pointer || pointer.origin) return null;
      const description = json(
        storageJSONBytes(
          (await host.repository.object(pointer.reference, pointer.object_id, signal)).commit
            .metadata,
          8 << 10,
        ),
      );
      if (description.kind !== driveBodyKind || text(description, 'epoch') !== epoch) return null;
      body = text(description, 'sha256') + '\0' + String(description.size_bytes);
    }
    return { raw: encodeStorageJSON(read.raw), body, binary: read.entry.binary };
  } finally {
    read.raw.fill(0);
  }
}

/**
 * Dos importaciones de la misma época a la vez (la web en dos pestañas, la web y
 * un nativo, dos nativos con su diario cada uno) dejan dos copias de la misma fila
 * congelada, o dos recibos. Antes, eso bastaba para que Drive no abriera nunca más.
 *
 * Aquí se juntan, sin borrar nada: si TODAS las cabezas de un objeto son copias
 * idénticas de la importación de `epoch` (fila y fichero iguales, autenticados) o
 * recibos idénticos de esa época, se publica una sola revisión que las tiene a
 * todas por padres. Lo que no sea exactamente eso (una edición, otra época, un
 * fichero distinto) se queda como está: es un conflicto de verdad y se rechaza
 * donde siempre. Devuelve cuántos objetos juntó.
 */
async function mergeImportCopies(
  host: DriveMigrationHost,
  epoch: string,
  catalog: readonly StorageObjectVersion[],
  signal: AbortSignal,
): Promise<number> {
  const repository = host.repository,
    heads = groupHeads(catalog);
  const receipt = 'drive_migration_' + (await digest(encoder.encode(epoch)));
  let merged = 0;
  for (const [object, unordered] of heads) {
    if (unordered.length < 2 || unordered.length > 16) continue;
    // En un orden fijo: dos clientes que juntan las mismas cabezas a la vez
    // publican exactamente el mismo registro con la misma transacción, y el
    // servidor se queda con uno (sin una tercera cabeza que juntar después).
    const versions = [...unordered].sort((a, b) =>
      a.reference.event_id < b.reference.event_id
        ? -1
        : a.reference.event_id > b.reference.event_id
          ? 1
          : 0,
    );
    const kinds = versions.map((v) => json(storageJSONBytes(v.commit.metadata, 8 << 10)).kind);
    let chosen: StorageObjectVersion | undefined;
    if (object === receipt && kinds.every((kind) => kind === driveMigrationKind)) {
      // Los recibos dicen lo mismo si las dos importaciones leyeron la misma fuente
      // congelada; que cada uno sea válido lo comprueba `completed` después.
      const claims = versions.map((v) => {
        const proof = json(storageJSONBytes(v.commit.metadata, 8 << 10));
        return JSON.stringify([proof.epoch, proof.rows, proof.blobs, proof.inventory_sha256]);
      });
      if (
        claims.every((claim) => claim === claims[0]) &&
        versions.every((v) => v.commit.state === 'live')
      )
        chosen = versions[0];
    } else if (kinds.every((kind) => kind === driveItemKind)) {
      const copies = [];
      for (const version of versions) copies.push(await importCopy(host, epoch, version, signal));
      const first = copies[0];
      if (
        first &&
        copies.every(
          (copy, i) =>
            copy &&
            copy.raw === first.raw &&
            copy.body === first.body &&
            versions[i]!.commit.state === versions[0]!.commit.state,
        )
      ) {
        // Lo que se va a enseñar se comprueba entero, también el fichero.
        if (first.binary)
          await host.verifyBinary(await summary(versions[0]!), first.binary, signal);
        chosen = versions[0];
      }
    }
    if (!chosen) continue;
    const pointer = chosen.commit.content;
    if (!pointer || pointer.kind !== 'record') continue;
    const content = await repository.readRecord(pointer.reference, object, 'content', signal);
    const parents = versions.map((v): MatrixReference => ({ ...v.reference })),
      payload = storageJSONBytes(content.payload),
      revision =
        'merge_web_' +
        (await digest(
          encoder.encode([object, ...parents.map((p) => p.room_id + '\0' + p.event_id)].join('\0')),
        )),
      target = repository.publicationTarget();
    try {
      // Sin diario: las transacciones salen de lo que se junta. Un corte deja, como
      // mucho, el contenido publicado; la siguiente apertura repite lo mismo y el
      // servidor devuelve lo ya aceptado.
      const reference = await repository.putRecord(
        revision + '_content',
        createStorageRecord(repository.scope, object, revision, 'content', payload),
        signal,
        target,
      );
      await repository.putObject(
        revision + '_index',
        object,
        revision + '_index',
        {
          kind: storageObjectKind,
          state: chosen.commit.state,
          metadata: chosen.commit.metadata,
          content: { kind: 'record', reference, revision_id: revision },
          parents,
        },
        signal,
        target,
      );
    } finally {
      payload.fill(0);
    }
    merged++;
  }
  return merged;
}
async function importRow(
  host: DriveMigrationHost,
  source: DriveMigrationSource,
  row: LegacySourceRow,
  epoch: string,
  jobPrefix: string,
  heads: Map<string, StorageObjectVersion[]>,
  signal: AbortSignal,
) {
  const repository = host.repository,
    { journal } = repository.writeContext(),
    kind = sourceKind(row.kind),
    raw = legacySourceBytes(row.payload);
  try {
    const item = await summarizeDriveRow(kind, row.id, epoch, raw),
      object = await driveObject(kind, row.id),
      target = repository.publicationTarget(),
      key =
        jobPrefix +
        'row_' +
        (await digest(
          encoder.encode(epoch + '\0' + kind + '\0' + row.id + '\0' + target.device_id),
        ));
    const existing = heads.get(object) ?? [];
    // Las copias idénticas de otra importación ya se juntaron (mergeImportCopies):
    // varias cabezas aquí son revisiones distintas de la fila, un conflicto de verdad.
    if (existing.length > 1) refuse('row_heads');
    const stored = await journal.read(key);
    let work: RowIntent;
    if (stored !== null) {
      try {
        work = await rowIntent(stored, host, item, row.payload);
      } finally {
        stored.fill(0);
      }
    } else {
      work = {
        v: 1,
        operation: 'import',
        epoch,
        scope: { ...repository.scope },
        target,
        revision: crypto.randomUUID(),
        item,
        raw: row.payload,
      };
    }
    if (existing.length) {
      await confirm(host, item, row.payload, existing[0]!, signal);
      if (work.staged) await repository.stagedFiles().discard(work.staged, signal);
      if (stored !== null) await journal.delete(key);
      return item;
    }
    if (stored === null) await saveDriveWriteIntent(journal, key, work, signal);
    const parents = async (purpose: string) => {
      const list = heads.get(await driveObject(kind + '_' + purpose, row.id)) ?? [];
      if (list.length > 16) invalid();
      return list.map((v) => v.reference);
    };
    if (binary(kind)) {
      if (!work.staged && !work.binary_work?.file) {
        const response = await source.openPrivateStorageSource(
          repository.scope,
          epoch,
          kind as 'file' | 'version',
          row.id,
          item.size_bytes,
          signal,
        );
        try {
          if (
            response.status !== 200 ||
            response.headers.has('content-encoding') ||
            response.headers.get('content-type') !== 'application/octet-stream' ||
            (response.headers.has('content-length') &&
              response.headers.get('content-length') !== String(item.size_bytes))
          )
            refuse('row_body');
          const body =
            response.body ??
            new ReadableStream<Uint8Array<ArrayBuffer>>({
              start(controller) {
                controller.close();
              },
            });
          work.staged = await repository
            .stagedFiles()
            .captureStream(body, item.size_bytes, item.checksum || undefined, signal);
          await saveDriveWriteIntent(journal, key, work, signal);
        } finally {
          await response.body?.cancel().catch(() => undefined);
        }
      }
      if (!work.binary_work) {
        work.binary_work = {
          revision: crypto.randomUUID(),
          parents: await parents('binary'),
          sha256: work.staged?.sha256 ?? invalid(),
        };
        await saveDriveWriteIntent(journal, key, work, signal);
      }
      const bodySource = work.binary_work?.file
        ? undefined
        : await repository.stagedFiles().open(work.staged ?? invalid(), signal);
      const pointer = await publishDriveBody(
        host,
        journal,
        key,
        work,
        'binary',
        item.size_bytes,
        bodySource,
        signal,
      );
      if (work.binary && !equal(work.binary, pointer)) invalid();
      work.binary = pointer;
      await saveDriveWriteIntent(journal, key, work, signal);
    }
    if (raw.length > 16 << 10 && !work.metadata) {
      work.metadata = {
        revision: crypto.randomUUID(),
        parents: await parents('metadata'),
        sha256: item.raw_sha256,
      };
      await saveDriveWriteIntent(journal, key, work, signal);
    }
    await publishDriveEntry(host, journal, key, work, signal);
    const version = await repository.object(work.catalog!, object, signal);
    await confirm(host, item, row.payload, version, signal);
    heads.set(object, [version]);
    for (const purpose of ['binary', 'metadata'] as const) {
      const body = purpose === 'binary' ? work.binary_work : work.metadata;
      if (body?.catalog) {
        const id = await driveObject(kind + '_' + purpose, row.id);
        heads.set(id, [await repository.object(body.catalog, id, signal)]);
      }
    }
    if (work.staged) await repository.stagedFiles().discard(work.staged, signal);
    await journal.delete(key);
    return item;
  } finally {
    raw.fill(0);
  }
}
async function begin(
  host: DriveMigrationHost,
  source: DriveMigrationSource,
  jobPrefix: string,
  signal: AbortSignal,
): Promise<LegacyMigrationState> {
  const repository = host.repository,
    { journal } = repository.writeContext(),
    catalog = await repository.catalog(signal);
  for (const version of catalog) {
    const meta = json(storageJSONBytes(version.commit.metadata, 8 << 10));
    // La fuente volvió a `legacy` (se soltó) con la bóveda llena: importar encima
    // mezclaría dos épocas. Se sale vaciando (la puerta lo ofrece).
    if ([driveItemKind, driveBodyKind, driveMigrationKind].includes(meta.kind as string))
      refuse('begin_items');
  }
  const target = repository.publicationTarget(),
    key = jobPrefix + 'begin_' + (await digest(encoder.encode(target.device_id))),
    saved = await journal.read(key);
  let epoch: string;
  if (saved === null) {
    epoch = crypto.randomUUID();
    const raw = encoder.encode(JSON.stringify({ v: 1, scope: repository.scope, epoch, target }));
    try {
      await journal.write(key, raw);
    } finally {
      raw.fill(0);
    }
  } else {
    try {
      const work = fields(
        JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(saved)) as unknown,
      );
      if (
        work.v !== 1 ||
        !equal(storageScope(work.scope), repository.scope) ||
        !isMigrationEpoch(work.epoch)
      )
        invalid();
      repository.publicationTarget(work.target as StoragePublicationTarget);
      epoch = work.epoch;
    } finally {
      saved.fill(0);
    }
  }
  let state: LegacyMigrationState;
  try {
    state = decodeLegacyMigrationState(
      await source.beginPrivateStorageMigration(repository.scope, epoch, signal),
      repository.scope,
    );
  } catch (error) {
    // Otro cliente (otra pestaña, un nativo) congeló la fuente un instante antes
    // con su época: el servidor rechaza la nuestra. Esa época se importa igual que
    // si la hubiéramos encontrado ya congelada al abrir.
    signal.throwIfAborted();
    const current = decodeLegacyMigrationState(
      await source.privateStorageState(repository.scope, signal),
      repository.scope,
    );
    if (current.mode === 'frozen' && current.epoch && current.epoch !== epoch) return current;
    throw error;
  }
  if (state.mode !== 'frozen' || state.epoch !== epoch) refuse('begin_state');
  return state;
}
async function complete(
  host: DriveMigrationHost,
  state: LegacyMigrationState,
  inventory: string,
  jobPrefix: string,
  signal: AbortSignal,
) {
  const repository = host.repository,
    { journal } = repository.writeContext(),
    object = 'drive_migration_' + (await digest(encoder.encode(state.epoch))),
    payload = encoder.encode(
      JSON.stringify({
        kind: driveMigrationKind,
        epoch: state.epoch,
        rows: state.rows,
        blobs: state.blobs,
        inventory_sha256: inventory,
      }),
    );
  try {
    if (await host.completed(state, inventory, signal)) {
      await cleanupCompletion(host, state, jobPrefix, signal);
      return;
    }
    const target = repository.publicationTarget(),
      key =
        jobPrefix +
        'complete_' +
        (await digest(encoder.encode(state.epoch + '\0' + target.device_id))),
      saved = await journal.read(key);
    let revision: string,
      selected = target,
      content: ReturnType<typeof storageReference> | undefined,
      parents: ReturnType<typeof storageReference>[],
      catalog: ReturnType<typeof storageReference> | undefined;
    if (saved === null) {
      revision = crypto.randomUUID();
      parents = (await repository.catalog(signal))
        .filter((v) => v.object_id === object)
        .map((v) => v.reference);
    } else {
      try {
        const work = fields(
          JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(saved)) as unknown,
        );
        if (
          work.v !== 1 ||
          work.epoch !== state.epoch ||
          !equal(storageScope(work.scope), repository.scope) ||
          work.raw !== encodeStorageJSON(payload) ||
          !storageIdentifier(work.revision) ||
          !Array.isArray(work.parents) ||
          work.parents.length > 16
        )
          invalid();
        revision = work.revision;
        selected = repository.publicationTarget(work.target as StoragePublicationTarget);
        parents = work.parents.map(storageReference);
        if (new Set(parents.map((r) => JSON.stringify(r))).size !== parents.length) invalid();
        if (work.content != null) content = storageReference(work.content);
        if (work.catalog != null) catalog = storageReference(work.catalog);
        if (catalog && !content) invalid();
      } finally {
        saved.fill(0);
      }
    }
    const save = async () => {
      signal.throwIfAborted();
      const raw = encoder.encode(
        JSON.stringify({
          v: 1,
          scope: repository.scope,
          epoch: state.epoch,
          raw: encodeStorageJSON(payload),
          target: selected,
          revision,
          parents,
          ...(content ? { content } : {}),
          ...(catalog ? { catalog } : {}),
        }),
      );
      try {
        await journal.write(key, raw);
      } finally {
        raw.fill(0);
      }
    };
    if (saved === null) await save();
    if (!content) {
      content = await repository.putRecord(
        revision + '_content',
        createStorageRecord(repository.scope, object, revision, 'content', payload),
        signal,
        selected,
      );
      await save();
    }
    if (!catalog) {
      catalog = (
        await repository.putObject(
          revision + '_index',
          object,
          revision + '_index',
          {
            kind: storageObjectKind,
            state: 'live',
            metadata: encodeStorageJSON(payload),
            content: { kind: 'record', reference: content, revision_id: revision },
            parents,
          },
          signal,
          selected,
        )
      ).reference;
      await save();
    }
    if (!(await host.completed(state, inventory, signal))) refuse('receipt');
    await journal.delete(key);
  } finally {
    payload.fill(0);
  }
}
async function cleanupCompletion(
  host: DriveMigrationHost,
  state: LegacyMigrationState,
  jobPrefix: string,
  signal: AbortSignal,
) {
  const repository = host.repository,
    { journal } = repository.writeContext(),
    target = repository.publicationTarget(),
    key =
      jobPrefix +
      'complete_' +
      (await digest(encoder.encode(state.epoch + '\0' + target.device_id))),
    saved = await journal.read(key);
  if (saved === null) return;
  try {
    const work = fields(
        JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(saved)) as unknown,
      ),
      raw = storageJSONBytes(work.raw, 8 << 10);
    try {
      const proof = json(raw);
      if (
        work.v !== 1 ||
        work.epoch !== state.epoch ||
        !equal(storageScope(work.scope), repository.scope) ||
        !storageIdentifier(work.revision) ||
        proof.kind !== driveMigrationKind ||
        proof.epoch !== state.epoch ||
        proof.rows !== state.rows ||
        proof.blobs !== state.blobs ||
        !hash(proof.inventory_sha256)
      )
        invalid();
      repository.publicationTarget(work.target as StoragePublicationTarget);
      if (!(await host.completed(state, proof.inventory_sha256, signal))) invalid();
    } finally {
      raw.fill(0);
    }
    await journal.delete(key);
  } finally {
    saved.fill(0);
  }
}
/** Caller holds the SDK catalog queue. Source originals are retained; no retirement endpoint is called. */
export async function migrateDriveSource(
  host: DriveMigrationHost,
  source: DriveMigrationSource,
  signal: AbortSignal,
): Promise<LegacyMigrationState> {
  const repository = host.repository;
  await repository.prepare(signal);
  let state = decodeLegacyMigrationState(
    await source.privateStorageState(repository.scope, signal),
    repository.scope,
  );
  const jobPrefix = await prefix(host);
  if (state.mode === 'legacy') state = await begin(host, source, jobPrefix, signal);
  if (await host.completed(state, undefined, signal)) {
    // Dos importaciones que terminaron a la vez dejan dos copias de algunas filas
    // (y dos recibos): abren, pero esas filas darían conflicto. Se juntan y se
    // vuelve a cargar; sin copias repetidas no se lee nada más.
    if (state.epoch && host.duplicated()) {
      await mergeImportCopies(host, state.epoch, await repository.catalog(signal), signal);
      if (!(await host.completed(state, undefined, signal))) refuse('receipt');
    }
    await cleanupCompletion(host, state, jobPrefix, signal);
    return state;
  }
  if (state.mode !== 'frozen' || !state.epoch) refuse('source_mode');
  let catalog = await repository.catalog(signal);
  // Dos clientes que importaron a la vez esta época dejan dos copias de las mismas
  // filas: se juntan antes de seguir.
  if (
    duplicated(groupHeads(catalog)) &&
    (await mergeImportCopies(host, state.epoch, catalog, signal))
  )
    catalog = await repository.catalog(signal);
  const heads = groupHeads(catalog);
  const hasher = await createSHA256();
  hasher.update(encoder.encode('org.sytue.drive.inventory.v1\0' + state.epoch + '\0'));
  let after: LegacySourceCursor | undefined,
    rows = 0,
    blobs = 0,
    roots = 0;
  try {
    for (;;) {
      const page = decodeLegacySourcePage(
        await source.privateStorageSourcePage(repository.scope, state.epoch, after, signal),
        repository.scope,
        state.epoch,
        after,
      );
      for (const row of page.rows) {
        const item = await importRow(host, source, row, state.epoch, jobPrefix, heads, signal);
        hasher.update(encoder.encode(row.kind + '\0' + row.id + '\0' + item.raw_sha256 + '\0'));
        rows++;
        if (binary(item.source_kind)) blobs++;
        if (item.source_kind === 'folder' && item.is_root && item.deleted_at === null) roots++;
        if (rows > state.rows || blobs > state.blobs) refuse('inventory');
        after = { kind: row.kind, id: row.id };
      }
      if (!page.next) break;
    }
    if (rows !== state.rows || blobs !== state.blobs) refuse('inventory');
    if (roots !== 1) refuse('roots');
    // Otra importación de esta época que corría a la vez deja sus copias junto a
    // las nuestras: se juntan si son idénticas. Una divergente sigue siendo un
    // conflicto explícito, nunca gana el reloj.
    catalog = await repository.catalog(signal);
    if (
      duplicated(groupHeads(catalog)) &&
      (await mergeImportCopies(host, state.epoch, catalog, signal))
    )
      catalog = await repository.catalog(signal);
    const seen = new Set<string>();
    for (const version of catalog) {
      const meta = json(storageJSONBytes(version.commit.metadata, 8 << 10));
      if (meta.kind === driveItemKind) {
        if (seen.has(version.object_id)) refuse('duplicate_items');
        seen.add(version.object_id);
      }
    }
    await complete(host, state, hasher.digest('hex'), jobPrefix, signal);
    return state;
  } finally {
    hasher.init().update(new Uint8Array(256)).digest();
  }
}
