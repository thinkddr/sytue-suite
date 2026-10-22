/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Juego propio de iconos de las webs (ICONOS del IR): trazos sobre una cuadrícula de 24 × 24, para
 * pintarlos en línea con `stroke="currentColor"`. Solo `<path d>`: es lo que admite la lista de SVG
 * permitidos de la publicación.
 */

import type { ICONOS } from '../ir/tipos';

export type NombreIcono = (typeof ICONOS)[number];

export const TRAZOS: Record<NombreIcono, string> = {
  phone:
    'M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2z',
  mail: 'M3 6h18v12H3zM3 7l9 7 9-7',
  'map-pin':
    'M12 21s7-6.2 7-11a7 7 0 0 0-14 0c0 4.8 7 11 7 11zM12 12a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z',
  clock: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 7v5l3 2',
  calendar: 'M4 6h16v14H4zM4 10h16M8 3v4M16 3v4',
  check: 'M5 12l5 5 9-10',
  star: 'M12 3l2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1L3.2 9.5l6.1-.9z',
  heart: 'M12 20s-8-5-8-11a4.5 4.5 0 0 1 8-2.5A4.5 4.5 0 0 1 20 9c0 6-8 11-8 11z',
  cart: 'M3 4h3l2.5 11h9.5l2-8H7M10 20h.01M17 20h.01',
  bag: 'M5 8h14l-1 12H6zM9 8V6a3 3 0 0 1 6 0v2',
  truck:
    'M3 6h11v10H3zM14 10h4l3 3v3h-7M5.5 16a2 2 0 1 0 4 0 2 2 0 0 0-4 0M14.5 16a2 2 0 1 0 4 0 2 2 0 0 0-4 0',
  gift: 'M4 11h16v9H4zM3 8h18v3H3zM12 8v12M12 8c-2-4-6-3-4.5-.5M12 8c2-4 6-3 4.5-.5',
  tag: 'M3 12V4h8l10 10-8 8zM7.5 8h.01',
  user: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0',
  users:
    'M9 11a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7zM2 20a7 7 0 0 1 14 0M16 4.5a3.5 3.5 0 0 1 0 6.5M18 14a7 7 0 0 1 4 6',
  building: 'M5 21V4h9v17M14 9h5v12M8 8h2M8 12h2M8 16h2M3 21h18',
  home: 'M3 11l9-7 9 7M5 10v10h14V10M10 20v-6h4v6',
  globe: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18',
  shield: 'M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z',
  leaf: 'M5 19c0-9 5-14 15-14 0 10-5 15-14 15M5 19l8-8',
  wrench: 'M14 6a4 4 0 0 0 5 5l-9 9a2.1 2.1 0 0 1-3-3l9-9a4 4 0 0 0-2-2z',
  hammer: 'M13 4l7 7-3 3-7-7zM12 9L4 17l3 3 8-8',
  stethoscope:
    'M6 3v6a4 4 0 0 0 8 0V3M10 13v2a5 5 0 0 0 10 0v-2M19 10.5a1.5 1.5 0 1 0 2 0 1.5 1.5 0 0 0-2 0',
  bed: 'M3 18V7M3 14h18v4M21 14v-3a3 3 0 0 0-3-3h-7v6M6 11.5a1.5 1.5 0 1 0 3 0 1.5 1.5 0 0 0-3 0',
  utensils: 'M7 3v8M5 3v5a2 2 0 0 0 4 0V3M7 11v10M17 21V3c-3 1-4 5-4 9h4',
  graduation: 'M2 9l10-5 10 5-10 5zM6 11v5c3 3 9 3 12 0v-5',
  briefcase: 'M3 8h18v12H3zM9 8V5h6v3M3 13h18',
  camera: 'M3 8h4l2-3h6l2 3h4v12H3zM12 17a4 4 0 1 0 0-8 4 4 0 0 0 0 8z',
  image: 'M3 5h18v14H3zM3 16l5-5 4 4 3-3 6 6M7.5 9.5a1 1 0 1 0 2 0 1 1 0 0 0-2 0',
  play: 'M8 5v14l11-7z',
  quote: 'M4 18v-6c0-4 2-6 5-7M14 18v-6c0-4 2-6 5-7M4 18h5v-5H4M14 18h5v-5h-5',
  chat: 'M4 5h16v11H9l-5 4z',
  'arrow-right': 'M4 12h16M14 6l6 6-6 6',
  'arrow-left': 'M20 12H4M10 6l-6 6 6 6',
  'chevron-down': 'M6 9l6 6 6-6',
  menu: 'M4 7h16M4 12h16M4 17h16',
  close: 'M6 6l12 12M18 6L6 18',
  plus: 'M12 5v14M5 12h14',
  minus: 'M5 12h14',
  external: 'M14 4h6v6M20 4l-9 9M18 14v6H4V6h6',
  download: 'M12 4v11M7 11l5 5 5-5M4 20h16',
  info: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 11v6M12 7.5h.01',
  whatsapp:
    'M4 20l1.3-4.2A8 8 0 1 1 8.4 18.8zM9 9c0 3 3 6 6 6l1-2-2-1-1 1c-1-.5-2-1.5-2.5-2.5l1-1-1-2z',
  instagram: 'M4 4h16v16H4zM12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM17 7h.01',
  facebook:
    'M14 21v-8h3l.5-3.5H14V7.5c0-1 .5-1.5 1.7-1.5H18V3a20 20 0 0 0-3-.2C12.5 2.8 11 4.3 11 7v2.5H8V13h3v8z',
  linkedin:
    'M4 9h4v11H4zM5.5 5a1 1 0 1 0 2 0 1 1 0 0 0-2 0M10 9h4v1.5c1-2 6-2.5 6 2.5v7h-4v-6c0-2-3-2-3 0v6h-3z',
  tiktok: 'M14 3v12a4 4 0 1 1-4-4M14 3c0 3 2 5 5 5',
  x: 'M5 5l14 14M19 5L5 19',
  youtube: 'M3 7a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2zM10 9l5 3-5 3z',
};
