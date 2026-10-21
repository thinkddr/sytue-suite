/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Meter — «cuánto llevas gastado de lo que tienes»: cuota de disco, buzones de
 * una licencia, aforo de una sala.
 *
 * Estaba escrito dos veces, y la diferencia entre las dos copias era justo lo que
 * importa. La de Mail avisa: al 80 % el relleno pasa a ámbar y al 95 % a rojo, y
 * —esto es lo que no se puede perder— lo dice TAMBIÉN con palabras («casi lleno»,
 * «lleno»), porque un aviso que solo existe como color no llega a quien no lo
 * percibe (WCAG 1.4.1). La de Drive pintaba siempre el mismo gris: el buzón podía
 * estar al 99 % y la barra no lo contaba, ni en color ni en texto. Además escribía
 * el porcentaje a mano («95%»), y en francés va con espacio antes del signo.
 *
 * Este componente es la de Mail, generalizada: el aviso, el color, el porcentaje
 * por `Intl` y el `role="progressbar"` con su valor viven aquí, una vez, para que
 * la próxima barra de uso de la suite no vuelva a nacer muda.
 *
 * NO formatea las cantidades: quien llama pasa `valueText` ya escrito («1,5 GB de
 * 15 GB», «7 de 10 asientos»), porque el conector y la unidad son suyos. Para
 * bytes, `formatBytes` de este mismo paquete.
 */
import type { JSX } from 'preact';
import { formatPercent } from '@sytue/i18n';
import { useI18n } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';

/**
 * Umbrales de aviso, en fracción usada. Fijos y no configurables: dos productos
 * que avisan a distinto porcentaje enseñan al usuario a desconfiar de los dos.
 */
const AVISO = 0.8;
const LLENO = 0.95;

export interface MeterProps {
  /** Consumido. */
  value: number;
  /** Tope. Cero o negativo ⇒ no hay cuota que enseñar y el medidor no se pinta. */
  max: number;
  /** Rótulo visible («Almacenamiento»). Encabeza también el nombre accesible. */
  label: string;
  /** El uso en palabras, ya formateado: «1,5 GB de 15 GB». */
  valueText?: string;
  /**
   * Sobre el cromo (el pie del menú lateral), de tinta en los dos temas: los textos
   * van con la rampa `sidebar-*` y el ámbar es el del cromo.
   */
  onChrome?: boolean;
  class?: string;
}

export function Meter({
  value,
  max,
  label,
  valueText,
  onChrome = false,
  class: cls,
}: MeterProps): JSX.Element | null {
  const { locale, t } = useI18n();
  if (!(max > 0) || !Number.isFinite(value)) return null;

  const frac = Math.min(Math.max(value / max, 0), 1);
  // El entero es para `aria-valuenow`, que no acepta el texto del locale.
  const pct = Math.round(frac * 100);
  const nivel: 'ok' | 'aviso' | 'lleno' = frac >= LLENO ? 'lleno' : frac >= AVISO ? 'aviso' : 'ok';
  // El signo «%» y su separación los pone el idioma («95%» en español, «95 %» en
  // francés): por eso el porcentaje visible NO se concatena a mano.
  const pctText = formatPercent(locale, frac, { maximumFractionDigits: 0 });
  const nota =
    nivel === 'lleno' ? t('ui.meter.full') : nivel === 'aviso' ? t('ui.meter.almostFull') : '';

  // Neutro mientras sobra sitio → ámbar al acercarse → rojo al llegar. Sobre el
  // menú de tinta los neutros de página no contrastarían, de ahí las dos rampas.
  const relleno =
    nivel === 'lleno'
      ? 'bg-danger'
      : nivel === 'aviso'
        ? onChrome
          ? 'bg-on-chrome-warning'
          : 'bg-warning'
        : onChrome
          ? 'bg-on-chrome-muted'
          : 'bg-text-subtle';

  return (
    <div class={cx('flex flex-col gap-1.5', cls)}>
      <div class="flex items-baseline justify-between gap-2">
        {/* Se trunca: «Speicherplatz» es casi el doble de largo que «Espacio». */}
        <span
          class={cx(
            'min-w-0 truncate text-xs',
            onChrome ? 'text-sidebar-fg-muted' : 'text-text-muted',
          )}
        >
          {label}
        </span>
        <span
          class={cx(
            'shrink-0 text-xs tabular-nums',
            onChrome ? 'text-sidebar-fg-muted' : 'text-text-muted',
          )}
        >
          {pctText}
        </span>
      </div>
      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
        // Todo lo que se ve, junto, en el nombre accesible: quien no ve la barra
        // se entera del uso y del aviso sin tener que recomponerlo de tres nodos.
        aria-label={[label, valueText, pctText, nota].filter(Boolean).join(' · ')}
        class={cx(
          'h-1.5 w-full overflow-hidden rounded-full',
          onChrome ? 'bg-sidebar-fg/15' : 'bg-border',
        )}
      >
        <div
          class={cx('h-full rounded-full transition-[width] duration-300', relleno)}
          style={{ width: `${pct}%` }}
        />
      </div>
      {valueText || nota ? (
        <p class={cx('text-xs', onChrome ? 'text-sidebar-fg' : 'text-text')}>
          {valueText}
          {/* El aviso, en palabras y no solo en el color del relleno. */}
          {nota ? (
            <span class={onChrome ? 'text-sidebar-fg-muted' : 'text-text-muted'}> · {nota}</span>
          ) : null}
        </p>
      ) : null}
    </div>
  );
}
