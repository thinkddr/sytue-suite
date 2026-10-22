/*
 * Persistencia offline compartida por Docs, Sheets y Slides.
 *
 * Vive junto al editor web porque los tres clientes (navegador, Capacitor y
 * Wails) ejecutan exactamente este código. Los binarios nativos no mantienen
 * una segunda representación del documento ni una segunda cola de sync.
 */
import type { DocKind, DocumentMeta } from '@sytue/api-clients';
import { listOfflineValues, mutateOfflineValue, readOfflineValue } from '@sytue/offline';
import { contiene } from '@sytue/i18n';

const NAMESPACE = 'docs.documents';

export interface OfflineDocument {
  documentId: string;
  meta: DocumentMeta;
  content: string;
  /**
   * Con `contentDirty`: el contenido del que partió el borrador. Hojas y
   * presentaciones lo usan de ancestro para fusionar el borrador con lo que
   * otros escribieron mientras tanto (state/fusion.ts). Ausente en copias viejas.
   */
  base?: string | undefined;
  lamport: number;
  seq: number;
  contentDirty: boolean;
  titleDirty: boolean;
  cachedAt: number;
}

async function mutate(
  ownerId: string,
  documentId: string,
  update: (previous: OfflineDocument | undefined) => OfflineDocument | undefined,
): Promise<void> {
  await mutateOfflineValue<OfflineDocument>(
    ownerId,
    NAMESPACE,
    documentId,
    (previous) => update(previous ?? undefined) ?? null,
  );
}

/** Devuelve la última copia local, incluyendo cambios todavía no sincronizados. */
export async function readOfflineDocument(
  ownerId: string,
  documentId: string,
): Promise<OfflineDocument | null> {
  return readOfflineValue<OfflineDocument>(ownerId, NAMESPACE, documentId);
}

/** Adopta una lectura del servidor sin pisar un borrador offline pendiente. */
export async function cacheServerDocument(
  ownerId: string,
  meta: DocumentMeta,
  content: string,
  lamport: number,
  seq: number,
): Promise<void> {
  await mutate(ownerId, meta.id, (previous) => ({
    documentId: meta.id,
    meta: previous?.titleDirty ? { ...meta, title: previous.meta.title } : meta,
    content: previous?.contentDirty ? previous.content : content,
    base: previous?.contentDirty ? previous.base : content,
    lamport: Math.max(previous?.lamport ?? 0, lamport),
    seq: Math.max(previous?.seq ?? 0, seq),
    contentDirty: previous?.contentDirty ?? false,
    titleDirty: previous?.titleDirty ?? false,
    cachedAt: Date.now(),
  }));
}

/** Persiste cada edición antes de intentar enviarla. */
export async function cacheLocalContent(
  ownerId: string,
  meta: DocumentMeta,
  content: string,
  lamport: number,
  seq: number,
): Promise<void> {
  await mutate(ownerId, meta.id, (previous) => ({
    documentId: meta.id,
    meta: previous?.meta ?? meta,
    content,
    // La primera edición sin guardar fija la base: lo limpio que había antes.
    base: previous?.contentDirty ? previous.base : previous?.content,
    lamport: Math.max(previous?.lamport ?? 0, lamport),
    seq: Math.max(previous?.seq ?? 0, seq),
    contentDirty: true,
    titleDirty: previous?.titleDirty ?? false,
    cachedAt: Date.now(),
  }));
}

export async function markContentSynced(
  ownerId: string,
  meta: DocumentMeta,
  content: string,
  lamport: number,
  seq: number,
): Promise<void> {
  await mutate(ownerId, meta.id, (previous) => ({
    documentId: meta.id,
    meta: previous?.titleDirty ? { ...meta, title: previous.meta.title } : meta,
    content,
    base: content,
    lamport,
    seq,
    contentDirty: false,
    titleDirty: previous?.titleDirty ?? false,
    cachedAt: Date.now(),
  }));
}

export async function cacheLocalTitle(
  ownerId: string,
  meta: DocumentMeta,
  title: string,
): Promise<void> {
  await mutate(ownerId, meta.id, (previous) => ({
    documentId: meta.id,
    meta: { ...(previous?.meta ?? meta), title },
    content: previous?.content ?? '',
    base: previous?.base,
    lamport: previous?.lamport ?? 0,
    seq: previous?.seq ?? 0,
    contentDirty: previous?.contentDirty ?? false,
    titleDirty: true,
    cachedAt: Date.now(),
  }));
}

export async function markTitleSynced(
  ownerId: string,
  documentId: string,
  title: string,
): Promise<void> {
  await mutate(ownerId, documentId, (previous) =>
    previous === undefined
      ? undefined
      : {
          ...previous,
          meta: { ...previous.meta, title },
          titleDirty: false,
          cachedAt: Date.now(),
        },
  );
}

/** Lista solo documentos abiertos alguna vez: son los que tienen contenido local. */
export async function listOfflineDocuments(
  ownerId: string,
  options?: { kind?: DocKind; query?: string },
): Promise<DocumentMeta[]> {
  const records = await listOfflineValues<OfflineDocument>(ownerId, NAMESPACE);
  const query = options?.query?.trim().toLocaleLowerCase() ?? '';
  return records
    .map((record) => record.value)
    .filter((document) => !options?.kind || document.meta.kind === options.kind)
    .filter((document) => !query || contiene(document.meta.title, query))
    .sort((a, b) => b.cachedAt - a.cachedAt)
    .map((document) => document.meta);
}
