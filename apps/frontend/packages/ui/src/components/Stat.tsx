/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Stat — una cifra con su rótulo: «Errores (1 h) · 3», «Canales · 12».
 *
 * Había cinco tarjetas de cifra escritas a mano (`StatCard`, `Cifra` y tres `Kpi`)
 * y media docena de recetas en línea, con tres tipografías distintas: 1,75 rem a
 * pelo, `text-2xl` y `sytue-h3`. Una consola con dos paneles de cifras que no
 * miden lo mismo parece hecha por dos equipos. Esta es la de `sytue-h3`, que es la
 * que sigue a la escala de títulos del tema, con lo que cada copia aprendió por
 * su cuenta:
 *
 * - Cifras tabulares: al refrescar, «111» y «888» ocupan lo mismo y la fila no
 *   baila.
 * - El VALOR se recorta y lleva `title`: no siempre es un número (la release
 *   esperada, «2026.08.09-a1b2c3d», o un dominio) y a `sytue-h3` no cabe en una
 *   columna de cuatro. El rótulo también, por la misma razón en alemán.
 * - El tono pinta solo la cifra y nunca el rótulo: el aviso ya lo dice el texto,
 *   el color es el refuerzo (WCAG 1.4.1). Decidir si se enciende —«solo si no es
 *   cero»— es de quien llama, que es quien sabe qué cifra es mala.
 */
import type { ComponentChildren, JSX } from 'preact';
import { cx } from '../lib/cx.js';
import { Card, CardBody } from './Card.js';

export type StatTone = 'neutral' | 'success' | 'warning' | 'danger';

const toneClass: Record<StatTone, string> = {
  neutral: 'text-text',
  success: 'text-success-text',
  warning: 'text-warning-text',
  danger: 'text-danger-text',
};

export interface StatProps {
  /** Qué se cuenta («Usuarios activos»). */
  label: string;
  /** La cifra, ya formateada por quien llama (`Intl.NumberFormat`, «3/5», «—»). */
  value: number | string;
  /** Matiz bajo la cifra («suma de días», «+12 % vs. mes anterior»). */
  hint?: ComponentChildren;
  /** Color de la cifra. Por defecto, la tinta normal. */
  tone?: StatTone;
  /**
   * `card` (por defecto): tarjeta plana, para rejillas de cifras sueltas.
   * `plain`: sin superficie, para cifras que ya viven dentro de una tarjeta.
   */
  surface?: 'card' | 'plain';
  class?: string;
}

export function Stat({
  label,
  value,
  hint,
  tone = 'neutral',
  surface = 'card',
  class: cls,
}: StatProps): JSX.Element {
  const texto = String(value);
  const cuerpo = (
    <>
      <p class="text-text-muted truncate text-sm" title={label}>
        {label}
      </p>
      <p class={cx('sytue-h3 mt-1 truncate tabular-nums', toneClass[tone])} title={texto}>
        {texto}
      </p>
      {hint !== undefined && hint !== null && hint !== '' ? (
        <p class="sytue-meta mt-0.5">{hint}</p>
      ) : null}
    </>
  );
  // `min-w-0` deja recortar dentro de un flex o una rejilla; si quien llama trae su
  // propio ancho mínimo (`min-w-28` en Analytics), se respeta el suyo: dos `min-w-*`
  // en el mismo nodo los resuelve el orden de la hoja, no el del atributo.
  if (surface === 'plain') {
    const propio = cls !== undefined && /(^|\s)min-w-/.test(cls);
    return <div class={cx(propio ? null : 'min-w-0', cls)}>{cuerpo}</div>;
  }
  return (
    <Card raised={false} class={cls}>
      <CardBody>{cuerpo}</CardBody>
    </Card>
  );
}
