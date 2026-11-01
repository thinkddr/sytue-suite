/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * Arrastrar hacia abajo para actualizar en pantallas táctiles (paridad con
 * `ui.RefrescarAlTirar` de `apps/fyne/ui/gestosmoviles.go`).
 *
 * Solo captura el gesto cuando:
 *   · hay un único dedo,
 *   · ninguna superposición modal o menú está abierta,
 *   · el toque no nace en un campo editable, barra horizontal (`data-scroll-x`),
 *     barra de herramientas, lienzo o editor (`data-suelo-tactil`), y
 *   · toda caja desplazable entre el dedo y el contenedor está ya arriba del todo
 *     (`scrollTop <= 0.5`), de modo que subir una lista desplazada conserva siempre
 *     el desplazamiento nativo.
 *
 * Al soltar tras superar `UMBRAL_ACTUALIZAR`, ejecuta `alActualizar` si se indicó o
 * despacha `CustomEvent('sytue:refresh', { cancelable: true })` en `window` (con
 * recarga limpia de la página si ninguna isla lo absorbe con `preventDefault()`).
 */
import type { JSX, RefObject } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { cx } from './cx.js';
import { haySuperposicionAbierta } from './atajos.js';
import { useUltimo } from './useUltimo.js';
import { Icon } from '../components/Icon.js';

/** Recorrido mínimo antes de decidir si el gesto es vertical u horizontal (px). */
export const INICIO_GESTO_REFRESCO = 12;

/** Recorrido vertical tras la captura necesario para disparar la actualización (px). */
export const UMBRAL_ACTUALIZAR = 72;

const ZONAS_EXCLUIDAS =
  'input, textarea, select, [contenteditable="true"], [data-scroll-x], [data-sin-refresco], [data-suelo-tactil], [role="toolbar"], [role="slider"], [role="separator"], canvas, .touch-none';

export interface EstadoRefrescoTactil {
  /** Recorrido vertical capturado desde el punto de enganche (px). */
  recorrido: number;
  /** `true` mientras se ejecuta la actualización tras soltar por encima del umbral. */
  actualizando: boolean;
}

export interface OpcionesRefrescoTactil {
  /** Acción explícita al actualizar; si se omite, emite `sytue:refresh` / recarga. */
  alActualizar?: () => void | Promise<unknown>;
  /** Sustituye `window.location.reload()` (para pruebas sin navegación real). */
  recargar?: () => void;
}

interface PuntoToque {
  identifier: number;
  clientX: number;
  clientY: number;
}

interface EventoTactilCompatible extends Event {
  touches?: ArrayLike<PuntoToque>;
  changedTouches?: ArrayLike<PuntoToque>;
}

function buscarToque(lista: ArrayLike<PuntoToque> | undefined, id: number): PuntoToque | null {
  if (!lista) return null;
  for (let i = 0; i < lista.length; i++) {
    const t = lista[i];
    if (t && t.identifier === id) return t;
  }
  return null;
}

/** Comprueba si algún ancestro desplazable entre `origen` y `raiz` no está arriba del todo. */
export function hayDesplazamientoVerticalPrevio(
  origen: Element | null,
  raiz: HTMLElement,
): boolean {
  if (typeof window !== 'undefined' && window.scrollY > 0.5) return true;
  let actual: Element | null = origen;
  while (actual) {
    if (actual instanceof HTMLElement && actual.scrollTop > 0.5) return true;
    if (actual === raiz) break;
    actual = actual.parentElement;
  }
  return raiz.scrollTop > 0.5;
}

function hayCapaModalAbierta(): boolean {
  if (haySuperposicionAbierta()) return true;
  if (typeof document === 'undefined') return false;
  return document.querySelector('[role="alertdialog"], dialog[open]') !== null;
}

