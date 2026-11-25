/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Glifos propios de Sytue Notepads: los que el catálogo de `@sytue/ui` todavía no
 * tiene (tipos de nota, disposiciones, formato de texto). Se pintan con `Glifo`,
 * el mismo envoltorio que `Icon`: grid 24 y trazo de 1,5 px en pantalla a cualquier
 * tamaño, para que no se noten de otra familia.
 *
 * No van a `@sytue/ui` porque hoy solo los usa esta app; el día que otra los
 * pida, ese es el momento de promoverlos.
 *
 * UN DIBUJO POR CONCEPTO. Lo que el común ya dibuja —papelera, enlace, imagen,
 * deshacer, la negrita…— NO se vuelve a dibujar aquí: esos nombres se piden igual
 * (`<Glyph name="trash" />`) pero los pinta `Icon`, para que la misma acción se vea
 * igual en Notepads que en Mail o en Drive. El guardián `glifos-al-comun.test.ts`
 * de @sytue/ui impide que vuelvan a esta tabla.
 */
import type { JSX } from 'preact';
import { Glifo, Icon, type IconName } from '@sytue/ui';

const PATHS = {
  notebook: 'M6 4h11a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2ZM8.5 4v16',
  note: 'M7 4h8.5a1.5 1.5 0 0 1 1.5 1.5v6.5L12.5 17H7a1.5 1.5 0 0 1-1.5-1.5v-10A1.5 1.5 0 0 1 7 4ZM17 12h-3a1.5 1.5 0 0 0-1.5 1.5V17',
  todo: 'M4.5 6.5h3v3h-3zM4.5 14.5h3v3h-3zM11 8h9M11 16h9',
  file: 'M13.5 3.5H7A1.5 1.5 0 0 0 5.5 5v14A1.5 1.5 0 0 0 7 20.5h10a1.5 1.5 0 0 0 1.5-1.5V8.5ZM13.5 3.5V8.5h5',
  palette:
    'M12 3.5a8.5 8.5 0 0 0 0 17c1.4 0 2-.9 2-1.8 0-.5-.2-.9-.5-1.2-.3-.3-.5-.7-.5-1.2 0-.9.7-1.6 1.6-1.6h1.4A4.5 4.5 0 0 0 20.5 10c0-3.6-3.8-6.5-8.5-6.5ZM7.5 12h.01M9.5 8.5h.01M14 7.5h.01',
  layoutFree: 'M4 4.5h6v5H4zM13 6.5h7v4h-7zM6 13h7v6.5H6zM16 14.5h4v5h-4z',
  layoutColumns: 'M4.5 4.5h5v15h-5zM14.5 4.5h5v15h-5z',
  layoutList: 'M4.5 5.5h15v4h-15zM4.5 14.5h15v4h-15z',
  drag: 'M9 6h.01M15 6h.01M9 12h.01M15 12h.01M9 18h.01M15 18h.01',
  bulletList: 'M4.5 6.5h.01M4.5 12h.01M4.5 17.5h.01M9 6.5h10.5M9 12h10.5M9 17.5h10.5',
  numberedList: 'M4 5h1.5v4M4 9h3M4 13.5h3v2H4v3h3M10 7h10M10 14.5h10',
  unlock:
    'M8 10V7a4 4 0 0 1 7.5-2M5 11a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-8Z',
  pin: 'M9.5 3.5h5l-.8 5.5 3.3 3.5H7l3.3-3.5ZM12 12.5V20.5',
  archive: 'M3.5 5h17v4h-17zM5 9v10h14V9M10 13h4',
  // Dos hojas iguales, la de detrás asomando lo mismo por arriba que por la izquierda y
  // con sus dos esquinas: antes asomaba 3 de lado y 6,5 de alto, sin esquinas, y no se
  // leía como «copiar». Centrado en la caja de 24, como tijeras y pegar.
  copy: 'M10 8.5h8a1.5 1.5 0 0 1 1.5 1.5v8a1.5 1.5 0 0 1-1.5 1.5h-8A1.5 1.5 0 0 1 8.5 18v-8A1.5 1.5 0 0 1 10 8.5ZM8.5 15.5H6A1.5 1.5 0 0 1 4.5 14V6A1.5 1.5 0 0 1 6 4.5h8A1.5 1.5 0 0 1 15.5 6v2.5',
  send: 'M20.5 3.5 3.5 10l7 3 3 7Z',
  filter: 'M3.5 5.5h17l-6.5 7.5v6l-4 2v-8Z',
  // Dos flechas en sentidos opuestos: el orden de lectura, que en el móvil se queda
  // sin rótulo y tiene que decirse solo.
  sort: 'M7.5 4.5v15M7.5 4.5 4 8M7.5 4.5 11 8M16.5 19.5v-15M16.5 19.5 13 16M16.5 19.5 20 16',
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
  // Los bloques del editor (el menú «+» y «/»): un panel con su «i», la sección que se
  // pliega, la decisión que se bifurca, el símbolo (omega), el emoji, la arroba.
  panel:
    'M5 5.5h14A1.5 1.5 0 0 1 20.5 7v10a1.5 1.5 0 0 1-1.5 1.5H5A1.5 1.5 0 0 1 3.5 17V7A1.5 1.5 0 0 1 5 5.5ZM8 11v4.5M8 8.5h.01',
  expandir: 'M5 8l3 3 3-3M13.5 9.5h6M5 14.5h14.5M5 18.5h10',
  decision: 'M12 20.5V15L6.5 9.5v-5M12 15l5.5-5.5v-5M4.5 6.5l2-2 2 2M15.5 6.5l2-2 2 2',
  omega: 'M5 19.5h4.5v-1.8a6.5 6.5 0 1 1 5 0v1.8H19',
  carita:
    'M12 3.5a8.5 8.5 0 1 1 0 17 8.5 8.5 0 0 1 0-17ZM8.5 14a4 4 0 0 0 7 0M9 9.5h.01M15 9.5h.01',
  arroba:
    'M15.5 12a3.5 3.5 0 1 1-7 0 3.5 3.5 0 0 1 7 0v1.5a2.5 2.5 0 0 0 5 0V12a8.5 8.5 0 1 0-3.3 6.7',
  subindice: 'M4.5 6.5l7 9M11.5 6.5l-7 9M15.5 15a1.75 1.75 0 1 1 3.5 0c0 1.2-3.5 2.5-3.5 4.5h3.5',
  superindice: 'M4.5 8.5l7 9M11.5 8.5l-7 9M15.5 5a1.75 1.75 0 1 1 3.5 0c0 1.2-3.5 2.5-3.5 4.5h3.5',
  ajuste: 'M4.5 6.5h15M4.5 12h12a2.75 2.75 0 0 1 0 5.5h-4M14.5 15.5l-2 2 2 2M4.5 17.5h5',
  // «Coincidir mayúsculas» de la búsqueda: una A grande y una a pequeña.
  mayusculas:
    'M3.5 18 8 6l4.5 12M5 14h6M19.5 12.5V18M19.5 15.25a2.5 2.75 0 1 1-5 0 2.5 2.75 0 0 1 5 0Z',
  // «Más formato» de la burbuja de la selección: la «A» subrayada, la de Fyne. Un botón
  // como los demás, sin flecha: así la burbuja tiene el mismo hueco entre todos.
  formato: 'M6.5 16 11 4.5h2L17.5 16M8.2 12h7.6M4.5 20h15',
  // Cortar y pegar de la burbuja de la selección (copiar es `copy`, más arriba).
  tijeras:
    'M8.5 7a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0ZM8.5 17a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0ZM8 8.5l11.5 9M8 15.5l11.5-9',
  pegar:
    'M9 4.5h6v3H9ZM9 6H6.5A1.5 1.5 0 0 0 5 7.5v11A1.5 1.5 0 0 0 6.5 20h11a1.5 1.5 0 0 0 1.5-1.5v-11A1.5 1.5 0 0 0 17.5 6H15',
  // La barra de tabla: dónde va la fila o la columna nueva (la «+» fuera de la rejilla),
  // cuál se quita (la «×» dentro), y la cabecera rellena.
  filaArriba: 'M4.5 11.5h15v8h-15ZM4.5 15.5h15M12 3.5v5M9.5 6h5',
  filaAbajo: 'M4.5 4.5h15v8h-15ZM4.5 8.5h15M12 15.5v5M9.5 18h5',
  columnaAntes: 'M11.5 4.5h8v15h-8ZM15.5 4.5v15M3.5 12h5M6 9.5v5',
  columnaDespues: 'M4.5 4.5h8v15h-8ZM8.5 4.5v15M15.5 12h5M18 9.5v5',
  filaQuitar: 'M4.5 8.5h15v7h-15ZM10 10.5l4 3M14 10.5l-4 3',
  columnaQuitar: 'M8.5 4.5h7v15h-7ZM10.5 10l3 4M13.5 10l-3 4',
  cabeceraFila: 'M4.5 4.5h15v5h-15ZM4.5 9.5v10h15v-10M4.5 14.5h15M12 9.5v10',
  cabeceraColumna: 'M4.5 4.5h5v15h-5ZM9.5 4.5h10v15h-10M9.5 12h10M14.5 4.5v15',
  // Combinar con la de la derecha o con la de abajo, y separar: las de Fyne (combinar.go).
  combinar: 'M4.5 5.5h15v13h-15ZM8 12h8M13.5 9.5 16 12l-2.5 2.5M10.5 9.5 8 12l2.5 2.5',
  combinarAbajo: 'M5.5 4.5v15h13v-15ZM12 8v8M9.5 13.5 12 16l2.5-2.5M9.5 10.5 12 8l2.5 2.5',
  separar: 'M4.5 5.5h15v13h-15ZM12 5.5v13M9.5 10 7.5 12l2 2M14.5 10l2 2-2 2',
} as const;

