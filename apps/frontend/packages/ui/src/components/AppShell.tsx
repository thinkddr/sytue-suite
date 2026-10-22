/**
 * AppShell — estructura canonica sidebar + topbar + contenido (con panel
 * secundario opcional para apps de tipo bandeja: Mail, Chat). landmark `main`.
 *
 * En ≥768px las tres columnas van en flujo. En <768px no caben —la sidebar sola
 * se come 260 de los 320px de un iPhone SE— así que:
 *   · la sidebar pasa a drawer sobre overlay (mismo patrón que AdminShell), y
 *   · panel secundario y contenido se turnan a pantalla completa: la lista, y al
 *     elegir un elemento el detalle con «volver» (`detailOpen`/`onBack`).
 * docs/design.md §2 / §9 / §10.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useId, useState } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { useNavDrawer } from '../lib/useNavDrawer.js';
import { useMobileBack } from '../lib/useMobileBack.js';
import { Button } from './Button.js';
import { Icon } from './Icon.js';
import { ResizablePanel } from './ResizablePanel.js';

export interface AppShellProps {
  topbar: ComponentChildren;
  sidebar?: ComponentChildren;
  /** Panel secundario (lista) entre sidebar y contenido. */
  secondaryPanel?: ComponentChildren;
  /** Permite ajustar la lista con puntero o flechas y recordar el ancho. */
  secondaryPanelResize?: { label: string; storageKey: string };
  children: ComponentChildren;
  /** Restringe el contenido al ancho canonico (1280px) con gutter. */
  constrained?: boolean;
  /**
   * Solo con `secondaryPanel` y solo en <768px: `true` muestra el detalle
   * (`children`), `false` la lista. Sin él, en móvil el detalle sería
   * inalcanzable. En ≥768px no interviene: ahí conviven las dos columnas.
   */
  detailOpen?: boolean;
  /** Vuelve del detalle a la lista en <768px (botón de la barra móvil). */
  onBack?: () => void;
  /** Título de la barra móvil, junto al botón de menú. */
  title?: string;
  class?: string;
}

/**
 * Dónde empieza el cajón móvil: justo bajo la topbar de la suite.
 *
 * ANTES ARRANCABA TAMBIÉN BAJO LA BARRA DEL MENÚ (`topbar + 2.75rem`), y eso
 * dejaba una franja BLANCA de 44px entre dos negros —la topbar de la suite
 * arriba y el cajón debajo—, como si la barra de menú no fuera de ninguno de los
 * dos. Es lo que se veía en mail, drive, calendar, contacts, notes y fotos, que
 * son las que montan esta shell.
 *
 * `AdminShell` y Sytue Rooms ya arrancaban aquí; esta era la que se salía, así
 * que lo que hace este cambio es igualar las tres, no inventar una forma nueva.
 *
 * NO ES `inset-y-0`, que fue el error de la otra vez: con el z de modal (1060)
 * por encima del de la topbar (1040), el cajón tapaba la topbar y con ella el
 * selector de apps —abrir la navegación de UNA aplicación escondía la navegación
 * ENTRE aplicaciones—. Arrancando en el token de la topbar, esta se queda
 * entera y visible.
 */
const BAJO_LA_TOPBAR = 'var(--spacing-topbar)';

