/**
 * TopBar — barra superior global (landmark `banner`, 56px, fija). Slots para
 * marca/logo, zona central (busqueda) y acciones de la derecha (app switcher,
 * notificaciones, presencia, cuenta). El cableado de comportamiento vive en
 * @sytue/shell; este es el contenedor visual canonico. docs/design.md §2.
 */
import type { ComponentChildren, JSX, Ref } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import type { DestinosBarra } from './BarraMovil.js';
import { Icon } from './Icon.js';

/**
 * La barra ÚNICA del teléfono (<768px): la topbar acoge la barra de la app
 * (`BarraMovil`) y la suite queda tras una sola entrada. Dos caras de la misma barra:
 *
 *   app    [☰ / ‹]  título o centro  [acciones de la app] [entrada]
 *   suite  [✕]  marca ▾  buscador  [sin conexión] [campana] [cuenta]
 *
 * La entrada es el avatar de la cuenta, en el sitio exacto del de la topbar de siempre:
 * al tocarla la barra despliega la suite y el avatar se queda donde estaba, así que el
 * segundo toque abre la cuenta. ✕ (o Esc) vuelve a la cara de la app. En ≥768px estas
 * piezas no se pintan y la barra es la de siempre.
 */
export interface TopBarMovil {
  /** Lo que se lee sin título de la app: el nombre de marca del producto («Mail»). */
  titulo: string;
  /**
   * La marca del producto, en el sitio del ☰ cuando la app no tiene menú (ver abajo).
   */
  marca?: ComponentChildren;
  /** El contenido de la entrada a la suite (el avatar de la cuenta, `xs`). */
  entrada: ComponentChildren;
  /**
   * Avisos sin leer con la barra plegada (la campana no se ve): pinta el punto de la
   * entrada, el mismo de la campana, y se añade a su nombre.
   */
  aviso?: { texto: string };
  /**
   * Una pieza de la suite que no se pliega: el estado sin conexión en pequeño, que hay
   * que poder leer yendo por la calle. Va entre las acciones de la app y la entrada.
   */
  estado?: ComponentChildren;
  /** Recibe los huecos donde la app porta su barra (null al desmontar). */
  alDestinos: (d: DestinosBarra | null) => void;
}

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
  /** La barra única del teléfono (ver `TopBarMovil`). Sin ella, la de siempre. */
  movil?: TopBarMovil;
  class?: string;
}

