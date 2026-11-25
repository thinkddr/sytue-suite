/* SPDX-License-Identifier: Apache-2.0 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  BotonFlotante,
  Button,
  EmptyState,
  Input,
  MenuItem,
  Migas,
  Modal,
  ShortcutsModal,
  Spinner,
  marcarAppLista,
  useToast,
} from '@sytue/ui';
import { useI18n } from '@sytue/i18n/preact';
import type { DriveResource } from '@sytue/api-clients';
import { MatrixSecurityError, PrivateDriveError } from '@sytue/offline/matrix';
import { usePrivateDrive } from '../client/private-browser.js';
import { useT, type DriveMessageKey } from '../i18n/index.js';
import { accionDeEvento, gruposDeAtajos, type AccionAtajo } from '../lib/atajos.js';
import { FILTROS_TIPO, filtrarPorTipo, migasDe, vecinoDe, type FiltroTipo } from '../lib/tipos.js';
import { Glyph } from './glyphs.js';
import { VersionsDialog } from './VersionsDialog.js';
import {
  collectDropEntries,
  FileList,
  MoveDialog,
  previewKind,
  TrashView,
  type UploadEntry,
} from './FileBrowser.js';

/** Los atajos que esta vista atiende: buscar, subir, nueva carpeta y la hoja. Los de la
 *  selección (Supr, F2, ⌘A) no, porque aquí no se selecciona. */
const ACCIONES_BOVEDA: readonly AccionAtajo[] = ['search', 'upload', 'newFolder', 'help'];

const ROTULO_TIPO: Record<FiltroTipo, DriveMessageKey> = {
  todo: 'files.type.all',
  carpetas: 'files.type.folders',
  documentos: 'files.type.documents',
  pdf: 'files.type.pdf',
  imagenes: 'files.type.images',
  media: 'files.type.media',
};

const parent = (path: string) => path.slice(0, path.lastIndexOf('/')) || '/';
const join = (path: string, name: string) => (path === '/' ? '/' + name : path + '/' + name);
function remember(item: DriveResource) {
  const url = new URL(window.location.href);
  for (const key of ['path', 'preview', 'file', 'folder']) url.searchParams.delete(key);
  if (item.path !== '/' && item.id)
    url.searchParams.set(item.isDirectory ? 'folder' : 'file', item.id);
  window.history.replaceState(null, '', url);
}
/** The familiar file rows, backed exclusively by this account's verified private repository. */
export function PrivateFileBrowser({
  section = 'my',
  onOpenFolder,
}: {
  section?: string;
  onOpenFolder?: () => void;
}) {
  const [request, setRequest] = useState(() =>
    typeof window === 'undefined'
      ? null
      : new URL(window.location.href).searchParams.get('request'),
  );
  if (request !== null)
    return (
      <PrivateAccessRequest
        onBack={() => {
          const url = new URL(window.location.href);
          for (const key of ['request', 'path', 'preview', 'file', 'folder'])
            url.searchParams.delete(key);
          url.searchParams.set('section', 'my');
          window.history.replaceState(null, '', url);
          setRequest(null);
          onOpenFolder?.();
        }}
      />
    );
  return section === 'trash' ? (
    <TrashView />
  ) : (
    <PrivateFilesContent key={section} section={section} onOpenFolder={onOpenFolder} />
  );
}
/** Legacy notification links remain explicit while encrypted sharing is pending.
 * The request ID is never treated as a file, a name or an approval capability. */
