/**
 * SearchPanel — búsqueda server-side de eventos. Un buscador sobrio (Modal) que
 * consulta el servicio Go Sytue Calendar (/api/search vía BFF): casa por texto
 * (título/descripción/lugar/invitado), con debounce, y lista los resultados con
 * su fecha/hora en la ZONA DE VISUALIZACIÓN, el color y nombre de su calendario y
 * un fragmento; las series recurrentes se marcan «Serie» (aparecen una vez).
 * Paginación keyset («Cargar más»). Al pulsar un resultado, el calendario navega
 * a su día (donde el evento es visible y editable con su popover).
 *
 * Estados carga/vacío/error/inicial. Solo @sytue/ui + tokens --sytue-*; el Modal
 * aporta foco atrapado, Escape y backdrop. Accesible: campo etiquetado (FieldGroup),
 * resultados como lista de botones navegables por teclado. Anti AI-slop: hairlines,
 * sin degradados ni emojis.
 *
 * ponytail: el clic navega al día del evento (reusa toda la rejilla/popover CalDAV);
 * abrir el popover directo desde el resultado exigiría sintetizar href/etag CalDAV
 * que la búsqueda REST no devuelve — ese es el upgrade path si se pide.
 */
import type { JSX } from 'preact';
import type { Locale } from '@sytue/i18n';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { Badge, Button, Icon, Input, Modal, Spinner, cx } from '@sytue/ui';
import { searchEvents, type Calendar, type EventSearchResult } from '../client/calendarApi.js';
import { isRecurring, whenLabel, snippet, startDate } from '../lib/search.js';
import type { T } from '../i18n/index.js';

interface Props {
  /** Traductor ya atado al idioma de la petición; baja del padre. */
  t: T;
  locale: Locale;
  open: boolean;
  onClose: () => void;
  /** Calendarios cargados: para resolver color/nombre por calendar_id. */
  calendars: Calendar[];
  /** Abrir un resultado: navega el calendario al día de su inicio. */
  onOpenResult: (start: Date) => void;
}

const PAGE = 20; // tamaño de página del keyset
const MIN = 2; // longitud mínima del término (coherente con el backend)
const DEBOUNCE_MS = 250;

