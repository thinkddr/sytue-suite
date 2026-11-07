/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GuestList — quién organiza el evento y qué ha contestado cada invitado (sí,
 * quizá, no o aún nada), en la vista rápida. Antes se podía contestar desde aquí
 * pero no ver quién iba: para saber si la reunión seguía en pie había que abrir el
 * formulario de edición, que además solo enseñaba las direcciones.
 *
 * La respuesta va en TEXTO dentro de la insignia, con su tono: el color solo
 * acompaña (docs/design.md §2.6). Sale solo si el evento tiene organizador o
 * invitados; un evento personal no gana una sección vacía.
 */
import type { JSX } from 'preact';
import { Badge, type BadgeTone } from '@sytue/ui';
import type { Guest, GuestStatus } from '../lib/ical.js';
import type { T } from '../i18n/index.js';

/** Tono de la insignia de cada respuesta. Lo usa también el formulario. */
export const GUEST_TONE: Record<GuestStatus, BadgeTone> = {
  accepted: 'success',
  tentative: 'warning',
  declined: 'danger',
  pending: 'neutral',
};

/** Clave del catálogo de cada respuesta. */
export function guestStatusLabel(t: T, status: GuestStatus): string {
  return t(`guests.status.${status}`);
}

/** Orden de la lista: primero quien va, después quizá, a la espera y al final no. */
const ORDEN: Record<GuestStatus, number> = { accepted: 0, tentative: 1, pending: 2, declined: 3 };

export function sortGuests(guests: Guest[]): Guest[] {
  return [...guests].sort((a, b) => ORDEN[a.status] - ORDEN[b.status]);
}

interface Props {
  t: T;
  organizer: { email: string; name?: string | undefined } | null;
  guests: Guest[];
  /** En la hoja del teléfono el texto es `text-sm`; en el popover, `text-xs`. */
  large?: boolean;
}

export function GuestList({ t, organizer, guests, large }: Props): JSX.Element | null {
  if (!organizer && guests.length === 0) return null;
  const size = large ? 'text-sm' : 'text-xs';
  return (
    <div class="flex flex-col gap-1.5">
      {organizer ? (
        <dl class={`flex min-w-0 items-baseline gap-1.5 ${size}`}>
          <dt class="text-text-muted shrink-0">{t('guests.organizer')}</dt>
          <dd class="text-text min-w-0 truncate" title={organizer.email}>
            {organizer.name ?? organizer.email}
          </dd>
        </dl>
      ) : null}
      {guests.length > 0 ? (
        <>
          <span class="sytue-meta">{t('event.guests')}</span>
          <ul class="flex flex-col gap-1" aria-label={t('event.guests')}>
            {sortGuests(guests).map((g) => (
              <li key={g.email} class={`flex min-w-0 items-center gap-2 ${size}`}>
                <span class="text-text min-w-0 flex-1 truncate" title={g.email}>
                  {g.name ?? g.email}
                </span>
                <Badge tone={GUEST_TONE[g.status]} class="shrink-0">
                  {guestStatusLabel(t, g.status)}
                </Badge>
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </div>
  );
}
