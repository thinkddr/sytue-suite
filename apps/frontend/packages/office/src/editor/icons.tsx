/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Iconografía local del editor Sytue Docs (BLUEPRINT.md §9). Trazados PROPIOS
 * dibujados de cero (clean-room: nada tomado de OnlyOffice ni de terceros),
 * y pintados con el mismo envoltorio que `Icon` de @sytue/ui (`Glifo`): grid 24,
 * trazo de 1,5 px en pantalla a cualquier talla, relleno de acento en las formas
 * cerradas, decorativo salvo `label`.
 */
import type { JSX } from 'preact';
import { Glifo } from '@sytue/ui';

/**
 * Trazados por nombre. Varios subtrazados se unen en un único `d` con comandos
 * `M` intermedios (mismo estilo que @sytue/ui). Los 46 nombres de §9 primero;
 * al final, los extra pedidos por las pestañas (salto, símbolo, márgenes…).
 */
const PATHS = {
  /* --- Historia de edición --- */
  undo: 'M9 14 5 10l4-4 M5 10h9a4.5 4.5 0 0 1 0 9h-2.5',
  redo: 'M15 14 19 10l-4-4 M19 10h-9a4.5 4.5 0 0 0 0 9h2.5',
  /* --- Formato de carácter --- */
  bold: 'M7 12h6.5a3.5 3.5 0 0 1 0 7H7V5h6a3.5 3.5 0 0 1 0 7',
  italic: 'M11.5 5h7 M5.5 19h7 M14.5 5l-5 14',
  underline: 'M6.5 4.5v6a5.5 5.5 0 0 0 11 0v-6 M5.5 19.5h13',
  strike:
    'M16.7 6.6c-.8-1.6-2.6-2.6-4.7-2.6-2.8 0-4.8 1.4-4.8 3.5 0 1.3.8 2.3 2.4 3 M7.3 17.4c.8 1.6 2.6 2.6 4.7 2.6 2.8 0 4.8-1.4 4.8-3.5 0-1.1-.6-2-1.8-2.7 M4 12h16',
  textColor: 'm5.5 17 5.2-12h2.6l5.2 12 M8.2 11.5h7.6',
  highlight: 'M4 20h4L18.6 9.4a2 2 0 0 0 0-2.8l-1.2-1.2a2 2 0 0 0-2.8 0L4 16Z M13.1 7.9l3 3',
  clearFormat: 'M5 5h12 M11 5v14 M15.5 15.5 20 20 M20 15.5l-4.5 4.5',
  /* --- Párrafo --- */
  alignLeft: 'M4 6h16 M4 10h10 M4 14h16 M4 18h10',
  alignCenter: 'M4 6h16 M7 10h10 M4 14h16 M7 18h10',
  alignRight: 'M4 6h16 M10 10h10 M4 14h16 M10 18h10',
  alignJustify: 'M4 6h16 M4 10h16 M4 14h16 M4 18h16',
  listBullet: 'M4 6h.01 M4 12h.01 M4 18h.01 M9 6h11 M9 12h11 M9 18h11',
  listNumber:
    'M4 5.5 5.3 4.5v4.5 M3.8 13.3c0-.9.7-1.5 1.6-1.5 1 0 1.7.6 1.7 1.5 0 1.6-3.2 2.1-3.2 4.2h3.4 M10 6h10 M10 12h10 M10 18h10',
  indent: 'M4 5.5h16 M12 9.5h8 M12 13.5h8 M4 17.5h16 M4 9.5l3 2-3 2',
  outdent: 'M4 5.5h16 M12 9.5h8 M12 13.5h8 M4 17.5h16 M7 9.5l-3 2 3 2',
  lineHeight: 'M11 6h9 M11 12h9 M11 18h9 M5 5.5v13 M2.8 7.7 5 5.5l2.2 2.2 M2.8 16.3 5 18.5l2.2-2.2',
  /* --- Insertar --- */
  table:
    'M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1ZM3 9.5h18M9.5 5.5V18.5M14.5 5.5V18.5',
  image:
    'M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M4.5 16.5 9 12l4 4 2.5-2.5 4 4 M9.25 8.5a.75.75 0 1 0 1.5 0 .75.75 0 1 0-1.5 0Z',
  // Recortar imagen: dos escuadras que se cruzan (marca de recorte).
  crop: 'M8 2v14h14 M2 8h14v14',
  link: 'M9 15l6-6 M10.5 5.5 12 4a4.24 4.24 0 0 1 6 6l-1.5 1.5 M13.5 18.5 12 20a4.24 4.24 0 0 1-6-6l1.5-1.5',
  hr: 'M4 12h16 M9 6.5h6 M9 17.5h6',
  toc: 'M4 6h9 M4 12h9 M4 18h9 M18.5 6h1.5 M18.5 12h1.5 M18.5 18h1.5',
  /* --- Colaboración --- */
  comment: 'M6 5h12a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-7l-4.5 4v-4H6a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2Z',
  // El signo + crea un comentario; las líneas abren los comentarios existentes.
  commentAdd:
    'M6 5h12a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-7l-4.5 4v-4H6a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2Z M12 8v5 M9.5 10.5h5',
  commentList:
    'M6 5h12a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-7l-4.5 4v-4H6a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2Z M8 9h8 M8 12h6',
  history:
    'M4.5 12a7.5 7.5 0 1 0 7.5-7.5c-2.2 0-4.2.9-5.6 2.4L4.5 8.8 M4.5 4.5v4.3h4.3 M12 8.5v4l2.8 1.7',
  share:
    'M8.5 4.5H6a2 2 0 0 0-2 2V18a2 2 0 0 0 2 2h11.5a2 2 0 0 0 2-2v-2.5 M13.5 4H20v6.5 M20 4l-8.5 8.5',
  download: 'M12 4v10.5 M7 10l5 5 5-5 M4.5 19.5h15',
  /* --- Datos externos (conector Power Query) --- */
  // Cilindro de base de datos (dos bandas): origen de datos externo.
  database:
    'M12 3c-4.4 0-8 1.3-8 3s3.6 3 8 3 8-1.3 8-3-3.6-3-8-3ZM4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3',
  // Dos flechas en arco (refrescar/actualizar).
  refresh:
    'M20 11.5A8 8 0 0 0 6.3 6.3L4 8.5 M4 4v4.5h4.5 M4 12.5a8 8 0 0 0 13.7 5.2L20 15.5 M20 20v-4.5h-4.5',
  // Refrescar TODO: los arcos con dos barritas dentro (varias tablas).
  refreshAll:
    'M20 11.5A8 8 0 0 0 6.3 6.3L4 8.5 M4 4v4.5h4.5 M4 12.5a8 8 0 0 0 13.7 5.2L20 15.5 M20 20v-4.5h-4.5 M9.5 11h5 M9.5 13.5h5',
  /* --- Franja izquierda y vista --- */
  search: 'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14ZM20 20l-3.8-3.8',
  outline: 'M4.5 5.5h.01 M8 5.5h12 M7.5 12h.01 M11 12h9 M4.5 18.5h.01 M8 18.5h12',
  ruler: 'M4 8.5h16v7H4Z M8 8.5v3 M12 8.5v4 M16 8.5v3',
  panelRight: 'M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M15 4v16',
  /* --- Genéricos --- */
  plus: 'M12 4.5v15M4.5 12h15',
  // minus: Icon.tsx no tiene glifo propio; alineado al trazo horizontal de su `plus`
  // para que +/− casen como par tras converger `plus`.
  minus: 'M4.5 12h15',
  close: 'M18.5 5.5 5.5 18.5M5.5 5.5l13 13',
  check: 'M20 6.5 9.5 17.5 4 12',
  chevronDown: 'M5 8.5l7 7 7-7',
  // Glifo propio para «arriba»: girar chevronDown con `rotate-180` sobre un <span>
  // NO funciona (transform se ignora en cajas inline no reemplazadas) y dejaba la
  // flecha de «subir» apuntando abajo. Mismo trazado que el Icon del DS.
  chevronUp: 'M5 15.5l7-7 7 7',
  chevronLeft: 'M15.5 5l-7 7 7 7',
  trash:
    'M4.5 7h15 M9.5 7V5a1.3 1.3 0 0 1 1.3-1.3h2.4A1.3 1.3 0 0 1 14.5 5v2 M6.5 7l.7 12.1a1.6 1.6 0 0 0 1.6 1.5h6.4a1.6 1.6 0 0 0 1.6-1.5L17.5 7 M10 11v6 M14 11v6',
  /* --- Tabla contextual --- */
  rowAbove: 'M12 2.5v7 M9 5.5l3-3 3 3 M4 13h16v7.5H4Z M4 16.75h16',
  rowBelow: 'M4 3.5h16V11H4Z M4 7.25h16 M12 14.5v7 M9 18.5l3 3 3-3',
  colLeft: 'M13.5 4h7v16h-7Z M17 4v16 M10.5 12h-7 M6.5 9l-3 3 3 3',
  colRight: 'M3.5 4h7v16h-7Z M7 4v16 M13.5 12h7 M17.5 9l3 3-3 3',
  /* --- Disposición --- */
  pagePortrait: 'M7 3.5h10v17H7Z',
  pageLandscape: 'M3.5 7h17v10h-17Z',
  // Borde de página: folio con un recuadro interior (marco alrededor del contenido).
  pageBorder:
    'M5 3.5h14a1.5 1.5 0 0 1 1.5 1.5v14a1.5 1.5 0 0 1-1.5 1.5H5a1.5 1.5 0 0 1-1.5-1.5V5A1.5 1.5 0 0 1 5 3.5Z M8 7h8 M8 17h8 M8 7v10 M16 7v10',
  // Marca de agua: folio con una «W» tenue en diagonal (texto de fondo).
  watermark:
    'M5.5 4.5h9L18.5 8.5v11a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1v-14a1 1 0 0 1 1-1Z M14.5 4.5V8.5h4 M8 17l1.4-5 1.6 3 1.6-3 1.4 5',
  /* --- Presentaciones --- */
  slideAdd: 'M3.5 6h17v12h-17Z M12 9v6 M9 12h6',
  textBox: 'M4.5 6h15v12h-15Z M9 9.5h6 M12 9.5V15',
  // Patrón de diapositiva: marco con banda de título y un marcador de contenido
  // (la plantilla que heredan las diapositivas).
  slideMaster: 'M4.5 5.5h15v13h-15Z M4.5 9h15 M7 11.5h6v4.5H7Z',
  /* Formas de diapositiva (fase Slides-Elementos): los nombres coinciden con
   * SlideShape (types.ts) para mapear forma → icono sin tabla intermedia. */
  rect: 'M4.5 7h15v10h-15Z',
  ellipse: 'M3.5 12a8.5 6 0 1 0 17 0 8.5 6 0 1 0-17 0Z',
  line: 'M5 19 19 5',
  arrow: 'M5 19 19 5 M12 5h7v7',
  /* Formas ampliadas (fase Slides-Conectores + más formas): los nombres siguen
   * coincidiendo con SlideShape para el mapeo directo del ribbon. */
  triangle: 'M12 5 20 19 4 19Z',
  diamond: 'M12 4 20 12 12 20 4 12Z',
  pentagon: 'M12 3.5 20.1 9.4 17 18.9 7 18.9 3.9 9.4Z',
  hexagon: 'M20.5 12 16.3 18 7.7 18 3.5 12 7.7 6 16.3 6Z',
  star: 'M12 3.5 14.1 9.1 20.1 9.4 15.4 13.1 17 18.9 12 15.6 7 18.9 8.6 13.1 3.9 9.4 9.9 9.1Z',
  cylinder: 'M5 7a7 3 0 1 0 14 0 7 3 0 1 0-14 0 M5 7v10a7 3 0 0 0 14 0V7',
  callout:
    'M5 4.5h14a1.5 1.5 0 0 1 1.5 1.5v8a1.5 1.5 0 0 1-1.5 1.5h-7l-4 3.5V15.5H5A1.5 1.5 0 0 1 3.5 14V6A1.5 1.5 0 0 1 5 4.5Z',
  /* Dibujo (canvas-1): puntero de selección y lápiz. */
  eye: 'M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6Z M12 9.3a2.7 2.7 0 1 0 0 5.4 2.7 2.7 0 0 0 0-5.4Z',
  eraser:
    'M8 19h11 M4.5 14.5 13 6a2 2 0 0 1 2.8 0l2.2 2.2a2 2 0 0 1 0 2.8L10.5 19H8l-3.5-3.5a1.4 1.4 0 0 1 0-1Z',
  pointer: 'M5.5 3.5 18.5 10l-5.6 1.9L10.7 18Z',
  draw: 'M4 20l1-4.2L16.6 4.2a2 2 0 0 1 2.8 0l.4.4a2 2 0 0 1 0 2.8L8.2 19Z M14.5 6.3l3.2 3.2',
  /* Conectores (fase Slides-Conectores): línea/codo/curva con tiradores (nubs)
   * en los extremos, para leerse como «une dos puntos». */
  connStraight:
    'M8.4 17 15.6 6.9 M5 19a2 2 0 1 0 4 0 2 2 0 1 0-4 0Z M15 5a2 2 0 1 0 4 0 2 2 0 1 0-4 0Z',
  connElbow:
    'M7 17.2V12h9.8V6.8 M5 19a2 2 0 1 0 4 0 2 2 0 1 0-4 0Z M15 5a2 2 0 1 0 4 0 2 2 0 1 0-4 0Z',
  connCurved:
    'M7 17.2C7 10 17 14 17 6.8 M5 19a2 2 0 1 0 4 0 2 2 0 1 0-4 0Z M15 5a2 2 0 1 0 4 0 2 2 0 1 0-4 0Z',
  /* SmartArt (fase Slides-SmartArt): un glifo por disposición. */
  // Lista: tres tarjetas apiladas.
  smartList: 'M4 5h16v3.2H4Z M4 10.4h16v3.2H4Z M4 15.8h16v3.2H4Z',
  // Proceso: dos cajas unidas por una flecha.
  smartProcess: 'M3.5 9h5.5v6H3.5Z M15 9h5.5v6H15Z M10 12h3.8 M12.3 10.3 14.1 12l-1.8 1.8',
  // Ciclo: flecha circular.
  smartCycle: 'M18.5 12a6.5 6.5 0 1 1-1.9-4.6 M18.6 5v3.4h-3.4',
  // Jerarquía: organigrama de una raíz con dos hijos.
  smartHierarchy:
    'M9.5 4h5v3.4h-5Z M12 7.4v4.1 M6.8 11.5h10.2 M6.8 11.5v1.1 M17 11.5v1.1 M3.8 12.6h6v3.4h-6Z M14 12.6h6v3.4h-6Z',
  /* --- Extra sobre §9, pedidos por la tarea (salto, símbolo, márgenes, tamaño,
   *     nota, control de cambios, zoom, pantalla completa, ajustes, combinar).
   *     Unión aditiva: no rompe a ningún consumidor del contrato original. --- */
  pageBreak:
    'M5 8.5V5a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v3.5 M5 15.5V19a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-3.5 M4 12h2.8 M10.6 12h2.8 M17.2 12h2.8',
  symbol: 'M6.5 19.5h4v-2.6a5.8 5.8 0 1 1 3 0v2.6h4',
  margins: 'M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M8 8h8v8H8Z',
  pageSize: 'M4.5 4.5h15v15h-15Z M14.5 9.5l-5 5 M14.5 12.5v-3h-3 M9.5 11.5v3h3',
  footnote: 'M6.5 3.5H14l3.5 3.5v13.5h-11Z M14 3.5V7h3.5 M9.5 13h5 M9.5 16h3',
  // Nota al final: folio dañado con una flecha que baja hasta una línea al pie
  // (el contenido «fluye» hasta la nota del final del documento); se distingue
  // de la nota al pie (líneas centrales) por la flecha descendente.
  endnote:
    'M6.5 3.5H13l3.5 3.5v13.5h-11Z M13 3.5V7h3.5 M11.5 9.5v4 M9.7 11.7 11.5 13.5 13.3 11.7 M9 16.5h5',
  trackChanges:
    'M5 19.5l.8-3.3L16.1 5.9a1.8 1.8 0 0 1 2.5 2.5L8.3 18.7l-3.3.8Z M14.3 7.7l2.5 2.5 M4 5.5h6 M4 9h3.5',
  zoom: 'M10.5 17a6.5 6.5 0 1 1 0-13 6.5 6.5 0 0 1 0 13Z M15.4 15.4 20 20 M10.5 8v5 M8 10.5h5',
  fullscreen:
    'M4 9V5a1 1 0 0 1 1-1h4 M15 4h4a1 1 0 0 1 1 1v4 M20 15v4a1 1 0 0 1-1 1h-4 M9 20H5a1 1 0 0 1-1-1v-4',
  // Ajustes: engranaje (convención universal), unificado con el `settings` de
  // @sytue/ui Icon — reemplaza los 3 deslizadores para no chocar con el cromo.
  settings:
    'M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6Z M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z',
  // Combinar celdas: celda + dos flechas que convergen en la costura central.
  merge:
    'M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M7.5 12h3.5 M9.5 10.5 11 12l-1.5 1.5 M16.5 12h-3.5 M14.5 10.5 13 12l1.5 1.5',
  // Aumentar/disminuir decimales: «,00» con +/− arriba a la izquierda.
  decimalMore:
    'M6 3.5v5 M3.5 6h5 M5.6 16.6l-.8 2.2 M8.5 14.5a2.1 3.3 0 1 0 4.2 0 2.1 3.3 0 1 0-4.2 0Z M14.7 14.5a2.1 3.3 0 1 0 4.2 0 2.1 3.3 0 1 0-4.2 0Z',
  decimalLess:
    'M3.5 6h5 M5.6 16.6l-.8 2.2 M8.5 14.5a2.1 3.3 0 1 0 4.2 0 2.1 3.3 0 1 0-4.2 0Z M14.7 14.5a2.1 3.3 0 1 0 4.2 0 2.1 3.3 0 1 0-4.2 0Z',
  // Formato personalizado (máscara): almohadilla «#».
  numFormat: 'M4 9h16 M4 15h16 M10 4 8 20 M16 4 14 20',
  // Formato condicional: celda con un «menor que» (regla) dentro.
  condFormat:
    'M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M13.8 8.5 9.8 12l4 3.5',
  // Ordenar: barras crecientes/decrecientes + flecha de dirección.
  sortAsc: 'M4 6h4 M4 12h8 M4 18h12 M18.5 19V5 M15.5 8l3-3 3 3',
  sortDesc: 'M4 6h12 M4 12h8 M4 18h4 M18.5 5v14 M15.5 16l3 3 3-3',
  // Filtro: embudo.
  filter: 'M4 5h16l-6 6.5V19l-4-2v-5.5Z',
  // Congelar paneles: rejilla 2×2 con el panel superior-izquierdo rayado.
  freeze:
    'M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M4 9.5h16 M9.5 4v16 M5.5 7.5l2.5-2.5 M6 9.4l3.4-3.4',
  /* --- Fase Visual de Sheets --- */
  // Gráfico: ejes en L con tres columnas apoyadas en la base.
  chart: 'M4 4v16h16M8 20v-4M12 20v-8M16 20v-11',
  // Duplicar: dos hojas superpuestas (la trasera asoma por la esquina).
  copy: 'M9.5 8h9A1.5 1.5 0 0 1 20 9.5v9a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 8 18.5v-9A1.5 1.5 0 0 1 9.5 8Z M5.5 16H5a1.5 1.5 0 0 1-1.5-1.5v-9A1.5 1.5 0 0 1 5 4h9a1.5 1.5 0 0 1 1.5 1.5V6',
  /* --- Fase Slides-Presentación --- */
  // Presentar: marco de diapositiva 16:9 con triángulo de reproducción dentro.
  present: 'M3.5 6h17v12h-17Z M10.3 9.5v5l4.4-2.5Z',
  // Presentar con Cast: pantalla con las ondas de la emisión saliendo de su esquina inferior izquierda.
  cast: 'M3.5 6h17v12h-5 M3.5 18h.01 M3.5 14.5a3.5 3.5 0 0 1 3.5 3.5 M3.5 11a7 7 0 0 1 7 7',
  // Abrir en la aplicación instalada: monitor con su peana (no la pantalla de «presentar»).
  openInApp:
    'M4 4.5h16a1 1 0 0 1 1 1V15a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V5.5a1 1 0 0 1 1-1Z M9 20h6 M12 16v4',
  /* --- Fase Slides-Organizar (z-order/alinear/agrupar) --- */
  // Organizar objetos: dos cuadrados solapados (un objeto delante de otro).
  arrange: 'M4.5 4.5h9v9h-9Z M10.5 10.5h9v9h-9Z',
  /* --- Fase Slides-Multimedia (audio/vídeo incrustado) --- */
  // Vídeo: cuerpo de cámara con el visor/lente en trapecio a la derecha.
  video:
    'M3.5 7.5h10a1.5 1.5 0 0 1 1.5 1.5v6a1.5 1.5 0 0 1-1.5 1.5h-10A1.5 1.5 0 0 1 2 15V9a1.5 1.5 0 0 1 1.5-1.5Z M15 11l5-3v8l-5-3Z',
  // Audio: altavoz con dos ondas de sonido a la derecha.
  audio: 'M4.5 9.5H8l4-3.4v11.8l-4-3.4H4.5Z M15.5 9.5a4 4 0 0 1 0 5 M18 7.2a7.5 7.5 0 0 1 0 9.6',
  /* --- Documento: sub/superíndice, tabla (dividir), encabezado/pie, leyendas --- */
  // Superíndice: «X» con un «2» volado arriba a la derecha.
  superscript:
    'M4 8l6 9 M4 17l6-9 M14.7 6.4c0-1.1.9-1.9 2.1-1.9s2.2.7 2.2 1.9c0 1.9-4.3 2.3-4.3 4.6h4.3',
  // Subíndice: «X» con un «2» bajado abajo a la derecha.
  subscript:
    'M4 6l6 9 M4 15l6-9 M14.7 15.4c0-1.1.9-1.9 2.1-1.9s2.2.7 2.2 1.9c0 1.9-4.3 2.3-4.3 4.6h4.3',
  // Dividir celda: celda con costura central y dos flechas que DIVERGEN.
  splitCells:
    'M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M12 7.5v9 M10 12H6.8 M8.2 10.5 6.8 12l1.4 1.5 M14 12h3.2 M15.8 10.5 17.2 12l-1.4 1.5',
  // Encabezado: folio con banda superior resaltada.
  header:
    'M5.5 3.5h13a1 1 0 0 1 1 1v15a1 1 0 0 1-1 1h-13a1 1 0 0 1-1-1v-15a1 1 0 0 1 1-1Z M4.5 8.5h15 M8 6h8',
  // Pie de página: folio con banda inferior resaltada.
  footer:
    'M5.5 3.5h13a1 1 0 0 1 1 1v15a1 1 0 0 1-1 1h-13a1 1 0 0 1-1-1v-15a1 1 0 0 1 1-1Z M4.5 15.5h15 M8 18h8',
  // Número de página: folio con almohadilla «#» dentro.
  pageNumber:
    'M5.5 3.5h13a1 1 0 0 1 1 1v15a1 1 0 0 1-1 1h-13a1 1 0 0 1-1-1v-15a1 1 0 0 1 1-1Z M10.4 11l-.8 5 M13.6 11l-.8 5 M9 12.6h5 M8.6 14.4h5',
  // Leyenda: marco de figura con una línea de rótulo debajo.
  caption: 'M4.5 5h15v9.5h-15Z M4.5 18h10',
  // Referencia cruzada: corchetes [ ] con una flecha dentro.
  xref: 'M8.5 5.5H6a1.5 1.5 0 0 0-1.5 1.5v10A1.5 1.5 0 0 0 6 18.5h2.5 M15.5 5.5H18a1.5 1.5 0 0 1 1.5 1.5v10a1.5 1.5 0 0 1-1.5 1.5h-2.5 M8.5 12h6 M12 9.5l3 2.5-3 2.5',
  /* --- Citas y bibliografía --- */
  // Gestor de fuentes: libro encuadernado con una línea de título (catálogo).
  book: 'M6 4.5A1.5 1.5 0 0 1 7.5 3H18v14H7.5A1.5 1.5 0 0 0 6 18.5Z M6 18.5A1.5 1.5 0 0 0 7.5 20H18 M10 7.5h4',
  // Insertar cita: par de comillas dobles de apertura.
  citation:
    'M7.5 8a2 2 0 0 0-2 2 2 2 0 0 0 2 2c0 1.4-.7 2.4-2 3 M16 8a2 2 0 0 0-2 2 2 2 0 0 0 2 2c0 1.4-.7 2.4-2 3',
  // Bibliografía: libro abierto (lista de referencias).
  bibliography:
    'M12 6.5C10.5 5.5 8 5 5.5 5.5v11c2.5-.5 5 0 6.5 1 M12 6.5c1.5-1 4-1.5 6.5-1v11c-2.5-.5-5 0-6.5 1 M12 6.5v11',
  /* --- Índice alfabético --- */
  // Marcar entrada de índice: marcador (bookmark) con la muesca inferior.
  indexEntry: 'M7 4h10v16l-5-4-5 4Z',
  // Índice alfabético: una «A» junto a líneas de entradas (lista ordenada por letra).
  indexAlpha: 'M5 19 8 5.5 11 19 M6 14.5h4 M14 8h6 M14 13h6 M14 18h4',
  /* --- Campos de formulario rellenables --- */
  // Casilla de verificación: caja redondeada con una marca dentro.
  checkbox:
    'M6 4.5h12a1.5 1.5 0 0 1 1.5 1.5v12a1.5 1.5 0 0 1-1.5 1.5H6a1.5 1.5 0 0 1-1.5-1.5V6A1.5 1.5 0 0 1 6 4.5Z M8.5 12l2.5 2.5 4.5-5.5',
  // Campo desplegable: ficha con un chevron a la derecha.
  dropdown:
    'M5 6.5h14a1.5 1.5 0 0 1 1.5 1.5v8a1.5 1.5 0 0 1-1.5 1.5H5A1.5 1.5 0 0 1 3.5 16V8A1.5 1.5 0 0 1 5 6.5Z M14.5 11l1.75 1.75L18 11',
  // Rellenar formulario: página con líneas y una marca de validación.
  formFill:
    'M5 4.5h14a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1v-13a1 1 0 0 1 1-1Z M7.5 9h9 M7.5 12.5h4.5 M7.5 16h3 M13 15.5l1.5 1.5 3-3.5',
  /* --- Protección de hoja y celda --- */
  // Candado: cuerpo redondeado, arco superior y ojo de la cerradura.
  lock: 'M8 10V7a4 4 0 0 1 8 0v3M5 11a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-8ZM12 14.5v2.5',
  // Escudo con marca de verificación: proteger la hoja.
  shield: 'M12 3.5 19 6v5c0 4.4-3 8-7 9.5C8 18 5 14.4 5 10V6Z M9.2 11.8l2 2 3.6-3.8',
  /* --- Análisis Y-si (Buscar objetivo) --- */
  // Diana: dos círculos concéntricos y un punto central (buscar el objetivo).
  target:
    'M12 20a8 8 0 1 1 0-16 8 8 0 0 1 0 16Z M12 16a4 4 0 1 1 0-8 4 4 0 0 1 0 8Z M12 12.6a.6 .6 0 1 1 0-1.2 .6 .6 0 0 1 0 1.2Z',
  // Escenarios: tres capas apiladas (variantes alternativas del mismo modelo).
  scenarios: 'M12 3 3 7.5 12 12 21 7.5Z M3 12 12 16.5 21 12 M3 16.5 12 21 21 16.5',
  // Tabla de datos: rejilla con franja de cabecera de fila y de columna (matriz de sensibilidad).
  datatable:
    'M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M4 9h16 M9 4v16',
  // Solver: tres controles deslizantes con sus perillas (ajustar variables hacia el óptimo).
  solver:
    'M4 7h16 M4 12h16 M4 17h16 M17 7a2 2 0 1 1-4 0 2 2 0 1 1 4 0Z M11 12a2 2 0 1 1-4 0 2 2 0 1 1 4 0Z M18 17a2 2 0 1 1-4 0 2 2 0 1 1 4 0Z',
  /* --- Esquema (agrupar/outline) y subtotales --- */
  // Agrupar filas: corchete «[» a la izquierda que abraza tres filas.
  groupRows: 'M8 4H5v16h3 M12 8h8 M12 12h8 M12 16h8',
  // Desagrupar filas: corchete roto (solo las esquinas) junto a las filas.
  ungroupRows: 'M8 4H5v3 M5 17v3h3 M12 8h8 M12 12h8 M12 16h8',
  // Agrupar columnas: corchete superior que abraza tres columnas.
  groupCols: 'M4 8V5h16v3 M8 12v8 M12 12v8 M16 12v8',
  // Desagrupar columnas: corchete superior roto (solo las esquinas) sobre las columnas.
  ungroupCols: 'M4 8V5h3 M17 5h3v3 M8 12v8 M12 12v8 M16 12v8',
  // Subtotal: sigma «Σ».
  subtotal: 'M17 5H7l5 7-5 7h10',
  /* --- Segmentación de datos (slicer) --- */
  // Slicer: tarjeta con banda de cabecera y dos filas de valores (lista filtrable).
  slicer: 'M5 4.5h14v15H5Z M5 8.5h14 M8 12h8 M8 15.5h8',
  // Borrar filtro: embudo con una diagonal que lo tacha (filtro desactivado).
  filterOff: 'M4 5h16l-6 6.5V19l-4-2v-5.5Z M4.5 4.5l15 15',
  /* --- Combinación de correspondencia (mail merge) --- */
  // Combinar correspondencia: sobre con la solapa marcada.
  mailMerge: 'M3.5 6.5h17v11h-17Z M4 7l8 6 8-6',
  // Campo de combinación: comillas angulares « » que encierran un hueco (marcador).
  mergeField: 'M10.5 8 7 12l3.5 4 M13.5 8 17 12l-3.5 4',
  // Previsualizar: ojo con pupila (ver el resultado por registro).
  preview:
    'M2.5 12c2.4-4 5.7-6 9.5-6s7.1 2 9.5 6c-2.4 4-5.7 6-9.5 6s-7.1-2-9.5-6Z M12 15a3 3 0 1 1 0-6 3 3 0 0 1 0 6Z',
  // Navegación: siguiente registro (compañero de chevronLeft).
  chevronRight: 'M8.5 5l7 7-7 7',
  /* --- Macros (grabar/reproducir) --- */
  // Grabar: anillo con núcleo (lenguaje universal de «REC»).
  record: 'M12 4.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 1 0 0-15Z M12 9a3 3 0 1 0 0 6 3 3 0 1 0 0-6Z',
  // Detener grabación: cuadrado.
  stopRec: 'M7 7h10v10H7Z',
  // Gestor de macros: lista con un triángulo de reproducción (secuencia ejecutable).
  macro: 'M4 6h9 M4 12h7 M4 18h5 M15 12.5v7l5.5-3.5Z',
  // Reproducir: triángulo de play.
  play: 'M8 5.5v13l11-6.5Z',
  // Renombrar: lápiz.
  pencil:
    'M4.5 19.5 4 20l.5-4L15 5.5a1.8 1.8 0 0 1 2.6 0l.9.9a1.8 1.8 0 0 1 0 2.6L8 19.5Z M13.5 7l3.5 3.5',
  // Nombre definido: etiqueta colgante con su ojal y una línea de texto.
  definedName: 'M3.5 12 8.5 6.5h12v11h-12Z M7.5 12h.01 M11.5 12h5.5',
  /* --- Asistente de IA --- */
  // Destellos: una estrella de cuatro puntas grande + otra pequeña (lenguaje
  // universal de «IA/magia», sin caer en el logotipo de nadie: trazado propio).
  sparkles:
    'M10.5 5.5 12.1 10.4 17 12 12.1 13.6 10.5 18.5 8.9 13.6 4 12 8.9 10.4Z M17.5 4 18.1 5.9 20 6.5 18.1 7.1 17.5 9 16.9 7.1 15 6.5 16.9 5.9Z',
} as const;

