/**
 * BotonFlotante — la acción de CREAR de cada app: un «+» redondo que flota abajo a la
 * derecha, en todos los anchos. «Crear evento», «Redactar», «Nuevo contacto», «Nuevo» de
 * Drive, «Subir» de Cast, «Añadir fotos».
 *
 * Fue `MobileFab`, solo por debajo de 768 y cuadrado con la esquina de los botones: en
 * escritorio la acción la llevaba el botón primario del menú lateral, y la suite tenía
 * dos controles para lo mismo según el ancho. Por decisión de producto (2026-11-25) el
 * «+» flotante es EL control de crear en toda la suite, así que el del menú se quita. Es
 * la excepción a los botones rectangulares de CONVENTIONS §5: un círculo con un «+» se
 * reconoce sin leer, que es lo que pide un control que no lleva rótulo.
 *
 * El glifo es siempre el «+», sin icono a elegir: la de Drive y la de Cast llevaban la
 * flecha de subir y cada app acababa diciendo «crear» a su manera.
 *
 * Con `menu`, el «+» no crea: abre el menú de lo que se puede crear (el «Nuevo» de Drive),
 * colgado de él. Es el `Dropdown` de siempre, que abre hacia arriba porque abajo no cabe,
 * y en el teléfono es la hoja inferior.
 *
 * Va FUERA de `AppShell`: en <768px la shell esconde el contenido mientras enseña la
 * lista, y un botón dentro desaparecería con él. Es `fixed`, así que da igual dónde se
 * monte. En el teléfono no se pinta con el DETALLE a la vista (la ficha de un contacto,
 * el lector de Mail; `data-sytue-detalle` lo pone `AppShell`): tapaba el final de lo que
 * se lee y no es la acción de esa pantalla. Lo que desplaza debajo le deja sitio con
 * `sytue-reserva-flotante` (styles.css), y los avisos y el candado de la bóveda se
 * apartan al verlo (`data-sytue-fab`). docs/design.md §3.5 (primario) y §2.2 (responsive).
 *
 * La pareja en Fyne es `ui.BotonFlotante` (apps/fyne/ui/botonflotante.go).
 */
import type { JSX } from 'preact';
import { cx } from '../lib/cx.js';
import { Dropdown, type DropdownProps } from './Dropdown.js';
import { Icon } from './Icon.js';

export interface BotonFlotanteProps {
  /** Nombre accesible: lo que crea («Crear evento», «Redactar»). El botón es solo icono. */
  label: string;
  /** Tooltip, si dice más que el nombre (el atajo: «Redactar (C)»). Por defecto, `label`. */
  title?: string;
  /** Lo que hace el «+». Sin efecto con `menu`, que lo abre. */
  onClick?: () => void;
  /**
   * El menú de lo que se puede crear, si hay más de una cosa (el «Nuevo» de Drive). Los
   * mismos hijos que `Dropdown`: `MenuItem`, o una función que recibe `close`.
   */
  menu?: DropdownProps['children'];
  disabled?: boolean;
  /** Clases de lo que se fija a la esquina (p. ej. esconderlo en un ancho). */
  class?: string;
}

/**
 * La esquina: `right-4` y `bottom-4`, o el borde seguro si es mayor (la barra de gestos
 * de iOS lo tapaba a medias), DENTRO de la hoja: en «Marco» la hoja flota a
 * `--sytue-sheet-gap` del borde (10 px desde 768; 0 en «Carril» y en el teléfono), y sin
 * sumarlo el botón quedaba a caballo entre la hoja y el marco, medio fundido con un marco
 * que es del mismo color que él. `z-20` por encima del contenido y por debajo del cajón
 * del teléfono (`z-sidebar`), los menús, los diálogos y los avisos.
 */
const ESQUINA = cx(
  'right-[calc(max(1rem,var(--sytue-safe-right,0px))+var(--sytue-sheet-gap,0px))]',
  'bottom-[calc(max(1rem,var(--sytue-safe-bottom,0px))+var(--sytue-sheet-gap,0px))] z-20',
  'max-md:[body:has([data-sytue-detalle])_&]:hidden',
);

/**
 * El botón. 56 px (`h-14`): el tamaño canónico del botón flotante, por encima de los 44
 * del dedo. Los colores de `Button primary`: es LA acción primaria, en otro sitio.
 */
const BOTON = cx(
  'bg-primary text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] active:bg-[var(--sytue-primary-active)]',
  'grid h-14 w-14 place-items-center rounded-full',
  'shadow-[var(--shadow-lg)] transition-transform duration-150 active:scale-95',
  'sytue-focus-ring disabled:pointer-events-none disabled:opacity-50',
);

export function BotonFlotante({
  label,
  title,
  onClick,
  menu,
  disabled,
  class: cls,
}: BotonFlotanteProps): JSX.Element {
  const boton = (
    extra: { class?: string } & Partial<
      Pick<JSX.ButtonHTMLAttributes<HTMLButtonElement>, 'onClick' | 'aria-expanded'>
    > & { 'aria-haspopup'?: 'menu' },
  ): JSX.Element => (
    <button
      type="button"
      aria-label={label}
      title={title ?? label}
      disabled={disabled}
      // Lo que flota en la misma esquina (los avisos, el candado de reserva de la
      // bóveda) se aparta al verlo.
      data-sytue-fab=""
      {...extra}
      class={cx(BOTON, extra.class)}
    >
      <Icon name="plus" size={24} />
    </button>
  );
  if (menu === undefined) return boton({ class: cx('fixed', ESQUINA, cls), onClick });
  // Con menú lo que se fija a la esquina es el envoltorio del `Dropdown`, no el botón:
  // el panel se coloca con la caja de lo que envuelve al disparador, y con el botón
  // `fixed` dentro esa caja se quedaba en cero, donde se montara. Con `!` porque el
  // envoltorio ya trae `relative`, y dos `position` en el mismo elemento los resuelve el
  // orden de la hoja generada, no el de la cadena.
  return (
    <Dropdown
      label={label}
      align="end"
      side="top"
      class={cx('fixed!', ESQUINA, cls)}
      trigger={boton}
    >
      {menu}
    </Dropdown>
  );
}
