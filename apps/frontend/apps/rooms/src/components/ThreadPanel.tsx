/**
 * ThreadPanel — panel lateral de HILO de Sytue Chat (estilo Mattermost): mensaje
 * raíz + respuestas en orden cronológico + composer de respuesta. Se abre desde el
 * indicador «N respuestas» de un mensaje. Carga por REST (BFF /api/thread); las
 * respuestas nuevas llegan por WebSocket (las inserta el padre). docs/design.md §7.
 *
 * ponytail: el hilo no muestra reacciones (solo cuerpo + menciones); las reacciones
 * viven en el histórico del canal. Portar QUICK_REACTIONS aquí si se pide.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { BlockedFold } from './Moderation.js';
import { Avatar, Skeleton, Icon, cx, IconButton } from '@sytue/ui';
import { formatTime, type Locale } from '@sytue/i18n';
import type { ChatMessage, ChannelMember, Attachment } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { MessageBody } from './MessageBody.js';
import { CardView } from './CardView.js';
import { Attachments } from './Attachments.js';
import { Composer } from './Composer.js';
import { ThreadsGlyph } from './glyphs.js';

interface Props {
  root: ChatMessage;
  replies: ChatMessage[];
  loading: boolean;
  error?: boolean;
  members?: ChannelMember[];
  /** sub -> nombre legible, para resaltar @menciones. */
  memberNames?: Record<string, string>;
  channelName?: string;
  /** ¿El usuario sigue este hilo? Refleja el botón Seguir/Siguiendo (Ola 9). */
  following?: boolean;
  /** Alterna seguir/dejar de seguir el hilo. */
  onToggleFollow?: () => void;
  onClose: () => void;
  onRetry?: () => void;
  onSendReply: (body: string, mentions: string[], attachments: Attachment[]) => void;
  /** Marca o retira un valor de la tarjeta de un mensaje del hilo (una encuesta se
   *  responde igual de bien desde aquí que desde el canal). */
  onCardVote?: (messageId: string, value: string, add: boolean) => void;
  /** Subs bloqueados por el usuario: sus mensajes van plegados (ADR 0073). */
  blockedSubs?: ReadonlySet<string>;
  /** Scope de borrador por cuenta; sin red se redacta localmente, pero no se envía. */
  ownerId: string;
  disabled?: boolean;
  offline?: boolean;
  /** Idioma activo (reloj de 12/24 h) y traductor ya atado a él. */
  locale: Locale;
  t: T;
}

/** Hora corta en el idioma del usuario (el reloj de 12/24 h lo decide el locale). */
function timeLabel(locale: Locale, iso: string): string {
  return formatTime(locale, iso, { hour: '2-digit', minute: '2-digit' });
}

function MessageRow({
  m,
  memberNames,
  blocked = false,
  onCardVote,
  locale,
  t,
}: {
  m: ChatMessage;
  memberNames?: Record<string, string>;
  /** El autor está bloqueado: el mensaje se ve plegado hasta que se pide verlo. */
  blocked?: boolean;
  onCardVote?: (messageId: string, value: string, add: boolean) => void;
  locale: Locale;
  t: T;
}): JSX.Element {
  const mentionNames = (m.mentions ?? []).map((s) => memberNames?.[s] ?? s);
  const [mostrado, setMostrado] = useState(false);
  if (blocked && !mostrado) {
    return (
      <div class="px-4 py-2">
        <BlockedFold t={t} onShow={() => setMostrado(true)} />
      </div>
    );
  }
  return (
    <div class="flex gap-3 px-4 py-2">
      <div class="w-9 shrink-0 pt-0.5">
        <Avatar name={m.author.name} size="sm" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex min-w-0 items-baseline gap-2">
          <span class="text-text min-w-0 truncate text-sm font-semibold" title={m.author.name}>
            {m.author.name}
          </span>
          <span class="sytue-meta shrink-0">{timeLabel(locale, m.createdAt)}</span>
        </div>
        {m.body ? (
          <p
            class={cx(
              'text-base break-words whitespace-pre-wrap',
              m.pending ? 'text-text-muted' : 'text-text',
            )}
          >
            <MessageBody body={m.body} authorName={m.author.name} mentionNames={mentionNames} />
            {m.pending ? <span class="sytue-meta ml-1">{t('message.sending')}</span> : null}
          </p>
        ) : null}
        <Attachments attachments={m.attachments} channelId={m.channelId} t={t} locale={locale} />
        {m.card ? (
          <CardView
            card={m.card}
            messageId={m.id}
            {...(onCardVote && !m.pending && !m.failed ? { onVote: onCardVote } : {})}
            locale={locale}
            t={t}
          />
        ) : null}
        {!m.body && m.pending ? <span class="sytue-meta">{t('message.sending')}</span> : null}
        {/* ponytail: respuesta fallida se marca sin botón «Reintentar» (reescribir
            para reenviar); el composer del canal sí lo tiene. Añadir si molesta. */}
        {m.failed ? (
          <p class="text-danger-text mt-0.5 flex items-center gap-1.5 text-xs">
            <Icon name="warning" size={12} />
            <span>{t('message.sendFailed')}</span>
          </p>
        ) : null}
      </div>
    </div>
  );
}