/** Los nombres de Notepads que dibuja el catálogo común. Una nota guarda el suyo en
 *  `data.icon`, así que el NOMBRE se conserva aunque el dibujo ya no sea de aquí. */
const DEL_COMUN = [
  'link',
  'image',
  'bold',
  'italic',
  'underline',
  'strikethrough',
  'trash',
  'undo',
  'redo',
  'quote',
  'table',
  'download',
  'upload',
] as const satisfies readonly IconName[];

type Propio = keyof typeof PATHS;
export type GlyphName = Propio | (typeof DEL_COMUN)[number];

function esPropio(name: GlyphName): name is Propio {
  return Object.hasOwn(PATHS, name);
}

export interface GlyphProps {
  name: GlyphName;
  size?: number;
  class?: string;
  /** Etiqueta accesible. Sin ella el glifo es decorativo (aria-hidden), que es
   *  lo normal: la etiqueta la lleva el botón que lo contiene. */
  'aria-label'?: string;
}

export function Glyph({ name, size = 18, class: cls, ...rest }: GlyphProps): JSX.Element {
  // El trazo y el relleno los decide `Glifo`, como en el resto de la suite: aquí solo
  // vive el dibujo. Sobre el cromo «papel» de Notepads el relleno de acento no se pinta.
  const comunes = {
    size,
    ...(cls ? { class: cls } : {}),
    ...(rest['aria-label'] ? { label: rest['aria-label'] } : {}),
  };
  return esPropio(name) ? (
    <Glifo d={PATHS[name]} {...comunes} />
  ) : (
    <Icon name={name} {...comunes} />
  );
}
