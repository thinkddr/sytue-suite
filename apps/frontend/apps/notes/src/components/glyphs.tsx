/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Glifos propios de Sytue Notepads: los que el catálogo de `@sytue/ui` todavía no
 * tiene (tipos de nota, disposiciones, formato de texto). Mismo trazo 1.7 y
 * mismo grid 24 que `Icon`, para que no se noten de otra familia.
 *
 * No van a `@sytue/ui` porque hoy solo los usa esta app; el día que otra los
 * pida, ese es el momento de promoverlos.
 */
import type { JSX } from 'preact';

const PATHS = {
  notebook: 'M6 4h11a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2ZM8.5 4v16',
  note: 'M7 4h8.5a1.5 1.5 0 0 1 1.5 1.5v6.5L12.5 17H7a1.5 1.5 0 0 1-1.5-1.5v-10A1.5 1.5 0 0 1 7 4ZM17 12h-3a1.5 1.5 0 0 0-1.5 1.5V17',
  todo: 'M4.5 6.5h3v3h-3zM4.5 14.5h3v3h-3zM11 8h9M11 16h9',
  link: 'M10 13.5a3.5 3.5 0 0 0 5 0l3-3a3.5 3.5 0 0 0-5-5l-1 1M14 10.5a3.5 3.5 0 0 0-5 0l-3 3a3.5 3.5 0 0 0 5 5l1-1',
  image: 'M4 5.5h16v13H4zM8 11a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3ZM4 16l4.5-4 4 3.5L16 12l4 4',
  file: 'M13.5 3.5H7A1.5 1.5 0 0 0 5.5 5v14A1.5 1.5 0 0 0 7 20.5h10a1.5 1.5 0 0 0 1.5-1.5V8.5ZM13.5 3.5V8.5h5',
  palette:
    'M12 3.5a8.5 8.5 0 0 0 0 17c1.4 0 2-.9 2-1.8 0-.5-.2-.9-.5-1.2-.3-.3-.5-.7-.5-1.2 0-.9.7-1.6 1.6-1.6h1.4A4.5 4.5 0 0 0 20.5 10c0-3.6-3.8-6.5-8.5-6.5ZM7.5 12h.01M9.5 8.5h.01M14 7.5h.01',
  layoutFree: 'M4 4.5h6v5H4zM13 6.5h7v4h-7zM6 13h7v6.5H6zM16 14.5h4v5h-4z',
  layoutColumns: 'M4.5 4.5h5v15h-5zM14.5 4.5h5v15h-5z',
  layoutList: 'M4.5 5.5h15v4h-15zM4.5 14.5h15v4h-15z',
  drag: 'M9 6h.01M15 6h.01M9 12h.01M15 12h.01M9 18h.01M15 18h.01',
  bold: 'M6.5 4.5h6a3.75 3.75 0 0 1 0 7.5h-6ZM6.5 12h7a3.75 3.75 0 0 1 0 7.5h-7Z',
  italic: 'M14.5 4.5h-5M14.5 19.5h-5M14 4.5 10 19.5',
  bulletList: 'M4.5 6.5h.01M4.5 12h.01M4.5 17.5h.01M9 6.5h10.5M9 12h10.5M9 17.5h10.5',
  numberedList: 'M4 5h1.5v4M4 9h3M4 13.5h3v2H4v3h3M10 7h10M10 14.5h10',
  underline: 'M7 4v6a5 5 0 0 0 10 0V4M5.5 20.5h13',
  strikethrough:
    'M4.5 12h15M8 7.5A3.5 3.5 0 0 1 11.5 4h1.5a3.5 3.5 0 0 1 3.3 2.3M16 16.2a3.5 3.5 0 0 1-3.3 3.8h-1.5A3.5 3.5 0 0 1 7.7 16',
  unlock:
    'M8 10V7a4 4 0 0 1 7.5-2M5 11a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-8Z',
  pin: 'M9.5 3.5h5l-.8 5.5 3.3 3.5H7l3.3-3.5ZM12 12.5V20.5',
  archive: 'M3.5 5h17v4h-17zM5 9v10h14V9M10 13h4',
  copy: 'M9 9h9.5a1.5 1.5 0 0 1 1.5 1.5V20a1.5 1.5 0 0 1-1.5 1.5H9A1.5 1.5 0 0 1 7.5 20v-9.5A1.5 1.5 0 0 1 9 9ZM4.5 15V4a1.5 1.5 0 0 1 1.5-1.5h9',
  trash:
    'M4.5 6.5h15M9.5 6.5V4.5h5v2M6.5 6.5 7.5 20a1.5 1.5 0 0 0 1.5 1.4h6a1.5 1.5 0 0 0 1.5-1.4l1-13.5',
  send: 'M20.5 3.5 3.5 10l7 3 3 7Z',
  filter: 'M3.5 5.5h17l-6.5 7.5v6l-4 2v-8Z',
  // Dos flechas en sentidos opuestos: el orden de lectura, que en el móvil se queda
  // sin rótulo y tiene que decirse solo.
  sort: 'M7.5 4.5v15M7.5 4.5 4 8M7.5 4.5 11 8M16.5 19.5v-15M16.5 19.5 13 16M16.5 19.5 20 16',
  // Bandeja con la flecha ENTRANDO y SALIENDO: el mismo dibujo con la punta al
  // revés, para que exportar e importar se lean como la ida y la vuelta de lo
  // mismo y no como dos acciones sin relación.
  undo: 'M8.5 8H14a4.5 4.5 0 0 1 0 9H9M8.5 8l3.5-3.5M8.5 8 12 11.5',
  redo: 'M15.5 8H10a4.5 4.5 0 0 0 0 9h5M15.5 8 12 4.5M15.5 8 12 11.5',
  quote:
    'M7 16.5c-1.5 0-2.5-1-2.5-2.5v-3A3.5 3.5 0 0 1 8 7.5M17 16.5c-1.5 0-2.5-1-2.5-2.5v-3A3.5 3.5 0 0 1 18 7.5M4.5 11h3.5v5.5M14.5 11H18v5.5',
  // Asistente: la chispa de cuatro puntas, la misma que usa el redactor de Sytue
  // Mail. Dos tamaños para que no se lea como un asterisco.
  sparkles:
    'M12 3.5 13.7 8.3 18.5 10 13.7 11.7 12 16.5 10.3 11.7 5.5 10 10.3 8.3ZM18 15.5l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8Z',
  eraser:
    'M9 19.5h10.5M4.6 15.4l8.8-8.8a1.5 1.5 0 0 1 2.1 0l3.9 3.9a1.5 1.5 0 0 1 0 2.1L13.5 18.5H9.4l-4.8-3.1Z',
  indent: 'M4.5 5.5h15M10 10h9.5M10 14h9.5M4.5 18.5h15M4.5 9.5 7.5 12l-3 2.5',
  outdent: 'M4.5 5.5h15M10 10h9.5M10 14h9.5M4.5 18.5h15M7.5 9.5 4.5 12l3 2.5',
  alignLeft: 'M4.5 6h15M4.5 10h9M4.5 14h15M4.5 18h9',
  alignCenter: 'M4.5 6h15M7.5 10h9M4.5 14h15M7.5 18h9',
  alignRight: 'M4.5 6h15M10.5 10h9M4.5 14h15M10.5 18h9',
  heading: 'M6 5v14M18 5v14M6 12h12',
  layoutTree: 'M4 4.5h6v4H4zM12 11h8v4h-8zM12 17.5h8v4h-8zM7 8.5v11h5M7 13h5',
  table: 'M3 5h18v14H3zM3 10h18M3 15h18M9 5v14M15 5v14',
  download: 'M12 3.5v11M8 10.5l4 4 4-4M4.5 16.5v3a1 1 0 0 0 1 1h13a1 1 0 0 0 1-1v-3',
  upload: 'M12 14.5v-11M8 7.5l4-4 4 4M4.5 16.5v3a1 1 0 0 0 1 1h13a1 1 0 0 0 1-1v-3',
} as const;

export type GlyphName = keyof typeof PATHS;

export interface GlyphProps {
  name: GlyphName;
  size?: number;
  class?: string;
  /** Etiqueta accesible. Sin ella el glifo es decorativo (aria-hidden), que es
   *  lo normal: la etiqueta la lleva el botón que lo contiene. */
  'aria-label'?: string;
}

export function Glyph({ name, size = 18, class: cls, ...rest }: GlyphProps): JSX.Element {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width={1.7}
      stroke-linecap="round"
      stroke-linejoin="round"
      class={cls ? `${cls} shrink-0` : 'shrink-0'}
      role={rest['aria-label'] ? 'img' : undefined}
      aria-hidden={rest['aria-label'] ? undefined : 'true'}
      {...rest}
    >
      <path d={PATHS[name]} />
    </svg>
  );
}
