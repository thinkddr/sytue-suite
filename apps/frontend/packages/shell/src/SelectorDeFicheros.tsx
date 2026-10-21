/**
 * SelectorDeFicheros — selector de fichero de Sytue Drive compartido.
 *
 * Extraído de DrivePickerModal de Mail y Rooms (PAGES-PLAN.md §14).
 * Navega el árbol de carpetas contra el BFF (/api/drive-files) y permite
 * elegir un fichero adjuntándolo por referencia o enlace.
 *
 * Accesible, keyset pagination, estados explícitos de carga/error/vacío.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Button, EmptyState, formatBytes, Icon, Modal, Spinner, useToast, Glifo } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';

/** Ítem devuelto por el BFF: carpeta navegable o fichero adjuntable. */
export interface PickerItem {
  id?: string;
  name: string;
  isDirectory: boolean;
  path: string;
  size?: number;
  contentType?: string;
  link?: string;
}

export interface SelectorDeFicherosTextos {
  pickTitle: string;
  cancel: string;
  path: string;
  myDrive: string;
  loading: string;
  loadMore: string;
  emptyFolder: string;
  byReference?: string;
  loadFailed: string;
  loadMoreFailed?: string;
  retry: string;
  sessionExpired: string;
  sessionExpiredHelp: string;
  signInAgain: string;
  checkConnection: string;
}

export interface SelectorDeFicherosProps {
  onClose: () => void;
  onPick: (item: PickerItem) => void;
  textos: SelectorDeFicherosTextos;
  locale: Locale;
  apiEndpoint?: string;
}

function FolderGlyph({ size = 16, class: cls }: { size?: number; class?: string }): JSX.Element {
  return (
    <Glifo
      d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2Z"
      size={size}
      class={cls}
    />
  );
}

function FileGlyph({ size = 16, class: cls }: { size?: number; class?: string }): JSX.Element {
  return (
    <Glifo size={size} class={cls}>
      <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8Z" />
      <path d="M14 3v5h5" />
    </Glifo>
  );
}

/** Segmentos de la ruta actual para las migas de pan. */
function segments(path: string): { name: string; path: string }[] {
  const acc: { name: string; path: string }[] = [];
  let cur = '';
  for (const part of path.split('/').filter(Boolean)) {
    cur += `/${part}`;
    acc.push({ name: decodeURIComponent(part), path: cur });
  }
  return acc;
}

