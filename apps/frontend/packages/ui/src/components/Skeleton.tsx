/** Skeleton — placeholder de carga (aria-hidden; el contenedor usa aria-busy). */
import type { ComponentChildren, JSX } from 'preact';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';

export interface SkeletonProps extends JSX.HTMLAttributes<HTMLDivElement> {
  width?: string;
  height?: string;
  circle?: boolean;
}

export function Skeleton({
  width,
  height = '1rem',
  circle = false,
  class: cls,
  style,
  ...rest
}: SkeletonProps): JSX.Element {
  return (
    <div
      aria-hidden="true"
      class={cx('sytue-skeleton', circle ? 'rounded-full' : 'rounded-md', cls)}
      style={{ width: width ?? '100%', height, ...(style as Record<string, unknown>) }}
      {...rest}
    />
  );
}

/** Lo que va delante del texto de cada fila: nada, un avatar (`Avatar size="sm"`) o la tesela de tipo de Drive. */
export type SkeletonRowsLead = 'none' | 'avatar' | 'tile';

/**
 * Una columna de la derecha: su ancho, su alto (por defecto el de una línea de meta) y
 * clases para la columna (`hidden sm:block`, `w-24`…), que es como Drive esconde tamaño y
 * fecha en ventanas estrechas.
 */
export interface SkeletonRowsMeta {
  width: string;
  height?: string;
  class?: string;
}

export interface SkeletonRowsProps {
  /** Cuántas filas. Las justas para llenar la vista que sustituyen. */
  rows?: number;
  lead?: SkeletonRowsLead;
  /** Una línea (título) o dos (título y subtítulo), como `ListItem`. */
  lines?: 1 | 2;
  /** `true` es una raya corta a la derecha; una lista, una raya por columna. */
  meta?: boolean | readonly SkeletonRowsMeta[];
  /** Lo que se anuncia una vez. Por defecto «Cargando» (`ui.loading`). */
  label?: string;
  /** Algo que va encima de las filas dentro de la misma región ocupada (la cabecera de columnas de Drive). */
  header?: ComponentChildren;
  class?: string;
  /** Clases de cada fila: el `border-b` de una lista con filetes. */
  rowClass?: string;
}

/** Lado de la pieza delantera: `Avatar size="sm"` (32) y la tesela de tipo de Drive (36). */
export const LADO_DELANTERO: Record<Exclude<SkeletonRowsLead, 'none'>, string> = {
  avatar: '32px',
  tile: '36px',
};

/** Las cajas de línea de `ListItem`: `text-base` para el título y `text-sm` debajo. */
const ALTO_TEXTO = { 1: '1.375rem', 2: '2.5rem' } as const;

/**
 * El alto de una fila de esqueleto: `py-2` arriba y abajo más lo más alto de sus hijos, que
 * es lo que mide un `ListItem` (`flex items-center`). Es la ÚNICA cuenta, y la misma que
 * `ui.AltoFilaEsqueleto` en Fyne: con ella el esqueleto ocupa lo que ocuparán las filas de
 * verdad y la vista no salta al llegar los datos.
 */
export function altoFilaEsqueleto(lead: SkeletonRowsLead = 'none', lines: 1 | 2 = 1): string {
  const texto = ALTO_TEXTO[lines];
  return lead === 'none'
    ? `calc(1rem + ${texto})`
    : `calc(1rem + max(${LADO_DELANTERO[lead]}, ${texto}))`;
}

/**
 * SkeletonRows — las filas de carga de una lista, ya montadas.
 *
 * Cada app se hacía las suyas (Drive tenía tres) y ninguna anunciaba la carga como pide la
 * hoja (§3.18): el contenedor con `aria-busy`, los bloques ocultos al lector y un «Cargando»
 * dicho UNA vez en una región `role="status"`, no uno por fila. Los anchos del título varían
 * por fila: iguales, el esqueleto parece una tabla vacía en vez de algo que llega.
 */
export function SkeletonRows({
  rows = 6,
  lead = 'none',
  lines = 1,
  meta,
  label,
  header,
  class: cls,
  rowClass,
}: SkeletonRowsProps): JSX.Element {
  const t = useT();
  const columnas: readonly SkeletonRowsMeta[] =
    meta === true ? [{ width: '3rem' }] : meta ? meta : [];
  const alto = altoFilaEsqueleto(lead, lines);
  return (
    <div class={cls} aria-busy="true">
      <span class="sr-only" role="status">
        {label ?? t('ui.loading')}
      </span>
      {header}
      <ul class="flex flex-col" aria-hidden="true">
        {Array.from({ length: rows }, (_, i) => (
          <li
            key={i}
            data-skeleton-row=""
            class={cx('flex items-center gap-3 px-3', rowClass)}
            style={{ height: alto }}
          >
            {lead === 'none' ? null : (
              <Skeleton
                width={LADO_DELANTERO[lead]}
                height={LADO_DELANTERO[lead]}
                circle={lead === 'avatar'}
                class="shrink-0"
              />
            )}
            <span class="flex min-w-0 flex-1 flex-col gap-1.5">
              <Skeleton width={`${38 + ((i * 11) % 42)}%`} height="0.85rem" />
              {lines === 2 ? <Skeleton width={`${50 + ((i * 17) % 35)}%`} height="0.7rem" /> : null}
            </span>
            {columnas.length > 0 ? (
              <span class="flex shrink-0 items-center gap-8">
                {columnas.map((c, j) => (
                  <span key={j} class={c.class}>
                    <Skeleton width={c.width} height={c.height ?? '0.7rem'} />
                  </span>
                ))}
              </span>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}
