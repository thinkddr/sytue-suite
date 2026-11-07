/**
 * BarraMovil — lo que una aplicación pone ARRIBA en el teléfono (<768px): el botón de
 * menú o «volver», su título (o algo centrado, como el mes de Calendar) y sus acciones.
 *
 * UNA SOLA BARRA. Hasta el 2026-11 el teléfono apilaba dos: la topbar de la suite (56)
 * y, debajo, la barra de 44 de cada armazón —AppShell, AdminShell y la cabecera de
 * canal de Rooms, cada una a su manera—. Ahora, si hay una topbar que la acoge (la de
 * `SytueShell`, que publica sus huecos en `BarraUnicaContext`), estas piezas se PORTAN
 * dentro de ella y la barra de 44 no se pinta: la topbar muestra la cara de la app
 * (`[☰] título [acciones] [entrada a la suite]`) y, al tocar la entrada, la de la suite.
 * Ver TopBar.tsx y docs/design.md §2.
 *
 * Sin topbar que la acoja —la app nativa, que no tiene topbar; la barra pública de
 * Cast; un banco sin Shell— pinta la barra de 44 de siempre, con el mismo marcado.
 * Es la pieza de `ui.BarraMovil` en Fyne.
 */
import { createContext } from 'preact';
import type { ComponentChildren, JSX } from 'preact';
import { createPortal } from 'preact/compat';
import { useContext, useLayoutEffect, useRef } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { Button } from './Button.js';
import { Icon } from './Icon.js';

/** Los huecos de la topbar donde caen las piezas de la app. */
export interface DestinosBarra {
  inicio: HTMLElement;
  medio: HTMLElement;
  acciones: HTMLElement;
}

/**
 * Lo que publica la Sytue Shell cuando su topbar puede acoger la barra de la app.
 * `destinos` es null hasta que la topbar se monta (en el servidor, y en el primer
 * pintado): mientras, la app no pinta nada arriba —sin JS nada es pulsable todavía— y
 * la topbar ya enseña el nombre del producto y la entrada a la suite.
 */
export interface BarraUnica {
  destinos: DestinosBarra | null;
}

export const BarraUnicaContext = createContext<BarraUnica | null>(null);

/** ¿Va la barra de esta app dentro de la topbar de la suite? */
export function useBarraUnica(): boolean {
  return useContext(BarraUnicaContext) !== null;
}

/** El botón de menú (☰) que abre el cajón de la app. */
export interface MenuBarraMovil {
  /** Id del botón: adonde vuelve el foco al cerrar el cajón (`useNavDrawer`). */
  id: string;
  /** Id del cajón que abre (`aria-controls`). */
  controla: string;
  abierto: boolean;
  alPulsar: () => void;
  /** Nombre del botón cerrado; por defecto «Abrir navegación». */
  rotulo?: string | undefined;
  /**
   * Hay algo pendiente dentro del cajón (recordatorios, invitaciones): el nombre
   * accesible del botón cerrado, ya traducido, y un punto (el color de señal,
   * `indicator`) en la esquina de la hamburguesa.
   */
  aviso?: string | undefined;
}

/** Lo que solo existe SIN barra única (la app nativa): el mando que ya sube a la topbar. */
export function SinBarraUnica({ children }: { children: ComponentChildren }): JSX.Element | null {
  return useBarraUnica() ? null : <>{children}</>;
}

export interface BarraMovilProps {
  /** El ☰ de la app. Con `volver`, manda «volver». */
  menu?: MenuBarraMovil | undefined;
  /** Vuelve del detalle a la lista: el botón ‹ en lugar del ☰. */
  volver?: (() => void) | undefined;
  /** Título alineado tras el botón (`text-md`; en la barra de 44, `text-sm`). */
  titulo?: ComponentChildren;
  /**
   * Lo que va CENTRADO, en vez de `titulo` (el mes del calendario del teléfono).
   * Centrado en el ancho entero de la barra, no en el hueco que dejan los botones: así
   * no baila según cuántas acciones haya a cada lado. Si así no cabe, se acerca al
   * centro todo lo que deja el lado más ocupado y usa el hueco REAL entre los botones;
   * si ni ahí cabe, baja a `text-sm` antes de truncar (a 320, «Σεπτέμβριος 2027»
   * perdía el año). `BarraMovil.TituloCentrado` en Fyne.
   */
  centro?: ComponentChildren;
  /** Acciones a la derecha (Hoy y «+» en el mes del teléfono). */
  acciones?: ComponentChildren;
  /**
   * Solo dentro de la topbar: sin ella no pinta nada. Para la app que fuera de la
   * barra única ya tiene su propia cabecera (la del canal de Rooms).
   */
  soloEnLaTopbar?: boolean;
}

/** Pone `data-apretado` en el centro de la barra si su contenido no cabe en su talla. */
function medirCentro(centro: HTMLElement): void {
  centro.removeAttribute('data-apretado');
  const hijo = centro.firstElementChild;
  if (hijo && hijo.scrollWidth > hijo.clientWidth) centro.setAttribute('data-apretado', '');
}

