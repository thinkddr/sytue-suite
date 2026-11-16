/**
 * Tooltip — ayuda contextual breve. Accesible por teclado (foco), ratón (hover) y
 * descartable con Escape (WCAG 1.4.13). No debe contener información esencial única.
 * docs/design.md §4.3, §9.
 *
 * Por qué es así (v2):
 *   – SIN envoltorio. Los manejadores se inyectan con `cloneElement` en el propio
 *     control. El `<span>` de la primera versión metía un nodo en cada botón y le
 *     cambiaba el contexto de maquetación (un `inline-flex` dentro de una barra
 *     `flex`), que es justo lo que impidió adoptarlo en la cinta del editor.
 *   – El panel va en un PORTAL al `body` con `position: fixed`: anclado dentro del
 *     control, cualquier ancestro con `overflow` lo recortaba, y por los bordes de la
 *     ventana se salía. Ahora se coloca con el rectángulo del control, se voltea
 *     arriba/abajo (o derecha/izquierda) si no cabe y se acota a la ventana.
 *   – Retardo de GRUPO: si una ayuda se acaba de cerrar, la siguiente abre sin
 *     esperar. Recorrer una barra de iconos con el ratón no debe costar 300 ms por
 *     icono.
 */
import type { ComponentChildren, JSX, VNode } from 'preact';
import { cloneElement, isValidElement } from 'preact';
import { createPortal } from 'preact/compat';
import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'preact/hooks';
import { cx } from '../lib/cx.js';
import { useFitInViewport } from '../lib/useFitInViewport.js';
import { limitesViewport, observarViewport } from '../lib/viewport.js';

export type TooltipPlacement = 'top' | 'bottom' | 'right';

export interface TooltipProps {
  label: string;
  children: ComponentChildren;
  /**
   * Lado preferido. Si no cabe, se voltea al opuesto. `right` es el del carril
   * plegado: encima del icono taparía al vecino de arriba.
   */
  placement?: TooltipPlacement;
}

/** Retardo de apertura por puntero (spec §Tooltip). El foco abre al instante. */
const RETARDO = 300;
/** Ventana del retardo de grupo tras cerrarse otra ayuda (ms). */
const GRUPO = 500;
/** Separación control↔panel (px), a juego con Popover/Dropdown. */
const GAP = 6;
/** Aire mínimo con el borde de la ventana (px), el mismo de `useFitInViewport`. */
const MARGEN = 8;

// Compartido por todas las ayudas de la página: es lo que hace «de grupo» al retardo.
let ultimoCierre = Number.NEGATIVE_INFINITY;

/**
 * El temporizador de apertura de UNA ayuda. Vive fuera del render (en un `useMemo`) y
 * no en un `ref`: los manejadores que lo usan se inyectan en el hijo con `cloneElement`.
 */
function crearTemporizador(setOpen: (fn: (antes: boolean) => boolean) => void) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const abrir = (): void => {
    clearTimeout(timer);
    setOpen(() => true);
  };
  return {
    abrir,
    programar: (): void => {
      clearTimeout(timer);
      if (Date.now() - ultimoCierre < GRUPO) abrir();
      else timer = setTimeout(abrir, RETARDO);
    },
    cerrar: (): void => {
      clearTimeout(timer);
      setOpen((antes) => {
        if (antes) ultimoCierre = Date.now();
        return false;
      });
    },
    soltar: (): void => {
      clearTimeout(timer);
    },
  };
}

type Lado = 'top' | 'bottom' | 'right' | 'left';

interface Posicion {
  top: number;
  left: number;
  lado: Lado;
  /** Desplazamiento de la flecha a lo largo del borde del panel (px). */
  flecha: number;
}

/** Coloca el panel junto al control, volteado si no cabe y acotado a la ventana. */
export function colocarTooltip(
  ancla: DOMRect,
  panel: { width: number; height: number },
  preferido: TooltipPlacement,
): Posicion {
  const v = limitesViewport(MARGEN);
  const acotar = (x: number, min: number, max: number) => Math.max(min, Math.min(x, max));
  if (preferido === 'right') {
    const cabeDerecha = ancla.right + GAP + panel.width <= v.right;
    const cabeIzquierda = ancla.left - GAP - panel.width >= v.left;
    const lado: Lado = cabeDerecha || !cabeIzquierda ? 'right' : 'left';
    const left = acotar(
      lado === 'right' ? ancla.right + GAP : ancla.left - GAP - panel.width,
      v.left,
      v.right - panel.width,
    );
    const centro = ancla.top + ancla.height / 2;
    const top = acotar(centro - panel.height / 2, v.top, v.bottom - panel.height);
    return { top, left, lado, flecha: acotar(centro - top, 6, panel.height - 6) };
  }
  const cabeArriba = ancla.top - GAP - panel.height >= v.top;
  const cabeAbajo = ancla.bottom + GAP + panel.height <= v.bottom;
  const lado: Lado =
    preferido === 'top'
      ? cabeArriba || !cabeAbajo
        ? 'top'
        : 'bottom'
      : cabeAbajo || !cabeArriba
        ? 'bottom'
        : 'top';
  const top = lado === 'top' ? ancla.top - GAP - panel.height : ancla.bottom + GAP;
  const centro = ancla.left + ancla.width / 2;
  const left = acotar(centro - panel.width / 2, v.left, v.right - panel.width);
  return { top, left, lado, flecha: acotar(centro - left, 6, panel.width - 6) };
}

