/**
 * Popover — panel flotante anclado a un disparador (role=dialog). A diferencia
 * del Modal (centrado, bloquea el fondo) es contextual y no modal: se abre junto
 * al trigger, se cierra con Esc o clic fuera, atrapa el Tab mientras vive y
 * devuelve el foco al disparador al cerrar por teclado. Posicionamiento en
 * coordenadas de ventana (como Dropdown) con volteo/colisión básicos contra el
 * viewport. Abierto controlado o no controlado. docs/design.md §9.
 */
import type { ComponentChildren, JSX } from 'preact';
import { createPortal } from 'preact/compat';
import { useCallback, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { cx } from '../lib/cx.js';
import { useUltimo } from '../lib/useUltimo.js';
import { useMobileBack } from '../lib/useMobileBack.js';
import { tecladoEnOtraSuperposicion } from '../lib/atajos.js';
import { limitesViewport, observarViewport } from '../lib/viewport.js';

type Side = 'top' | 'bottom';
type Align = 'start' | 'end';

export interface PopoverProps {
  /** Disparador: recibe props de accesibilidad (aria-expanded/haspopup, onClick). */
  trigger: (props: {
    onClick: () => void;
    'aria-expanded': boolean;
    'aria-haspopup': 'dialog';
  }) => ComponentChildren;
  /** Contenido del panel; como render-prop recibe `close` para cerrarse desde dentro. */
  children: ComponentChildren | ((close: () => void) => ComponentChildren);
  /** Lado preferido respecto al disparador; se voltea si no cabe. */
  side?: Side;
  /** Borde de anclaje horizontal; se cambia si el panel se sale del viewport. */
  align?: Align;
  /** Nombre accesible del panel (role=dialog). */
  label?: string;
  /** Apertura controlada (omitir para modo no controlado). */
  open?: boolean;
  /** Aviso de cambio de apertura (modo controlado o telemetría). */
  onOpenChange?: (open: boolean) => void;
}

// Mismos focusables que Modal: define los extremos del atrapado de Tab.
const FOCUSABLE =
  'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])';

const GAP = 6; // separación disparador↔panel (px), a juego con Dropdown/Tooltip
/** Aire mínimo entre el panel y el borde de la ventana (px), como en Dropdown. */
const MARGEN_VENTANA = 8;

export function Popover({
  trigger,
  children,
  side = 'bottom',
  align = 'start',
  label,
  open: openProp,
  onOpenChange,
}: PopoverProps): JSX.Element {
  const controlled = openProp !== undefined;
  const [openState, setOpenState] = useState(false);
  const open = controlled ? openProp : openState;

  const panelRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLDivElement>(null);
  // Si el cierre debe devolver el foco al disparador: sí por teclado (Esc/close),
  // no por clic fuera (robaría el foco de donde el usuario acaba de pulsar).
  const restoreRef = useRef(true);
  // onOpenChange via ref (como onClose de Modal): el efecto se enlaza a [open],
  // así invoca SIEMPRE el callback más reciente aunque el padre re-renderice.
  const onOpenChangeRef = useUltimo(onOpenChange);

  const setOpen = useCallback(
    (v: boolean): void => {
      if (!controlled) setOpenState(v);
      onOpenChangeRef.current?.(v);
    },
    [controlled, onOpenChangeRef],
  );
  const cerrar = useCallback(() => setOpen(false), [setOpen]);
  useMobileBack(open, cerrar, { nivel: 2 });

  // Colocación efectiva (tras volteo/colisión). Parte de la preferida.
  const [placement, setPlacement] = useState<{ side: Side; align: Align }>({ side, align });

  // EL PANEL VA A UN PORTAL, Y NO ES UN ADORNO. Mismo motivo que Dropdown, que
  // lo documentó primero: anclado dentro de la barra, un ancestro con
  // `transform`, `filter` u `opacity` crea un contexto de apilamiento que se
  // come el z-index, y un ancestro con `overflow-x-auto` —una barra de
  // herramientas que se desplaza en móvil, que es el caso de Notes y del
  // redactor— RECORTA lo que sobresale de la fila. En el body no hay ancestro
  // que atrape nada. SALVO DENTRO DE UN <dialog> MODAL, donde va al propio
  // diálogo: showModal() lo sube a la capa superior y un panel en el body
  // quedaba DEBAJO (medido en el visor de Fotos, igual que en Dropdown).
  //
  // La caja en coordenadas de VENTANA la da `colocar()`, no `useFitInViewport`:
  // ese empuja con `translate` una caja anclada al disparador por clases
  // (`left-0`/`right-0`), y una vez fuera del árbol el anclaje es fijo, así que
  // la posición se calcula y se acota aquí entera.
  const [caja, setCaja] = useState<{ top: number; left: number } | null>(null);
  // El <dialog> que contiene al disparador, si lo hay. Se busca al montar —un nodo no
  // cambia de antepasados en toda su vida— y no leyendo la ref en el render, donde en
  // el primer pase aún está vacía.
  const [dialogo, setDialogo] = useState<HTMLDialogElement | null>(null);
  useLayoutEffect(() => {
    setDialogo(triggerRef.current?.closest('dialog') ?? null);
  }, []);

  useLayoutEffect(() => {
    if (!open) return;
    const colocar = (): void => {
      const d = triggerRef.current?.getBoundingClientRect();
      const panel = panelRef.current;
      if (!d || !panel) return;
      // Safari conserva innerHeight al abrir el teclado y puede desplazar el
      // viewport visual. Tamaño Y origen deben referirse a la misma ventana.
      const v = limitesViewport(MARGEN_VENTANA);
      panel.style.maxWidth = `${v.width}px`;
      panel.style.maxHeight = `${v.height}px`;
      // La animación arranca al 98 %: la caja pintada subestimaría el tamaño
      // final y dejaría ese resto fuera del viewport, como ocurría en Dropdown.
      const ancho = panel.offsetWidth;
      const alto = panel.offsetHeight;

      let s: Side = side;
      const fitsBelow = d.bottom + GAP + alto <= v.bottom;
      const fitsAbove = d.top - GAP - alto >= v.top;
      if (side === 'bottom' && !fitsBelow && fitsAbove) s = 'top';
      else if (side === 'top' && !fitsAbove && fitsBelow) s = 'bottom';

      let a: Align = align;
      const overflowStart = d.left + ancho > v.right;
      const overflowEnd = d.right - ancho < v.left;
      if (align === 'start' && overflowStart && !overflowEnd) a = 'end';
      else if (align === 'end' && overflowEnd && !overflowStart) a = 'start';
      setPlacement((prev) => (prev.side === s && prev.align === a ? prev : { side: s, align: a }));

      const tope = (v: number, min: number, max: number): number => Math.max(min, Math.min(v, max));
      setCaja({
        top: tope(s === 'bottom' ? d.bottom + GAP : d.top - alto - GAP, v.top, v.bottom - alto),
        left: tope(a === 'end' ? d.right - ancho : d.left, v.left, v.right - ancho),
      });
    };
    colocar();
    // En CAPTURA, como Dropdown: el contenido no scrollea la ventana sino un
    // div interno, y sin captura el panel se queda clavado mientras la barra
    // que lo contiene se desplaza bajo el panel.
    const dejarDeObservar = observarViewport(colocar);
    // El contenido también puede crecer después de abrir (personas, enlaces,
    // formularios): el acotado debe seguir valiendo tras esa carga.
    const observer = new ResizeObserver(colocar);
    if (panelRef.current) observer.observe(panelRef.current);
    return () => {
      dejarDeObservar();
      observer.disconnect();
    };
  }, [open, side, align]);

  // Cierre por Esc/clic fuera, atrapado de Tab, foco inicial y retorno de foco.
  // useLayoutEffect (síncrono al commit) como Modal: el foco entra/vuelve sin
  // ventana de carrera y sin robárselo a quien ya esté tecleando.
  useLayoutEffect(() => {
    if (!open) return;
    // El disparador vive todo el ciclo del popover: capturarlo aquí da al cleanup
    // el mismo nodo (y calla el aviso de ref cambiante de exhaustive-deps).
    const triggerEl = triggerRef.current;
    // Cierre desde el efecto usando solo setters/refs estables (sin `setOpen`,
    // que es una closure nueva por render): así el efecto se enlaza a [open] sin
    // avisos de exhaustive-deps. `setOpenState(false)` es inocuo si es controlado.
    const requestClose = (restore: boolean): void => {
      restoreRef.current = restore;
      setOpenState(false);
      onOpenChangeRef.current?.(false);
    };
    const onPointerDown = (e: MouseEvent): void => {
      const target = e.target as Node;
      if (!panelRef.current?.contains(target) && !triggerRef.current?.contains(target)) {
        requestClose(false); // cierre externo: no devolver el foco al disparador
      }
    };
    const onKeyDown = (e: KeyboardEvent): void => {
      if (tecladoEnOtraSuperposicion(panelRef.current)) return;
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopImmediatePropagation();
        requestClose(true);
        return;
      }
      if (e.key !== 'Tab') return;
      const panel = panelRef.current;
      if (!panel) return;
      const nodes = Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE));
      if (nodes.length === 0) {
        e.preventDefault(); // panel sin focusables: el Tab no debe escapar
        return;
      }
      const first = nodes[0]!;
      const last = nodes[nodes.length - 1]!;
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };

    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKeyDown, true);
    // Foco inicial dentro del panel (primer focusable, o el propio panel).
    const panel = panelRef.current;
    (panel?.querySelector<HTMLElement>(FOCUSABLE) ?? panel)?.focus();

    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown, true);
      if (restoreRef.current) {
        triggerEl?.querySelector<HTMLElement>(FOCUSABLE)?.focus();
      }
      restoreRef.current = true; // rearmar para el próximo ciclo
    };
  }, [open, onOpenChangeRef]);

  const panel = open ? (
    <div
      ref={panelRef}
      role="dialog"
      aria-label={label}
      tabIndex={-1}
      class={cx(
        // Elevación nivel 2 (popover): superficie elevada, hairline 1px, sombra
        // media y radio lg, igual que el menú de Dropdown. `fixed` y no
        // `absolute`: la caja ya va en coordenadas de ventana.
        'sytue-pop-in sytue-popover-surface border-border fixed w-max max-w-[calc(100vw-16px)]',
        'max-h-[calc(100dvh-16px)] overflow-y-auto overscroll-contain',
        'rounded-lg border p-4 shadow-[var(--shadow-md)] focus-visible:outline-none',
        // La orientación del volteo solo decide de qué lado CAE y desde dónde
        // escala la animación; la posición exacta la da `caja`.
        placement.side === 'bottom' ? 'origin-top' : 'origin-bottom',
      )}
      style={{
        zIndex: 'var(--z-popover)',
        // Fuera de la ventana hasta que se mide, como en Dropdown: el
        // useLayoutEffect coloca el panel ANTES del pintado, así que el valor
        // provisional no llega a verse.
        top: caja ? `${caja.top}px` : '-9999px',
        left: caja ? `${caja.left}px` : '-9999px',
      }}
    >
      {typeof children === 'function' ? children(cerrar) : children}
    </div>
  ) : null;

  return (
    <div class="relative inline-flex">
      <div ref={triggerRef} class="inline-flex">
        {trigger({
          onClick: () => setOpen(!open),
          'aria-expanded': open,
          'aria-haspopup': 'dialog',
        })}
      </div>
      {panel && typeof document !== 'undefined'
        ? createPortal(panel, dialogo ?? document.body)
        : null}
    </div>
  );
}
