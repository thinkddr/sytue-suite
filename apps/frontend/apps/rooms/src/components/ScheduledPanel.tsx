/**
 * ScheduledPanel — panel lateral (RHS) de mensajes PROGRAMADOS del usuario (Ola 12).
 * Mismo patrón que MessageListPanel (cabecera sólida bg-header + cerrar, cuerpo
 * desplazable con filas), pero cada fila lleva su instante de envío y las acciones
 * del programado: reprogramar (datetime-local nativo), enviar ahora y cancelar.
 *
 * ponytail: sin virtualización — los programados de un usuario son un conjunto
 * acotado (los redacta a mano); si crecieran, portar a <VirtualList> como el feed.
 * La lista es una foto al abrir el panel: un programado que VENZA mientras está
 * abierto sigue en la lista hasta recargar (llega al canal por WS igualmente).
 */
import type { JSX } from 'preact';
import { EmptyState, Illustration, Avatar, Button, Icon, Skeleton, IconButton } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { ScheduledMessage } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { MessageBody } from './MessageBody.js';

interface Props {
  scheduled: ScheduledMessage[];
  loading: boolean;
  error?: boolean;
  /** channelId -> nombre de canal, para mostrar el destino (cross-canal). */
  channelNames?: Record<string, string>;
  onCancel: (m: ScheduledMessage) => void;
  onSendNow: (m: ScheduledMessage) => void;
  /** Reprograma al nuevo instante (ISO 8601). */
  onReschedule: (m: ScheduledMessage, sendAtISO: string) => void;
  onClose: () => void;
  onRetry?: () => void;
  /** Idioma activo (formato de fecha/hora) y traductor ya atado a él. */
  locale: Locale;
  t: T;
}

/** Etiqueta legible de un instante ISO (fecha + hora local, en el idioma del usuario). */
function whenLabel(locale: Locale, iso: string): string {
  return formatDateTime(locale, iso, {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
}

/** ISO 8601 -> valor de <input type=datetime-local> (hora local, sin zona). */
function toLocalInputValue(iso: string): string {
  const d = new Date(iso);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

/** Valor `min` del datetime-local = ahora (hora local), impide reprogramar al pasado. */
function localNowValue(): string {
  return new Date(Date.now() - new Date().getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

export function ScheduledPanel({
  scheduled,
  loading,
  error = false,
  channelNames,
  onCancel,
  onSendNow,
  onReschedule,
  onClose,
  onRetry,
  locale,
  t,
}: Props): JSX.Element {
  return (
    <aside
      class="sytue-sheet-corners border-border bg-surface marco:overflow-clip flex h-full w-full shrink-0 flex-col border-l md:w-[var(--rooms-rhs-width)] md:[--sytue-sheet-bl:0px] md:[--sytue-sheet-tl:0px]"
      aria-label={t('scheduled.aria')}
    >
      <header class="bg-header text-header-fg flex h-14 shrink-0 items-center justify-between gap-2 px-4">
        <h2 class="text-md text-header-fg leading-tight font-semibold">{t('scheduled.title')}</h2>
        <IconButton
          size="lg"
          tone="header"
          label={t('scheduled.close')}
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
        ) : scheduled.length === 0 ? (
          <EmptyState
            class="py-4"
            title={t('scheduled.empty')}
            illustration={<Illustration name="emptyChat" size={96} />}
          />
        ) : (
          scheduled.map((m) => (
            <div key={m.id} class="border-border flex gap-3 border-b px-4 py-3 last:border-b-0">
              <div class="w-9 shrink-0 pt-0.5">
                <Avatar name={m.author.name} size="sm" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="flex min-w-0 flex-wrap items-baseline gap-x-2">
                  <span class="text-text text-sm font-semibold">{m.author.name}</span>
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
                    <MessageBody body={m.body} authorName={m.author.name} mentionNames={[]} />
                  </p>
                ) : (
                  <p class="text-text-subtle text-sm italic">{t('message.attachmentsOnly')}</p>
                )}
                {/* Las @menciones no viajan en la lista de programados (ver la nota de
                    scheduled.go); se resaltan al publicarse el mensaje en el canal. */}
                <p class="text-text-muted mt-1 text-xs font-medium">
                  <time dateTime={m.sendAt}>
                    {t('scheduled.sendsAt', { when: whenLabel(locale, m.sendAt) })}
                  </time>
                </p>
                <div class="mt-2 flex flex-wrap items-center gap-2">
                  <label class="sytue-meta inline-flex items-center gap-1">
                    <span class="sr-only">{t('scheduled.reschedule')}</span>
                    <input
                      type="datetime-local"
                      value={toLocalInputValue(m.sendAt)}
                      min={localNowValue()}
                      aria-label={t('scheduled.rescheduleOf', {
                        when: whenLabel(locale, m.sendAt),
                      })}
                      onChange={(e) => {
                        const v = e.currentTarget.value;
                        if (v) onReschedule(m, new Date(v).toISOString());
                      }}
                      class="border-border bg-bg text-text sytue-focus-ring rounded-md border px-2 py-1 text-xs"
                    />
                  </label>
                  <Button size="sm" variant="secondary" onClick={() => onSendNow(m)}>
                    {t('scheduled.sendNow')}
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => onCancel(m)}>
                    {t('action.cancel')}
                  </Button>
                </div>
              </div>
            </div>
          ))
        )}
      </div>
    </aside>
  );
}
