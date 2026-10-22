/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Presentar con Cast» (Sytue Diapos), lo que ve quien mira: en directo, en qué
 * diapositiva va quien presenta; en la grabación, un capítulo por cada cambio de
 * diapositiva para saltar a ella. Solo hay la posición —número y título—: el contenido
 * de la presentación y las notas de quien presenta no salen de Diapos.
 *
 * El directo se sondea cada 3 s (la posición es una fila pequeña y el sondeo no necesita
 * sesión); la grabación se pide una vez.
 *
 * ponytail: sondeo y no empuje por el WebSocket del chat, que solo está abierto con el chat
 * encendido. Pasar a una trama `slide` por el hub cuando haya muchas personas mirando.
 * Los capítulos cuentan desde que empezó el directo y la grabación arranca unos segundos
 * después con la primera pista: el salto puede caer algo antes de lo exacto.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { useSondeo } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { presentacion } from '../client/castApi.js';
import { duracion } from '../lib/formato.js';
import type { Presentacion } from '../lib/tipos.js';

export function Diapositivas({
  t,
  videoId,
  conSesion,
  enDirecto,
  activo,
  saltar,
}: {
  t: T;
  videoId: string;
  conSesion: boolean;
  /** El directo está en emisión: se sondea. Si no, es la grabación (o un directo acabado). */
  enDirecto: boolean;
  /** Hay conexión. */
  activo: boolean;
  /** Lleva la reproducción a ese instante (ms) de la grabación. */
  saltar: (ms: number) => void;
}): JSX.Element | null {
  const [p, setP] = useState<Presentacion | null>(null);
  const cargar = (): void => {
    presentacion(videoId, conSesion)
      .then(setP)
      .catch(() => undefined); // sin presentación o sin red: no se pinta nada
  };
  useEffect(cargar, [videoId, conSesion, enDirecto]);
  useSondeo(cargar, 3_000, enDirecto && activo);

  if (!p) return null;
  const actual = t('watch.slides.now', { n: p.index + 1, total: p.total });
  if (enDirecto) {
    return (
      <p class="text-text-muted bg-surface-sunken rounded-md px-3 py-2 text-sm" aria-live="polite">
        <span class="text-text font-semibold">{actual}</span>
        {p.title ? <span> · {p.title}</span> : null}
      </p>
    );
  }
  if (p.marks.length === 0) return null;
  return (
    <section class="flex flex-col gap-2" aria-label={t('watch.slides.title')}>
      <h2 class="text-text text-sm font-semibold">{t('watch.slides.title')}</h2>
      <ol class="flex max-h-56 flex-col overflow-auto">
        {p.marks.map((m, i) => (
          <li key={`${m.at_ms}-${i}`}>
            <button
              type="button"
              class="hover:bg-bg-subtle sytue-focus-ring flex min-h-11 w-full items-baseline gap-3 rounded-md px-2 py-1 text-left text-sm"
              onClick={() => saltar(m.at_ms)}
            >
              <span class="text-text-muted w-12 shrink-0 tabular-nums">{duracion(m.at_ms)}</span>
              <span class="text-text min-w-0 truncate">
                {t('watch.slides.chapter', { n: m.index + 1 })}
                {m.title ? ` · ${m.title}` : ''}
              </span>
            </button>
          </li>
        ))}
      </ol>
    </section>
  );
}
