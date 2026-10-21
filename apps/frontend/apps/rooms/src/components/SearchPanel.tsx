/**
 * SearchPanel — panel lateral (RHS) de búsqueda global de Sytue Rooms. Reusa el
 * patrón visual de MessageListPanel (cabecera sólida bg-header + cerrar, cuerpo
 * desplazable con filas de mensaje), añadiendo una caja de búsqueda con debounce y
 * resultados clicables. Consulta GET /api/search (cross-team, respeta el acceso) y
 * resuelve el nombre del canal contra la lista del cliente (el mensaje no lo lleva).
 *
 * La query admite operadores from:/in:/before:/after:, que se pasan tal cual al
 * backend. Accesible: input con foco al abrir, resultados como botones (teclado),
 * conteo en aria-live, y Esc cierra. docs/design.md §7.
 *
 * El clic en un resultado navega al mensaje (abre su canal, carga una ventana centrada
 * y hace scroll-to + resalte, vía onSelect → goToMessage en ChatApp). ponytail: sin
 * virtualización (el backend acota a 30 resultados).
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { EmptyState, Illustration, Avatar, Icon, Input, Skeleton, IconButton } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import { searchMessages, type ChatMessage } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { MessageBody } from './MessageBody.js';
import { Attachments } from './Attachments.js';

interface Props {
  /** channelId -> nombre de canal, para mostrar el origen de cada resultado. */
  channelNames: Record<string, string>;
  /** sub -> nombre legible, para resaltar @menciones en el fragmento. */
  memberNames?: Record<string, string>;
  /** Clic en un resultado: navega a ese mensaje (abre su canal + scroll-to + resalte). */
  onSelect: (channelId: string, messageId: string) => void;
  onClose: () => void;
  /** Query inicial (p. ej. la del slash command «/search <consulta>»). */
  initialQuery?: string;
  /** Idioma activo (formato de fecha/hora) y traductor ya atado a él. */
  locale: Locale;
  t: T;
}

/** Longitud mínima de la query para lanzar la búsqueda (evita el ruido de 1 letra). */
const MIN_QUERY = 2;

