/**
 * IconButton — botón CUADRADO de solo icono para cromo denso: carriles, cabeceras de
 * panel, barras de estado y filas de lista.
 *
 * `Button iconOnly` no baja de `h-8` (32 px) porque su suelo es el objetivo táctil de una
 * acción principal. El cromo del editor necesita 24 y 28, así que cada sitio se escribió
 * la suya: 14 constantes (`RAIL_BTN`, `NAV_BTN`, `ROW_BTN`, `TRASH_BTN`, `ZOOM_BTN`,
 * `ICON_BTN`…) que son la MISMA receta con la talla cambiada, más las copias inline. Y
 * copiar clases no copia el anillo de foco doble ni el estado `aria-pressed`: cada copia
 * perdía algo distinto por el camino, que es como el editor acabó con tres anillos de foco.
 *
 * 24 px es el suelo, no una elección estética: WCAG 2.5.8 (AA) exige un objetivo de al
 * menos 24×24 CSS px. Por eso `sm` es exactamente eso y no hay talla menor.
 *
 * `label` es obligatorio a propósito: un botón sin texto que tampoco tiene nombre
 * accesible es invisible para un lector de pantalla, y en TypeScript un campo opcional
 * es un campo que la mitad de los sitios olvida.
 *
 * docs/design.md §2.3, §4.3.
 */
import type { JSX } from 'preact';
import { cx } from '../lib/cx.js';

/**
 * Las tres tallas del cromo denso (`sm`, `md`, `lg`) y `xl`, la de los mandos SOBRE UNA
 * FOTO (las flechas de un visor): 48 px, que se encuentran de un vistazo sobre cualquier
 * imagen y superan el 44 del dedo sin pedirle nada al puntero.
 */
export type IconButtonSize = 'sm' | 'md' | 'lg' | 'xl';

/**
 * - `default`: sobre superficie clara (paneles, listas).
 * - `danger`: solo tiñe el HOVER; en reposo un botón destructivo no debe gritar.
 * - `header`: sobre la BANDA de cabecera (`bg-header`). Sus velos son los tokens de cromo,
 *   no los de superficie: un `hover:bg-bg-subtle` sobre la banda de tinta pinta un velo
 *   claro y deja el icono a 1,1:1 — el defecto que ya costó una ronda entera de auditoría.
 * - `media`: SOBRE UNA FOTO O UN VÍDEO (las flechas de un visor). Lo de debajo no es del
 *   tema —una foto es blanca o negra en los tres—, así que el botón trae su propio fondo:
 *   un fondo negro al 60 % con el icono en blanco. Sobre el peor caso, una foto blanca, el
 *   fondo queda en gris 102 y el icono a 5,7:1; sobre cualquier otra, más. Los tokens de
 *   superficie no sirven aquí: `surface` es translúcida en el tema traslúcido.
 *   Y un FILETE blanco al 40 %: sobre una foto negra la placa negra se funde con ella
 *   (medido: 1:1 contra la foto) y del botón solo quedaba el glifo; con el filete el borde
 *   queda a 3,7:1 sobre negro, el 3:1 de WCAG 1.4.11.
 */
export type IconButtonTone = 'default' | 'danger' | 'header' | 'media';

export interface IconButtonProps extends Omit<
  JSX.ButtonHTMLAttributes<HTMLButtonElement>,
  'size' | 'label'
> {
  /** Nombre accesible. Obligatorio: el icono no dice nada a quien no lo ve. */
  label: string;
  size?: IconButtonSize;
  tone?: IconButtonTone;
  /**
   * Estado de conmutador. Emite `aria-pressed` y el relleno de seleccionado — los dos,
   * porque el color por sí solo no comunica estado (WCAG 1.4.1).
   */
  pressed?: boolean;
  /** Muestra el `label` también como `title` (tooltip nativo del sistema). */
  showTitle?: boolean;
  /**
   * Destino: renderiza un `<a>` con el mismo aspecto, igual que `Button` y por lo mismo:
   * lo que navega es un enlace —se abre en otra pestaña y se anuncia como tal—, y sin
   * esto un icono que navega acababa siendo un `Button iconOnly`, con otro radio que los
   * de al lado.
   */
  href?: string;
  children: JSX.Element | JSX.Element[];
}

