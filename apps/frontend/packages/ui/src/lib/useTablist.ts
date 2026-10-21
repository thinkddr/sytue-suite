/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * useTablist — el comportamiento de un `tablist` de verdad (patrón APG de pestañas con
 * activación automática) para quien dibuja SUS pestañas y no puede usar `Tabs`.
 *
 * Escribir `role="tab"` es gratis y cumplirlo no: sin esto cada pestaña era una parada de
 * tabulación más, las flechas no hacían nada y nadie enlazaba pestaña y panel. Aquí:
 *
 *   - roving tabindex: solo la pestaña activa es parada de Tab (las demás, -1);
 *   - ←/→ mueven la pestaña ACTIVA y el foco con ella; Inicio y Fin van a los extremos;
 *   - `aria-controls`/`aria-labelledby` unen cada pestaña con el panel.
 *
 * Es lo que hacen `Tabs` y la cinta del editor de Docs, y lo que tres tiras de Sytue PDF
 * se habían saltado. Con `orientacion: 'vertical'`, ↑/↓.
 */
import { useId, useRef } from 'preact/hooks';

export interface Tablist<T extends string> {
  /** El `onKeyDown` del contenedor `role="tablist"`. */
  onKeyDown: (e: KeyboardEvent) => void;
  /** Lo que lleva cada pestaña (`<button {...tab(id)}>`), sin el rótulo ni la clase. */
  tab: (id: T) => {
    role: 'tab';
    id: string;
    'aria-selected': boolean;
    'aria-controls': string;
    tabIndex: number;
    ref: (el: HTMLElement | null) => void;
    onClick: () => void;
  };
  /** Lo que lleva el panel de la pestaña activa (`<div {...panel}>`). */
  panel: { id: string; role: 'tabpanel'; 'aria-labelledby': string };
}

export function useTablist<T extends string>(
  ids: readonly T[],
  activa: T,
  activar: (id: T) => void,
  orientacion: 'horizontal' | 'vertical' = 'horizontal',
): Tablist<T> {
  const base = useId();
  const refs = useRef(new Map<T, HTMLElement>());
  const idTab = (id: T): string => `${base}-tab-${id}`;
  const idPanel = `${base}-panel`;

  const onKeyDown = (e: KeyboardEvent): void => {
    const i = ids.indexOf(activa);
    const [atras, adelante] =
      orientacion === 'vertical' ? ['ArrowUp', 'ArrowDown'] : ['ArrowLeft', 'ArrowRight'];
    let siguiente: number;
    if (e.key === adelante) siguiente = (i + 1) % ids.length;
    else if (e.key === atras) siguiente = (i - 1 + ids.length) % ids.length;
    else if (e.key === 'Home') siguiente = 0;
    else if (e.key === 'End') siguiente = ids.length - 1;
    else return;
    e.preventDefault();
    const id = ids[siguiente]!;
    activar(id);
    refs.current.get(id)?.focus();
  };

  return {
    onKeyDown,
    tab: (id) => ({
      role: 'tab',
      id: idTab(id),
      'aria-selected': id === activa,
      'aria-controls': idPanel,
      tabIndex: id === activa ? 0 : -1,
      ref: (el) => {
        if (el) refs.current.set(id, el);
        else refs.current.delete(id);
      },
      onClick: () => activar(id),
    }),
    panel: { id: idPanel, role: 'tabpanel', 'aria-labelledby': idTab(activa) },
  };
}