/** Unión exacta de §9 (46 nombres) + extras aditivos; derivada de PATHS para no divergir. */
export type EditorIconName = keyof typeof PATHS;

/** Presets de tamaño (px cuadrados), no px libres: sm/md para paneles densos y
 *  menús, lg (por defecto) para el ribbon. Un mismo glifo puede aparecer a varias
 *  tallas según el contexto: es lo normal (stroke fijo, estilo Feather). */
const ICON_SIZE = { sm: 14, md: 16, lg: 18 } as const;

export interface EditorIconProps {
  name: EditorIconName;
  /** Preset de tamaño ('sm' 14 · 'md' 16 · 'lg' 18, por defecto) — SIEMPRE en el
   *  cromo. Se admiten px libres solo para un icono proporcional a su caja (p. ej.
   *  el overlay de multimedia que escala con la diapositiva), no para el cromo. */
  size?: keyof typeof ICON_SIZE | number;
  /** Etiqueta accesible; si se omite, el icono es decorativo (aria-hidden). */
  label?: string;
}

export function EditorIcon({ name, size = 'lg', label }: EditorIconProps): JSX.Element {
  const px = typeof size === 'number' ? size : ICON_SIZE[size];
  return <Glifo d={PATHS[name]} size={px} label={label} />;
}