export function SelectorDeFicheros({
  onClose,
  onPick,
  textos,
  locale,
  apiEndpoint = '/api/drive-files',
}: SelectorDeFicherosProps): JSX.Element {
  const toast = useToast();
  const [path, setPath] = useState('/');
  const [items, setItems] = useState<PickerItem[] | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<'session' | 'network' | null>(null);

  const fetchPage = useCallback(
    async (target: string, cursor?: string) => {
      const q = `path=${encodeURIComponent(target)}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`;
      const res = await fetch(`${apiEndpoint}?${q}`, {
        credentials: 'include',
        headers: { accept: 'application/json' },
      });
      if (!res.ok) {
        const err = new Error(String(res.status)) as Error & { status: number };
        err.status = res.status;
        throw err;
      }
      return (await res.json()) as { items: PickerItem[]; nextCursor?: string };
    },
    [apiEndpoint],
  );

  const load = useCallback(
    async (target: string) => {
      setItems(null);
      setNextCursor(null);
      setError(null);
      try {
        const page = await fetchPage(target);
        setItems(page.items);
        setNextCursor(page.nextCursor ?? null);
      } catch (e) {
        setError((e as { status?: number }).status === 401 ? 'session' : 'network');
      }
    },
    [fetchPage],
  );

  const loadMore = useCallback(async () => {
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    try {
      const page = await fetchPage(path, nextCursor);
      setItems((prev) => [...(prev ?? []), ...page.items]);
      setNextCursor(page.nextCursor ?? null);
    } catch {
      if (textos.loadMoreFailed) {
        toast.notify({ tone: 'danger', title: textos.loadMoreFailed });
      }
    } finally {
      setLoadingMore(false);
    }
  }, [path, nextCursor, loadingMore, fetchPage, toast, textos.loadMoreFailed]);

  useEffect(() => {
    void load(path);
  }, [path, load]);

  const crumbs = segments(path);

  return (
    <Modal
      open
      onClose={onClose}
      title={textos.pickTitle}
      footer={
        <Button variant="ghost" onClick={onClose}>
          {textos.cancel}
        </Button>
      }
    >
      <nav aria-label={textos.path} class="mb-2 flex flex-wrap items-center gap-0.5 text-base">
        <button
          type="button"
          onClick={() => setPath('/')}
          aria-current={path === '/' ? 'location' : undefined}
          class="hover:bg-bg-subtle sytue-focus-ring rounded-control px-2 py-1 font-medium whitespace-nowrap transition-colors duration-150"
        >
          {textos.myDrive}
        </button>
        {crumbs.map((c) => (
          <span key={c.path} class="flex min-w-0 items-center gap-0.5">
            <Icon name="chevronRight" size={14} class="text-text-subtle shrink-0" />
            <button
              type="button"
              onClick={() => setPath(c.path)}
              aria-current={c.path === path ? 'location' : undefined}
              title={c.name}
              class="hover:bg-bg-subtle sytue-focus-ring rounded-control truncate px-2 py-1 font-medium transition-colors duration-150"
            >
              {c.name}
            </button>
          </span>
        ))}
      </nav>

      <div class="border-border h-72 overflow-y-auto rounded-md border">
        {items === null && !error ? (
          <div class="grid h-full place-items-center" aria-busy="true">
            <Spinner size={20} />
            <span class="sr-only" role="status">
              {textos.loading}
            </span>
          </div>
        ) : error ? (
          <div class="grid h-full place-items-center p-4 text-center">
            <EmptyState
              title={error === 'session' ? textos.sessionExpired : textos.loadFailed}
              description={error === 'session' ? textos.sessionExpiredHelp : textos.checkConnection}
              illustration={<Icon name="warning" size={36} class="text-danger-text" />}
              action={
                error === 'session' ? (
                  <Button size="sm" onClick={() => location.reload()}>
                    {textos.signInAgain}
                  </Button>
                ) : (
                  <Button variant="secondary" size="sm" onClick={() => void load(path)}>
                    {textos.retry}
                  </Button>
                )
              }
            />
          </div>
        ) : items && items.length === 0 ? (
          <div class="grid h-full place-items-center p-4">
            <p class="text-text-muted text-base">{textos.emptyFolder}</p>
          </div>
        ) : (
          <>
            <ul>
              {items!.map((it) =>
                it.isDirectory ? (
                  <li key={it.path}>
                    <button
                      type="button"
                      onClick={() => setPath(it.path)}
                      class="border-border-subtle hover:bg-bg-subtle focus-visible:bg-bg-subtle text-primary-text flex w-full items-center gap-2.5 border-b px-3 py-2 text-left transition-colors duration-150 focus-visible:outline-none"
                    >
                      <FolderGlyph size={16} class="shrink-0" />
                      <span class="text-text min-w-0 flex-1 truncate font-medium" title={it.name}>
                        {it.name}
                      </span>
                      <Icon name="chevronRight" size={14} class="text-text-subtle shrink-0" />
                    </button>
                  </li>
                ) : (
                  <li key={it.path}>
                    <button
                      type="button"
                      disabled={!it.id && !it.link}
                      onClick={() => onPick(it)}
                      class="border-border-subtle hover:bg-bg-subtle focus-visible:bg-bg-subtle text-text-muted flex w-full items-center gap-2.5 border-b px-3 py-2 text-left transition-colors duration-150 focus-visible:outline-none disabled:opacity-50"
                    >
                      <FileGlyph size={16} class="shrink-0" />
                      <span class="text-text min-w-0 flex-1 truncate" title={it.name}>
                        {it.name}
                      </span>
                      <span class="sytue-meta shrink-0 tabular-nums">
                        {it.size ? formatBytes(locale, it.size) : ''}
                      </span>
                    </button>
                  </li>
                ),
              )}
            </ul>
            {nextCursor !== null ? (
              <div class="flex justify-center py-3">
                <Button
                  variant="secondary"
                  size="sm"
                  loading={loadingMore}
                  onClick={() => void loadMore()}
                >
                  {textos.loadMore}
                </Button>
              </div>
            ) : null}
          </>
        )}
      </div>
      {textos.byReference ? (
        <p class="text-text-subtle mt-2 text-sm">{textos.byReference}</p>
      ) : null}
    </Modal>
  );
}
