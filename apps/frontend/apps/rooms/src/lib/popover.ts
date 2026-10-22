/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cuándo se cierra un popover anclado (el selector de emoji, el menú de un
 * mensaje) y cuándo NO.
 *
 * Los dos se pintan en un portal con `position: fixed` calculada al abrir, así
 * que cualquier cosa que mueva el ancla los deja flotando en el sitio
 * equivocado. La regla era «ante cualquier scroll o resize, cerrar», y en un
 * móvil eso los hacía inservibles:
 *
 *   - Al abrir el selector se enfocaba el buscador. Enfocar un input en un móvil
 *     levanta el TECLADO VIRTUAL, el teclado redimensiona el viewport, y el
 *     `resize` cerraba el popover a los milisegundos de abrirlo. Se abría y se
 *     cerraba solo, sin tocar nada.
 *   - El listener de scroll iba en CAPTURA, así que también recibía el scroll de
 *     la propia rejilla de emoji (que tiene su `overflow-y-auto`): bajar por el
 *     catálogo con el dedo lo cerraba.
 *
 * Aquí vive la decisión, en funciones puras y con sus checks, porque el fallo no
 * estaba en los listeners sino en lo que se decidía dentro de ellos.
 */
import { useEffect } from 'preact/hooks';
import type { RefObject } from 'preact';
import { useMobileBack } from '@sytue/ui';

/**
 * ¿Un scroll invalida la posición del popover?
 *
 * Solo si viene de FUERA. El de dentro es su propia lista: cerrarlo con él
 * impide recorrerla, que es justo para lo que está.
 */
export function cierraPorScroll(
  destino: Node | null,
  dentro: (n: Node | null) => boolean,
): boolean {
  return !dentro(destino);
}

/**
 * ¿Un cambio de tamaño de ventana invalida la posición?
 *
 * Solo si cambió el ANCHO: rotar el móvil o redimensionar la ventana recolocan
 * todo. El teclado virtual cambia únicamente el ALTO, y no es motivo para cerrar
 * nada — era la causa de que el selector se cerrara solo.
 */
export function cierraPorResize(anchoAlAbrir: number, anchoAhora: number): boolean {
  return anchoAlAbrir !== anchoAhora;
}

/**
 * ¿Hay teclado físico? Se usa para decidir si robar el foco al abrir.
 *
 * `(hover: hover)` es la señal de que el puntero es un ratón o un lápiz de
 * precisión. No es una detección de «móvil» —que no existe y no se debe
 * inventar—, es la pregunta que de verdad importa: si enfocar un campo va a
 * levantar un teclado que se coma media pantalla.
 */
export function hayPunteroFino(): boolean {
  return typeof window !== 'undefined' && window.matchMedia?.('(hover: hover)').matches === true;
}

/**
 * ¿Este clic debe desplegar por TOQUE lo que en escritorio revela el hover (la
 * barra de acciones de un mensaje)?
 *
 * Tailwind 4 encierra `hover:` en `@media (hover: hover)`, así que con el dedo esos
 * estados no llegan NUNCA y la barra se quedaba invisible e intocable. El sustituto
 * táctil es el propio toque, pero solo el suyo:
 *
 *   - con ratón manda el hover de siempre y fijar la barra sería un estado de más;
 *   - un `pointerType` vacío es un clic que no vino de un puntero (Enter sobre un
 *     botón de dentro), y ahí ya la revela `focus-within`.
 */
export function revelaPorToque(pointerType: string): boolean {
  return pointerType !== '' && pointerType !== 'mouse';
}

/**
 * Cierra el popover cuando toca: al pulsar fuera, al desplazar algo de fuera o
 * al cambiar el ancho de la ventana.
 *
 * `pointerdown` y no `mousedown`: el mismo manejador vale para ratón, dedo y
 * lápiz. Con `mousedown`, en táctil el evento llega sintetizado y tarde —
 * después del `touchend`—, así que el cierre al tocar fuera iba con retraso.
 */
export function useCierreDePopover(
  abierto: boolean,
  cerrar: () => void,
  refs: RefObject<HTMLElement | null>[],
): void {
  useMobileBack(abierto, cerrar);

  useEffect(() => {
    if (!abierto) return;
    const dentro = (n: Node | null): boolean =>
      refs.some((r) => Boolean(n && r.current?.contains(n)));

    const alPulsar = (e: PointerEvent): void => {
      if (!dentro(e.target as Node)) cerrar();
    };
    const alDesplazar = (e: Event): void => {
      if (cierraPorScroll(e.target as Node, dentro)) cerrar();
    };
    const anchoAlAbrir = window.innerWidth;
    const alRedimensionar = (): void => {
      if (cierraPorResize(anchoAlAbrir, window.innerWidth)) cerrar();
    };

    document.addEventListener('pointerdown', alPulsar);
    window.addEventListener('scroll', alDesplazar, true);
    window.addEventListener('resize', alRedimensionar);
    return () => {
      document.removeEventListener('pointerdown', alPulsar);
      window.removeEventListener('scroll', alDesplazar, true);
      window.removeEventListener('resize', alRedimensionar);
    };
    // `refs` es un array literal en cada render del llamante: incluirlo re-ataría
    // los listeners en cada render. Los refs en sí no cambian de identidad.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [abierto, cerrar]);
}
