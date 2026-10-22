/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SuitePicker — UN selector para las cuatro cosas que se pueden enganchar a una
 * nota: un fichero de Sytue Drive, un documento de Sytue Docs, un evento de
 * Sytue Calendar o una sala de Sytue Rooms.
 *
 * ...y la quinta, que es otra NOTA: la referencia que Sytue Notepads hereda de
 * BasKet Note Pads. Es el mismo gesto —«enséñame lo que puedo enganchar aquí y
 * déjame elegir»—, así que es un solo componente y un solo endpoint
 * (`/api/suite?kind=`). Cinco modales con el mismo esqueleto divergían a la
 * primera corrección.
 *
 * Drive se NAVEGA (carpetas + migas de pan) porque su modelo es un árbol; los
 * otros cuatro se BUSCAN, porque el suyo es una lista. La diferencia la marca el
 * propio origen, no una opción que haya que recordar.
 *
 * Lo que se elige se guarda como REFERENCIA (id + nombre + enlace), nunca como
 * copia: si mañana se revoca el permiso, la nota deja de abrirlo.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { Button, EmptyState, Icon, Input, Modal, Spinner } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { listarDeLaSuite, type Elegible, type OrigenSuite } from '../client/notesApi.js';
import { Glyph, type GlyphName } from './glyphs.js';

interface Props {
  t: T;
  origen: OrigenSuite;
  onClose: () => void;
  onPick: (elegido: Elegible) => void;
}

const TITULO: Record<OrigenSuite, string> = {
  drive: 'picker.drive.title',
  docs: 'picker.docs.title',
  calendar: 'picker.calendar.title',
  rooms: 'picker.rooms.title',
  notes: 'picker.notes.title',
};

const GLIFO: Record<OrigenSuite, GlyphName> = {
  drive: 'file',
  docs: 'file',
  calendar: 'note',
  rooms: 'link',
  notes: 'note',
};

/** Segmentos de la ruta actual, para las migas de pan de Drive. */
function segmentos(path: string): { name: string; path: string }[] {
  const acc: { name: string; path: string }[] = [];
  let cur = '';
  for (const parte of path.split('/').filter(Boolean)) {
    cur += `/${parte}`;
    acc.push({ name: decodeURIComponent(parte), path: cur });
  }
  return acc;
}

export function SuitePicker({ t, origen, onClose, onPick }: Props): JSX.Element {
  const navegable = origen === 'drive';
  const [path, setPath] = useState('/');
  const [q, setQ] = useState('');
  const [items, setItems] = useState<Elegible[] | null>(null);
  const [cursor, setCursor] = useState<string | null>(null);
  const [error, setError] = useState(false);
  // El término se consulta con retardo: escribir «informe» son siete pulsaciones
  // y siete viajes al servicio para seis resultados que nadie llegó a leer.
  const debounceRef = useRef<ReturnType<typeof setTimeout>>();

  const cargar = useCallback(
    async (opts: { path?: string; q?: string; cursor?: string; anadir?: boolean }) => {
      if (!opts.anadir) setItems(null);
      setError(false);
      try {
        const page = await listarDeLaSuite(origen, {
          ...(opts.path ? { path: opts.path } : {}),
          ...(opts.q ? { q: opts.q } : {}),
          ...(opts.cursor ? { cursor: opts.cursor } : {}),
        });
        setItems((prev) => (opts.anadir ? [...(prev ?? []), ...page.items] : page.items));
        setCursor(page.nextCursor);
      } catch {
        setError(true);
      }
    },
    [origen],
  );

  useEffect(() => {
    if (navegable) {
      void cargar({ path });
      return;
    }
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => void cargar({ q }), 250);
    return () => clearTimeout(debounceRef.current);
  }, [navegable, path, q, cargar]);

  const migas = segmentos(path);

  return (
    <Modal
      open
      onClose={onClose}
      title={t(TITULO[origen] as never)}
      footer={
        <Button variant="ghost" onClick={onClose}>
          {t('common.cancel')}
        </Button>
      }
    >
      {navegable ? (
        <nav
          aria-label={t('picker.breadcrumb')}
          class="mb-2 flex flex-wrap items-center gap-0.5 text-base"
        >
          <button
            type="button"
            onClick={() => setPath('/')}
            aria-current={path === '/' ? 'location' : undefined}
            class="hover:bg-bg-subtle sytue-focus-ring rounded-control px-2 py-1 font-medium whitespace-nowrap transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)]"
          >
            {t('picker.myDrive')}
          </button>
          {migas.map((c) => (
            <span key={c.path} class="flex min-w-0 items-center gap-0.5">
              <Icon name="chevronRight" size={14} class="text-text-subtle shrink-0" />
              <button
                type="button"
                onClick={() => setPath(c.path)}
                aria-current={c.path === path ? 'location' : undefined}
                title={c.name}
                class="hover:bg-bg-subtle sytue-focus-ring rounded-control truncate px-2 py-1 font-medium transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)]"
              >
                {c.name}
              </button>
            </span>
          ))}
        </nav>
      ) : (
        <div class="mb-2">
          <Input
            type="search"
            value={q}
            aria-label={t('picker.search')}
            placeholder={t('picker.search')}
            onInput={(e) => setQ((e.target as HTMLInputElement).value)}
          />
        </div>
      )}

      <div class="border-border h-72 overflow-y-auto rounded-md border">
        {items === null && !error ? (
          <div class="grid h-full place-items-center" aria-busy="true">
            <Spinner size={20} />
          </div>
        ) : error ? (
          <div class="grid h-full place-items-center p-4 text-center">
            <EmptyState
              title={t('picker.failed')}
              illustration={<Icon name="warning" size={36} class="text-danger-text" />}
              action={
                <Button variant="secondary" size="sm" onClick={() => void cargar({ path, q })}>
                  {t('common.retry')}
                </Button>
              }
            />
          </div>
        ) : (items?.length ?? 0) === 0 ? (
          <div class="grid h-full place-items-center p-4">
            <p class="text-text-muted text-base">{t('picker.empty')}</p>
          </div>
        ) : (
          <>
            <ul>
              {items!.map((it) => (
                <li key={`${it.id}-${it.path ?? ''}`}>
                  <button
                    type="button"
                    disabled={!it.isDirectory && !it.url}
                    onClick={() => (it.isDirectory ? setPath(it.path ?? '/') : onPick(it))}
                    class="border-border-subtle hover:bg-bg-subtle focus-visible:bg-bg-subtle flex w-full items-center gap-2.5 border-b px-3 py-2 text-left transition-colors duration-150 focus-visible:outline-none disabled:opacity-50 pointer-coarse:min-h-[max(44px,2.75rem)]"
                  >
                    <Glyph
                      name={it.isDirectory ? 'notebook' : GLIFO[origen]}
                      size={16}
                      class={
                        it.isDirectory ? 'text-primary-text shrink-0' : 'text-text-muted shrink-0'
                      }
                    />
                    <span class="text-text min-w-0 flex-1 truncate" title={it.name}>
                      {it.name}
                    </span>
                    {it.isDirectory ? (
                      <Icon name="chevronRight" size={14} class="text-text-subtle shrink-0" />
                    ) : it.meta ? (
                      <span class="sytue-meta max-w-32 shrink-0 truncate">{it.meta}</span>
                    ) : null}
                  </button>
                </li>
              ))}
            </ul>
            {cursor ? (
              <div class="flex justify-center py-3">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => void cargar({ path, q, cursor, anadir: true })}
                >
                  {t('picker.loadMore')}
                </Button>
              </div>
            ) : null}
          </>
        )}
      </div>
    </Modal>
  );
}