export function Tooltip({ label, children, placement = 'top' }: TooltipProps): JSX.Element {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<Posicion | null>(null);
  const id = useId();
  const [ancla, setAncla] = useState<HTMLElement | null>(null);
  const panelRef = useRef<HTMLSpanElement | null>(null);

  const { abrir, programar, cerrar, soltar } = useMemo(() => crearTemporizador(setOpen), []);
  useEffect(() => soltar, [soltar]);

  // Escape cierra la ayuda sin tocar el foco ni la acción (WAI-ARIA APG): un tooltip que
  // tapa lo que hay debajo y no se puede quitar con el teclado es una trampa. El listener
  // solo vive mientras está abierto.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') cerrar();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, cerrar]);

  // Se mide ANTES de pintar: el panel nace invisible en (0,0) y aquí toma su sitio,
  // así no parpadea en la esquina. Se recoloca si la ventana o un panel se desplazan.
  useLayoutEffect(() => {
    if (!open) {
      setPos(null);
      return;
    }
    const medir = (): void => {
      const panel = panelRef.current;
      if (!ancla || !panel) return;
      // Un control que ya no está en el documento no tiene dónde anclar la ayuda.
      if (!ancla.isConnected) {
        cerrar();
        return;
      }
      const r = panel.getBoundingClientRect();
      const nueva = colocarTooltip(
        ancla.getBoundingClientRect(),
        { width: r.width, height: r.height },
        placement,
      );
      setPos((p) =>
        p &&
        p.top === nueva.top &&
        p.left === nueva.left &&
        p.lado === nueva.lado &&
        p.flecha === nueva.flecha
          ? p
          : nueva,
      );
    };
    medir();
    return observarViewport(medir);
  }, [open, ancla, label, placement, cerrar]);

  // Red de seguridad: si el panel cambia de tamaño después de colocarlo (una fuente
  // que llega tarde), el empujón lo devuelve dentro de la ventana.
  useFitInViewport(panelRef, open && pos ? pos.lado : null);

  // El control se recuerda por el evento, no por `ref`: un `ref` inyectado pisaría el
  // que ya traiga el hijo (el disparador de un Popover lo trae).
  const recordar = (e: Event): void => {
    if (e.currentTarget instanceof HTMLElement) setAncla(e.currentTarget);
  };
  const llamar = (propio: unknown, e: Event): void => {
    if (typeof propio === 'function') (propio as (e: Event) => void)(e);
  };

  let control: ComponentChildren = children;
  if (isValidElement(children)) {
    const hijo = children as VNode<Record<string, unknown>>;
    const p = hijo.props;
    // `preact/compat` (que carga cualquier página con un Popover) reescribe `onFocus` y
    // `onBlur` del hijo como `onfocusin`/`onfocusout`: hay que leer los dos nombres o el
    // manejador propio se perdería al inyectar el nuestro.
    const extra: Record<string, unknown> = {
      onMouseEnter: (e: Event) => {
        llamar(p.onMouseEnter, e);
        recordar(e);
        programar();
      },
      onMouseLeave: (e: Event) => {
        llamar(p.onMouseLeave, e);
        cerrar();
      },
      onFocus: (e: Event) => {
        llamar(p.onFocus ?? p.onfocusin, e);
        recordar(e);
        abrir();
      },
      onBlur: (e: Event) => {
        llamar(p.onBlur ?? p.onfocusout, e);
        cerrar();
      },
      // Pulsar el control descarta la ayuda: ya se ha elegido, y si abre un menú la
      // ayuda quedaría encima de él.
      onPointerDown: (e: Event) => {
        llamar(p.onPointerDown, e);
        cerrar();
      },
    };
    // `aria-describedby` va en el CONTROL: el lector de pantalla anuncia la descripción
    // del elemento que recibe el foco. Si la ayuda repite su nombre accesible —un
    // IconButton cuyo `label` es a la vez `aria-label`— no se enlaza: el lector lo
    // leería dos veces seguidas.
    if (open && p['aria-label'] !== label) {
      extra['aria-describedby'] = [p['aria-describedby'], id].filter(Boolean).join(' ');
    }
    control = cloneElement(hijo, extra);
  }

  const lado = pos?.lado ?? (placement === 'right' ? 'right' : placement);
  const vertical = lado === 'top' || lado === 'bottom';
  const panel = open ? (
    <span
      ref={panelRef}
      id={id}
      role="tooltip"
      data-placement={lado}
      class={cx(
        'pointer-events-none fixed top-0 left-0 block w-max max-w-[240px] rounded-md text-center',
        // Superficie invertida vía tokens de rol: contraste alto en claro y oscuro.
        'sytue-fade-in sytue-tooltip-surface px-2 py-1 text-xs font-medium shadow-[var(--shadow-md)]',
      )}
      style={{
        zIndex: 'var(--z-tooltip)',
        top: pos ? `${Math.round(pos.top)}px` : '0px',
        left: pos ? `${Math.round(pos.left)}px` : '0px',
        visibility: pos ? undefined : 'hidden',
      }}
    >
      {label}
      <span
        aria-hidden="true"
        class={cx(
          'sytue-tooltip-arrow absolute h-1.5 w-1.5 rotate-45',
          lado === 'top' && '-bottom-[3px] -translate-x-1/2',
          lado === 'bottom' && '-top-[3px] -translate-x-1/2',
          lado === 'right' && '-left-[3px] -translate-y-1/2',
          lado === 'left' && '-right-[3px] -translate-y-1/2',
        )}
        style={
          pos
            ? vertical
              ? { left: `${Math.round(pos.flecha)}px` }
              : { top: `${Math.round(pos.flecha)}px` }
            : undefined
        }
      />
    </span>
  ) : null;

  return (
    <>
      {control}
      {panel && typeof document !== 'undefined'
        ? // Dentro de un <dialog> modal, al dialog: lo que cuelga del body queda DEBAJO de
          // la capa superior y la ayuda no se vería (lo mismo que hace Popover).
          createPortal(panel, ancla?.closest('dialog') ?? document.body)
        : null}
    </>
  );
}