function PrivateAccessRequest({ onBack }: { onBack: () => void }) {
  const { locale } = useI18n(),
    t = useT(locale);
  return (
    <div class="flex flex-col items-center gap-4 px-6 py-8 text-center">
      <h1 class="sytue-h3">{t('access.request.title')}</h1>
      <p class="text-text-muted max-w-sm">{t('access.request.unavailable')}</p>
      <Button onClick={onBack}>{t('nav.myDrive')}</Button>
    </div>
  );
}
function PrivateFilesContent({
  section,
  onOpenFolder,
}: {
  section: string;
  onOpenFolder?: (() => void) | undefined;
}) {
  const data = usePrivateDrive(),
    { locale } = useI18n(),
    t = useT(locale),
    toast = useToast();
  const [initial] = useState(() =>
    typeof window === 'undefined'
      ? new URLSearchParams()
      : new URL(window.location.href).searchParams,
  );
  const [path, setPath] = useState('/'),
    [initialized, setInitialized] = useState(false);
  const [initialFailed, setInitialFailed] = useState(false),
    [initialAttempt, setInitialAttempt] = useState(0);
  const [items, setItems] = useState<DriveResource[] | null>(null),
    [cursor, setCursor] = useState<string | undefined>();
  const [favoriteIds, setFavoriteIds] = useState<ReadonlySet<string>>(new Set());
  const [failed, setFailed] = useState(false),
    [busy, setBusy] = useState(false),
    [more, setMore] = useState(false);
  const [query, setQuery] = useState(''),
    [search, setSearch] = useState(''),
    [revision, setRevision] = useState(0);
  // «Filtrar por tipo» (lib/tipos.ts): sobre lo cargado, sin otra consulta.
  const [tipo, setTipo] = useState<FiltroTipo>('todo');
  const [atajosOpen, setAtajosOpen] = useState(false);
  // `sytue:app-ready`: la carpeta ya enseña sus archivos (o por qué no).
  useEffect(() => {
    if (items !== null || failed) marcarAppLista();
  }, [items, failed]);
  const [preview, setPreview] = useState<DriveResource | null>(null),
    [moving, setMoving] = useState<DriveResource | null>(null);
  const [versions, setVersions] = useState<DriveResource | null>(null);
  const [edit, setEdit] = useState<{ kind: 'create' | 'rename'; item?: DriveResource } | null>(
      null,
    ),
    [name, setName] = useState('');
  const requestsRef = useRef<AbortSignal | null>(null);
  const fetchingRef = useRef(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const reload = () => setRevision((value) => value + 1);
  useEffect(() => {
    const controller = new AbortController(),
      signal = controller.signal;
    setInitialFailed(false);
    setFailed(false);
    void (async () => {
      try {
        // Flat views must not reopen a stale folder or file from another section.
        if (section !== 'my') {
          setInitialized(true);
          return;
        }
        const file = initial.get('file'),
          folder = initial.get('folder');
        const item = file
          ? await data.byID('file', file, signal)
          : folder
            ? await data.byID('folder', folder, signal)
            : await data.resolve(initial.get('path') || '/', signal);
        if (signal.aborted) return;
        setPath(item.isDirectory ? item.path : parent(item.path));
        if (!item.isDirectory) setPreview(item);
        else if (initial.get('preview'))
          setPreview(await data.resolve(initial.get('preview')!, signal));
        if (signal.aborted) return;
        remember(item);
        setInitialized(true);
      } catch {
        if (!signal.aborted) {
          setFailed(true);
          setInitialFailed(true);
        }
      }
    })();
    return () => {
      controller.abort();
    };
  }, [data, initial, initialAttempt, section]);
  useEffect(() => {
    const timer = setTimeout(() => setSearch(query), 300);
    return () => clearTimeout(timer);
  }, [query]);
  useEffect(() => {
    const refresh = () => setRevision((value) => value + 1);
    data.session.events.addEventListener('change', refresh);
    return () => data.session.events.removeEventListener('change', refresh);
  }, [data]);
  useEffect(() => {
    if (!initialized) return;
    const controller = new AbortController();
    const signal = AbortSignal.any([controller.signal, data.session.signal]);
    requestsRef.current = signal;
    setItems(null);
    setCursor(undefined);
    setFailed(false);
    fetchingRef.current = false;
    setMore(false);
    void (async () => {
      try {
        const favorites = await data.favorites(signal);
        let page: { items: DriveResource[]; nextCursor?: string };
        if (section === 'my')
          page = search.trim()
            ? { items: await data.search(search, signal) }
            : await data.list(path, undefined, signal);
        else if (section === 'recent') page = await data.recent(undefined, signal);
        else if (section === 'favorites') page = { items: favorites };
        else throw new MatrixSecurityError('unsupported');
        if (signal.aborted) return;
        setFavoriteIds(new Set(favorites.flatMap((item) => (item.id ? [item.id] : []))));
        setItems(page.items);
        setCursor(page.nextCursor);
      } catch {
        if (!signal.aborted) setFailed(true);
      }
    })();
    return () => {
      controller.abort();
    };
  }, [data, path, section, search, revision, initialized]);
  const loadMore = useCallback(async () => {
    const signal = requestsRef.current;
    if (
      !cursor ||
      !signal ||
      signal.aborted ||
      fetchingRef.current ||
      (section !== 'my' && section !== 'recent') ||
      (section === 'my' && search.trim())
    )
      return;
    fetchingRef.current = true;
    setMore(true);
    try {
      const page =
        section === 'recent'
          ? await data.recent(cursor, signal)
          : await data.list(path, cursor, signal);
      if (!signal.aborted) {
        setItems((old) => [...(old ?? []), ...page.items]);
        setCursor(page.nextCursor);
      }
    } catch {
      if (!signal.aborted) toast.notify({ tone: 'danger', title: t('files.loadMoreFailed') });
    } finally {
      if (!signal.aborted) {
        fetchingRef.current = false;
        setMore(false);
      }
    }
  }, [cursor, section, search, data, path, toast, t]);
  // Ir a una carpeta de la ruta desde las migas, saliendo de la búsqueda.
  const irA = (ruta: string) => {
    setPath(ruta);
    setQuery('');
    void data
      .resolve(ruta)
      .then(remember)
      .catch(() => toast.notify({ tone: 'danger', title: t('files.error.loadTitle') }));
  };
  // «Mostrar en su carpeta» de un resultado de búsqueda, de Recientes o de Destacados:
  // la carpeta que lo contiene, abierta como cualquier otra (y en «Mi unidad»).
  const showInFolder = (item: DriveResource) => {
    void data
      .resolve(parent(item.path))
      .then(open)
      .catch(() => toast.notify({ tone: 'danger', title: t('files.error.loadTitle') }));
  };
  // Los atajos de la hoja de Drive que tienen sentido aquí: buscar, subir, nueva carpeta
  // y la propia hoja. Los de la selección (Supr, F2, ⌘A) no: esta vista no selecciona.
  useEffect(() => {
    const puedeCrear = section === 'my' && initialized && !busy;
    const onKey = (e: KeyboardEvent): void => {
      const accion = accionDeEvento(e, { offline: false });
      if (accion === 'help') setAtajosOpen(true);
      else if (accion === 'search' && section === 'my')
        document.getElementById('drive-private-search')?.focus();
      else if (accion === 'upload' && puedeCrear) inputRef.current?.click();
      else if (accion === 'newFolder' && puedeCrear) {
        setName('');
        setEdit({ kind: 'create' });
      } else return;
      e.preventDefault();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [section, initialized, busy]);
  const visibles = useMemo(() => (items ? filtrarPorTipo(items, tipo) : null), [items, tipo]);
  const download = (item: DriveResource) => {
    void data
      .download(item)
      .catch(() => toast.notify({ tone: 'danger', title: t('preview.failed') }));
  };
  const open = (item: DriveResource) => {
    if (item.isDirectory) {
      remember(item);
      if (section !== 'my') {
        const url = new URL(window.location.href);
        url.searchParams.set('section', 'my');
        window.history.replaceState(null, '', url);
        if (onOpenFolder) onOpenFolder();
        else window.location.assign(url);
        return;
      }
      setPath(item.path);
      setQuery('');
    } else {
      remember(item);
      setPreview(item);
    }
  };
  const toggleFavorite = (item: DriveResource) => {
    if (busy || !item.id) return;
    setBusy(true);
    void data.api
      .setFavorite(item, !favoriteIds.has(item.id))
      .then(reload)
      .catch(() => toast.notify({ tone: 'danger', title: t('files.favorite.failed') }))
      .finally(() => setBusy(false));
  };
  const remove = (item: DriveResource) => {
    if (busy) return;
    setBusy(true);
    void data.api
      .remove(item.path)
      .then(() => {
        reload();
        toast.notify({
          tone: 'success',
          title: t('files.delete.done', { name: item.name }),
          action: {
            label: t('files.delete.undo'),
            onClick: () => {
              if (item.id)
                void data.api
                  .restore(item.id, item.isDirectory ? 'folder' : 'file')
                  .then(reload)
                  .catch(() => toast.notify({ tone: 'danger', title: t('files.delete.failed') }));
            },
          },
        });
      })
      .catch(() => toast.notify({ tone: 'danger', title: t('files.delete.failed') }))
      .finally(() => setBusy(false));
  };
  async function upload(entries: UploadEntry[]) {
    if (busy || !initialized || section !== 'my' || !entries.length) return;
    setBusy(true);
    try {
      const made = new Set<string>();
      for (const entry of entries) {
        let target = path;
        for (const part of entry.dir.split('/').filter(Boolean)) {
          if (part === '.' || part === '..') throw new MatrixSecurityError('reference');
          target = join(target, part);
          if (!made.has(target)) {
            // Existing folders are reused; a file or a conflict never becomes a guessed destination.
            try {
              const found = await data.resolve(target);
              if (!found.isDirectory) throw new MatrixSecurityError('reference');
            } catch (error) {
              if (!(error instanceof PrivateDriveError) || error.reason !== 'not_found')
                throw error;
              await data.api.createFolder(target);
            }
            made.add(target);
          }
        }
        await data.api.upload(join(target, entry.file.name), entry.file);
      }
      reload();
      toast.notify({ tone: 'success', title: t('files.upload.done', { count: entries.length }) });
    } catch {
      toast.notify({ tone: 'danger', title: t('shared.uploadFailed') });
      reload();
    } finally {
      setBusy(false);
    }
  }
  async function save() {
    if (!edit || busy || !name.trim()) return;
    setBusy(true);
    try {
      if (edit.kind === 'create') await data.api.createFolder(join(path, name));
      else if (edit.item) await data.api.rename(edit.item.path, name);
      setEdit(null);
      reload();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t(edit.kind === 'create' ? 'files.newFolder.failed' : 'files.rename.failed'),
      });
    } finally {
      setBusy(false);
    }
  }
  const refresh = () => {
    if (busy || !initialized) return;
    setBusy(true);
    void data.session.repository
      .refresh(data.session.signal)
      .then(reload)
      .catch(() => toast.notify({ tone: 'danger', title: t('files.error.loadTitle') }))
      .finally(() => setBusy(false));
  };
  return (
    <div
      class="flex h-full min-h-[60dvh] flex-col"
      onDragOver={(event) => {
        if (event.dataTransfer?.types.includes('Files')) event.preventDefault();
      }}
      onDrop={(event) => {
        if (!event.dataTransfer?.types.includes('Files')) return;
        event.preventDefault();
        void collectDropEntries(event.dataTransfer)
          .then(upload)
          .catch(() => toast.notify({ tone: 'danger', title: t('shared.uploadFailed') }));
      }}
    >
      <div class="flex flex-wrap items-center justify-between gap-3 p-4">
        {section === 'recent' || section === 'favorites' ? (
          <h1 class="sytue-h3">{t(section === 'recent' ? 'recent.title' : 'favorites.title')}</h1>
        ) : (
          // La ruta entera con las migas de siempre (las de «Mi unidad» del Drive de
          // servidor): antes era «←» y el nombre de la última carpeta, y volver tres
          // niveles eran tres clics a ciegas.
          <Migas
            label={t('files.breadcrumb')}
            class="min-w-0 grow"
            migas={[
              { clave: '/', nombre: t('nav.myDrive'), onIr: () => irA('/') },
              ...migasDe(path).map((m) => ({
                clave: m.ruta,
                nombre: m.nombre,
                onIr: () => irA(m.ruta),
              })),
            ]}
          />
        )}
        <div class="flex flex-wrap gap-2">
          <Button variant="secondary" disabled={busy || !initialized} onClick={refresh}>
            {t('files.retry')}
          </Button>
        </div>
        {/* «Subir» y «Nueva carpeta» son el «+» flotante, abajo a la derecha y en todos
            los anchos, como «Redactar» en Mail: estaban en esta fila y en el teléfono
            quedaban arriba del todo. Solo en «Mi unidad», como estaban. */}
        {section === 'my' ? (
          <BotonFlotante
            label={t('files.new')}
            disabled={busy || !initialized}
            menu={
              <>
                <MenuItem
                  icon={<Glyph name="upload" size={16} />}
                  onClick={() => inputRef.current?.click()}
                >
                  {t('files.upload')}
                </MenuItem>
                <MenuItem
                  icon={<Glyph name="folder" size={16} />}
                  onClick={() => {
                    setName('');
                    setEdit({ kind: 'create' });
                  }}
                >
                  {t('files.newFolder.title')}
                </MenuItem>
              </>
            }
          />
        ) : null}
      </div>
      {section === 'my' ? (
        <div class="px-4 pb-4">
          <Input
            id="drive-private-search"
            value={query}
            aria-label={t('files.searchAria')}
            placeholder={t('files.search')}
            onInput={(event) => setQuery(event.currentTarget.value)}
          />
        </div>
      ) : null}
      {items && (items.length > 0 || tipo !== 'todo') ? (
        <div class="flex flex-wrap gap-1 px-4 pb-3" role="group" aria-label={t('files.type.label')}>
          {FILTROS_TIPO.map((f) => (
            <Button
              key={f}
              variant={tipo === f ? 'secondary' : 'ghost'}
              size="xs"
              aria-pressed={tipo === f}
              onClick={() => setTipo(f)}
            >
              {t(ROTULO_TIPO[f])}
            </Button>
          ))}
        </div>
      ) : null}
      <input
        ref={inputRef}
        type="file"
        multiple
        hidden
        onChange={(event) => {
          const files = Array.from(event.currentTarget.files ?? []);
          event.currentTarget.value = '';
          void upload(files.map((file) => ({ file, dir: '' })));
        }}
      />
      {data.session.uploadFailed ? (
        <p class="text-danger-text px-4 text-sm" role="alert">
          {t('shared.uploadFailed')}{' '}
          <Button
            variant="ghost"
            disabled={busy}
            onClick={() => {
              void data.session.resume().catch(() => {});
            }}
          >
            {t('files.retry')}
          </Button>
        </p>
      ) : null}
      {failed ? (
        <EmptyState
          title={t('files.error.loadTitle')}
          description={t('files.error.network')}
          action={
            <Button
              onClick={() => (initialFailed ? setInitialAttempt((value) => value + 1) : reload())}
            >
              {t('files.retry')}
            </Button>
          }
        />
      ) : !items ? (
        <p role="status" class="flex items-center gap-2 p-6">
          <Spinner size={18} />
          {t('files.loading')}
        </p>
      ) : items.length === 0 && search.trim() ? (
        // Una búsqueda sin resultados no es una carpeta vacía: decía «Esta carpeta está
        // vacía» y no dejaba volver más que borrando a mano.
        <EmptyState
          title={t('files.search.emptyTitle', { query: search.trim() })}
          description={t('files.search.emptyBody')}
          spot="emptySearch"
          action={
            <Button variant="secondary" size="sm" onClick={() => setQuery('')}>
              {t('files.search.clear')}
            </Button>
          }
        />
      ) : items.length === 0 ? (
        <EmptyState
          title={t(section === 'favorites' ? 'favorites.emptyTitle' : 'files.empty.title')}
          description={t(section === 'favorites' ? 'favorites.emptyBody' : 'files.empty.body')}
        />
      ) : visibles && visibles.length === 0 ? (
        <EmptyState
          title={t('files.type.empty')}
          action={
            <Button variant="secondary" size="sm" onClick={() => setTipo('todo')}>
              {t('files.type.all')}
            </Button>
          }
        />
      ) : (
        <FileList
          items={visibles ?? items}
          onShowInFolder={
            section === 'recent' || section === 'favorites' || search.trim()
              ? showInFolder
              : undefined
          }
          showLocation={section === 'recent' || section === 'favorites' || !!search.trim()}
          favoriteIds={favoriteIds}
          onToggleFavorite={busy ? undefined : toggleFavorite}
          hasMore={!!cursor}
          loadingMore={more}
          onLoadMore={() => void loadMore()}
          onOpen={open}
          onDownload={download}
          onRename={(item) => {
            setName(item.name);
            setEdit({ kind: 'rename', item });
          }}
          onMove={setMoving}
          onDelete={remove}
          onVersions={setVersions}
        />
      )}
      {edit ? (
        <Modal
          open
          onClose={() => {
            if (!busy) setEdit(null);
          }}
          title={t(
            edit.kind === 'create'
              ? 'files.newFolder.title'
              : edit.item?.isDirectory
                ? 'files.rename.folderTitle'
                : 'files.rename.fileTitle',
          )}
          footer={
            <>
              <Button variant="secondary" disabled={busy} onClick={() => setEdit(null)}>
                {t('files.cancel')}
              </Button>
              <Button disabled={busy || !name.trim()} onClick={() => void save()}>
                {t(edit.kind === 'create' ? 'files.create' : 'files.rename.cta')}
              </Button>
            </>
          }
        >
          <Input
            value={name}
            aria-label={t(edit.kind === 'create' ? 'files.newFolder.label' : 'files.rename.label')}
            onInput={(event) => setName(event.currentTarget.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault();
                void save();
              }
            }}
          />
        </Modal>
      ) : null}
      {moving ? (
        <MoveDialog
          item={moving}
          onClose={() => setMoving(null)}
          onMoved={() => {
            setMoving(null);
            reload();
          }}
        />
      ) : null}
      {preview ? (
        <PrivatePreview
          item={preview}
          onClose={() => setPreview(null)}
          onDownload={download}
          anterior={vecinoDe(visibles ?? [], preview, -1)}
          siguiente={vecinoDe(visibles ?? [], preview, 1)}
          onCambiar={(item) => {
            remember(item);
            setPreview(item);
          }}
        />
      ) : null}
      <ShortcutsModal
        open={atajosOpen}
        title={t('shortcuts.title')}
        groups={gruposDeAtajos(t, undefined, ACCIONES_BOVEDA)}
        onClose={() => setAtajosOpen(false)}
      />
      {versions ? (
        <VersionsDialog
          item={versions}
          onClose={() => setVersions(null)}
          onRestored={() => {
            setVersions(null);
            reload();
          }}
        />
      ) : null}
    </div>
  );
}
function PrivatePreview({
  item,
  onClose,
  onDownload,
  anterior,
  siguiente,
  onCambiar,
}: {
  item: DriveResource;
  onClose: () => void;
  onDownload: (item: DriveResource) => void;
  /** Los ficheros de al lado en la lista que se ve: ← y → pasan de uno a otro. */
  anterior?: DriveResource | null;
  siguiente?: DriveResource | null;
  onCambiar?: (item: DriveResource) => void;
}) {
  const data = usePrivateDrive(),
    { locale } = useI18n(),
    t = useT(locale),
    kind = previewKind(item);
  const [url, setURL] = useState<string | null>(null),
    [text, setText] = useState<string | null>(null),
    [failed, setFailed] = useState(false);
  useEffect(() => {
    let live = true,
      owned: string | undefined;
    const controller = new AbortController();
    setFailed(false);
    setURL(null);
    setText(null);
    void (async () => {
      try {
        if (kind === 'text') {
          const blob = await data.blob(item, 1_000_000, controller.signal),
            value = new TextDecoder('utf-8', { fatal: true }).decode(await blob.arrayBuffer());
          if (live) setText(value);
        } else if (kind === 'image' || kind === 'pdf') {
          owned = await data.blobURL(item, 32 * 1024 * 1024, controller.signal);
          if (live) setURL(owned);
          else data.releaseURL(owned);
        } else throw new MatrixSecurityError('unsupported');
      } catch {
        if (live) setFailed(true);
      }
    })();
    return () => {
      live = false;
      controller.abort();
      if (owned) data.releaseURL(owned);
    };
  }, [data, item, kind]);
  // ← y → pasan al fichero de al lado sin cerrar: mirar diez fotos eran diez «abrir y
  // cerrar».
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.altKey || e.ctrlKey || e.metaKey) return;
      const destino = e.key === 'ArrowLeft' ? anterior : e.key === 'ArrowRight' ? siguiente : null;
      if (!destino || !onCambiar) return;
      e.preventDefault();
      onCambiar(destino);
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [anterior, siguiente, onCambiar]);
  return (
    <Modal
      open
      size="lg"
      title={item.name}
      onClose={onClose}
      footer={
        <>
          {onCambiar ? (
            <>
              <Button
                variant="ghost"
                disabled={!anterior}
                onClick={() => anterior && onCambiar(anterior)}
              >
                {t('preview.prev')}
              </Button>
              <Button
                variant="ghost"
                disabled={!siguiente}
                onClick={() => siguiente && onCambiar(siguiente)}
              >
                {t('preview.next')}
              </Button>
            </>
          ) : null}
          <Button variant="secondary" onClick={() => onDownload(item)}>
            {t('action.download')}
          </Button>
        </>
      }
    >
      {failed ? (
        <p role="alert">{t('preview.failed')}</p>
      ) : text !== null ? (
        <pre class="max-h-[65dvh] overflow-auto break-words whitespace-pre-wrap">{text}</pre>
      ) : url && kind === 'image' ? (
        <img src={url} alt={item.name} class="mx-auto max-h-[65dvh] max-w-full object-contain" />
      ) : url && kind === 'pdf' ? (
        <iframe src={url} title={item.name} sandbox="" class="h-[65dvh] w-full" />
      ) : (
        <p class="flex items-center gap-2" role="status">
          <Spinner size={18} />
          {t('preview.loading')}
        </p>
      )}
    </Modal>
  );
}
