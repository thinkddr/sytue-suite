/**
 * MessageListPanel — panel lateral (RHS) para listas de mensajes: «Fijados» (por
 * canal) y «Guardados» (por usuario, cross-canal). Reusa el patrón de ThreadPanel
 * (cabecera sólida bg-header + cerrar, cuerpo desplazable con filas mensaje). Cada
 * fila puede llevar el #canal (guardados) y una acción de retirar (desfijar / quitar
 * de guardados). docs/design.md §7.
 *
 * La fila es un BOTÓN: activarla salta al mensaje en su canal (mismo destino que un
 * resultado de búsqueda, vía onSelect → goToMessage). Las acciones de la fila
 * (copiar enlace, retirar) viven FUERA de ese botón, nunca anidadas dentro.
 *
 * ponytail: sin virtualización (fijados/guardados son conjuntos acotados y con
 * paginación keyset); si crecieran a miles, portar a <VirtualList> como el feed.
 */
import type { JSX } from 'preact';
import { Avatar, Button, Icon, Skeleton, useToast, IconButton } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import { messageLink, type ChatMessage } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { copyToClipboard } from '../lib/clipboard.js';
import { MessageBody } from './MessageBody.js';
import { Attachments } from './Attachments.js';
import { LinkGlyph } from './glyphs.js';

interface Props {
  title: string;
  /** Subtítulo bajo el título (p. ej. «#general»). */
  subtitle?: string;
  messages: ChatMessage[];
  loading: boolean;
  error?: boolean;
  /** sub -> nombre legible, para resaltar @menciones. */
  memberNames?: Record<string, string>;
  /** channelId -> nombre de canal, para mostrar el origen (guardados cross-canal). */
  channelNames?: Record<string, string>;
  emptyText: string;
  /** Etiqueta accesible de la acción de retirar por fila (desfijar / quitar). */
  removeLabel?: string;
  onRemove?: (m: ChatMessage) => void;
  /** Activar la fila: salta al mensaje en su canal (abre el canal + scroll + resalte). */
  onSelect: (m: ChatMessage) => void;
  onClose: () => void;
  onRetry?: () => void;
  /** Paginación keyset (guardados): pide la siguiente página. */
  onLoadMore?: () => void;
  loadingMore?: boolean;
  /** Idioma activo (formato de fecha/hora) y traductor ya atado a él. */
  locale: Locale;
  t: T;
}

function timeLabel(locale: Locale, iso: string): string {
  return formatDateTime(locale, iso, {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function MessageListPanel({
  title,
  subtitle,
  messages,
  loading,
  error = false,
  memberNames,
  channelNames,
  emptyText,
  removeLabel,
  onRemove,
  onSelect,
  onClose,
  onRetry,
  onLoadMore,
  loadingMore = false,
  locale,
  t,
}: Props): JSX.Element {
  const toast = useToast();

  /** Copia el permalink del mensaje (misma acción que «Copiar enlace» del menú «…»). */
  async function copyLink(m: ChatMessage): Promise<void> {
    const ok = await copyToClipboard(messageLink(m.channelId, m.id));
    toast.notify(
      ok
        ? { tone: 'success', title: t('message.linkCopied') }
        : { tone: 'danger', title: t('message.copyFailed') },
    );
  }

  return (
    <aside
      class="sytue-sheet-corners border-border bg-surface marco:overflow-clip flex h-full w-full shrink-0 flex-col border-l md:w-[var(--rooms-rhs-width)] md:[--sytue-sheet-bl:0px] md:[--sytue-sheet-tl:0px]"
      aria-label={title}
    >
      <header class="bg-header text-header-fg flex h-14 shrink-0 items-center justify-between gap-2 px-4">
        <div class="flex min-w-0 flex-col justify-center">
          <h2 class="text-md text-header-fg leading-tight font-semibold">{title}</h2>
          {subtitle ? (
            <p class="text-header-fg/80 truncate text-xs leading-tight">{subtitle}</p>
          ) : null}
        </div>
        <IconButton
          size="lg"
          tone="header"
          label={t('action.closeNamed', { name: title })}
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
              <span>{t('error.loadFailed')}</span>
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
              {t('state.loading')}
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
        ) : messages.length === 0 ? (
          <p class="text-text-muted px-4 py-6 text-center text-sm">{emptyText}</p>
        ) : (
          <>
            {messages.map((m) => (
              <div key={m.id} class="group hover:bg-bg-subtle relative flex items-start">
                {/* «Ir al mensaje»: botón real que cubre la fila (patrón stretched
                    link). No envuelve al contenido a propósito — el cuerpo lleva
                    enlaces y los adjuntos, botones, y anidar controles dentro de un
                    <button> es HTML inválido (el parser lo rompe al hidratar). El
                    contenido y las acciones se elevan con `relative` para quedar por
                    encima de él y seguir siendo pulsables. */}
                <button
                  type="button"
                  aria-label={t('message.goToFrom', { name: m.author.name })}
                  title={t('message.goTo')}
                  onClick={() => onSelect(m)}
                  class="sytue-focus-ring-inner absolute inset-0"
                />
                <div class="flex min-w-0 flex-1 gap-3 px-4 py-2 [&_a]:relative [&_button]:relative">
                  <div class="w-9 shrink-0 pt-0.5">
                    <Avatar name={m.author.name} size="sm" />
                  </div>
                  <div class="min-w-0 flex-1">
                    <div class="flex min-w-0 items-baseline gap-2">
                      <span
                        class="text-text min-w-0 truncate text-sm font-semibold"
                        title={m.author.name}
                      >
                        {m.author.name}
                      </span>
                      <span class="sytue-meta shrink-0">{timeLabel(locale, m.createdAt)}</span>
                      {channelNames?.[m.channelId] ? (
                        <span
                          class="sytue-meta shrink-0 truncate"
                          title={`#${channelNames[m.channelId]}`}
                        >
                          #{channelNames[m.channelId]}
                        </span>
                      ) : null}
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
                </div>
                {/* Acciones de la fila, hermanas del botón de salto: copiar el enlace
                    del mensaje y retirarlo de la lista (desfijar / quitar). */}
                <div class="relative flex shrink-0 items-center gap-1 py-2 pr-2">
                  <button
                    type="button"
                    aria-label={t('message.copyLinkLong')}
                    title={t('message.copyLink')}
                    onClick={() => void copyLink(m)}
                    class="text-text-subtle hover:bg-surface hover:text-text sytue-focus-ring h-7 w-7 shrink-0 rounded opacity-0 transition-colors duration-150 group-hover:opacity-100 focus-visible:opacity-100 pointer-coarse:opacity-100"
                  >
                    <span class="grid h-full w-full place-items-center">
                      <LinkGlyph size={16} />
                    </span>
                  </button>
                  {onRemove && removeLabel ? (
                    <button
                      type="button"
                      aria-label={removeLabel}
                      title={removeLabel}
                      onClick={() => onRemove(m)}
                      class="text-text-subtle hover:bg-surface hover:text-text sytue-focus-ring h-7 w-7 shrink-0 rounded opacity-0 transition-colors duration-150 group-hover:opacity-100 focus-visible:opacity-100 pointer-coarse:opacity-100"
                    >
                      <span class="grid h-full w-full place-items-center">
                        <Icon name="close" size={16} />
                      </span>
                    </button>
                  ) : null}
                </div>
              </div>
            ))}
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