export function ThreadPanel({
  root,
  replies,
  loading,
  error = false,
  members,
  memberNames,
  blockedSubs,
  channelName,
  following = false,
  onToggleFollow,
  onClose,
  onRetry,
  onSendReply,
  onCardVote,
  ownerId,
  disabled = false,
  offline = false,
  locale,
  t,
}: Props): JSX.Element {
  const endRef = useRef<HTMLDivElement>(null);

  // Auto-scroll al llegar respuestas nuevas (el hilo es corto; siempre al final).
  useEffect(() => {
    endRef.current?.scrollIntoView({ block: 'end' });
  }, [replies.length]);

  const count = replies.length;

  return (
    <aside
      class="sytue-sheet-corners border-border bg-surface marco:overflow-clip flex h-full w-full shrink-0 flex-col border-l md:w-[var(--rooms-rhs-width)] md:[--sytue-sheet-bl:0px] md:[--sytue-sheet-tl:0px]"
      aria-label={t('thread.title')}
    >
      {/* Cabecera del hilo con el acento sólido del producto (bg-header), a juego
          con la topbar y la cabecera de canal. docs/design.md §2. */}
      <header class="bg-header text-header-fg flex h-14 shrink-0 items-center justify-between gap-2 px-4">
        <div class="flex min-w-0 flex-col justify-center">
          <h2 class="text-md text-header-fg leading-tight font-semibold">{t('thread.title')}</h2>
          {channelName ? (
            <p class="text-header-fg/80 truncate text-xs leading-tight">#{channelName}</p>
          ) : null}
        </div>
        <div class="flex shrink-0 items-center gap-1">
          {/* Seguir / Siguiendo el hilo (Ola 9): aria-pressed refleja el estado. */}
          {onToggleFollow ? (
            <button
              type="button"
              aria-pressed={following}
              aria-label={following ? t('thread.unfollow') : t('thread.follow')}
              title={following ? t('thread.followingHint') : t('thread.followHint')}
              onClick={onToggleFollow}
              class={cx(
                'text-header-fg hover:bg-chrome-hover sytue-focus-ring rounded-control inline-flex h-8 shrink-0 items-center gap-1.5 px-2.5 text-sm font-medium transition-colors duration-150',
                following && 'bg-chrome-active',
              )}
            >
              <ThreadsGlyph size={16} />
              <span>{following ? t('thread.following') : t('thread.followShort')}</span>
            </button>
          ) : null}
          {/* Botón sobre la cabecera de CROMO grafito: el hover/active son velos
              blancos (bg-chrome-*), no grises claros —sobre grafito no se ven—. */}
          <IconButton size="lg" tone="header" label={t('thread.close')} onClick={onClose}>
            <Icon name="close" size={18} />
          </IconButton>
        </div>
      </header>

      <div class="min-h-0 flex-1 overflow-y-auto py-2">
        {/* Mensaje raíz, destacado. */}
        <div class="border-border border-b pb-1">
          <MessageRow
            m={root}
            memberNames={memberNames ?? {}}
            blocked={blockedSubs?.has(root.author.id) ?? false}
            {...(onCardVote ? { onCardVote } : {})}
            locale={locale}
            t={t}
          />
        </div>

        {/* Separador con el conteo de respuestas. */}
        <div class="my-2 flex items-center gap-3 px-4" role="separator">
          <span class="text-text-muted shrink-0 text-xs font-semibold">
            {count === 0 ? t('thread.noReplies') : t('thread.replyCount', { count })}
          </span>
          <span class="bg-border h-px flex-1" />
        </div>

        {error ? (
          <div role="alert" class="text-danger-text px-4 py-3 text-sm">
            <p class="flex items-center gap-1.5">
              <Icon name="warning" size={14} />
              <span>{t('thread.loadFailed')}</span>
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
              {t('thread.loading')}
            </span>
            {[60, 45].map((w, i) => (
              <div key={i} class="flex gap-3">
                <Skeleton width="2rem" height="2rem" class="shrink-0" />
                <div class="flex min-w-0 flex-1 flex-col gap-2 pt-1">
                  <Skeleton width="6rem" height="0.75rem" />
                  <Skeleton width={`${w}%`} height="0.75rem" />
                </div>
              </div>
            ))}
          </div>
        ) : (
          replies.map((m) => (
            <MessageRow
              key={m.id}
              m={m}
              memberNames={memberNames ?? {}}
              blocked={blockedSubs?.has(m.author.id) ?? false}
              {...(onCardVote ? { onCardVote } : {})}
              locale={locale}
              t={t}
            />
          ))
        )}
        <div ref={endRef} />
      </div>

      <Composer
        // key + draftId propios del hilo: borrador aparte del canal y sin arrastrar
        // texto entre hilos al cambiar de raíz (Ola 8).
        key={root.id}
        channelName={channelName ?? ''}
        members={members ?? []}
        draftId={`${ownerId}:${root.channelId}:thread:${root.id}`}
        disabled={disabled}
        offline={offline}
        channelId={root.channelId}
        placeholder={t('thread.replyPlaceholder')}
        ariaLabel={t('thread.replyAria')}
        onSend={onSendReply}
        locale={locale}
        t={t}
      />
    </aside>
  );
}
