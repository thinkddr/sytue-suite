/**
 * SegmentedControl — la «variante segmented» de las pestañas (docs/design.md §3.13): un
 * carril `bg-subtle` con radio `md` y la opción elegida sobre `surface` con `shadows.xs`.
 * Elige UNA de varias opciones que se ven todas a la vez: la apariencia del menú de
 * cuenta, la vista del calendario, lista o cuadrícula en Drive.
 *
 * Antes eran tres recetas a mano con tres modelos (`radiogroup` sin flechas, botones
 * con `aria-pressed`, botones sueltos con borde). Lo que fija:
 *
 * - Es un `radiogroup` con tabulación itinerante: UNA parada de Tab (la elegida) y
 *   flechas, Inicio y Fin para moverse, que eligen al llegar, como pide el patrón de
 *   radios de WAI-ARIA. Tres botones seguidos obligaban a tabular tres veces.
 * - Dos tallas. `sm` (24 px, `text-xs`) es la de los menús y las preferencias; `md`
 *   (28 px, `text-sm` seminegrita) la de las cabeceras, con la elegida en
 *   `primary-text`: el relleno `primary` no se reasigna en oscuro y sobre `surface`
 *   oscuro daba 2,96:1; el rol de texto sí invierte.
 * - Iconos opcionales delante del rótulo, o solos (`soloIcono`): entonces el rótulo
 *   pasa a ser el nombre accesible y el `title`, y el estado nunca va solo por color
 *   porque la elegida cambia de fondo y de sombra.
 * - `rejilla` reparte las opciones en N columnas iguales a todo el ancho y cada botón
 *   trunca antes que desbordar: el cajón del teléfono del calendario (2×2), donde en
 *   una fila el finés llegaba a 294 px en un cajón de 260.
 *
 * Con modificadores (Alt, Ctrl, Meta) las flechas son del navegador o del sistema y no
 * se roban. En Fyne es `ui.Segmentado` (`Talla`, `Iconos`, `Rejilla`).
 */
import type { JSX } from 'preact';
import { useRef } from 'preact/hooks';
import { cx } from '../lib/cx.js';
import { Icon } from './Icon.js';
import type { IconName } from './Icon.js';

export type SegmentedControlSize = 'sm' | 'md';

export interface SegmentedOption<T extends string> {
  /** Lo que se devuelve al elegir. */
  valor: T;
  /** Rótulo ya traducido; con `soloIcono` es el nombre accesible. */
  rotulo: string;
  /** Icono delante del rótulo (o en su lugar, con `soloIcono`). */
  icono?: IconName | undefined;
  /** Círculo de color delante del rótulo: un color CSS (o un `conic-gradient`). */
  muestra?: string | undefined;
  /** `title` del botón: un atajo de teclado, p. ej. Por defecto, nada (o el rótulo). */
  title?: string | undefined;
}

export interface SegmentedControlProps<T extends string> {
  opciones: ReadonlyArray<SegmentedOption<T>>;
  /** Valor elegido. Si no está entre las opciones, la primera recibe la parada de Tab. */
  valor: T;
  onChange: (valor: T) => void;
  /** Nombre del grupo cuando no hay una etiqueta visible a la que apuntar. */
  'aria-label'?: string | undefined;
  /** Id de la etiqueta visible del grupo. */
  etiquetadoPor?: string | undefined;
  /** `sm` (24 px) en menús y ajustes; `md` (28 px) en cabeceras. Por defecto `md`. */
  size?: SegmentedControlSize;
  /** Pinta solo los iconos; el rótulo queda como nombre accesible y `title`. */
  soloIcono?: boolean;
  /** Columnas de la rejilla a todo el ancho; sin ella, una fila a su ancho natural. */
  rejilla?: number | undefined;
  class?: string | undefined;
}

/** Medidas por talla: alto del botón, tipo y la tinta de la elegida. */
const TALLA: Record<
  SegmentedControlSize,
  {
    boton: string;
    texto: string;
    margen: string;
    activa: string;
    resto: string;
    icono: number;
    lado: string;
  }
> = {
  sm: {
    boton: 'h-6 rounded-control',
    texto: 'text-xs font-medium',
    margen: 'px-2',
    activa: 'bg-surface text-text shadow-[var(--shadow-xs)]',
    resto: 'text-text-muted hover:text-text',
    icono: 14,
    lado: 'w-6',
  },
  md: {
    boton: 'h-7 rounded-sm',
    texto: 'text-sm font-semibold',
    margen: 'px-2.5',
    activa: 'bg-surface text-primary-text shadow-[var(--shadow-xs)]',
    // El carril es una píldora CLARA también sobre el cromo grafito de las cabeceras:
    // al pasar por encima el texto se oscurece sobre `surface`; `text-text` sin ese
    // fondo dejaba la etiqueta invisible.
    resto: 'text-text-muted hover:bg-surface hover:text-text',
    icono: 16,
    lado: 'w-7',
  },
};

