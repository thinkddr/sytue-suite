/**
 * EmptyState — estado vacio con marca + titulo + descripcion + CTA.
 * docs/design.md §9.
 */
import type { ComponentChildren, JSX } from 'preact';
import { cx } from '../lib/cx.js';

export interface EmptyStateProps {
  title: string;
  description?: string;
  /** Ilustracion o icono de marca. */
  illustration?: ComponentChildren;
  /** Accion principal (p.ej. Button). */
  action?: ComponentChildren;
  class?: string;
}

export function EmptyState({
  title,
  description,
  illustration,
  action,
  class: cls,
}: EmptyStateProps): JSX.Element {
  return (
    <div class={cx('flex flex-col items-center justify-center gap-3 px-6 py-8 text-center', cls)}>
      {illustration ? (
        // El icono va SUELTO, sin tesela de fondo. Antes iba dentro de un rectángulo
        // beige redondeado que no aportaba nada: no agrupaba, no separaba y no indicaba
        // estado — solo enmarcaba. Tres rellenos apilados (tesela + icono + página) es
        // la firma visual del estado vacío genérico, y el contenedor teñido llamaba más
        // la atención que el texto que sí dice qué hacer.
        //
        // Se conserva la normalización de tamaño: algunos llamantes pasan `h-full
        // w-full` y sin el tope la ilustración colapsaba o se desbordaba.
        <div class="text-text-subtle mb-2 inline-flex max-w-[200px] items-center justify-center [&>img]:h-auto [&>img]:w-full [&>img]:max-w-[160px] [&>svg]:h-auto [&>svg]:max-w-full">
          {illustration}
        </div>
      ) : null}
      <h3 class="sytue-h4 text-text">{title}</h3>
      {description ? <p class="text-text-muted max-w-sm text-base">{description}</p> : null}
      {action ? <div class="mt-2">{action}</div> : null}
    </div>
  );
}
