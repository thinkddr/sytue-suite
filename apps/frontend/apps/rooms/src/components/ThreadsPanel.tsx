/**
 * ThreadsPanel — panel lateral (RHS) de la vista global «Hilos» (Ola 9, paridad
 * Mattermost): lista los hilos que SIGUES, cross-canal, con el mensaje raíz (autor +
 * fragmento), el #canal, el nº de respuestas, un indicador de respuestas no leídas y
 * la última actividad. Cada fila abre ese hilo en el ThreadPanel (y lo marca visto).
 * Reusa el patrón de MessageListPanel (cabecera sólida bg-header + cerrar, cuerpo
 * desplazable, paginación keyset «Cargar más»). docs/design.md §7.
 *
 * ponytail: sin virtualización (los hilos seguidos son un conjunto acotado con
 * paginación keyset); si crecieran a miles, portar a <VirtualList> como el feed.
 */
import type { JSX } from 'preact';
import {
  EmptyState,
  Illustration,
  Avatar,
  Button,
  Icon,
  Skeleton,
  cx,
  IconButton,
} from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { ThreadSummary } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { MessageBody } from './MessageBody.js';

interface Props {
  threads: ThreadSummary[];
  loading: boolean;
  error?: boolean;
  /** sub -> nombre legible, para resaltar @menciones del fragmento. */
  memberNames?: Record<string, string>;
  /** channelId -> nombre de canal, para mostrar el origen (cross-canal). */
  channelNames?: Record<string, string>;
  /** Abre el hilo seleccionado en el ThreadPanel. */
  onSelect: (thread: ThreadSummary) => void;
  onClose: () => void;
  onRetry?: () => void;
  /** Paginación keyset: pide la siguiente página. */
  onLoadMore?: () => void;
  loadingMore?: boolean;
  /** Idioma activo (formato de fecha/hora) y traductor ya atado a él. */
  locale: Locale;
  t: T;
}

/** Fecha corta + hora en el idioma del usuario (el orden y el reloj los pone el locale). */
function timeLabel(locale: Locale, iso: string): string {
  return formatDateTime(locale, iso, {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function ThreadsPanel({
  threads,
  loading,
  error = false,
  memberNames,
  channelNames,
  onSelect,
  onClose,
  onRetry,
  onLoadMore,
  loadingMore = false,
  locale,
  t,
}: Props): JSX.Element {
  return (
    <aside
      class="sytue-sheet-corners border-border bg-surface marco:overflow-clip flex h-full w-full shrink-0 flex-col border-l md:w-[var(--rooms-rhs-width)] md:[--sytue-sheet-bl:0px] md:[--sytue-sheet-tl:0px]"
      aria-label={t('threads.title')}
    >
      <header class="bg-header text-header-fg flex h-14 shrink-0 items-center justify-between gap-2 px-4">
        <h2 class="text-md text-header-fg leading-tight font-semibold">{t('threads.title')}</h2>
        <IconButton
          size="lg"
          tone="header"
          label={t('threads.close')}
          showTitle={false}
          title={t('action.close')}
          onClick={onClose}
        >
          <Icon name="close" size={18} />
        </IconButton>
      </header>

      <div class="min-h-0 flex-1 overflow-y-auto py-2">
        {error ? (
          <div role="alert" class="text-danger-text px-4 py-3 text-sm">
            <p class="flex items-center gap-1.5">
              <Icon name="warning" size={14} />
              <span>{t('threads.loadFailed')}</span>
              {onRetry ? (
                <button
                  type="button"
                  class="hover:text-text sytue-focus-ring rounded-sm font-semibold underline underline-offset-2"
                  onClick={onRetry}
                >
                  {t('action.retry')}
                </button>
              ) : null}
            </p>
          </div>
        ) : loading ? (
          <div class="flex flex-col gap-3 px-4 py-2" aria-busy="true">
            <span role="status" class="sr-only">
              {t('threads.loading')}
            </span>
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
        ) : threads.length === 0 ? (
          <EmptyState
            class="py-4"
            title={t('threads.empty')}
            illustration={<Illustration name="emptyChat" size={96} />}
          />
        ) : (
          <>
            {threads.map((thread) => {
              const { root, unreadReplies } = thread;
              const channelName = channelNames?.[root.channelId];
              const replyCount = root.replyCount ?? 0;
              return (
                <button
                  key={root.id}
                  type="button"
                  onClick={() => onSelect(thread)}
                  class="hover:bg-bg-subtle sytue-focus-ring-inner flex w-full gap-3 px-4 py-2 text-left transition-colors duration-150"
                >
                  <div class="w-9 shrink-0 pt-0.5">
                    <Avatar name={root.author.name} size="sm" />
                  </div>
                  <div class="min-w-0 flex-1">
                    <div class="flex min-w-0 items-baseline gap-2">
                      <span
                        class="text-text min-w-0 truncate text-sm font-semibold"
                        title={root.author.name}
                      >
                        {root.author.name}
                      </span>
                      {channelName ? (
                        <span class="sytue-meta shrink-0 truncate" title={`#${channelName}`}>
                          #{channelName}
                        </span>
                      ) : null}
                      <span class="sytue-meta ml-auto shrink-0">
                        {timeLabel(locale, thread.lastActivity)}
                      </span>
                    </div>
                    {root.body ? (
                      <p class="text-text-muted line-clamp-2 text-base break-words whitespace-pre-wrap">
                        <MessageBody
                          body={root.body}
                          authorName={root.author.name}
                          mentionNames={(root.mentions ?? []).map((s) => memberNames?.[s] ?? s)}
                        />
                      </p>
                    ) : null}
                    <div class="sytue-meta mt-1 flex items-center gap-2">
                      <span class="tabular-nums">
                        {t('thread.replyCount', { count: replyCount })}
                      </span>
                      {unreadReplies > 0 ? (
                        <span
                          class={cx(
                            'grid h-[18px] min-w-[18px] place-items-center rounded-full px-1 text-xs leading-none font-bold tabular-nums',
                            // El relleno es el rol primario con su rótulo, y no `primary-text`:
                            // aquel es un token de TINTA, para escribir sobre una superficie, y en
                            // tema oscuro se aclara, con lo que el blanco de encima se quedaba en
                            // 2,11:1. Mismo arreglo que en `ui.Badge count`, del que esta pastilla
                            // es una copia.
                            'bg-primary text-on-primary',
                          )}
                          aria-label={t('thread.newReplies', { count: unreadReplies })}
                        >
                          {unreadReplies > 99 ? '99+' : unreadReplies}
                        </span>
                      ) : null}
                    </div>
                  </div>
                </button>
              );
            })}
            {onLoadMore ? (
              <div class="px-4 py-2">
                <Button
                  size="sm"
                  variant="secondary"
                  loading={loadingMore}
                  onClick={onLoadMore}
                  class="w-full"
                >
                  {t('action.loadMore')}
                </Button>
              </div>
            ) : null}
          </>
        )}
      </div>
    </aside>
  );
}
