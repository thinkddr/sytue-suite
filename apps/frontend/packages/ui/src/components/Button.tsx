/**
 * Button — boton canonico. Variantes primary/secondary/ghost/danger; tamaños
 * xs/sm/md/lg; estados icon, loading, disabled. Foco visible, aria-busy en loading.
 * La talla mínima es 24px (`xs`, el suelo de WCAG 2.5.8); de `sm` en adelante, >= 32px.
 * docs/design.md §9.
 */
import type { JSX } from 'preact';
import { cx } from '../lib/cx.js';
import { Spinner } from './Spinner.js';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
/** `xs` es el botón del CROMO denso (ribbon, paneles laterales), donde `sm` = 32 px es
 *  demasiado alto y por eso cada panel se hacía el suyo. 24 px es el suelo de WCAG 2.5.8:
 *  no hay ni habrá talla menor. */
export type ButtonSize = 'xs' | 'sm' | 'md' | 'lg';

export interface ButtonProps extends Omit<
  JSX.ButtonHTMLAttributes<HTMLButtonElement>,
  'size' | 'icon' | 'loading'
> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  /** Boton solo-icono (cuadrado); requiere aria-label. */
  iconOnly?: boolean;
  /** Icono al inicio. */
  leadingIcon?: JSX.Element;
  /** Icono al final. */
  trailingIcon?: JSX.Element;
  fullWidth?: boolean;
  /**
   * Destino: renderiza un `<a>` con EXACTAMENTE el mismo aspecto.
   *
   * Existe porque «botón que navega» es un control real y frecuente, y sin esto se
   * reinventaba a mano: 14 sitios con 4 recetas distintas, la mayoría anchors con las
   * clases de `variant="secondary"` copiadas y ya divergidas. Copiar clases no copia el
   * anillo de foco doble ni los estados, así que cada copia perdía algo por el camino.
   *
   * Un enlace no es un botón: navega, se puede abrir en otra pestaña y el lector de
   * pantalla lo anuncia como enlace. Por eso se renderiza `<a>` de verdad, no un
   * `<button>` con `onClick={() => location.assign(...)}`.
   */
  href?: string;
  target?: string;
  rel?: string;
}

const base =
  // `whitespace-nowrap` NO es decoración. Un botón tiene alto fijo (`h-8`…) y un
  // rótulo que salta de línea no cabe en él: se sale por arriba y por abajo. En el
  // ERP, al abrir el formulario lateral la barra de la lista se estrecha y
  // «Exportar CSV» salía en tres líneas e «Importar CSV o facturas» en cuatro, con
  // el texto fuera del botón. Si no hay sitio, que salte la FILA, no el rótulo.
  // `rounded-control`: 4 px en «Marco» y «Carril», compartidos por toda la suite.
  'inline-flex items-center justify-center whitespace-nowrap rounded-control font-semibold ' +
  'transition-colors duration-150 ease-[var(--ease-standard)] select-none ' +
  'focus-visible:outline-none ' +
  // Anillo de foco con hueco (AA): 1px del color de la superficie y 2px de anillo,
  // así se ve sobre cualquier relleno —el azul del primario incluido— y en cualquier
  // tema.
  'sytue-focus-ring-inset';

// bg-border como fondo de pulsación: un paso más marcado que el hover en claro
// y en oscuro (surface-sunken coincide con el hover en tema oscuro y no se ve).
//
// SIN SOMBRA. Los rellenos llevaban `shadow-xs`, que sobre superficies planas y
// hairlines de 1px —el registro del sistema— leía como relieve prestado de otra época:
// un botón sólido con acento propio ya se distingue del fondo sin necesidad de flotar.
// Y fuera el `active:translate-y-px` de la clase base: el empujón físico al pulsar
// afectaba a TODOS los botones de la suite, desplaza el texto un píxel en cada clic y
// el cambio de relleno de `active:` ya da la respuesta táctil. Dos técnicas para decir
// lo mismo, y la que se queda es la que no mueve nada de sitio.
const variants: Record<ButtonVariant, string> = {
  primary:
    'bg-primary text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] active:bg-[var(--sytue-primary-active)]',
  secondary:
    'bg-primary text-[var(--sytue-on-primary)] border border-border-strong hover:bg-[var(--sytue-primary-hover)] active:bg-[var(--sytue-primary-active)]',
  ghost: 'bg-transparent text-primary-text hover:bg-selected active:bg-selected',
  // Blanco y no `on-primary`: aquel es el rótulo del relleno PRIMARIO, y en oscuro es casi
  // negro. Sobre el rojo de peligro daría 3,7:1; el blanco, 5,1:1 en los tres temas.
  danger:
    'bg-danger text-white hover:bg-[var(--sytue-danger-hover)] active:bg-[var(--sytue-danger-hover)]',
};

