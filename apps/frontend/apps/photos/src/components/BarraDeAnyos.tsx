/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La barra de años de la galería: saltar a 2019 sin desplazarse por siete años de
 * fotos con la rueda. Cada año es un botón con `aria-pressed`, y «Recientes»
 * vuelve al principio de la línea.
 *
 * En móvil es UNA fila que se desliza de lado (scrollX de @sytue/ui): con diez
 * años plegados en tres filas, la barra se comería la primera pantalla de fotos.
 */
import { Button, cx, FILA_UNICA_EN_MOVIL, MASCARA_SCROLL_X, useScrollRestante } from '@sytue/ui';
import type { AnyoDeGaleria } from '../client/photosApi.js';
import type { useT } from '../i18n/index.js';

export interface BarraDeAnyosProps {
  t: ReturnType<typeof useT>;
  anyos: readonly AnyoDeGaleria[];
  /** El año al que se saltó, o `null` si se mira desde lo más reciente. */
  activo: number | null;
  onElegir: (anyo: number | null) => void;
}

export function BarraDeAnyos({ t, anyos, activo, onElegir }: BarraDeAnyosProps) {
  // Los años llegan después del primer pintado: hasta entonces la fila no
  // existe, y sin `depende` el observador no se volvería a enlazar.
  const { ref, queda } = useScrollRestante<HTMLDivElement>(anyos.length >= 2);
  // Con un solo año no hay adónde saltar: la barra sería un adorno.
  if (anyos.length < 2) return null;
  return (
    <nav aria-label={t('gallery.jumpToYear')} class="mb-4">
      <div
        ref={ref}
        data-scroll-x
        class={cx(
          'flex flex-wrap items-center gap-1',
          FILA_UNICA_EN_MOVIL,
          queda && MASCARA_SCROLL_X,
        )}
      >
        <Button
          variant={activo === null ? 'secondary' : 'ghost'}
          size="sm"
          aria-pressed={activo === null}
          onClick={() => onElegir(null)}
        >
          {t('gallery.latest')}
        </Button>
        {anyos.map((a) => (
          <Button
            key={a.year}
            variant={activo === a.year ? 'secondary' : 'ghost'}
            size="sm"
            aria-pressed={activo === a.year}
            title={t('albums.count', { count: a.count })}
            class="tabular-nums"
            onClick={() => onElegir(a.year)}
          >
            {String(a.year)}
          </Button>
        ))}
      </div>
    </nav>
  );
}
