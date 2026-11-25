/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ConMarcas — un texto (la descripción, un comentario) con sus marcas de tiempo
 * pulsables: «a partir del 12:05» salta allí. Sin `saltar` (un directo, la página sin
 * reproductor) el texto sale tal cual. Y Capitulos, el índice de la descripción.
 */
import type { JSX } from 'preact';
import type { T } from '../i18n/index.js';
import { duracion } from '../lib/formato.js';
import { trozosDe, type Capitulo } from '../lib/marcas.js';

const MARCA =
  'sytue-focus-ring text-primary-text rounded-sm font-medium tabular-nums hover:underline';

export function ConMarcas({
  t,
  texto,
  saltar,
}: {
  t: T;
  texto: string;
  saltar?: ((segundos: number) => void) | undefined;
}): JSX.Element {
  if (!saltar) return <>{texto}</>;
  return (
    <>
      {trozosDe(texto).map((p, i) =>
        p.segundos === undefined ? (
          p.texto
        ) : (
          <button
            key={i}
            type="button"
            class={MARCA}
            aria-label={t('watch.jumpTo', { time: p.texto })}
            onClick={() => saltar(p.segundos!)}
          >
            {p.texto}
          </button>
        ),
      )}
    </>
  );
}

/** «Capítulos»: cada uno con su marca y su título, y pulsarlo lleva allí. */
export function Capitulos({
  t,
  capitulos,
  saltar,
}: {
  t: T;
  capitulos: readonly Capitulo[];
  saltar: (segundos: number) => void;
}): JSX.Element {
  return (
    <nav aria-label={t('watch.chapters')} class="border-border mt-3 border-t pt-3">
      <h2 class="text-text text-sm font-semibold">{t('watch.chapters')}</h2>
      <ol class="mt-1 flex flex-col">
        {capitulos.map((c) => (
          <li key={c.segundos}>
            <button
              type="button"
              class="sytue-focus-ring hover:bg-surface-sunken flex w-full items-baseline gap-3 rounded-sm px-1 py-1 text-left text-sm"
              onClick={() => saltar(c.segundos)}
            >
              <span class="text-primary-text w-14 shrink-0 font-medium tabular-nums">
                {duracion(c.segundos * 1000)}
              </span>
              <span class="text-text min-w-0 break-words">{c.titulo}</span>
            </button>
          </li>
        ))}
      </ol>
    </nav>
  );
}
