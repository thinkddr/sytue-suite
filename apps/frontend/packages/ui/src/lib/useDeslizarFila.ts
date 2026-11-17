/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DESLIZAR UNA FILA con el dedo para actuar sobre ella: en Mail, a la derecha
 * archiva y a la izquierda manda a la papelera (lo que haya elegido cada cuenta).
 *
 * Es la regla de `decidirDeslizamiento` (la misma de Photos y Calendar: cuánto y
 * qué rápido tiene que ir el dedo) aplicada a una fila de lista en vez de a una
 * página. Lo propio de una fila es CUÁNDO se empieza a arrastrar: la lista se
 * desplaza en vertical con el mismo dedo, así que el gesto solo se queda con el
 * puntero cuando el movimiento es claramente horizontal; si antes va en vertical,
 * la fila no hace nada y la lista se desplaza como siempre (`touch-action: pan-y`
 * deja el vertical al navegador).
 *
 * Solo con el DEDO (pointerType 'touch'): con ratón arrastrar es seleccionar
 * texto, y en escritorio cada fila ya tiene sus botones de triaje.
 */
import { useCallback, useRef, useState } from 'preact/hooks';
import { DOMINIO_HORIZONTAL, decidirDeslizamiento } from './deslizar.js';
import { useUltimo } from './useUltimo.js';

/** Lo que el dedo tiene que recorrer antes de decidir si el gesto es horizontal. */
export const UMBRAL_INICIO = 10;

export type SentidoDeslizar = 'derecha' | 'izquierda';

/** Qué gesto arranca, si alguno, con lo recorrido hasta ahora. */
export function iniciarDeslizar(
  dx: number,
  dy: number,
  hay: { derecha: boolean; izquierda: boolean },
): 'arrastrar' | 'soltar' | 'esperar' {
  const ax = Math.abs(dx);
  const ay = Math.abs(dy);
  if (ax < UMBRAL_INICIO && ay < UMBRAL_INICIO) return 'esperar';
  // Vertical (o diagonal dudosa): es desplazar la lista, no un gesto de la fila.
  if (ax < DOMINIO_HORIZONTAL * ay) return 'soltar';
  return (dx > 0 ? hay.derecha : hay.izquierda) ? 'arrastrar' : 'soltar';
}

/**
 * Cuánto se mueve la fila con el dedo: hacia un lado sin acción se queda quieta, y
 * nunca más allá del ancho (la acción ya está decidida mucho antes).
 */
export function desplazamientoVisible(
  dx: number,
  ancho: number,
  hay: { derecha: boolean; izquierda: boolean },
): number {
  if ((dx > 0 && !hay.derecha) || (dx < 0 && !hay.izquierda)) return 0;
  // Sin ancho medido (la fila aún no se ha pintado) no hay tope que aplicar.
  if (ancho <= 0) return dx;
  return Math.max(-ancho, Math.min(ancho, dx));
}

interface Gesto {
  id: number;
  x: number;
  y: number;
  t: number;
  ancho: number;
  fase: 'pendiente' | 'arrastrando';
}

export interface DeslizarFila {
  /** Desplazamiento actual en px (0 en reposo). */
  dx: number;
  /** Manejadores para el elemento que se desliza. */
  onPointerDown: (e: PointerEvent) => void;
  onPointerMove: (e: PointerEvent) => void;
  onPointerUp: (e: PointerEvent) => void;
  onPointerCancel: (e: PointerEvent) => void;
  /** Tras un deslizamiento, el «click» que el navegador manda al soltar no abre la fila. */
  onClickCapture: (e: MouseEvent) => void;
}

/**
 * El gesto de una fila. `alSoltar` recibe el sentido cuando el deslizamiento
 * cuenta (ver `decidirDeslizamiento`); el que no cuenta devuelve la fila a su sitio.
 */
export function useDeslizarFila(opciones: {
  derecha: boolean;
  izquierda: boolean;
  alSoltar: (sentido: SentidoDeslizar) => void;
}): DeslizarFila {
  const [dx, setDx] = useState(0);
  const gestoRef = useRef<Gesto | null>(null);
  const omitirClickRef = useRef(false);
  const ultimo = useUltimo(opciones);

  const reiniciar = useCallback(() => {
    gestoRef.current = null;
    setDx(0);
  }, []);

  const onPointerDown = useCallback(
    (e: PointerEvent) => {
      omitirClickRef.current = false;
      const { derecha, izquierda } = ultimo.current;
      if (e.pointerType !== 'touch' || !e.isPrimary || (!derecha && !izquierda)) return;
      const el = e.currentTarget as HTMLElement;
      gestoRef.current = {
        id: e.pointerId,
        x: e.clientX,
        y: e.clientY,
        t: e.timeStamp,
        ancho: el.getBoundingClientRect().width,
        fase: 'pendiente',
      };
    },
    [ultimo],
  );

  const onPointerMove = useCallback(
    (e: PointerEvent) => {
      const g = gestoRef.current;
      if (!g || e.pointerId !== g.id) return;
      const ddx = e.clientX - g.x;
      const ddy = e.clientY - g.y;
      const hay = ultimo.current;
      if (g.fase === 'pendiente') {
        const decision = iniciarDeslizar(ddx, ddy, hay);
        if (decision === 'esperar') return;
        if (decision === 'soltar') {
          gestoRef.current = null;
          return;
        }
        g.fase = 'arrastrando';
        // Con el puntero capturado, el dedo puede salir de la fila sin perder el gesto.
        try {
          (e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
        } catch {
          /* happy-dom y algunos navegadores sin captura: el gesto sigue igual */
        }
      }
      setDx(desplazamientoVisible(ddx, g.ancho, hay));
    },
    [ultimo],
  );

  const onPointerUp = useCallback(
    (e: PointerEvent) => {
      const g = gestoRef.current;
      if (!g || e.pointerId !== g.id) return;
      if (g.fase !== 'arrastrando') {
        gestoRef.current = null;
        return;
      }
      omitirClickRef.current = true;
      const ddx = e.clientX - g.x;
      const ddy = e.clientY - g.y;
      const paso = decidirDeslizamiento(ddx, ddy, g.ancho, e.timeStamp - g.t);
      reiniciar();
      const { derecha, izquierda, alSoltar } = ultimo.current;
      // −1 es «lo anterior», que en una página es arrastrar hacia la DERECHA.
      if (paso === -1 && derecha) alSoltar('derecha');
      else if (paso === 1 && izquierda) alSoltar('izquierda');
    },
    [reiniciar, ultimo],
  );

  return {
    dx,
    onPointerDown,
    onPointerMove,
    onPointerUp,
    onPointerCancel: reiniciar,
    onClickCapture: (e: MouseEvent) => {
      if (!omitirClickRef.current) return;
      omitirClickRef.current = false;
      e.preventDefault();
      e.stopImmediatePropagation();
    },
  };
}