export function usePullToRefresh(
  contenedorRef: RefObject<HTMLElement>,
  opciones?: OpcionesRefrescoTactil,
): EstadoRefrescoTactil {
  const [recorrido, setRecorrido] = useState(0);
  const [actualizando, setActualizando] = useState(false);
  const actualizandoRef = useUltimo(actualizando);
  const alActualizarRef = useUltimo(opciones?.alActualizar);
  const recargarRef = useUltimo(opciones?.recargar);
  const gestoRef = useRef<{
    id: number;
    x0: number;
    y0: number;
    yCaptura: number;
    bruto: number;
    capturado: boolean;
    origen: Element | null;
  } | null>(null);

  useEffect(() => {
    const el = contenedorRef.current;
    if (!el || typeof window === 'undefined') return;

    let vivo = true;
    let temporizador: ReturnType<typeof setTimeout> | undefined;

    const limpiarGesto = (): void => {
      gestoRef.current = null;
      if (vivo) setRecorrido(0);
    };

    const ejecutarActualizacion = (): void => {
      gestoRef.current = null;
      setActualizando(true);
      setRecorrido(UMBRAL_ACTUALIZAR);

      const finalizar = (): void => {
        temporizador = setTimeout(() => {
          if (!vivo) return;
          setActualizando(false);
          setRecorrido(0);
        }, 220);
      };

      const explicita = alActualizarRef.current;
      if (explicita) {
        void Promise.resolve(explicita()).finally(finalizar);
        return;
      }

      let promesas: Promise<unknown>[] = [];
      const ev = new CustomEvent('sytue:refresh', {
        cancelable: true,
        detail: {
          esperar: (p: Promise<unknown>) => {
            promesas = [...promesas, p];
          },
        },
      });
      window.dispatchEvent(ev);
      if (ev.defaultPrevented || promesas.length > 0) {
        void Promise.allSettled(promesas).finally(finalizar);
        return;
      }
      const recargar = recargarRef.current ?? (() => window.location.reload());
      recargar();
      finalizar();
    };

    const alEmpezar = (ev: Event): void => {
      const e = ev as EventoTactilCompatible;
      if (actualizandoRef.current || !e.touches || e.touches.length !== 1) {
        gestoRef.current = null;
        return;
      }
      if (hayCapaModalAbierta()) return;
      const origen = e.target instanceof Element ? e.target : null;
      if (origen?.closest(ZONAS_EXCLUIDAS)) return;
      if (hayDesplazamientoVerticalPrevio(origen, el)) return;
      const t = e.touches[0];
      if (!t) return;
      gestoRef.current = {
        id: t.identifier,
        x0: t.clientX,
        y0: t.clientY,
        yCaptura: t.clientY,
        bruto: 0,
        capturado: false,
        origen,
      };
    };

    const alMover = (ev: Event): void => {
      const e = ev as EventoTactilCompatible;
      const g = gestoRef.current;
      if (!g) return;
      if (!e.touches || e.touches.length !== 1) {
        limpiarGesto();
        return;
      }
      const t = buscarToque(e.touches, g.id);
      if (!t) return;

      if (!g.capturado) {
        if (hayCapaModalAbierta() || hayDesplazamientoVerticalPrevio(g.origen, el)) {
          gestoRef.current = null;
          return;
        }
        const dx = t.clientX - g.x0;
        const dy = t.clientY - g.y0;
        if (Math.abs(dx) < INICIO_GESTO_REFRESCO && Math.abs(dy) < INICIO_GESTO_REFRESCO) return;
        if (dy < INICIO_GESTO_REFRESCO || dy < 1.5 * Math.abs(dx)) {
          gestoRef.current = null;
          return;
        }
        g.capturado = true;
        g.yCaptura = g.y0 + INICIO_GESTO_REFRESCO;
      }

      if (e.cancelable) e.preventDefault();
      g.bruto = Math.max(0, t.clientY - g.yCaptura);
      setRecorrido(g.bruto);
    };

    const alTerminar = (ev: Event): void => {
      const e = ev as EventoTactilCompatible;
      const g = gestoRef.current;
      if (!g) return;
      const t = buscarToque(e.changedTouches, g.id);
      const brutoFinal = t && g.capturado ? Math.max(g.bruto, t.clientY - g.yCaptura) : g.bruto;
      if (g.capturado && brutoFinal >= UMBRAL_ACTUALIZAR) {
        ejecutarActualizacion();
        return;
      }
      limpiarGesto();
    };

    el.addEventListener('touchstart', alEmpezar, { passive: true });
    el.addEventListener('touchmove', alMover, { passive: false });
    el.addEventListener('touchend', alTerminar, { passive: true });
    el.addEventListener('touchcancel', limpiarGesto, { passive: true });

    return () => {
      vivo = false;
      if (temporizador !== undefined) clearTimeout(temporizador);
      el.removeEventListener('touchstart', alEmpezar);
      el.removeEventListener('touchmove', alMover);
      el.removeEventListener('touchend', alTerminar);
      el.removeEventListener('touchcancel', limpiarGesto);
    };
  }, [contenedorRef, actualizandoRef, alActualizarRef, recargarRef]);

  return { recorrido, actualizando };
}

/**
 * Disco visual de 36 × 36 px que acompaña el gesto táctil de refresco, idéntico al
 * indicador de `raizGestos` en Fyne (`apps/fyne/ui/gestosmoviles.go`). En reposo no
 * monta ningún nodo en el DOM.
 */
export function IndicadorRefrescoTactil({
  estado,
  fijo = false,
}: {
  estado: EstadoRefrescoTactil;
  /** Usa posicionamiento fijo bajo la barra superior (superficies `display: contents`). */
  fijo?: boolean;
}): JSX.Element | null {
  const { recorrido, actualizando } = estado;
  if (recorrido <= 0 && !actualizando) return null;

  const avance = actualizando ? 1 : Math.min(1, recorrido / UMBRAL_ACTUALIZAR);
  const exceso = actualizando ? 0 : Math.max(0, recorrido - UMBRAL_ACTUALIZAR) * 0.18;
  const desplazamientoY = Math.min(64, -36 + avance * 52 + exceso);
  const listo = actualizando || recorrido >= UMBRAL_ACTUALIZAR;

  return (
    <div
      data-sytue-pull-indicator=""
      aria-hidden="true"
      class={cx(
        'pointer-events-none left-1/2 z-30 flex h-9 w-9 -translate-x-1/2 items-center justify-center rounded-full',
        'border-border bg-surface-raised border shadow-[var(--shadow-md)]',
        listo ? 'text-primary-text' : 'text-text-muted',
        fijo ? 'fixed top-[var(--spacing-topbar,0px)]' : 'absolute top-0',
      )}
      style={{
        transform: `translate(-50%, ${Math.round(desplazamientoY)}px)`,
        opacity: Math.min(1, 0.35 + avance * 0.65),
      }}
    >
      <Icon
        name="rotate"
        size={18}
        class={cx(actualizando && 'animate-spin')}
        style={actualizando ? undefined : { transform: `rotate(${Math.round(avance * 270)}deg)` }}
      />
    </div>
  );
}
