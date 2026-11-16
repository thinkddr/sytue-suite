/**
 * El hilo visto desde el canal (D-ROOM-05).
 *
 * `IndicadorHilo` es la línea bajo una raíz con respuestas: los avatares de quienes
 * respondieron más recientemente, «N respuestas» y «Última respuesta hace 5 min», todo
 * un único botón que abre el hilo (Slack, Teams). Los avatares son decorativos: el
 * nombre accesible lo dan el número y la hora, que son el contenido.
 *
 * `RespondioEnHilo` es la línea sobre una respuesta mandada «también al canal»: dice
 * que es parte de un hilo y lleva a él, para que no se lea como un mensaje suelto.
 */
import type { JSX } from 'preact';
import { Avatar, Icon } from '@sytue/ui';
import { formatTimeAgo, type Locale } from '@sytue/i18n';
import type { ChatMessage } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { esDifundida } from '../lib/hilos.js';
import { ThreadsGlyph } from './glyphs.js';

export function IndicadorHilo({
  m,
  onOpen,
  locale,
  t,
  now,
}: {
  m: ChatMessage;
  onOpen?: (m: ChatMessage) => void;
  locale: Locale;
  t: T;
  /** Reloj inyectable para los tests; por defecto, ahora. */
  now?: number;
}): JSX.Element | null {
  const count = m.replyCount ?? 0;
  if (count <= 0) return null;
  const gente = m.threadParticipants ?? [];
  return (
    <button
      type="button"
      onClick={() => onOpen?.(m)}
      class="rounded-control text-primary-text hover:bg-selected sytue-focus-ring inline-flex items-center gap-1 px-1.5 py-0.5 text-xs font-medium transition-colors duration-150"
    >
      {gente.length > 0 ? (
        // Solapados como en Slack; el aro del color de la superficie los separa.
        <span class="flex -space-x-1.5" aria-hidden="true" data-avatares-hilo="">
          {gente.map((p) => (
            <Avatar key={p.id} name={p.name} size="xs" class="ring-surface rounded-full ring-2" />
          ))}
        </span>
      ) : (
        <Icon name="chevronRight" size={12} />
      )}
      <span>{t('thread.replyCountInThread', { count })}</span>
      {m.lastReplyAt ? (
        <span class="text-text-muted font-normal">
          {t('thread.lastReply', { when: formatTimeAgo(locale, m.lastReplyAt, now) })}
        </span>
      ) : null}
    </button>
  );
}

export function RespondioEnHilo({
  m,
  onOpen,
  t,
}: {
  m: ChatMessage;
  onOpen?: (m: ChatMessage) => void;
  t: T;
}): JSX.Element | null {
  if (!esDifundida(m)) return null;
  return (
    <p class="sytue-meta mb-0.5 flex flex-wrap items-center gap-1">
      <ThreadsGlyph size={12} />
      <span>{t('thread.repliedInThread')}</span>
      {onOpen ? (
        <>
          <span aria-hidden="true">·</span>
          <button
            type="button"
            onClick={() => onOpen(m)}
            class="text-primary-text sytue-focus-ring rounded-sm font-medium underline-offset-2 hover:underline"
          >
            {t('thread.viewThread')}
          </button>
        </>
      ) : null}
    </p>
  );
}
