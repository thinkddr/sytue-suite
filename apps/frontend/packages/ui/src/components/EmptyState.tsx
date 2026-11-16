/**
 * EmptyState — estado vacío con marca + título + descripción + acciones.
 * docs/design.md §9 y spec §3.17.
 *
 * Tres variantes de contenido —vacío inicial, sin resultados y error con reintento— y
 * dos tamaños: el de página (por defecto) y el `compact` de paneles, diálogos y
 * popovers. La marca se elige por NOMBRE (`icon` o `spot`) y no por JSX suelto: con JSX
 * libre la suite acabó con siete tamaños de icono y cuatro de ilustración distintos.
 * `illustration` se conserva para los usos que ya existen.
 */
import type { ComponentChildren, JSX } from 'preact';
import { cx } from '../lib/cx.js';
import { Icon, type IconName } from './Icon.js';
import { Illustration, type SpotName } from './Illustration.js';

/** Lado del glifo de `icon`: 32 en página, 20 en compacto. Espejo de `ui.LadoGlifoVacio`. */
export const EMPTY_ICON_SIZE = 32;
export const EMPTY_ICON_SIZE_COMPACT = 20;
/** Ancho del spot de `branding/illustrations`: el tamaño de marca fijo de la spec (96–120). */
export const EMPTY_SPOT_SIZE = 120;

export type EmptyStateTone = 'neutral' | 'error';
export type EmptyStateHeadingLevel = 2 | 3 | 4 | 5 | 6;

export interface EmptyStateProps {
  title: string;
  description?: string;
  /** Glifo del catálogo, a 32 (20 en compacto) y en `text-subtle` (`danger-text` en error). */
  icon?: IconName;
  /** Ilustración de marca a 120 de ancho. No se pinta en compacto. */
  spot?: SpotName;
  /**
   * Ilustración o icono libre. Se conserva por compatibilidad: lo nuevo usa `icon` o
   * `spot`, que fijan el tamaño.
   */
  illustration?: ComponentChildren;
  /**
   * `error` anuncia el estado (`role="alert"`) y, si no se pide otra marca, pone el
   * glifo de peligro en `danger-text`.
   */
  tone?: EmptyStateTone;
  /** Versión de panel, diálogo o popover: `text-sm`, `py-6` y sin ilustración. */
  compact?: boolean;
  /** Acción principal (p.ej. Button). */
  action?: ComponentChildren;
  /** Acción secundaria, a la derecha de la principal (p.ej. «Quitar filtro»). */
  secondaryAction?: ComponentChildren;
  /** Nivel del título en el esquema del documento. Por defecto `<h3>`. */
  headingLevel?: EmptyStateHeadingLevel;
  class?: string;
}

export function EmptyState({
  title,
  description,
  icon,
  spot,
  illustration,
  tone = 'neutral',
  compact = false,
  action,
  secondaryAction,
  headingLevel = 3,
  class: cls,
}: EmptyStateProps): JSX.Element {
  const error = tone === 'error';
  // El glifo de peligro es la marca POR DEFECTO del error, no una imposición: quien
  // pide el spot `errorRetry` lo tiene. En compacto no hay ilustraciones, así que ahí
  // el error siempre lleva su glifo.
  const glifo = icon ?? (error && (compact || (!spot && !illustration)) ? 'danger' : undefined);
  const Titulo = `h${headingLevel}` as 'h3';
  const tinta = error ? 'text-danger-text' : 'text-text-subtle';

  let marca: ComponentChildren = null;
  if (glifo) {
    marca = (
      <div class={cx(tinta, 'inline-flex items-center justify-center', !compact && 'mb-2')}>
        <Icon name={glifo} size={compact ? EMPTY_ICON_SIZE_COMPACT : EMPTY_ICON_SIZE} />
      </div>
    );
  } else if (!compact && spot) {
    marca = (
      <div class="mb-2 inline-flex items-center justify-center">
        <Illustration name={spot} size={EMPTY_SPOT_SIZE} />
      </div>
    );
  } else if (!compact && illustration) {
    marca = (
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
    );
  }

  const acciones =
    action && secondaryAction ? (
      <div class="mt-2 flex flex-wrap items-center justify-center gap-2">
        {action}
        {secondaryAction}
      </div>
    ) : action || secondaryAction ? (
      <div class="mt-2">{action ?? secondaryAction}</div>
    ) : null;

  return (
    <div
      // `alert` y no `status`: el error sustituye a lo que el usuario esperaba ver, y
      // tiene que oírlo aunque el foco esté en otra parte.
      role={error ? 'alert' : undefined}
      class={cx(
        compact
          ? 'flex flex-col items-center justify-center gap-2 px-4 py-6 text-center'
          : 'flex flex-col items-center justify-center gap-3 px-6 py-8 text-center',
        cls,
      )}
    >
      {marca}
      <Titulo class={compact ? 'text-text text-sm font-semibold' : 'sytue-h4 text-text'}>
        {title}
      </Titulo>
      {description ? (
        <p
          class={
            compact ? 'text-text-muted max-w-xs text-sm' : 'text-text-muted max-w-sm text-base'
          }
        >
          {description}
        </p>
      ) : null}
      {acciones}
    </div>
  );
}
