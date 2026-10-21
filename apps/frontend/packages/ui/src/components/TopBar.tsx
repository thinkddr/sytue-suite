/**
 * TopBar — barra superior global (landmark `banner`, 56px, fija). Slots para
 * marca/logo, zona central (busqueda) y acciones de la derecha (app switcher,
 * notificaciones, presencia, cuenta). El cableado de comportamiento vive en
 * @sytue/shell; este es el contenedor visual canonico. docs/design.md §2.
 */
import type { ComponentChildren, JSX } from 'preact';
import { cx } from '../lib/cx.js';

export interface TopBarProps {
  /** Marca/logo a la izquierda. */
  brand?: ComponentChildren;
  /** Zona central (p.ej. busqueda global). */
  center?: ComponentChildren;
  /** Acciones a la derecha (app switcher, notificaciones, cuenta). */
  actions?: ComponentChildren;
  /**
   * Pinta la zona de marca como CABECERA DEL CARRIL: del color del menú lateral y, desde
   * 768 px, con su mismo ancho, así que el carril se lee de arriba abajo sin mover la
   * barra de sitio. En una app sin menú lateral queda como esquina de marca. La pide la
   * shell de la suite; el sitio público y las barras sin sesión no la llevan.
   */
  rail?: boolean;
  class?: string;
}

export function TopBar({ brand, center, actions, rail, class: cls }: TopBarProps): JSX.Element {
  return (
    <header
      role="banner"
      class={cx(
        // EL CANALÓN CRECE CON LA VENTANA. Con `px-4` a secas, la marca y el
        // avatar quedaban a 14px del cristal lo mismo en un teléfono que en un
        // monitor de 1440 —medido en las cinco apps: `padding: 14px/14px`, y son
        // 14 y no 16 porque la raíz de la suite está en 14px—. En pantalla grande
        // eso se lee como un cromo pinchado contra el borde, que es justo lo que
        // no hace ninguna suite con la que nos comparan.
        //
        // En el teléfono NO se toca: ahí 14px ya es el canalón de la casa y
        // estrecharlo sería robarle sitio al buscador.
        // LA LÍNEA DE ABAJO NO OCUPA ALTURA. Con `border-b` sí la ocupaba: la
        // barra mide 56px contando el borde, así que el área donde `items-center`
        // reparte es de 55 y todo el contenido quedaba 1px alto. Medido en las
        // once apps y en el portal: `arriba 10 / abajo 11` en cada control, sin
        // excepción. Un píxel no se ve de uno en uno; se ve en que la fila entera
        // parece flotar hacia arriba.
        //
        // Con la sombra interior la línea se pinta DENTRO de los 56px sin
        // participar en el reparto, y el contenido cae en el centro exacto.
        'sytue-topbar sytue-glass-surface bg-topbar text-topbar-fg shadow-[inset_0_-1px_0_var(--color-border)]',
        'sticky top-0 flex items-center gap-3 px-4 lg:px-6',
        // En <768px no hay menú lateral a la vista y la barra ENTERA es el carril:
        // color de marca, texto blanco y buscador en velo oscuro. Lo pinta y reasigna
        // los tokens `.sytue-topbar-carril` (tokens/appearance.css), solo a ese ancho.
        // En la forma «Marco» lo es A CUALQUIER ancho: la barra y el menú lateral forman
        // un marco de color continuo, y la misma clase lo pinta (bloque de la forma).
        rail && 'sytue-topbar-carril',
        cls,
      )}
      style={{ height: 'var(--spacing-topbar)', zIndex: 'var(--z-topbar)' }}
    >
      {brand && rail ? (
        // DOS CAJAS. La de fuera RESERVA el sitio: se estira a los 56 px, se come el
        // canalón izquierdo de la barra —`px-4 lg:px-6`— para llegar al cristal y, desde
        // 768 px, mide lo que el menú lateral. La de dentro PINTA: `bg-sidebar` es lo que
        // reasigna los tokens del cromo aquí dentro (ver tokens/appearance.css), y el
        // nombre del producto y el velo del hover salen en blanco sin que el conmutador
        // sepa dónde está; al tapar la barra, su línea inferior no cruza el carril. Su
        // relleno es SIEMPRE el `px-4` del menú lateral, no el de la barra: la marca cae
        // en la misma vertical que el botón de la cabecera del menú.
        // Van separadas porque en «Carril», en una app SIN menú lateral, la que pinta se
        // ciñe a su contenido (`.sytue-esquina-marca`, en la hoja) y la de fuera sigue
        // reservando el ancho: el buscador no se mueve al pasar de una app a otra.
        <div class="-ms-4 flex shrink-0 self-stretch md:w-[var(--spacing-sidebar)] lg:-ms-6">
          <div class="sytue-esquina-marca bg-sidebar text-sidebar-fg flex w-full items-center gap-2 ps-4 pe-3">
            {brand}
          </div>
        </div>
      ) : brand ? (
        <div class="flex shrink-0 items-center gap-2">{brand}</div>
      ) : null}
      {center ? (
        // La zona central (buscador global) se acota al ancho canónico ~480px
        // (design-system §3.3) y se centra; el envoltorio interno mantiene ese
        // tope sin estirar contenidos más estrechos (p. ej. nav de marca).
        <div class="flex min-w-0 flex-1 justify-center">
          <div class="flex w-full max-w-[30rem] justify-center">{center}</div>
        </div>
      ) : (
        <div class="flex-1" />
      )}
      {actions ? <div class="flex shrink-0 items-center gap-1">{actions}</div> : null}
    </header>
  );
}

/** Boton de cromo de la topbar (icon-only, hit area >= 36px). */
export interface TopBarButtonProps extends Omit<JSX.HTMLAttributes<HTMLButtonElement>, 'icon'> {
  label: string;
  /** Punto/badge superpuesto (p.ej. notificaciones sin leer). */
  indicator?: boolean;
}

export function TopBarButton({
  label,
  indicator,
  class: cls,
  children,
  ...rest
}: TopBarButtonProps): JSX.Element {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      class={cx(
        'text-topbar-fg rounded-control relative inline-flex h-9 w-9 items-center justify-center',
        // CON EL DEDO, 44px. La escala de botones ya reserva ese mínimo para
        // puntero grueso, pero este control está escrito a mano y se quedaba en
        // los 31,5px de `h-9`: medido en un teléfono, 32×32 en las once apps.
        'pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]',
        // Velo blanco del propio cromo: un gris fijo se vería como un parche sobre
        // la barra de tinta.
        'hover:bg-chrome-hover active:bg-chrome-active transition-colors duration-150',
        // Anillo del cromo, como los items de la sidebar: blanco, porque el azul
        // global no contrasta sobre la tinta (WCAG AA).
        'sytue-focus-ring-chrome focus-visible:outline-none',
        cls,
      )}
      {...rest}
    >
      {children}
      {indicator ? (
        // Sobre la barra clara, un hueco del color de la barra lo separa del icono. Sobre
        // la barra de COLOR (`.sytue-topbar-carril`) el rojo de peligro no llega a 3:1
        // —y con el color de Mail es rojo sobre rojo—: ahí `sytue-aro-carril` le pone el
        // aro del texto del carril, como a las muestras de color (tokens/appearance.css).
        <span
          class="sytue-aro-carril bg-danger absolute top-2 right-2 h-2 w-2 rounded-full ring-2 ring-[var(--sytue-topbar-bg)]"
          aria-hidden="true"
        />
      ) : null}
    </button>
  );
}
