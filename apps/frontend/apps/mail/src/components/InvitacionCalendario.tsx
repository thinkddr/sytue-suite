/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA TARJETA DE UNA INVITACIÓN DE FUERA EN EL LECTOR (D-MAIL-04).
 *
 * Un correo de invitación de Google, Outlook o Apple solo era un `invite.ics` que
 * había que bajar e importar a mano, y no había manera de contestar: quien organiza
 * se quedaba sin saber si ibas. Ahora el lector lo resume —qué, cuándo, dónde, quién
 * convoca— y ofrece Sí/Quizá/No: el evento entra en tu calendario con esa respuesta y
 * a quien organiza le llega el REPLY, que su calendario apunta solo.
 *
 * Contestar otra vez cambia la respuesta (no duplica el evento). Lo elegido se marca
 * con `aria-pressed`, como el «¿vas a ir?» de Calendar.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Icon, useToast } from '@sytue/ui';
import { formatDate, formatDateTime, formatTime, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { responderInvitacion } from '../client/mailApi.js';
import { RESPUESTAS, type Respuesta, type ResumenInvitacion } from '../lib/invitacion.js';

const ETIQUETA = {
  ACCEPTED: 'reader.invite.yes',
  TENTATIVE: 'reader.invite.maybe',
  DECLINED: 'reader.invite.no',
} as const;

const HECHO = {
  ACCEPTED: 'reader.invite.accepted',
  TENTATIVE: 'reader.invite.tentative',
  DECLINED: 'reader.invite.declined',
} as const;

/** Cuándo, en una línea: «mié, 12 nov 2026, 9:00–10:00» o solo la fecha si es de día entero. */
function cuando(t: T, locale: Locale, inv: ResumenInvitacion): string {
  if (!inv.start) return '';
  if (inv.allDay) {
    return `${formatDate(locale, inv.start, { dateStyle: 'full' })} · ${t('reader.invite.allDay')}`;
  }
  const inicio = formatDateTime(locale, inv.start, { dateStyle: 'full', timeStyle: 'short' });
  if (!inv.end) return inicio;
  const mismoDia = inv.end.toDateString() === inv.start.toDateString();
  return `${inicio}–${
    mismoDia
      ? formatTime(locale, inv.end)
      : formatDateTime(locale, inv.end, { dateStyle: 'medium', timeStyle: 'short' })
  }`;
}

export function InvitacionCalendario({
  t,
  locale,
  emailId,
  invitacion,
  responder = responderInvitacion,
}: {
  t: T;
  locale: Locale;
  emailId: string;
  invitacion: ResumenInvitacion;
  /** Inyectable en los tests; por defecto, el BFF de Mail. */
  responder?: typeof responderInvitacion | undefined;
}): JSX.Element {
  const [elegida, setElegida] = useState<Respuesta | null>(null);
  const [enviando, setEnviando] = useState<Respuesta | null>(null);
  const [sinAviso, setSinAviso] = useState(false);
  const toast = useToast();

  async function contestar(r: Respuesta): Promise<void> {
    if (enviando || r === elegida) return;
    setEnviando(r);
    try {
      const res = await responder(emailId, r);
      setElegida(r);
      setSinAviso(!res.replySent);
    } catch {
      toast.notify({ tone: 'danger', title: t('reader.invite.failed') });
    } finally {
      setEnviando(null);
    }
  }

  const quien = invitacion.organizerName || invitacion.organizer;
  return (
    <section
      aria-label={t('reader.invite.label')}
      class="border-border bg-surface-raised mb-5 rounded-lg border p-4"
    >
      <div class="flex items-start gap-3">
        <span class="bg-surface-sunken text-primary-text flex size-10 shrink-0 items-center justify-center rounded-md">
          <Icon name="calendar" size={20} />
        </span>
        <div class="min-w-0 flex-1">
          <p class="sytue-overline text-text-subtle">{t('reader.invite.label')}</p>
          <h3 class="text-text truncate font-semibold">
            {invitacion.summary || t('reader.invite.untitled')}
          </h3>
          <p class="text-text-muted text-sm">{cuando(t, locale, invitacion)}</p>
          {invitacion.location ? (
            <p class="text-text-muted truncate text-sm">{invitacion.location}</p>
          ) : null}
          <p class="text-text-subtle truncate text-sm" title={invitacion.organizer}>
            {t('reader.invite.organizer', { name: quien })}
          </p>
        </div>
      </div>
      <div
        role="group"
        aria-label={t('reader.invite.question')}
        class="mt-4 flex flex-wrap items-center gap-2"
      >
        <span class="text-text me-1 text-sm font-medium">{t('reader.invite.question')}</span>
        {RESPUESTAS.map((r) => (
          <Button
            key={r}
            size="sm"
            variant={elegida === r ? 'primary' : 'secondary'}
            aria-pressed={elegida === r}
            loading={enviando === r}
            disabled={enviando !== null}
            onClick={() => void contestar(r)}
          >
            {t(ETIQUETA[r])}
          </Button>
        ))}
      </div>
      <p role="status" class="text-text-muted mt-2 text-sm empty:hidden">
        {elegida ? t(HECHO[elegida]) : ''}
        {elegida && sinAviso ? ` ${t('reader.invite.replyNotSent')}` : ''}
      </p>
    </section>
  );
}
