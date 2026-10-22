/**
 * CardView — el objeto rico de un mensaje: ENCUESTA o TAREA.
 *
 * La pregunta de la encuesta y el título de la tarea son el CUERPO del mensaje (los
 * pinta MessageView justo encima); aquí va solo lo que el texto no puede llevar: las
 * opciones votables con su resultado, o la casilla de «hecha» con responsable y fecha.
 *
 * Accesible: cada opción es un botón con `aria-pressed` (radio/casilla según la
 * encuesta admita una o varias respuestas) y la tarea es un `role="checkbox"`. El
 * resultado NUNCA se comunica solo con la barra: van el número de votos y el
 * porcentaje en texto, y lo votado por el usuario lleva además su marca (WCAG 1.4.1).
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Icon } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { MessageCard } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';

interface Props {
  card: MessageCard;
  messageId: string;
  /** Marca (add) o retira un valor de la tarjeta. Ausente = tarjeta de solo lectura. */
  onVote?: (messageId: string, value: string, add: boolean) => void;
  locale: Locale;
  t: T;
}

/** Voto del usuario y total de un valor concreto de la tarjeta. */
function voteOf(card: MessageCard, value: string): { count: number; mine: boolean } {
  const v = card.votes?.find((x) => x.value === value);
  return { count: v?.count ?? 0, mine: Boolean(v?.mine) };
}

/** Total de votos EMITIDOS (no de votantes: con `multi` una persona emite varios). */
function totalVotes(card: MessageCard): number {
  return (card.votes ?? []).reduce((acc, v) => acc + v.count, 0);
}

/** Encuesta: opciones con barra de resultado, recuento y porcentaje en texto. */
function Poll({ card, messageId, onVote, t }: Omit<Props, 'locale'>): JSX.Element {
  const total = totalVotes(card);
  return (
    <div class="border-border bg-surface-sunken mt-1.5 rounded-md border p-2.5">
      <ul class="flex flex-col gap-1.5">
        {(card.options ?? []).map((opt, i) => {
          const value = String(i);
          const { count, mine } = voteOf(card, value);
          const pct = total > 0 ? Math.round((count / total) * 100) : 0;
          return (
            <li key={value}>
              <button
                type="button"
                disabled={!onVote}
                aria-pressed={mine}
                onClick={() => onVote?.(messageId, value, !mine)}
                class={`sytue-focus-ring border-border hover:bg-bg-subtle relative w-full overflow-hidden rounded-md border px-2.5 py-1.5 text-left transition-colors duration-150 disabled:opacity-70 ${
                  mine ? 'border-border-strong' : ''
                }`}
              >
                {/* Barra de resultado: decorativa (el dato va en texto, a la derecha).
                    Al 15 % del acento para que el texto encima conserve su contraste:
                    la barra ambienta, no comunica. */}
                <span
                  aria-hidden="true"
                  class="bg-primary/15 absolute inset-y-0 left-0"
                  style={{ width: `${pct}%` }}
                />
                <span class="relative flex items-center gap-2">
                  <span class="text-text min-w-0 flex-1 break-words">{opt}</span>
                  {mine ? <Icon name="check" size={14} class="text-primary-text shrink-0" /> : null}
                  <span class="sytue-meta shrink-0 tabular-nums">
                    {t('card.pollOptionResult', { count, pct })}
                  </span>
                </span>
              </button>
            </li>
          );
        })}
      </ul>
      <p class="sytue-meta mt-1.5">
        {t('card.pollTotal', { count: total })}
        {card.multi ? ` · ${t('card.pollMulti')}` : ''}
      </p>
    </div>
  );
}

/** Tarea: casilla «hecha», responsable y fecha límite. */
function Task({ card, messageId, onVote, locale, t }: Props): JSX.Element {
  const { count, mine } = voteOf(card, 'done');
  const done = count > 0;
  const due = card.due ? new Date(card.due) : null;
  // La hora se toma al montar la tarjeta: el render no puede leer el reloj.
  const [ahora] = useState(Date.now);
  const overdue = due !== null && !done && due.getTime() < ahora;
  return (
    <div class="border-border bg-surface-sunken mt-1.5 flex flex-wrap items-center gap-2 rounded-md border p-2.5">
      <button
        type="button"
        role="checkbox"
        aria-checked={done}
        disabled={!onVote}
        onClick={() => onVote?.(messageId, 'done', !mine)}
        class="sytue-focus-ring border-border hover:bg-bg-subtle rounded-control flex items-center gap-2 border px-2.5 py-1.5 transition-colors duration-150 disabled:opacity-70"
      >
        <Icon
          name={done ? 'check' : 'square'}
          size={14}
          class={done ? 'text-primary-text' : 'text-text-subtle'}
        />
        <span class="text-text">{done ? t('card.taskDone') : t('card.taskMarkDone')}</span>
      </button>
      {card.assigneeName ? (
        <span class="sytue-meta">{t('card.taskAssignee', { name: card.assigneeName })}</span>
      ) : null}
      {due ? (
        <span class={overdue ? 'text-danger-text text-xs' : 'sytue-meta'}>
          {overdue ? `${t('card.taskOverdue')} · ` : ''}
          {t('card.taskDue', {
            date: new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(due),
          })}
        </span>
      ) : null}
    </div>
  );
}

export function CardView(props: Props): JSX.Element | null {
  if (props.card.kind === 'task') return <Task {...props} />;
  if (props.card.kind === 'poll' && (props.card.options?.length ?? 0) > 0) {
    const { locale: _locale, ...rest } = props;
    return <Poll {...rest} />;
  }
  return null;
}