export function TopBar({
  brand,
  center,
  actions,
  rail,
  movil,
  class: cls,
}: TopBarProps): JSX.Element {
  const t = useT();
  // La cara de la suite (desplegada) o la de la app; solo cuenta por debajo de 768.
  const [suite, setSuite] = useState(false);
  const inicioRef = useRef<HTMLDivElement>(null);
  const medioRef = useRef<HTMLDivElement>(null);
  const accionesRef = useRef<HTMLDivElement>(null);
  const entradaRef = useRef<HTMLButtonElement>(null);
  const suiteRef = useRef<HTMLDivElement>(null);
  const alDestinos = movil?.alDestinos;
  const hayMovil = movil !== undefined;
  useEffect(() => {
    if (!alDestinos) return;
    const [inicio, medio, acciones] = [inicioRef.current, medioRef.current, accionesRef.current];
    if (inicio && medio && acciones) alDestinos({ inicio, medio, acciones });
    return () => alDestinos(null);
  }, [alDestinos, hayMovil]);
  // Al desplegar, el foco va a lo primero de la suite (el selector de aplicaciones); al
  // plegar, vuelve a la entrada. Sin esto el foco se quedaba en un botón que acaba de
  // desaparecer y el teclado volvía al principio de la página.
  const primeraVezRef = useRef(true);
  useEffect(() => {
    if (primeraVezRef.current) {
      primeraVezRef.current = false;
      return;
    }
    if (suite) suiteRef.current?.querySelector<HTMLElement>('button,a[href],input')?.focus();
    else entradaRef.current?.focus();
  }, [suite]);
  // Esc pliega la suite desde dentro de la barra (si un menú abierto no lo ha atendido).
  const headerRef = useRef<HTMLElement>(null);
  useEffect(() => {
    const h = headerRef.current;
    if (!suite || !h) return;
    const alTeclado = (e: KeyboardEvent): void => {
      if (e.key === 'Escape' && !e.defaultPrevented) setSuite(false);
    };
    h.addEventListener('keydown', alTeclado);
    return () => h.removeEventListener('keydown', alTeclado);
  }, [suite]);
  const plegada = hayMovil && !suite;
  const desplegada = hayMovil && suite;
  // Lo de la suite que se esconde en la cara de la app del teléfono.
  const deLaSuite = plegada ? 'max-md:hidden' : undefined;
  const nombreEntrada = movil
    ? movil.aviso
      ? `${t('ui.suiteOpen')} · ${movil.aviso.texto}`
      : t('ui.suiteOpen')
    : '';

  return (
    <header
      ref={headerRef}
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
        // La cara de la app: con un título, una fila (el título ocupa lo que queda); con
        // algo centrado (el mes), una REJILLA de tres columnas como la barra de 44 de
        // AppShell: los lados son `1fr` con su contenido de mínimo y el centro su
        // contenido de máximo, así que cae en el centro de la barra y no baila según las
        // acciones.
        plegada &&
          'max-md:gap-0.5 max-md:has-[[data-centrado]]:grid max-md:has-[[data-centrado]]:grid-cols-[minmax(max-content,1fr)_minmax(0,max-content)_minmax(max-content,1fr)]',
        // Desplegada, la de siempre con el ✕ delante: el hueco entre zonas baja a `gap-2`
        // para que el buscador no pierda los 44 px del ✕ entero a 320.
        desplegada && 'max-md:gap-2',
        cls,
      )}
      style={{ height: 'var(--spacing-topbar)', zIndex: 'var(--z-topbar)' }}
      // La barra única estrecha su canalón en el teléfono (theme.css, `.sytue-topbar`).
      data-barra-unica={hayMovil ? '' : undefined}
    >
      {desplegada ? (
        // El ✕ va donde estaba el botón de menú, que es donde estaba el dedo.
        <TopBarButton label={t('ui.suiteClose')} class="md:hidden" onClick={() => setSuite(false)}>
          <Icon name="close" size={20} />
        </TopBarButton>
      ) : null}
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
        <div
          ref={suiteRef}
          class={cx(
            '-ms-4 flex shrink-0 self-stretch md:w-[var(--spacing-sidebar)] lg:-ms-6',
            deLaSuite,
            // Tras el ✕ no hay cristal que alcanzar: ni margen negativo ni el relleno del
            // menú lateral, o la marca quedaba a 26 px del ✕ en vez de al hueco de la barra.
            desplegada && 'max-md:ms-0 max-md:[&>div]:ps-0',
          )}
        >
          <div class="sytue-esquina-marca bg-sidebar text-sidebar-fg flex w-full items-center gap-2 ps-4 pe-3">
            {brand}
          </div>
        </div>
      ) : brand ? (
        <div ref={suiteRef} class={cx('flex shrink-0 items-center gap-2', deLaSuite)}>
          {brand}
        </div>
      ) : null}
      {center ? (
        // La zona central (buscador global) se acota al ancho canónico ~480px
        // (design-system §3.3) y se centra; el envoltorio interno mantiene ese
        // tope sin estirar contenidos más estrechos (p. ej. nav de marca).
        <div class={cx('flex min-w-0 flex-1 justify-center', deLaSuite)}>
          <div class="flex w-full max-w-[30rem] justify-center">{center}</div>
        </div>
      ) : (
        <div class={cx('flex-1', deLaSuite)} />
      )}
      {actions ? (
        <div class={cx('flex shrink-0 items-center gap-1', deLaSuite)}>{actions}</div>
      ) : null}
      {movil ? (
        <>
          {/* Los huecos de la cara de la app. `md:hidden`: en escritorio no existen. */}
          <div
            ref={inicioRef}
            class={cx(
              'peer/inicio col-start-1 flex shrink-0 items-center empty:hidden md:hidden',
              desplegada && 'max-md:hidden',
            )}
          />
          {movil.marca ? (
            // Sin ☰, la marca del producto en su sitio y de su tamaño exacto (el botón
            // `ghost sm iconOnly`; con el dedo, 44): el título empieza en la misma x en
            // todas las apps y a su izquierda está la identidad de la app, no un hueco.
            <span
              aria-hidden="true"
              data-marca-app
              class={cx(
                'col-start-1 hidden h-8 w-8 shrink-0 items-center justify-center peer-empty/inicio:flex md:hidden pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]',
                desplegada && 'max-md:hidden!',
              )}
            >
              {movil.marca}
            </span>
          ) : null}
          <div
            class={cx(
              'col-start-2 flex min-w-0 flex-1 items-center md:hidden',
              desplegada && 'max-md:hidden',
            )}
          >
            {/* Sin título de la app, el NOMBRE DEL PRODUCTO: la barra nunca se queda
                vacía, y a <640 la marca ya no lo decía. */}
            <div ref={medioRef} class="peer flex min-w-0 empty:hidden" />
            <span class="text-md hidden truncate font-semibold peer-empty:block">
              {movil.titulo}
            </span>
          </div>
          <div
            class={cx(
              'col-start-3 flex shrink-0 items-center gap-0.5 justify-self-end md:hidden',
              desplegada && 'max-md:hidden',
            )}
          >
            <div ref={accionesRef} class="flex items-center gap-1 empty:hidden" />
            {movil.estado}
            <TopBarButton
              botonRef={entradaRef}
              label={nombreEntrada}
              aria-expanded={false}
              indicator={movil.aviso !== undefined}
              onClick={() => setSuite(true)}
            >
              {movil.entrada}
            </TopBarButton>
          </div>
        </>
      ) : null}
    </header>
  );
}

/** Boton de cromo de la topbar (icon-only, hit area >= 36px). */
export interface TopBarButtonProps extends Omit<
  JSX.HTMLAttributes<HTMLButtonElement>,
  'icon' | 'ref'
> {
  label: string;
  /** Punto/badge superpuesto (p.ej. notificaciones sin leer). */
  indicator?: boolean;
  /** El botón, para devolverle el foco (`ref` no llega a un componente de función). */
  botonRef?: Ref<HTMLButtonElement>;
}

export function TopBarButton({
  label,
  indicator,
  class: cls,
  children,
  botonRef,
  ...rest
}: TopBarButtonProps): JSX.Element {
  return (
    <button
      {...(botonRef ? { ref: botonRef } : {})}
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