export function SegmentedControl<T extends string>({
  opciones,
  valor,
  onChange,
  'aria-label': ariaLabel,
  etiquetadoPor,
  size = 'md',
  soloIcono = false,
  rejilla,
  class: cls,
}: SegmentedControlProps<T>): JSX.Element {
  const grupo = useRef<HTMLDivElement>(null);
  const elegida = opciones.findIndex((o) => o.valor === valor);
  const parada = elegida < 0 ? 0 : elegida;
  const talla = TALLA[size];
  const columnas = rejilla && rejilla > 0 ? rejilla : 0;

  function onKeyDown(e: KeyboardEvent): void {
    const n = opciones.length;
    if (!n || e.altKey || e.ctrlKey || e.metaKey) return;
    // En la rejilla, ↑/↓ saltan de fila; en una fila son lo mismo que ←/→.
    const vertical = columnas || 1;
    let destino = -1;
    if (e.key === 'ArrowRight') destino = (parada + 1) % n;
    else if (e.key === 'ArrowLeft') destino = (parada - 1 + n) % n;
    else if (e.key === 'ArrowDown') destino = (parada + vertical) % n;
    else if (e.key === 'ArrowUp') destino = (parada - vertical + n * vertical) % n;
    else if (e.key === 'Home') destino = 0;
    else if (e.key === 'End') destino = n - 1;
    if (destino < 0) return;
    e.preventDefault();
    // La tecla es del grupo: dentro de un menú (la apariencia en el de cuenta) el
    // `Dropdown` escucha en `document` y, sin esto, ↑/↓, Inicio y Fin cambiaban la opción
    // Y ADEMÁS llevaban el foco a otro ítem del menú.
    e.stopPropagation();
    if (destino !== elegida) onChange(opciones[destino]!.valor);
    grupo.current?.querySelectorAll<HTMLButtonElement>('[role="radio"]')[destino]?.focus();
  }

  return (
    <div
      ref={grupo}
      role="radiogroup"
      // Focusable solo por programa: la parada de Tab es el radio elegido.
      tabIndex={-1}
      {...(ariaLabel ? { 'aria-label': ariaLabel } : {})}
      {...(etiquetadoPor ? { 'aria-labelledby': etiquetadoPor } : {})}
      class={cx(
        'bg-bg-subtle rounded-md p-0.5',
        columnas
          ? 'grid w-full gap-0.5'
          : size === 'sm'
            ? 'flex flex-wrap'
            : 'inline-flex items-center gap-0.5',
        cls,
      )}
      style={columnas ? { gridTemplateColumns: `repeat(${columnas}, minmax(0, 1fr))` } : undefined}
      onKeyDown={onKeyDown}
    >
      {opciones.map((o, i) => {
        const marcada = i === elegida;
        const title = o.title ?? (soloIcono ? o.rotulo : undefined);
        return (
          <button
            key={o.valor}
            type="button"
            role="radio"
            aria-checked={marcada}
            {...(soloIcono ? { 'aria-label': o.rotulo } : {})}
            {...(title ? { title } : {})}
            tabIndex={i === parada ? 0 : -1}
            onClick={() => {
              if (!marcada) onChange(o.valor);
            }}
            class={cx(
              'sytue-focus-ring whitespace-nowrap transition-colors duration-150 motion-reduce:transition-none',
              talla.boton,
              talla.texto,
              // En la rejilla el ancho lo pone la columna y el botón trunca antes que
              // desbordar. Es `block` y no `flex` porque un `text-overflow` no recorta el
              // texto suelto de un flex; el botón ya centra su contenido en vertical.
              columnas
                ? 'block min-w-0 truncate px-1.5 text-center'
                : cx(
                    'flex items-center justify-center gap-1.5',
                    soloIcono ? talla.lado : talla.margen,
                  ),
              marcada ? talla.activa : talla.resto,
            )}
          >
            {o.muestra && (
              <span
                aria-hidden="true"
                class={cx('size-2.5 shrink-0 rounded-full', columnas && 'mr-1.5 inline-block')}
                style={{ background: o.muestra }}
              />
            )}
            {o.icono && (
              <Icon
                name={o.icono}
                size={talla.icono}
                class={cx(columnas && !soloIcono && 'mr-1.5 inline-block align-[-0.125em]')}
              />
            )}
            {!soloIcono && o.rotulo}
          </button>
        );
      })}
    </div>
  );
}
