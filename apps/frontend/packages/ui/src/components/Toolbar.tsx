/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Toolbar — una barra de herramientas que cumple lo que promete.
 *
 * `role="toolbar"` le dice al lector de pantalla que ahí dentro hay UNA sola parada de
 * tabulación y que se navega con las flechas. Escribir el rol es gratis; cumplirlo, no, y
 * por eso las cuatro barras de la suite lo prometían sin cumplirlo: cada botón era su
 * propia parada, así que salir de la barra de acciones de un correo costaba seis tabs.
 *
 * El componente existe para que el rol y el comportamiento no puedan volver a separarse.
 */
import type { JSX, Ref } from 'preact';
import { useImperativeHandle, useLayoutEffect, useRef } from 'preact/hooks';
import { FOCUSABLE } from './Modal.js';

export interface ToolbarProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /** Obligatorio: una barra sin nombre se anuncia como «barra de herramientas» a secas. */
  'aria-label': string;
  /**
   * El nodo de la barra, prestado a quien lo necesite además del roving tabindex.
   *
   * No es `ref` porque Preact se lo queda para sí en los componentes de función, y la
   * barra ya usa el suyo. Lo pide la de Sytue PDF, que mide su propio scroll para saber
   * si queda cinta a la derecha.
   */
  innerRef?: Ref<HTMLDivElement>;
  /**
   * Selector CSS de subárboles cuyos controles NO son de la barra: los de un popover o un
   * diálogo que se abre dentro de ella. Sin esto, la barra les quitaría la parada de
   * tabulación y no se podría tabular el formulario de un popover. La cinta del editor
   * de Docs lo usa (`.ribbon-popover, [role="dialog"]`).
   */
  omitir?: string;
}

/** Teclas que mueven el foco dentro de la barra, según el eje. */
const ADELANTE = new Set(['ArrowRight', 'ArrowDown']);
const ATRAS = new Set(['ArrowLeft', 'ArrowUp']);

/**
 * Las flechas dentro de un campo son del campo (un `select` cambia de valor, un texto
 * mueve el cursor). La barra solo las intercepta cuando el foco está en un botón.
 */
function esCampo(el: EventTarget | null): boolean {
  return el instanceof HTMLElement && /^(SELECT|INPUT|TEXTAREA)$/.test(el.tagName);
}

export function Toolbar({
  children,
  onKeyDown: alTeclear,
  innerRef,
  omitir,
  ...rest
}: ToolbarProps): JSX.Element {
  const ref = useRef<HTMLDivElement>(null);
  /** Cuál de los controles es la parada de tabulación. */
  const activoRef = useRef(0);

  const controles = (): HTMLElement[] =>
    ref.current
      ? Array.from(ref.current.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
          // `data-toolbar-skip`: una región que se desplaza dentro de la barra y es
          // enfocable por sí misma (WCAG 2.1.1, regla `scrollable-region-focusable`); no
          // es un control de la barra y no le toca el roving.
          (el) => !el.hasAttribute('data-toolbar-skip') && (!omitir || !el.closest(omitir)),
        )
      : [];

  const marcar = (lista: HTMLElement[], i: number): void => {
    activoRef.current = i;
    lista.forEach((el, n) => {
      el.tabIndex = n === i ? 0 : -1;
    });
  };

  // La barra se entrega al consumidor al montar y se retira al desmontar; el nodo no
  // cambia en toda la vida del componente.
  // Al montar el <div> ya está puesto: se renderiza siempre.
  useImperativeHandle(innerRef ?? null, () => ref.current!, []);

  /* De maqueta y sin lista de dependencias, a propósito. Sin dependencias porque los
     controles de una barra aparecen y desaparecen (según lo que haya seleccionado, según
     los permisos) y el que era la parada de tabulación puede irse; de maqueta porque debe
     quedar puesto ANTES de pintar: un Tab inmediato no puede pillar la barra con tres
     paradas. */
  useLayoutEffect(() => {
    const lista = controles();
    if (lista.length > 0) marcar(lista, Math.min(activoRef.current, lista.length - 1));
  });

  const onKeyDown = (e: JSX.TargetedKeyboardEvent<HTMLDivElement>): void => {
    if (esCampo(e.target)) return;
    // Las teclas de un popover o un diálogo que cuelga de la barra son suyas.
    if (omitir && e.target instanceof Element && e.target.closest(omitir)) return;
    const lista = controles();
    if (lista.length === 0) return;
    let siguiente: number;
    if (ADELANTE.has(e.key) || ATRAS.has(e.key)) {
      const desde = lista.findIndex((el) => el === document.activeElement);
      const paso = ADELANTE.has(e.key) ? 1 : -1;
      siguiente = ((desde < 0 ? 0 : desde) + paso + lista.length) % lista.length;
    } else if (e.key === 'Home') siguiente = 0;
    else if (e.key === 'End') siguiente = lista.length - 1;
    else return;
    e.preventDefault();
    marcar(lista, siguiente);
    lista[siguiente]?.focus();
  };

  return (
    // Roving tabindex (patrón APG): el foco vive en el botón activo de la barra, no
    // en el contenedor — darle `tabIndex` metería una parada de tabulación de más.
    // oxlint-disable-next-line jsx-a11y/interactive-supports-focus
    <div
      {...rest}
      ref={ref}
      role="toolbar"
      onKeyDown={(e) => {
        onKeyDown(e);
        if (typeof alTeclear === 'function') alTeclear.call(e.currentTarget, e);
      }}
    >
      {children}
    </div>
  );
}
