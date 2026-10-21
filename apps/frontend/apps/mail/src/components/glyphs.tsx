/**
 * Glifos SVG locales del triaje de Sytue Mail (estrella, archivar, papelera,
 * no leído, carpeta). No existen en el set Icon de @sytue/ui y ese paquete es
 * compartido: aquí vive el DIBUJO, y el envoltorio es `Glifo`, el de toda la
 * suite (misma rejilla, mismo trazo y mismo relleno que Icon).
 */
import type { JSX } from 'preact';
import { Glifo } from '@sytue/ui';

const PATHS = {
  star: 'm12 2.8 2.9 5.9 6.5 1-4.7 4.6 1.1 6.4L12 17.7l-5.8 3 1.1-6.4-4.7-4.6 6.5-1L12 2.8Z',
  archive: 'M3 4h18v5H3zM5 9v10a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9M10 13h4',
  trash: 'M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6',
  unread: 'M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Zm-1 2 9 6 9-6',
  read: 'M3 10 12 3l9 7v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1Zm18 0-9 6-9-6',
  folder: 'M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2Z',
  // Responder/reenviar: punta + curva que da la vuelta, el gesto reconocible. Los
  // anteriores dibujaban una CAJA rectangular con una punta pegada al vértice y
  // no leían como flecha: se veía un cuadrado colgando del ángulo.
  reply: 'M9 17 4 12l5-5M4 12h9a5 5 0 0 1 5 5v2',
  replyAll: 'M7 17 2 12l5-5M12 17 7 12l5-5M7 12h8a5 5 0 0 1 5 5v2',
  forward: 'm15 17 5-5-5-5M20 12h-9a5 5 0 0 0-5 5v2',
  download: 'M12 3v12M7 10l5 5 5-5M5 21h14',
  // Barra de formato del compositor rich-text.
  bold: 'M7 5h6a3.5 3.5 0 0 1 0 7H7zM7 12h7a3.5 3.5 0 0 1 0 7H7z',
  italic: 'M19 5h-7M12 19H5M15 5 9 19',
  underline: 'M7 4v7a5 5 0 0 0 10 0V4M5 21h14',
  bulletList: 'M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01',
  numberList: 'M10 6h10M10 12h10M10 18h10M4 5h1v4M4 9h2M4 14h2v1l-2 2v1h2',
  link: 'M9 15l6-6M10 6l1-1a4 4 0 0 1 6 6l-1 1M14 18l-1 1a4 4 0 0 1-6-6l1-1',
  clearFormat: 'M6 5h12M9 5 7 19M12 19h7M15 11l6 6M21 11l-6 6',
  // Asistente: la chispa de cuatro puntas, que es lo que la suite entera usa
  // para «esto lo ha escrito un modelo». Dos tamaños para que no parezca un
  // asterisco.
  // Barra de formato ampliada (paridad con Outlook/eM Client).
  strikethrough: 'M5 12h14M7 7a4 3 0 0 1 4-3h2a4 3 0 0 1 4 3M7 16a4 3 0 0 0 4 3h2a4 3 0 0 0 4-3',
  alignLeft: 'M4 6h16M4 10h10M4 14h16M4 18h10',
  alignCenter: 'M4 6h16M7 10h10M4 14h16M7 18h10',
  alignRight: 'M4 6h16M10 10h10M4 14h16M10 18h10',
  alignJustify: 'M4 6h16M4 10h16M4 14h16M4 18h16',
  indent: 'M11 6h9M11 12h9M11 18h9M4 9l3 3-3 3',
  outdent: 'M11 6h9M11 12h9M11 18h9M7 9l-3 3 3 3',
  quote: 'M7 7H4v5h5V8a4 4 0 0 0-2-1M18 7h-3v5h5V8a4 4 0 0 0-2-1',
  table: 'M3 5h18v14H3zM3 10h18M3 15h18M9 5v14M15 5v14',
  // Edición de tabla. La rejilla queda en 3×2 y la flecha o el aspa dicen QUÉ le
  // pasa: una fila arriba, una columna a la derecha, esto se va. Con el mismo
  // dibujo para las cuatro inserciones no se distinguirían a tamaño 16.
  rowAbove: 'M3 11h18v10H3zM3 16h18M9 11v10M15 11v10M12 2v6M9 5l3-3 3 3',
  rowBelow: 'M3 3h18v10H3zM3 8h18M9 3v10M15 3v10M12 22v-6M9 19l3 3 3-3',
  colBefore: 'M11 3h10v18H11zM16 3v18M11 8h10M11 14h10M2 12h6M5 9l-3 3 3 3',
  colAfter: 'M3 3h10v18H3zM8 3v18M3 8h10M3 14h10M22 12h-6M19 9l3 3-3 3',
  rowDelete: 'M3 8h18v8H3zM9 8v8M15 8v8M4 3l4 4M8 3 4 7M16 17l4 4M20 17l-4 4',
  colDelete: 'M8 3v18M8 8h8M8 14h8M16 3v18M2 5l4 4M6 5 2 9M18 15l4 4M22 15l-4 4',
  tableDelete: 'M3 5h18v14H3zM3 10h18M9 5v14M6 8l12 8M18 8 6 16',
  image: 'M3 5h18v14H3zM3 16l5-5 4 4 3-3 6 6',
  rule: 'M3 12h18',
  textColor: 'M5 20h14M8 16 12 5l4 11M9.5 13h5',
  undo: 'M9 14 4 9l5-5M4 9h9a7 7 0 0 1 0 14h-3',
  redo: 'm15 14 5-5-5-5M20 9h-9a7 7 0 0 0 0 14h3',
  // Reloj: programar / deshacer envío (círculo + agujas).
  clock: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18ZM12 8v4l3 2',
  // Etiqueta (tag): aplicar/quitar etiquetas de una conversación.
  tag: 'M20.59 13.41 13.42 20.58a2 2 0 0 1-2.83 0L3 13V4a1 1 0 0 1 1-1h9l7.59 7.59a2 2 0 0 1 0 2.82ZM7.5 7.5h.01',
  // Clip: la fila de la lista dice «trae adjunto» con el símbolo que usa todo el
  // correo, no con una palabra. Antes era un Badge de texto en negrita, o sea el
  // elemento con MÁS peso visual de la fila puesto al dato menos importante, y en
  // francés («Pièce jointe») le robaba al asunto media columna.
  paperclip:
    'm21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l8.57-8.57A4 4 0 1 1 18 8.84l-8.59 8.57a2 2 0 0 1-2.83-2.83l8.49-8.48',
  // Prohibido (círculo + barra): marcar como spam / no es spam.
  spam: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18M5.64 5.64l12.72 12.72',
  // Sobre tachado: darse de baja de una lista (List-Unsubscribe).
  unsubscribe:
    'M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Zm-1 2 9 6 9-6M4 4l16 16',
} as const;

export type GlyphName = keyof typeof PATHS;

export interface GlyphProps {
  name: GlyphName;
  /** Rellena la forma (estrella activa). */
  filled?: boolean;
  size?: number;
  class?: string;
}

/** Glifo decorativo (aria-hidden): el botón que lo contiene lleva el aria-label. */
export function Glyph({ name, filled = false, size = 16, class: cls }: GlyphProps): JSX.Element {
  return (
    <Glifo d={PATHS[name]} size={size} class={cls} fill={filled ? 'currentColor' : undefined} />
  );
}