/**
 * `pointer-coarse:min-h-[max(44px,2.75rem)]` — 44 px con el dedo, y solo con el dedo.
 *
 * Un botón de 32 px se pulsa bien con un ratón y se falla con la yema: la guía de
 * Apple pide 44 y es la vara que se aplicó en el repaso de las doce apps. Pero
 * subirlo también en escritorio rompería la densidad, que es criterio de la casa
 * —sobrio y denso—, así que la talla depende del PUNTERO y no del ancho de la
 * pantalla: un portátil táctil de 15 pulgadas necesita el objetivo grande, y un
 * móvil con teclado y ratón conectados, no.
 *
 * No se hace con un `::after` invisible que agrande solo la zona pulsable. Es lo
 * que suele recomendarse y aquí sería peor: en una fila de acciones con `gap-0.5`,
 * las áreas de dos botones vecinos se solaparían cinco píxeles por lado, y tocar
 * el borde de uno activaría el otro. Un fallo así se diagnostica fatal.
 *
 * `max(44px,2.75rem)` Y NO `min-h-11`, que es lo que había. `11` son 2.75rem, y
 * eso solo mide 44 px si la raíz del navegador está en 16: con la base en 14 —un
 * ajuste de accesibilidad corriente— el objetivo quedaba en 38,5 px, por debajo
 * del mínimo que esta regla existe para garantizar. Justo a quien reduce el
 * tamaño de letra, que suele ser quien más precisión necesita.
 *
 * El `max` conserva la otra mitad: a quien SUBE la base a 20, `2.75rem` son 55 px
 * y se respetan. 44 es un suelo, nunca un techo. Es la misma forma que la regla
 * de los campos de formulario en `styles.css`, y por el mismo motivo.
 */
const sizes: Record<ButtonSize, string> = {
  // El gap va en la TALLA y no en la base: dos utilidades `gap-*` en el mismo elemento
  // se resuelven por el orden de la hoja generada, no por el de la cadena de clases, así
  // que un `gap-1` local no habría ganado de forma fiable al `gap-2` de la base.
  // Suelo de 24 px en px por lo mismo que IconButton `sm`: `h-6` son 21 px con la
  // raíz a 14 (medido: «Apagar»/«Encender» del ERP, 52×21).
  xs: 'h-6 min-h-[24px] gap-1 px-2 text-xs pointer-coarse:min-h-[max(44px,2.75rem)]',
  sm: 'h-8 gap-2 px-3 text-sm pointer-coarse:min-h-[max(44px,2.75rem)]',
  md: 'h-9 gap-2 px-4 text-base pointer-coarse:min-h-[max(44px,2.75rem)]',
  // `lg` LLEVA LA RESERVA COMO LOS DEMÁS. Se había dejado fuera dando por hecho
  // que `h-11` ya eran los 44px del dedo, y en esta suite NO: la raíz está en
  // 14px, así que 2.75rem son 38,5. Medido en el portal en un teléfono: el CTA
  // principal, 38,5px de alto.
  lg: 'h-11 gap-2 px-5 text-md pointer-coarse:min-h-[max(44px,2.75rem)]',
};

// Los de solo icono necesitan además el ANCHO: un botón alto y estrecho se sigue
// fallando, porque el dedo yerra en los dos ejes.
const iconSizes: Record<ButtonSize, string> = {
  xs: 'h-6 w-6 min-h-[24px] min-w-[24px] p-0 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]',
  sm: 'h-8 w-8 p-0 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]',
  md: 'h-9 w-9 p-0 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]',
  lg: 'h-11 w-11 p-0 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]',
};

export function Button({
  variant = 'primary',
  size = 'md',
  loading = false,
  iconOnly = false,
  leadingIcon,
  trailingIcon,
  fullWidth = false,
  disabled,
  type = 'button',
  href,
  class: cls,
  children,
  ...rest
}: ButtonProps): JSX.Element {
  const inerte = disabled || loading;
  const klass = cx(
    base,
    variants[variant],
    iconOnly ? iconSizes[size] : sizes[size],
    // Cargando ≠ deshabilitado: el botón ocupado se atenúa menos y muestra
    // cursor de espera; el deshabilitado real queda al 50 % sin puntero.
    loading ? 'cursor-wait opacity-75' : 'disabled:pointer-events-none disabled:opacity-50',
    fullWidth && 'w-full',
    cls,
  );
  // `iconOnly` es la GEOMETRÍA (cuadrada), que es lo que dice su tipo y lo único
  // para lo que se usa. Antes también descartaba los hijos, y eso dejaba VACÍO a
  // todo el que pasa el icono como contenido en vez de por `leadingIcon`: la X de
  // cada modal de la suite, el botón de enviar de Rooms, el menú móvil y el
  // conmutador de tema del sitio público. Salían con su `aria-label` intacto
  // —así que el teclado y el lector de pantalla funcionaban— y sin nada dibujado,
  // que es justo el fallo que ninguna revisión visual detecta si nunca abre ese
  // modal. Quien no quiera texto, que no lo pase.
  const inner = (
    <>
      {loading ? <Spinner size={size === 'lg' ? 20 : size === 'xs' ? 12 : 16} /> : leadingIcon}
      {children}
      {!loading && trailingIcon}
    </>
  );

  // Variante ENLACE. Un `<a>` no entiende `disabled`: quitarle el `href` es lo que de
  // verdad lo saca de la navegación y del orden de tabulación, y `aria-disabled` es lo
  // que se lo cuenta al lector de pantalla. Poner `disabled` a secas no haría nada.
  if (href !== undefined) {
    return (
      <a
        href={inerte ? undefined : href}
        aria-disabled={inerte ? 'true' : undefined}
        aria-busy={loading ? 'true' : undefined}
        class={cx(klass, inerte && 'pointer-events-none opacity-50')}
        {...(rest as unknown as JSX.HTMLAttributes<HTMLAnchorElement>)}
      >
        {inner}
      </a>
    );
  }

  return (
    <button
      type={type}
      class={klass}
      disabled={inerte}
      aria-busy={loading ? 'true' : undefined}
      {...rest}
    >
      {inner}
    </button>
  );
}