/**
 * Las tres tallas del cromo denso, y el mínimo táctil encima.
 *
 * 24 px cumple WCAG 2.5.8 y con un ratón está bien; con la yema de un dedo, no.
 * `pointer-coarse:` sube el objetivo a 44 px —la guía de Apple— SOLO donde el
 * puntero es grueso, para no engordar el cromo del editor en escritorio. Se
 * decide por el puntero y no por el ancho: un portátil táctil grande lo necesita
 * y un móvil con ratón conectado no.
 */
const SIZE: Record<IconButtonSize, string> = {
  // `h-6` son 1.5rem, y la raíz de la suite está en 14 px: 21 px, por debajo de los
  // 24 que exige WCAG 2.5.8 (medido el 2026-09-14 en Notepads: deshacer y rehacer
  // de 21×21). El suelo va en px para que la talla lo cumpla con cualquier raíz.
  sm: 'h-6 w-6 min-h-[24px] min-w-[24px] pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]',
  // 24,5 y 28 px, NO 28 y 32: `h-7` son 1.75rem y la raíz de esta suite está en 14.
  // El comentario que decía 28/32 daba por hecha una raíz de 16 y engañó al menos a
  // una lectura del catálogo — es la misma fosilización que tenía el esqueleto de la
  // bandeja de Mail con su `h-[83px]`.
  md: 'h-7 w-7 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]', // 24,5 px
  lg: 'h-8 w-8 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]', // 28 px
  // 48 px con cualquier raíz y con cualquier puntero: pasa ya del suelo táctil.
  xl: 'h-12 w-12 min-h-[48px] min-w-[48px]',
};

const base =
  'inline-flex shrink-0 items-center justify-center rounded-control ' +
  'transition-colors duration-150 ease-[var(--ease-standard)] select-none ' +
  'disabled:pointer-events-none disabled:opacity-40 ' +
  'focus-visible:outline-none ' +
  // El anillo canónico. Aquí iba uno doble porque el latón de entonces solo medía 2,81:1
  // sobre marfil, por debajo del 3:1 que exige WCAG 1.4.11 — y en tema oscuro sí pasa,
  // que es exactamente lo que hace que un fallo así sobreviva a la revisión.
  'sytue-focus-ring';

const TONE: Record<IconButtonTone, string> = {
  default: 'text-primary-text hover:bg-selected hover:text-primary-text',
  danger: 'text-primary-text hover:bg-bg-subtle hover:text-danger-text',
  header: 'text-primary-text hover:bg-chrome-hover active:bg-chrome-active',
  media:
    'border border-white/40 bg-black/60 text-white hover:border-white/60 hover:bg-black/75 active:bg-black/85',
};

export function IconButton({
  label,
  size = 'md',
  tone = 'default',
  pressed,
  showTitle = true,
  href,
  disabled,
  class: className,
  children,
  ...rest
}: IconButtonProps): JSX.Element {
  const klass = cx(
    base,
    SIZE[size],
    TONE[tone],
    pressed && (tone === 'header' ? 'bg-chrome-active' : 'bg-selected text-text'),
    className,
  );
  const nombre = { 'aria-label': label, ...(showTitle ? { title: label } : {}) };
  // Variante ENLACE, como la de `Button`: un `<a>` no entiende `disabled`, así que
  // inerte es sin `href` (fuera de la tabulación) y con `aria-disabled`.
  if (href !== undefined) {
    return (
      <a
        href={disabled ? undefined : href}
        aria-disabled={disabled ? 'true' : undefined}
        {...nombre}
        class={cx(klass, disabled && 'pointer-events-none opacity-40')}
        {...(rest as unknown as JSX.HTMLAttributes<HTMLAnchorElement>)}
      >
        {children}
      </a>
    );
  }
  return (
    <button
      type="button"
      disabled={disabled}
      {...nombre}
      {...(pressed === undefined ? {} : { 'aria-pressed': pressed })}
      class={klass}
      {...rest}
    >
      {children}
    </button>
  );
}
