/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ProgressBar — «cuánto falta de algo que está pasando»: una subida, una
 * exportación, un proceso con fin conocido.
 *
 * NO es `Meter`. El medidor cuenta lo GASTADO de un tope y avisa al acercarse
 * (ámbar al 80 %, rojo al 95 %); una subida al 95 % no es ninguna alarma, es casi
 * hecha. Reutilizarlo para eso pintaba de rojo cada fichero a punto de terminar.
 *
 * Sin `value` es indeterminada (se sabe que avanza, no cuánto): el relleno
 * recorre la barra y `aria-valuenow` se omite, que es como lo anuncia un lector
 * de pantalla. Con `prefers-reduced-motion` el recorrido se queda quieto.
 */
import type { JSX } from 'preact';
import { formatPercent } from '@sytue/i18n';
import { useI18n } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';

export interface ProgressBarProps {
  /** Fracción hecha, de 0 a 1. Ausente ⇒ indeterminada. */
  value?: number | undefined;
  /** Nombre accesible («Subiendo entrevista.mp4»). */
  label: string;
  /** Tono del relleno: el de siempre, o el de error si el proceso falló. */
  tone?: 'default' | 'danger';
  class?: string;
}

export function ProgressBar({
  value,
  label,
  tone = 'default',
  class: cls,
}: ProgressBarProps): JSX.Element {
  const { locale } = useI18n();
  const determinada = value !== undefined && Number.isFinite(value);
  const frac = determinada ? Math.min(Math.max(value, 0), 1) : 0;
  const pct = Math.round(frac * 100);
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={determinada ? pct : undefined}
      aria-valuetext={
        determinada ? formatPercent(locale, frac, { maximumFractionDigits: 0 }) : undefined
      }
      class={cx('bg-border relative h-1.5 w-full overflow-hidden rounded-full', cls)}
    >
      <div
        class={cx(
          'h-full rounded-full',
          tone === 'danger' ? 'bg-danger' : 'bg-primary',
          determinada
            ? 'transition-[width] duration-150'
            : 'sytue-progreso-indeterminado absolute inset-y-0 w-1/3',
        )}
        style={determinada ? { width: `${pct}%` } : undefined}
      />
    </div>
  );
}
