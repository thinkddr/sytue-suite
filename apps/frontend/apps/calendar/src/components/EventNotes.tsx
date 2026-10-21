/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EventNotes — las notas de Sytue Notepads que cuelgan de ESTE evento, dentro de su
 * ficha, más el atajo para escribir una nueva ya vinculada.
 *
 * El vínculo lo guarda Notes, no Calendar: aquí solo se pregunta (ver
 * `src/pages/api/notes.ts`). Por eso la sección aparece y desaparece sola según
 * lo que devuelva esa consulta, y no rompe nada si Sytue Notepads no está
 * desplegado en esta instalación.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Icon, withBase } from '@sytue/ui';
import type { T } from '../i18n/index.js';

interface NotaVinculada {
  id: string;
  notebook_id: string;
  title: string;
  encrypted: boolean;
}

interface Props {
  t: T;
  /**
   * UID del evento (RFC 5545), NO su identificador interno: es el que ve la
   * superficie CalDAV y el único estable entre las dos aplicaciones. Sytue Notepads
   * guarda exactamente este valor como destino del enlace.
   */
  eventUid: string;
  /** Título del evento: viaja al crear la nota para que nazca con nombre. */
  summary: string;
}

export function EventNotes({ t, eventUid, summary }: Props): JSX.Element | null {
  const [notas, setNotas] = useState<NotaVinculada[] | null>(null);
  const [notesUrl, setNotesUrl] = useState<string | null>(null);

  useEffect(() => {
    let vivo = true;
    setNotas(null);
    setNotesUrl(null);
    fetch(withBase(import.meta.env.BASE_URL, `/api/notes?event=${encodeURIComponent(eventUid)}`), {
      credentials: 'include',
      headers: { accept: 'application/json' },
    })
      .then((r) => (r.ok ? r.json() : { notes: [], notesUrl: null }))
      .then((d: { notes?: NotaVinculada[]; notesUrl?: string | null }) => {
        if (!vivo) return;
        setNotas(d.notes ?? []);
        setNotesUrl(d.notesUrl ?? null);
      })
      .catch(() => vivo && setNotas([]));
    return () => {
      vivo = false;
    };
  }, [eventUid]);

  // Sin Sytue Notepads desplegado no hay sección: ofrecer «crear nota» y que no
  // exista adónde ir es peor que no ofrecerlo.
  if (notas === null || !notesUrl) return null;

  const crear = `${notesUrl}?link=calendar_event:${encodeURIComponent(eventUid)}&label=${encodeURIComponent(summary)}`;

  return (
    <div class="border-border mt-3 grid gap-1.5 border-t pt-2.5">
      <span class="sytue-meta">{t('notes.linked')}</span>
      {notas.length > 0 ? (
        <ul class="grid gap-0.5">
          {notas.map((n) => (
            <li key={n.id}>
              <a
                href={`${notesUrl}?notebook=${encodeURIComponent(n.notebook_id)}&note=${encodeURIComponent(n.id)}`}
                class="text-primary-text sytue-focus-ring flex items-center gap-1.5 rounded text-xs hover:underline"
              >
                <Icon name={n.encrypted ? 'lock' : 'text'} size={13} class="shrink-0" />
                <span class="truncate">{n.title || t('notes.untitled')}</span>
              </a>
            </li>
          ))}
        </ul>
      ) : null}
      <a
        href={crear}
        class="text-primary-text sytue-focus-ring inline-flex items-center gap-1.5 rounded text-xs font-medium hover:underline"
      >
        <Icon name="plus" size={13} />
        {t('notes.create')}
      </a>
    </div>
  );
}