function timeLabel(locale: Locale, iso: string): string {
  return formatDateTime(locale, iso, {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function SearchPanel({
  channelNames,
  memberNames,
  onSelect,
  onClose,
  initialQuery,
  locale,
  t,
}: Props): JSX.Element {
  // Semilla del slash command «/search»; el panel se remonta al abrirse, así que
  // sembrar el estado inicial basta (no hace falta sincronizar con un efecto).
  const [query, setQuery] = useState(initialQuery ?? '');
  const [results, setResults] = useState<ChatMessage[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  // ¿Hay una búsqueda "vigente"? (query con longitud suficiente). Gobierna si el
  // conteo/estado vacío se anuncian o si solo se muestra la pista inicial.
  const active = query.trim().length >= MIN_QUERY;

  // Búsqueda con debounce (250 ms), como «Explorar canales». Query corta → limpia.
  useEffect(() => {
    if (!active) {
      setResults([]);
      setError(false);
      setLoading(false);
      return;
    }
    let live = true;
    setLoading(true);
    setError(false);
    const timer = setTimeout(() => {
      searchMessages(query.trim())
        .then(({ messages }) => live && setResults(messages))
        .catch(() => live && setError(true))
        .finally(() => live && setLoading(false));
    }, 250);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [query, active]);

  // Mensaje de estado accesible (aria-live): acompaña al conteo visible.
  const statusText = !active
    ? ''
    : loading
      ? t('search.busy')
      : error
        ? t('search.failed')
        : t('search.resultCount', { count: results.length });

  return (
    // onKeyDown a nivel de panel: Escape cierra desde cualquier hijo (burbujeo).
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <aside
      class="sytue-sheet-corners border-border bg-surface marco:overflow-clip flex h-full w-full shrink-0 flex-col border-l md:w-[var(--rooms-rhs-width)] md:[--sytue-sheet-bl:0px] md:[--sytue-sheet-tl:0px]"
      aria-label={t('search.messages')}
      onKeyDown={(e) => {
        if (e.key === 'Escape') {
          e.stopPropagation();
          onClose();
        }
      }}
    >
      <header class="bg-header text-header-fg flex h-14 shrink-0 items-center justify-between gap-2 px-4">
        <h2 class="text-md text-header-fg leading-tight font-semibold">{t('search.title')}</h2>
        <IconButton
          size="lg"
          tone="header"
          label={t('search.close')}
          showTitle={false}
          title={t('action.close')}
          onClick={onClose}
        >
          <Icon name="close" size={18} />
        </IconButton>
      </header>

      <div class="border-border shrink-0 border-b p-3">
        <Input
          type="search"
          value={query}
          // eslint-disable-next-line jsx-a11y/no-autofocus -- panel de búsqueda: foco inmediato al abrir
          autoFocus
          placeholder={t('search.placeholder')}
          leadingIcon={<Icon name="search" size={16} />}
          onInput={(e) => setQuery(e.currentTarget.value)}
          aria-label={t('search.messages')}
        />
      </div>

      {/* Conteo/estado: nunca solo visual — aria-live lo anuncia sin robar el foco. */}
      <div class="text-text-muted shrink-0 px-4 py-1.5 text-xs" role="status" aria-live="polite">
        {statusText}
      </div>

      <div class="min-h-0 flex-1 overflow-y-auto pb-2">
        {!active ? (
          <p class="text-text-muted px-4 py-6 text-center text-sm">{t('search.hint')}</p>
        ) : error ? (
          <div class="text-danger-text px-4 py-3 text-sm">
            <p class="flex items-center gap-1.5">
              <Icon name="warning" size={14} />
              <span>{t('search.failedRetry')}</span>
            </p>
          </div>
        ) : loading ? (
          <div class="flex flex-col gap-3 px-4 py-2" aria-busy="true">
            {[60, 45, 52].map((w, i) => (
              <div key={i} class="flex gap-3">
                <Skeleton width="2rem" height="2rem" class="shrink-0" />
                <div class="flex min-w-0 flex-1 flex-col gap-2 pt-1">
                  <Skeleton width="6rem" height="0.75rem" />
                  <Skeleton width={`${w}%`} height="0.75rem" />
                </div>
              </div>
            ))}
          </div>
        ) : results.length === 0 ? (
          <EmptyState
            class="py-4"
            title={t('search.noResults')}
            illustration={<Illustration name="emptySearch" size={96} />}
          />
        ) : (
          results.map((m) => {
            const channelName = channelNames[m.channelId];
            return (
              <button
                key={m.id}
                type="button"
                onClick={() => onSelect(m.channelId, m.id)}
                class="hover:bg-bg-subtle focus-visible:bg-bg-subtle sytue-focus-ring-inner flex w-full gap-3 px-4 py-2 text-left"
              >
                <div class="w-9 shrink-0 pt-0.5">
                  <Avatar name={m.author.name} size="sm" />
                </div>
                <div class="min-w-0 flex-1">
                  <div class="flex min-w-0 items-baseline gap-2">
                    <span class="text-text min-w-0 truncate text-sm font-semibold">
                      {m.author.name}
                    </span>
                    <span class="sytue-meta shrink-0">{timeLabel(locale, m.createdAt)}</span>
                    <span class="sytue-meta shrink-0 truncate">
                      {channelName ? `#${channelName}` : t('channel.word')}
                    </span>
                  </div>
                  {m.body ? (
                    <p class="text-text text-base break-words whitespace-pre-wrap">
                      <MessageBody
                        body={m.body}
                        authorName={m.author.name}
                        mentionNames={(m.mentions ?? []).map((s) => memberNames?.[s] ?? s)}
                      />
                    </p>
                  ) : null}
                  <Attachments
                    attachments={m.attachments}
                    channelId={m.channelId}
                    t={t}
                    locale={locale}
                  />
                </div>
              </button>
            );
          })
        )}
      </div>
    </aside>
  );
}
