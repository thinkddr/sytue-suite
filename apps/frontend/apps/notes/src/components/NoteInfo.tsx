/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NoteInfo — «Información» de una nota: cuánto texto tiene y de cuándo es.
 *
 * Bear, Notion o Docs lo enseñan y aquí no se sabía en ningún sitio cuántas palabras
 * llevaba una nota, ni cuándo se creó: la fecha de edición solo salía, abreviada, en la
 * fila del árbol. Va en un diálogo desde el «…» y no en la tarjeta: es una consulta de
 * vez en cuando, y una línea fija de cifras en cada nota del lienzo sería ruido.
 *
 * Las cuentas son las de `estadisticas` sobre el texto plano, el mismo que cuenta Fyne
 * (`htmltext.Stats`). En un cuaderno cifrado se cuenta el claro que ya está en memoria;
 * cerrado, no hay texto que contar y se dice «—» en vez de un cero que mentiría.
 *
 * Las notas que apuntan aquí se piden AL ABRIR, como en BacklinksPanel: pedirlas al
 * pintar el lienzo serían cuarenta peticiones para no enseñar nada.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Modal } from '@sytue/ui';
import { formatDateTime, formatNumber, type Locale } from '@sytue/i18n';
import type { Note } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import * as api from '../client/notesApi.js';
import { estadisticas, textoDeNota } from '../client/extracto.js';

const SIN_DATO = '—';

export function NoteInfo({
  t,
  locale,
  note,
  titulo,
  contenido,
  cuaderno,
  onClose,
}: {
  t: T;
  locale: Locale;
  note: Note;
  /** El título en claro (descifrado si hace falta). */
  titulo: string;
  /** El cuerpo en claro, o `null` si la nota sigue cifrada y no hay con qué abrirla. */
  contenido: string | null;
  /** Nombre del cuaderno de la nota. */
  cuaderno: string;
  onClose: () => void;
}): JSX.Element {
  const [enlazan, setEnlazan] = useState<number | 'cargando' | 'error'>('cargando');

  useEffect(() => {
    // `vivo` corta la respuesta que llega después de cerrar, como en BacklinksPanel.
    let vivo = true;
    setEnlazan('cargando');
    api
      .retroenlaces(note.id)
      .then((r) => vivo && setEnlazan(r.notes?.length ?? 0))
      .catch(() => vivo && setEnlazan('error'));
    return () => {
      vivo = false;
    };
  }, [note.id]);

  const e = contenido === null ? null : estadisticas(textoDeNota(contenido));
  const numero = (n: number | undefined): string =>
    n === undefined ? SIN_DATO : formatNumber(locale, n);
  const fecha = (iso: string | undefined): string =>
    iso && !Number.isNaN(Date.parse(iso)) ? formatDateTime(locale, iso) : SIN_DATO;

  const filas: [string, string][] = [
    [t('noteInfo.words'), numero(e?.palabras)],
    [t('noteInfo.characters'), numero(e?.caracteres)],
    [t('noteInfo.reading'), e ? t('noteInfo.readingValue', { count: e.minutos }) : SIN_DATO],
    [t('noteInfo.created'), fecha(note.created_at)],
    [t('noteInfo.edited'), fecha(note.updated_at)],
    [t('noteInfo.notebook'), cuaderno || SIN_DATO],
    // Una tarea dice en qué está: la tarjeta solo lo enseña con la casilla y el tachado.
    ...(note.kind === 'todo'
      ? ([
          [
            t('noteInfo.task'),
            t(
              note.cancelled_at
                ? 'noteInfo.task.cancelled'
                : note.checked
                  ? 'noteInfo.task.done'
                  : 'noteInfo.task.open',
            ),
          ],
        ] as [string, string][])
      : []),
    // Lo que sale de la nota hacia otras notas y ficheros, junto a lo que entra.
    [t('noteInfo.links'), numero(note.links?.length ?? 0)],
    [
      t('noteInfo.backlinks'),
      enlazan === 'cargando'
        ? '…'
        : enlazan === 'error'
          ? SIN_DATO
          : t('notebook.notes', { count: enlazan }),
    ],
  ];

  return (
    <Modal open onClose={onClose} title={t('note.info')} size="sm">
      {/* De QUÉ nota: el modal tapa la tarjeta de la que salió. */}
      <p class="sytue-meta mb-3 break-words">{titulo || t('note.new')}</p>
      <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-base">
        {filas.map(([rotulo, valor]) => (
          <div key={rotulo} class="contents">
            <dt class="text-text-muted">{rotulo}</dt>
            <dd class="text-text min-w-0 text-right break-words tabular-nums">{valor}</dd>
          </div>
        ))}
      </dl>
    </Modal>
  );
}
