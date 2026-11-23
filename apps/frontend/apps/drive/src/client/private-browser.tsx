/* SPDX-License-Identifier: Apache-2.0 */
import { createContext, type ComponentChildren } from 'preact';
import { useContext, useEffect, useMemo } from 'preact/hooks';
import { HttpClient, nodeResource, type DriveResource, type RawNode } from '@sytue/api-clients';
import { contiene } from '@sytue/i18n';
import { MatrixSecurityError, PrivateDriveError, safeAttachmentType } from '@sytue/offline/matrix';
import { encodeStorageScopeHeader, storageScopeHeader } from '@sytue/config/matrix';
import type { DriveBrowserAPI } from '../components/FileBrowser.js';
import { collectPrivateDriveFile } from '@sytue/offline/vault/drive';
import type { PrivateDriveSession } from './private-drive.js';

function unavailable(): never {
  throw new MatrixSecurityError('unsupported');
}
function text(row: Record<string, unknown>, key: string): string {
  const value = row[key];
  if (typeof value !== 'string' || !value) throw new MatrixSecurityError('reference');
  return value;
}
function resource(row: Record<string, unknown>, path: string, directory: boolean): DriveResource {
  const value: RawNode = {
    id: text(row, 'id'),
    name:
      directory && path === '/' && row.is_root === true && row.name === ''
        ? '/'
        : text(row, 'name'),
    path,
    is_directory: directory,
  };
  if (typeof row.size_bytes === 'number') value.size_bytes = row.size_bytes;
  if (typeof row.content_type === 'string') value.content_type = row.content_type;
  if (typeof row.updated_at === 'string') value.updated_at = row.updated_at;
  if (
    row.label &&
    typeof row.label === 'object' &&
    'name' in row.label &&
    typeof row.label.name === 'string'
  )
    value.label = {
      name: row.label.name,
      block_external_share:
        'block_external_share' in row.label && row.label.block_external_share === true,
    };
  return nodeResource(value);
}
const join = (parent: string, name: string) => (parent === '/' ? '/' + name : parent + '/' + name);
const parentPath = (path: string) => path.slice(0, path.lastIndexOf('/')) || '/';
function parts(path: string): string[] {
  if (!path.startsWith('/') || /[\0\r\n\\]/.test(path)) throw new MatrixSecurityError('reference');
  const names = path.split('/').filter(Boolean);
  if (names.some((name) => name === '.' || name === '..'))
    throw new MatrixSecurityError('reference');
  return names;
}

