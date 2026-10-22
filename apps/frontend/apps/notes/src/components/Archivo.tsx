/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las dos vistas que cruzan cuadernos sin ser una búsqueda: «Fijadas» (lo que
 * se fijó arriba en cualquier cuaderno) y «Archivo» (los cuadernos que salieron
 * del árbol sin borrarse). La lista de resultados es la de la búsqueda.
 */
import type { JSX } from 'preact';
import { Button } from '@sytue/ui';
import type { Notebook, NoteHit } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import { Glyph } from './glyphs.js';
import { Resaltado } from './Resaltado.js';

/**
 * Filas de notas de varios cuadernos: título y fragmento, y lleva a la nota. Con
 * `consulta` (la búsqueda), lo buscado sale resaltado.
 */
export function ListaDeHits({
  hits,
  consulta = '',
}: {
  hits: readonly NoteHit[];
  consulta?: string;
}): JSX.Element {
  return (
    <ul class="notes-rows">
      {hits.map((h) => {
        // Una nota sin título es su propio texto, no el nombre de su cuaderno.
        const titulo = h.title || h.snippet.split('\n')[0]!.trim() || h.notebook;
        const fragmento = h.title ? h.snippet : '';
        return (
          <li key={h.id}>
            <a
              href={`?notebook=${encodeURIComponent(h.notebook_id)}&note=${encodeURIComponent(h.id)}`}
              class="notes-row sytue-focus-ring"
            >
              <span class="notes-stream-glyph">
                {h.kind === 'todo' || h.kind === 'link' ? (
                  <Glyph name={h.kind} size={14} />
                ) : (
                  <span class="notes-stream-bullet" aria-hidden="true" />
                )}
              </span>
              <span class="min-w-0 flex-1">
                <span class="text-text block truncate text-base">
                  <Resaltado texto={titulo} consulta={consulta} />
                </span>
                {fragmento ? (
                  <span class="sytue-meta line-clamp-2 block">
                    <Resaltado texto={fragmento} consulta={consulta} />
                  </span>
                ) : null}
              </span>
              {h.notebook ? (
                <span class="text-text-subtle max-w-40 shrink-0 truncate pt-0.5 text-xs">
                  {h.notebook}
                </span>
              ) : null}
            </a>
          </li>
        );
      })}
    </ul>
  );
}

/** Una página sin nada: su título y una frase en voz baja, sin ilustración. */
function Vacia({ titulo, frase }: { titulo: string; frase: string }): JSX.Element {
  return (
    <div class="notes-col notes-page">
      <div class="notes-page-head">
        <h2 class="notes-page-title">{titulo}</h2>
      </div>
      <p class="text-text-subtle text-base">{frase}</p>
    </div>
  );
}

/** «Fijadas»: las notas fijadas de todos los cuadernos visibles. */
export function PanelFijadas({ t, hits }: { t: T; hits: NoteHit[] | null }): JSX.Element | null {
  if (hits === null) return null;
  if (hits.length === 0) return <Vacia titulo={t('focus.pinned')} frase={t('pinned.empty')} />;
  return (
    <div class="notes-col notes-page">
      <div class="notes-page-head">
        <h2 class="notes-page-title">{t('focus.pinned')}</h2>
      </div>
      <ListaDeHits hits={hits} />
    </div>
  );
}

/** «Archivo»: la cabeza de cada rama archivada, para abrirla o devolverla al árbol. */
export function PanelArchivo({
  t,
  cuadernos,
  userId,
  onAbrir,
  onDesarchivar,
}: {
  t: T;
  cuadernos: Notebook[];
  /** Solo el dueño devuelve un cuaderno; a los demás el servicio les dice 404. */
  userId: string;
  onAbrir: (nb: Notebook) => void;
  onDesarchivar: (nb: Notebook) => void;
}): JSX.Element {
  if (cuadernos.length === 0)
    return <Vacia titulo={t('archive.title')} frase={t('archive.empty')} />;
  return (
    <div class="notes-col notes-page">
      <div class="notes-page-head">
        <h2 class="notes-page-title">{t('archive.title')}</h2>
      </div>
      <ul class="notes-rows">
        {cuadernos.map((nb) => (
          <li key={nb.id} class="notes-row items-center">
            <span class="notes-stream-glyph mt-0!">
              <Glyph name="notebook" size={14} />
            </span>
            <button
              type="button"
              class="text-text sytue-focus-ring min-w-0 flex-1 truncate rounded-sm text-left text-base"
              onClick={() => onAbrir(nb)}
            >
              {nb.title}
            </button>
            <span class="text-text-subtle shrink-0 text-xs tabular-nums">{nb.note_count}</span>
            {nb.owner_sub === userId ? (
              <span class="notes-row-actions">
                <Button size="xs" variant="ghost" onClick={() => onDesarchivar(nb)}>
                  {t('archive.unarchive')}
                </Button>
              </span>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}