export function BarraMovil({
  menu,
  volver,
  titulo,
  centro,
  acciones,
  soloEnLaTopbar = false,
}: BarraMovilProps): JSX.Element | null {
  const unica = useContext(BarraUnicaContext);
  const t = useT();
  // El mismo botón en las tres barras que había (AppShell, AdminShell y la cabecera de
  // Rooms, que lo pintaba a 32 con un glifo de 16): el `ghost sm` de 18.
  const inicio = volver ? (
    <Button variant="ghost" size="sm" iconOnly aria-label={t('ui.backToList')} onClick={volver}>
      <Icon name="chevronLeft" size={18} />
    </Button>
  ) : menu ? (
    <Button
      id={menu.id}
      variant="ghost"
      size="sm"
      iconOnly
      aria-label={menu.abierto ? t('ui.closeNav') : (menu.aviso ?? menu.rotulo ?? t('ui.openNav'))}
      aria-expanded={menu.abierto}
      aria-controls={menu.controla}
      onClick={menu.alPulsar}
      class={menu.aviso ? 'relative' : undefined}
    >
      <Icon name="menu" size={18} />
      {/* El aviso de pendientes: el nombre accesible ya lo dice, el punto es para la
          vista. Va en la esquina del ICONO, no del botón, que con el dedo crece a 44.
          `bg-indicator` y no el primario: sobre la barra de color es el ámbar de la
          señal del menú; en «Carril», sobre la superficie, su turquesa. */}
      {menu.aviso ? (
        <span
          aria-hidden="true"
          data-aviso-nav
          class="bg-indicator absolute top-[calc(50%-15px)] right-[calc(50%-15px)] h-[8px] w-[8px] rounded-full"
        />
      ) : null}
    </Button>
  ) : undefined;
  // El centro baja a `text-sm` si en su talla no cabe ni en el hueco. Se mide en su
  // talla, antes de pintar, y cada vez que la barra cambia de ancho (girar el teléfono).
  //
  // Medir fuerza un reflow, así que se mide solo cuando cambia el TEXTO del centro (o el
  // ancho, con el vigía). El centro llega como JSX nuevo en cada render de la app —el
  // cursor, una carga, el sondeo de recordatorios—, y con él de dependencia cada render
  // quitaba y ponía el atributo, forzaba el reflow y rehacía el ResizeObserver.
  const centroRef = useRef<HTMLDivElement>(null);
  const textoCentroRef = useRef<string | null>(null);
  const hayCentro = centro !== undefined;
  // El vigía se rehace si el centro aparece, desaparece o cambia de barra (al montarse
  // la topbar el centro se vuelve a montar dentro de ella).
  const destino = unica?.destinos?.medio ?? null;
  useLayoutEffect(() => {
    const el = centroRef.current;
    const texto = el?.textContent ?? null;
    if (texto === textoCentroRef.current) return;
    textoCentroRef.current = texto;
    if (el) medirCentro(el);
  });
  useLayoutEffect(() => {
    const el = centroRef.current;
    if (!el || typeof ResizeObserver !== 'function' || !el.parentElement) return;
    // Al cambiar de barra el centro se vuelve a montar con el mismo texto: se mide aquí.
    textoCentroRef.current = el.textContent;
    medirCentro(el);
    const vigia = new ResizeObserver(() => medirCentro(el));
    vigia.observe(el.parentElement);
    return () => vigia.disconnect();
  }, [hayCentro, destino]);

  if (unica) {
    const d = unica.destinos;
    if (!d) return <></>;
    return (
      <>
        {inicio !== undefined ? createPortal(<>{inicio}</>, d.inicio) : null}
        {centro !== undefined
          ? createPortal(
              <div
                ref={centroRef}
                data-centrado=""
                class="min-w-0 truncate data-[apretado]:*:text-sm"
              >
                {centro}
              </div>,
              d.medio,
            )
          : titulo
            ? createPortal(<span class="text-md truncate font-semibold">{titulo}</span>, d.medio)
            : null}
        {acciones !== undefined ? createPortal(<>{acciones}</>, d.acciones) : null}
      </>
    );
  }

  if (soloEnLaTopbar) return null;

  // La barra de 44 de siempre.
  //
  // `px-4`, EL MISMO CANALÓN QUE LA TOPBAR: el botón de menú cae en la misma línea
  // vertical que la marca de arriba (con `px-2` eran 7px reales y el cromo se leía
  // escalonado). Con contenido centrado es una REJILLA de tres columnas: los lados son
  // `1fr` con su contenido de mínimo y el centro, su contenido de máximo. Si cabe, los
  // lados quedan iguales y el centro cae en el de la barra; si no, el lado más ocupado
  // manda y el centro ocupa el hueco que queda.
  return (
    <div
      class={
        centro === undefined
          ? 'sytue-mobilebar border-border bg-surface flex h-11 shrink-0 items-center gap-1.5 border-b px-4 md:hidden'
          : 'sytue-mobilebar border-border bg-surface grid h-11 shrink-0 grid-cols-[minmax(max-content,1fr)_minmax(0,max-content)_minmax(max-content,1fr)] items-center gap-1.5 border-b px-4 md:hidden'
      }
    >
      {inicio}
      {centro !== undefined ? (
        <div ref={centroRef} class="col-start-2 min-w-0 truncate data-[apretado]:*:text-sm">
          {centro}
        </div>
      ) : titulo ? (
        <span class="truncate text-sm font-semibold">{titulo}</span>
      ) : null}
      {acciones !== undefined ? (
        <div
          class={cx(
            'flex items-center gap-1',
            centro === undefined ? 'ml-auto' : 'col-start-3 justify-self-end',
          )}
        >
          {acciones}
        </div>
      ) : null}
    </div>
  );
}