/** Per-presenter data port. Unsupported content operations never reach a BFF. */
export class PrivateDriveBrowser {
  readonly api: DriveBrowserAPI;
  private readonly lifetime = new AbortController();
  private readonly urls = new Set<string>();
  private readonly signal: AbortSignal;
  private readonly controls: HttpClient;
  constructor(
    readonly session: PrivateDriveSession,
    baseURL: string,
    fetchImpl?: typeof fetch,
  ) {
    this.signal = AbortSignal.any([session.signal, this.lifetime.signal]);
    this.controls = new HttpClient({
      baseUrl: baseURL + '/api',
      fetchImpl,
      defaultHeaders: { [storageScopeHeader]: encodeStorageScopeHeader(session.storage.scope) },
    });
    this.api = {
      list: (path, cursor) => this.list(path, cursor),
      createFolder: async (path) => {
        const names = parts(path),
          name = names.at(-1);
        if (!name) unavailable();
        const parent = await this.resolve(parentPath(path));
        const row = await session.repository.createFolder(parent.id!, name, this.signal);
        return { resource: resource(row, join(parent.path, text(row, 'name')), true) };
      },
      rename: async (path, name) => {
        const item = await this.resolve(path);
        await session.repository.rename(
          item.isDirectory ? 'folder' : 'file',
          item.id!,
          name,
          this.signal,
        );
        return { ok: true };
      },
      move: async (path, target) => {
        const item = await this.resolve(path),
          parent = await this.resolve(parentPath(target));
        if (parts(target).at(-1) !== item.name || !parent.isDirectory) unavailable();
        await session.repository.move(
          item.isDirectory ? 'folder' : 'file',
          item.id!,
          parent.id!,
          this.signal,
        );
        return { ok: true };
      },
      remove: async (path) => {
        const item = await this.resolve(path);
        await session.repository.delete(
          item.isDirectory ? 'folder' : 'file',
          item.id!,
          this.signal,
        );
        return { ok: true };
      },
      restore: async (id, kind) => {
        this.check();
        await session.repository.restore(kind, id, this.signal);
        return { ok: true };
      },
      upload: async (path, file, progress) => {
        const name = parts(path).at(-1);
        if (!name) unavailable();
        const parent = await this.resolve(parentPath(path));
        if (!parent.isDirectory) unavailable();
        progress?.(0);
        const row = await session.repository.upload(
          parent.id!,
          name,
          file.type || 'application/octet-stream',
          file,
          this.signal,
        );
        this.check();
        progress?.(1);
        return { resource: resource(row, join(parent.path, text(row, 'name')), false) };
      },
      quota: () => this.controls.json({ path: '/quota', signal: this.signal }),
      myGroups: async () =>
        (await this.controls.json<{ groups: string[] }>({ path: '/groups', signal: this.signal }))
          .groups,
      sensitivityLabels: () => this.controls.json({ path: '/label', signal: this.signal }),
      batch: unavailable,
      newDoc: unavailable,
      importBases: unavailable,
      setLabel: unavailable,
      zipHref: unavailable,
      copy: unavailable,
      createShare: unavailable,
      shareWithPerson: unavailable,
      shareWithGroup: unavailable,
      listShares: unavailable,
      revokeShare: unavailable,
      listTrash: () => this.trash(),
      purge: unavailable,
      recent: (cursor) => this.recent(cursor),
      favorites: () => this.favorites(),
      setFavorite: async (item, on) => {
        if (!item.id) unavailable();
        await session.repository.setFavorite(
          item.isDirectory ? 'folder' : 'file',
          item.id,
          on,
          this.scope(),
        );
        this.check();
        session.events.dispatchEvent(new Event('change'));
        return { ok: true };
      },
      sharedWithMe: unavailable,
      sharedList: unavailable,
      sharedUpload: unavailable,
    };
  }
  private check(): void {
    this.signal.throwIfAborted();
    if (this.session.client.isClosed) throw new MatrixSecurityError('device');
  }
  private scope(child?: AbortSignal): AbortSignal {
    this.check();
    const signal = child ? AbortSignal.any([this.signal, child]) : this.signal;
    signal.throwIfAborted();
    return signal;
  }
  close(): void {
    this.lifetime.abort();
    for (const url of this.urls) URL.revokeObjectURL(url);
    this.urls.clear();
  }
  async resolve(path: string, child?: AbortSignal): Promise<DriveResource> {
    this.check();
    const signal = this.scope(child);
    const names = parts(path),
      root = await this.session.repository.root(signal);
    let current = resource(root, '/', true);
    for (const name of names) {
      if (!current.isDirectory) throw new MatrixSecurityError('reference');
      let found: DriveResource | undefined, cursor: string | undefined;
      do {
        const page = await this.listChildren(current, cursor, signal);
        for (const item of page.items)
          if (item.name === name) {
            if (found) throw new MatrixSecurityError('reference');
            found = item;
          }
        cursor = page.nextCursor;
      } while (cursor);
      if (!found) throw new PrivateDriveError('not_found');
      current = found;
    }
    this.check();
    signal.throwIfAborted();
    return current;
  }
  async byID(kind: 'file' | 'folder', id: string, child?: AbortSignal): Promise<DriveResource> {
    this.check();
    const signal = this.scope(child);
    const row =
      kind === 'file'
        ? await this.session.repository.file(id, signal)
        : await this.session.repository.folder(id, signal);
    const names: string[] = [],
      seen = new Set<string>();
    let current = row,
      currentKind = kind;
    for (;;) {
      const currentID = text(current, 'id');
      if (seen.has(currentID)) throw new MatrixSecurityError('reference');
      seen.add(currentID);
      if (currentKind === 'folder' && current.is_root === true) break;
      names.unshift(text(current, 'name'));
      current = await this.session.repository.folder(
        text(current, currentKind === 'file' ? 'folder_id' : 'parent_id'),
        signal,
      );
      currentKind = 'folder';
    }
    this.check();
    signal.throwIfAborted();
    return resource(row, '/' + names.join('/'), kind === 'folder');
  }
  private async listChildren(folder: DriveResource, cursor?: string, signal = this.signal) {
    this.check();
    if (!folder.isDirectory || !folder.id) throw new MatrixSecurityError('reference');
    const page = await this.session.repository.children(folder.id, cursor, signal);
    this.check();
    signal.throwIfAborted();
    // Folders accompany every engine page; expose them only on the first UI page.
    const items = [
      ...(cursor
        ? []
        : page.folders.map((row) => resource(row, join(folder.path, text(row, 'name')), true))),
      ...page.files.map((row) => resource(row, join(folder.path, text(row, 'name')), false)),
    ];
    return { items, ...(page.next_cursor ? { nextCursor: page.next_cursor } : {}) };
  }
  async list(path: string, cursor?: string, child?: AbortSignal) {
    const signal = this.scope(child);
    return this.listChildren(await this.resolve(path, signal), cursor, signal);
  }
  isConflict(error: unknown): boolean {
    return error instanceof PrivateDriveError && error.reason === 'conflict';
  }
  async trash(child?: AbortSignal) {
    const signal = this.scope(child),
      entries = await this.session.repository.trash(signal);
    this.check();
    signal.throwIfAborted();
    return entries.map(({ kind, row, path }) => {
      const item = resource(row, path, kind === 'folder');
      return {
        id: text(row, 'id'),
        name: item.name,
        isDirectory: item.isDirectory,
        ...(item.size === undefined ? {} : { size: item.size }),
        deletedAt: text(row, 'deleted_at'),
      };
    });
  }
  async recent(cursor?: string, child?: AbortSignal) {
    const signal = this.scope(child),
      page = await this.session.repository.recent(cursor, signal);
    this.check();
    signal.throwIfAborted();
    return {
      items: page.items.map(({ row, path }) => resource(row, path, false)),
      ...(page.next_cursor ? { nextCursor: page.next_cursor } : {}),
    };
  }
  async versions(id: string, cursor?: string, child?: AbortSignal) {
    const signal = this.scope(child),
      page = await this.session.repository.versions(id, cursor, signal);
    this.check();
    signal.throwIfAborted();
    return {
      versions: page.versions,
      ...(page.next_cursor ? { nextCursor: page.next_cursor } : {}),
    };
  }
  async restoreVersion(id: string, version: number, child?: AbortSignal): Promise<void> {
    const signal = this.scope(child);
    await this.session.repository.restoreVersion(id, version, signal);
    this.check();
    signal.throwIfAborted();
    this.session.events.dispatchEvent(new Event('change'));
  }
  async favorites(child?: AbortSignal): Promise<DriveResource[]> {
    const signal = this.scope(child),
      entries = await this.session.repository.favorites(signal);
    this.check();
    signal.throwIfAborted();
    return entries.map(({ kind, row, path }) => resource(row, path, kind === 'folder'));
  }
  async search(query: string, child?: AbortSignal): Promise<DriveResource[]> {
    this.check();
    const signal = this.scope(child);
    if (!query.trim()) return [];
    const pending = [await this.resolve('/', signal)],
      seen = new Set<string>(),
      result: DriveResource[] = [];
    for (let index = 0; index < pending.length; index++) {
      const folder = pending[index]!;
      if (!folder.id || seen.has(folder.id)) throw new MatrixSecurityError('reference');
      seen.add(folder.id);
      let cursor: string | undefined;
      do {
        const page = await this.listChildren(folder, cursor, signal);
        for (const item of page.items) {
          if (contiene(item.name, query)) result.push(item);
          if (item.isDirectory) pending.push(item);
        }
        cursor = page.nextCursor;
      } while (cursor);
    }
    this.check();
    signal.throwIfAborted();
    return result;
  }
  /** Bytes reach a preview/download only after the complete authenticated commit. */
  async blob(
    item: DriveResource,
    maximum = 128 * 1024 * 1024,
    child?: AbortSignal,
    version?: number,
  ): Promise<Blob> {
    this.check();
    const signal = this.scope(child);
    if (!item.id || item.isDirectory) unavailable();
    const file =
      version === undefined
        ? await this.session.repository.file(item.id, signal)
        : (
            await this.session.repository.versions(item.id, String(version + 1), signal)
          ).versions.find((v) => v.version === version);
    if (!file) throw new PrivateDriveError('not_found');
    const blob = await collectPrivateDriveFile({
      size: file.size_bytes,
      maximum,
      type: safeAttachmentType(typeof file.content_type === 'string' ? file.content_type : ''),
      check: () => this.check(),
      run: (sink) =>
        version === undefined
          ? this.session.repository.download(item.id!, sink, signal)
          : this.session.repository.downloadVersion(item.id!, version, sink, signal),
    });
    signal.throwIfAborted();
    return blob;
  }
  async blobURL(
    item: DriveResource,
    maximum?: number,
    child?: AbortSignal,
    version?: number,
  ): Promise<string> {
    const blob = await this.blob(item, maximum, child, version);
    this.scope(child).throwIfAborted();
    this.check();
    const url = URL.createObjectURL(blob);
    this.urls.add(url);
    return url;
  }
  releaseURL(url: string): void {
    URL.revokeObjectURL(url);
    this.urls.delete(url);
  }
  async download(item: DriveResource, child?: AbortSignal, version?: number): Promise<void> {
    const url = await this.blobURL(item, undefined, child, version);
    try {
      this.scope(child).throwIfAborted();
    } catch (error) {
      this.releaseURL(url);
      throw error;
    }
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = item.name;
    anchor.rel = 'noopener';
    anchor.click();
    setTimeout(() => this.releaseURL(url), 60000);
  }
}
const PrivateDriveContext = createContext<PrivateDriveBrowser | null>(null);
export function PrivateDriveProvider({
  session,
  baseURL,
  children,
}: {
  session: PrivateDriveSession;
  baseURL: string;
  children: ComponentChildren;
}) {
  const data = useMemo(() => new PrivateDriveBrowser(session, baseURL), [session, baseURL]);
  useEffect(() => () => data.close(), [data]);
  return <PrivateDriveContext.Provider value={data}>{children}</PrivateDriveContext.Provider>;
}
export function useOptionalPrivateDrive(): PrivateDriveBrowser | null {
  return useContext(PrivateDriveContext);
}
export function usePrivateDrive(): PrivateDriveBrowser {
  const value = useContext(PrivateDriveContext);
  if (!value) throw new MatrixSecurityError('device');
  return value;
}