export function AppShell({
  topbar,
  sidebar,
  secondaryPanel,
  secondaryPanelResize,
  children,
  constrained = false,
  detailOpen = false,
  onBack,
  title,
  class: cls,
}: AppShellProps): JSX.Element {
  const t = useT();
  const [open, setOpen] = useState(false);
  const base = useId();
  const drawerId = `sytue-app-nav-${base}`;
  const toggleId = `sytue-app-menu-${base}`;
  useMobileBack(Boolean(secondaryPanel && detailOpen && onBack), () => onBack?.(), {
    // Un teléfono que gira conserva la salida del detalle también desde 768 px.
    nivel: 0,
  });
  useNavDrawer(open, drawerId, toggleId, () => setOpen(false));

  // En <768px el panel secundario y el contenido se excluyen; sin panel, el
  // contenido manda siempre (no hay nada con lo que turnarse).
  const verDetalle = !secondaryPanel || detailOpen;

  return (
    // El marco ocupa el viewport y NO hace scroll: solo los paneles internos
    // (lista y contenido) desplazan en su propio eje. overflow-hidden evita que
    // un contenido ancho empuje la pagina y "recorte" la sidebar lateralmente.
    // 100dvh y no h-screen (100vh): en Chrome Android 100vh es la altura CON la
    // barra de direcciones retraída, o sea más que la pantalla visible al cargar,
    // y como el marco no desplaza, lo que sobra queda fuera y no se alcanza.
    // Es la unidad que ya usa AdminShell.
    // `sytue-frame` y, más abajo, `sytue-sheet`: en la forma «Marco» la raíz pinta el
    // marco de color y la columna del contenido es la hoja que flota dentro (en <768px,
    // a sangre). En «Carril» las dos clases no hacen nada. tokens/appearance.css.
    <div
      class={cx(
        'sytue-frame bg-bg text-text flex h-[100dvh] min-w-0 flex-col overflow-hidden',
        cls,
      )}
    >
      {topbar}
      <div class="sytue-safe-workspace flex min-h-0 min-w-0 flex-1">
        {sidebar ? (
          <>
            {open ? (
              <div
                // Empieza donde el cajón, bajo la topbar: si el velo empezara
                // más abajo, la franja de la barra de menú quedaría sin oscurecer
                // y volvería a leerse como una tira suelta entre los dos negros.
                class="sytue-fade-in fixed inset-x-0 bottom-0 bg-[var(--sytue-overlay)] md:hidden"
                style={{ top: BAJO_LA_TOPBAR, zIndex: 'var(--z-sticky)' }}
                onClick={() => setOpen(false)}
                aria-hidden="true"
              />
            ) : null}
            {/* EL TECLADO YA FUNCIONA, y por eso las dos reglas de abajo no
                aplican. El `onClick` no hace clicable al <aside>: es un DELEGADOR
                que escucha lo que burbujea desde botones y enlaces que ya son
                accesibles por sí mismos. Activar un botón con Enter o Espacio
                dispara un `click` que burbujea igual que el del dedo, así que un
                `onKeyDown` aquí no arreglaría nada: duplicaría el cierre. */}
            {/* oxlint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-noninteractive-element-interactions */}
            <aside
              id={drawerId}
              tabIndex={-1}
              class={cx(
                'sytue-nav-drawer flex shrink-0 focus-visible:outline-none',
                // Arranca bajo la TOPBAR (ver `BAJO_LA_TOPBAR`) y se queda por
                // debajo de ella en la pila. Sigue tapando el contenido, que es
                // lo que se quería: el contenido no declara z, así que el de la
                // sidebar (1030) le gana igual.
                'max-md:fixed max-md:bottom-0 max-md:left-0',
                'z-[var(--z-sidebar)]',
                open ? 'sytue-drawer-in' : 'max-md:hidden',
              )}
              // `top` en línea y sin variante `max-md:`: en escritorio el aside
              // es estático y un elemento estático IGNORA `top`, así que no hace
              // falta acotarlo al breakpoint.
              style={{ top: BAJO_LA_TOPBAR }}
              // ACTIVAR ALGO DEL MENÚ LO CIERRA, en móvil. El drawer tapa la
              // pantalla entera: pulsar «Redactar» abría el compositor DETRÁS del
              // menú, que seguía ahí, y parecía que el botón no hacía nada. Lo
              // mismo con elegir una carpeta, un cuaderno o un canal.
              //
              // Va aquí, en el contenedor, y no en cada botón de las cinco
              // aplicaciones que montan esta shell: el estado `open` es de este
              // componente y ninguna de ellas puede tocarlo, así que sin esto la
              // única salida era que cada una expusiera su propio cierre.
              //
              // OPT-OUT Y NO HEURÍSTICO. Se intentó deducirlo de `aria-expanded`
              // y no vale: en el árbol de cuadernos lo lleva el propio elemento
              // que SÍ debe cerrar —seleccionar un cuaderno— mientras el chevron
              // que solo expande no lo lleva. El criterio quedaba justo al revés.
              // Con `data-mantiene-nav` el que no debe cerrar lo dice, que son
              // pocos y se leen de un vistazo.
              onClick={(e) => {
                const el = (e.target as HTMLElement | null)?.closest?.(
                  'a[href],button,[role="treeitem"],[role="menuitem"]',
                );
                if (el && !el.closest('[data-mantiene-nav]')) setOpen(false);
              }}
            >
              {sidebar}
            </aside>
          </>
        ) : null}
        <div class="sytue-sheet-column flex min-h-0 min-w-0 flex-1 flex-col">
          {/* Barra móvil: única vía de abrir la nav —y de volver a la lista— en <768px. */}
          {sidebar || secondaryPanel ? (
            // `px-4`, EL MISMO CANALÓN QUE LA TOPBAR, y por eso y no por otra
            // cosa: esta barra va justo debajo, así que el botón de menú cae en
            // la misma línea vertical que la marca de arriba. Con `px-2` eran 7px
            // reales —la raíz de la suite está en 14px— y el cromo se leía
            // escalonado: marca a 14, hamburguesa a 7. Salió 28 veces en el
            // barrido, en mail, notes y platform.
            <div class="sytue-mobilebar border-border bg-surface flex h-11 shrink-0 items-center gap-1.5 border-b px-4 md:hidden">
              {secondaryPanel && detailOpen && onBack ? (
                <Button
                  variant="ghost"
                  size="sm"
                  iconOnly
                  aria-label={t('ui.backToList')}
                  onClick={onBack}
                >
                  <Icon name="chevronLeft" size={18} />
                </Button>
              ) : sidebar ? (
                <Button
                  id={toggleId}
                  variant="ghost"
                  size="sm"
                  iconOnly
                  aria-label={open ? t('ui.closeNav') : t('ui.openNav')}
                  aria-expanded={open}
                  aria-controls={drawerId}
                  onClick={() => setOpen((v) => !v)}
                >
                  <Icon name="menu" size={18} />
                </Button>
              ) : null}
              {/* Con panel secundario visible, el título sobra: la lista trae el suyo
                  justo debajo («Recibidos · 3 conversaciones») y se leía dos veces.
                  En el detalle sí hace falta: ahí es lo único que dice dónde estás. */}
              {title && (!secondaryPanel || detailOpen) ? (
                <span class="truncate text-sm font-semibold">{title}</span>
              ) : null}
            </div>
          ) : null}
          {/* La barra móvil pertenece al marco de color; la hoja empieza DEBAJO. Sus
              esquinas las redondea lo de dentro (`sytue-sheet-corners`). En escritorio
              panel y detalle comparten las esquinas; en móvil cada uno ocupa la hoja
              entera, con sus dos esquinas superiores. */}
          <div class="sytue-sheet flex min-h-0 min-w-0 flex-1">
            {secondaryPanel ? (
              <ResizablePanel
                label={
                  secondaryPanelResize?.label ??
                  (typeof title === 'string' && title ? title : 'Panel')
                }
                storageKey={secondaryPanelResize?.storageKey ?? 'sytue.shell.panel-width'}
                detailOpen={verDetalle}
              >
                {secondaryPanel}
              </ResizablePanel>
            ) : null}
            {/* tabIndex -1: destino programático del skip-link (recibe foco al saltar). */}
            <main
              id="contenido"
              tabIndex={-1}
              class={cx(
                // `relative`: bloque contenedor de lo que desplaza dentro. Ver la nota de
                // AdminShell: sin él, un `sr-only` alargaba el documento bajo el armazón.
                'sytue-sheet-corners relative min-w-0 flex-1 overflow-y-auto focus-visible:outline-none',
                secondaryPanel && 'md:[--sytue-sheet-bl:0px] md:[--sytue-sheet-tl:0px]',
                secondaryPanel && !verDetalle && 'max-md:hidden',
              )}
            >
              {constrained ? (
                <div
                  class="mx-auto w-full min-w-0 p-6"
                  style={{ maxWidth: 'var(--container-content)' }}
                >
                  {children}
                </div>
              ) : (
                children
              )}
            </main>
          </div>
        </div>
      </div>
    </div>
  );
}
