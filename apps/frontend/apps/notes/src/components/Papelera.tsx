/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La papelera: restaurar o borrar del todo. Cada fila dice de qué cuaderno viene la
 * nota y cuándo se vaciará sola, como «N días» en Notas de Apple: sin eso, la purga
 * automática del servicio era una sorpresa. Es `Papelera` de `paneles.go` en Fyne.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, EmptyState } from '@sytue/ui';
import type { TrashedNote } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import { caducidad } from '../client/papelera.js';
import { resumenNota } from '../client/diario.js';
import { Glyph } from './glyphs.js';

/** La línea de debajo del título: «Cuaderno · Se borra en N días». */
export function metaPapelera(t: T, n: TrashedNote, ahora: Date): string {
  const c = caducidad(n, ahora);
  const cuando = c.clave === 'trash.purgeIn' ? t('trash.purgeIn', { count: c.dias }) : t(c.clave);
  return [n.notebook_title, cuando].filter(Boolean).join(' · ');
}

export function PanelPapelera({
  t,
  notes,
  ahora: ahoraFijo,
  onRestore,
  onPurge,
  onVaciar,
}: {
  t: T;
  notes: TrashedNote[];
  /** El reloj, inyectable para las pruebas. */
  ahora?: Date;
  onRestore: (id: string) => void;
  onPurge: (id: string) => void;
  /** Vaciar la papelera entera: lo que la gente viene a hacer aquí. */
  onVaciar: () => void;
}): JSX.Element {
  // El reloj se lee una vez al montar la vista: los días no cambian mientras se mira.
  const [montada] = useState(() => new Date());
  const ahora = ahoraFijo ?? montada;
  if (notes.length === 0) {
    return (
      <div class="notes-col notes-page">
        <div class="notes-page-head">
          <h2 class="notes-page-title">{t('trash.title')}</h2>
        </div>
        <EmptyState compact title={t('trash.empty')} />
      </div>
    );
  }
  return (
    <div class="notes-col notes-page">
      <div class="notes-page-head">
        <h2 class="notes-page-title">{t('trash.title')}</h2>
        <Button size="sm" variant="ghost" onClick={onVaciar}>
          {t('trash.emptyAll')}
        </Button>
      </div>
      <ul class="notes-rows">
        {notes.map((n) => (
          <li key={n.id} class="notes-row items-center">
            <span class="notes-stream-glyph mt-0!">
              {n.kind === 'todo' ? (
                <Glyph name="todo" size={14} />
              ) : (
                <span class="notes-stream-bullet" aria-hidden="true" />
              )}
            </span>
            <span class="min-w-0 flex-1">
              <span class="text-text block truncate text-base">
                {n.title || resumenNota(n) || t('note.untitled')}
              </span>
              <span class="sytue-meta block truncate">{metaPapelera(t, n, ahora)}</span>
            </span>
            <span class="notes-row-actions">
              <Button size="xs" variant="ghost" onClick={() => onRestore(n.id)}>
                {t('trash.restore')}
              </Button>
              <Button size="xs" variant="ghost" onClick={() => onPurge(n.id)}>
                {t('trash.purge')}
              </Button>
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