export function SearchPanel({
  t,
  locale,
  open,
  onClose,
  calendars,
  onOpenResult,
}: Props): JSX.Element {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<EventSearchResult[]>([]);
  const [cursor, setCursor] = useState('');
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState(false);
  // Secuencia: descarta respuestas fuera de orden (una consulta más nueva gana).
  const seqRef = useRef(0);

  // color/nombre por id de calendario (para el punto y el pie de cada resultado).
  const calById = useMemo(() => {
    const m = new Map<string, Calendar>();
    for (const c of calendars) if (c.id) m.set(c.id, c);
    return m;
  }, [calendars]);

  // Búsqueda con debounce al teclear (solo con el panel abierto). Términos por
  // debajo del mínimo limpian los resultados sin llamar al servidor.
  useEffect(() => {
    if (!open) return;
    const term = query.trim();
    if (term.length < MIN) {
      setResults([]);
      setCursor('');
      setError(false);
      setLoading(false);
      return;
    }
    const id = ++seqRef.current;
    setLoading(true);
    setError(false);
    const timer = setTimeout(() => {
      void searchEvents({ q: term, limit: PAGE })
        .then((res) => {
          if (id !== seqRef.current) return;
          setResults(res.events);
          setCursor(res.next_cursor);
        })
        .catch(() => {
          if (id === seqRef.current) setError(true);
        })
        .finally(() => {
          if (id === seqRef.current) setLoading(false);
        });
    }, DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [query, open]);

  // Al cerrar, reinicia el estado (una nueva apertura empieza limpia).
  useEffect(() => {
    if (open) return;
    setQuery('');
    setResults([]);
    setCursor('');
    setError(false);
    seqRef.current++;
  }, [open]);

  function loadMore(): void {
    if (!cursor) return;
    const id = seqRef.current; // misma búsqueda: si el término cambia, se descarta
    setLoadingMore(true);
    void searchEvents({ q: query.trim(), limit: PAGE, cursor })
      .then((res) => {
        if (id !== seqRef.current) return;
        setResults((prev) => [...prev, ...res.events]);
        setCursor(res.next_cursor);
      })
      .catch(() => {
        if (id === seqRef.current) setError(true);
      })
      .finally(() => {
        if (id === seqRef.current) setLoadingMore(false);
      });
  }

  function openResult(r: EventSearchResult): void {
    onOpenResult(startDate(r));
    onClose();
  }

  const term = query.trim();
  const showResults = term.length >= MIN;

  return (
    <Modal open={open} onClose={onClose} title={t('search.title')} size="lg">
      <div class="flex flex-col gap-3">
        {/* La etiqueta la aporta el título del Modal («Buscar eventos»); el input
            lleva aria-label para no duplicar texto visible (accesible igualmente). */}
        <Input
          id="cal-search"
          type="search"
          value={query}
          autofocus
          autocomplete="off"
          aria-label={t('search.title')}
          placeholder={t('search.placeholder')}
          leadingIcon={<Icon name="search" size={16} />}
          onInput={(e) => setQuery(e.currentTarget.value)}
        />

        {/* Región viva: anuncia carga/resultados a lectores de pantalla. */}
        <div class="min-h-[8rem]" aria-live="polite" aria-busy={loading || undefined}>
          {!showResults ? (
            <p class="text-text-subtle px-1 py-8 text-center text-sm">
              {t('search.minChars', { count: MIN })}
            </p>
          ) : loading ? (
            <div class="flex items-center justify-center gap-2 py-10">
              <Spinner size={18} />
              <span class="text-text-muted text-sm">{t('search.searching')}</span>
            </div>
          ) : error ? (
            <div class="flex flex-col items-center gap-2 py-8">
              <p class="text-text-muted text-sm">{t('search.error')}</p>
              <Button variant="secondary" size="sm" onClick={() => setQuery((q) => q + ' ')}>
                {t('common.retry')}
              </Button>
            </div>
          ) : results.length === 0 ? (
            <p class="text-text-subtle px-1 py-10 text-center text-sm">
              {t('search.noResults', { term })}
            </p>
          ) : (
            <ul class="flex flex-col gap-0.5">
              {results.map((r) => {
                const cal = calById.get(r.calendar_id);
                const color = cal?.color ?? 'var(--sytue-primary-500)';
                const when = whenLabel(locale, r);
                const sub = snippet(r);
                return (
                  <li key={r.id} class="list-none">
                    <button
                      type="button"
                      onClick={() => openResult(r)}
                      class={cx(
                        'group hover:bg-bg-subtle flex w-full items-start gap-2.5 rounded-md px-2.5 py-2 text-left transition-colors',
                        'sytue-focus-ring',
                      )}
                    >
                      <span
                        class="mt-1 h-2.5 w-2.5 shrink-0 rounded-full"
                        style={{ backgroundColor: color }}
                        aria-hidden="true"
                      />
                      <span class="min-w-0 flex-1">
                        <span class="flex items-center gap-2">
                          <span class="text-text min-w-0 flex-1 truncate text-sm font-semibold">
                            {r.summary || t('event.untitled')}
                          </span>
                          {isRecurring(r) ? (
                            <Badge tone="neutral" class="shrink-0">
                              {t('search.series')}
                            </Badge>
                          ) : null}
                        </span>
                        <span class="text-text-muted mt-0.5 block text-xs tabular-nums">
                          {when}
                        </span>
                        {sub ? (
                          <span class="sytue-meta mt-0.5 block truncate" title={sub}>
                            {sub}
                          </span>
                        ) : null}
                        {cal ? (
                          <span class="sytue-meta mt-0.5 block truncate">{cal.displayName}</span>
                        ) : null}
                      </span>
                    </button>
                  </li>
                );
              })}
              {cursor ? (
                <li class="list-none px-2.5 pt-1.5">
                  <Button variant="secondary" size="sm" loading={loadingMore} onClick={loadMore}>
                    {t('search.loadMore')}
                  </Button>
                </li>
              ) : null}
            </ul>
          )}
        </div>
      </div>
    </Modal>
  );
}
