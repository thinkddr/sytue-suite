/**
 * Glifos SVG locales del triaje de Sytue Mail (estrella, archivar, no leído,
 * carpeta…). Aquí vive solo el DIBUJO de lo que el catálogo común (`Icon` de
 * @sytue/ui) no tiene; el envoltorio es `Glifo`, el de toda la suite.
 *
 * UN DIBUJO POR CONCEPTO. La papelera, la descarga, el enlace, deshacer o la
 * negrita se dibujaban aquí OTRA VEZ con un trazado distinto del común, así que la
 * misma acción se veía distinta en Mail que en Drive o en Notepads. Ahora esos
 * nombres se piden igual (`<Glyph name="trash" />`) pero los pinta `Icon`, y el
 * guardián `glifos-al-comun.test.ts` de @sytue/ui impide que vuelvan a esta tabla.
 */
import type { JSX } from 'preact';
import { Glifo, Icon, type IconName } from '@sytue/ui';

const PATHS = {
  star: 'M12 3.6L14.4 9.3L20.6 9.8L15.9 13.9L17.3 19.9L12 16.7L6.7 19.9L8.1 13.9L3.4 9.8L9.6 9.3Z',
  archive: 'M3 4h18v5H3zM5 9v10a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9M10 13h4',
  unread: 'M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Zm-1 2 9 6 9-6',
  read: 'M3 10 12 3l9 7v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1Zm18 0-9 6-9-6',
  folder:
    'M3.5 6.5A1.5 1.5 0 0 1 5 5h4.4l2 2.5H19a1.5 1.5 0 0 1 1.5 1.5v9A1.5 1.5 0 0 1 19 19.5H5a1.5 1.5 0 0 1-1.5-1.5ZM3.5 10.5h17',
  // Responder/reenviar: punta + curva que da la vuelta, el gesto reconocible. Los
  // anteriores dibujaban una CAJA rectangular con una punta pegada al vértice y
  // no leían como flecha: se veía un cuadrado colgando del ángulo.
  reply: 'M9.5 6 4.5 11l5 5M4.5 11H14a5.5 5.5 0 0 1 5.5 5.5V18',
  replyAll: 'M8 6 3 11l5 5M12.5 6l-5 5 5 5M7.5 11H15a5 5 0 0 1 5 5v2',
  forward: 'M14.5 6l5 5-5 5M19.5 11H10a5.5 5.5 0 0 0-5.5 5.5V18',
  // Barra de formato del compositor rich-text (lo que no está en el común).
  bulletList: 'M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01',
  numberList: 'M10 6h10M10 12h10M10 18h10M4 5h1v4M4 9h2M4 14h2v1l-2 2v1h2',
  clearFormat: 'M6 5h12M9 5 7 19M12 19h7M15 11l6 6M21 11l-6 6',
  alignLeft: 'M4 6h16M4 10h10M4 14h16M4 18h10',
  alignCenter: 'M4 6h16M7 10h10M4 14h16M7 18h10',
  alignRight: 'M4 6h16M10 10h10M4 14h16M10 18h10',
  alignJustify: 'M4 6h16M4 10h16M4 14h16M4 18h16',
  indent: 'M11 6h9M11 12h9M11 18h9M4 9l3 3-3 3',
  outdent: 'M11 6h9M11 12h9M11 18h9M7 9l-3 3 3 3',
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
  rule: 'M3 12h18',
  textColor: 'M5 20h14M8 16 12 5l4 11M9.5 13h5',
  // Campana tachada: silenciar una conversación.
  mute: 'M18 8.5a6 6 0 1 0-12 0c0 5.5-2.2 7-2.2 7h16.4s-2.2-1.5-2.2-7ZM10.3 20a2 2 0 0 0 3.4 0M4 3l16 18',
  // Clip: la fila de la lista dice «trae adjunto» con el símbolo que usa todo el
  // correo, no con una palabra. Antes era un Badge de texto en negrita, o sea el
  // elemento con MÁS peso visual de la fila puesto al dato menos importante, y en
  // francés («Pièce jointe») le robaba al asunto media columna.
  paperclip: 'M6.75 10v5a5.25 5.25 0 0 0 10.5 0V7a3.5 3.5 0 0 0-7 0v8.5a1.75 1.75 0 0 0 3.5 0V9',
  // Bandeja con la flecha que sale: la «Bandeja de salida» de Outlook (rol `outbox`).
  // Con la carpeta neutra se leía como una carpeta propia más.
  outbox: 'M3 13h5l1.5 3h5l1.5-3h5M3 13v6a1 1 0 0 0 1 1h16a1 1 0 0 0 1-1v-6M12 3v9M8 7l4-4 4 4',
  // Prohibido (círculo + barra): marcar como spam / no es spam.
  spam: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18M5.64 5.64l12.72 12.72',
  // Sobre tachado: darse de baja de una lista (List-Unsubscribe).
  unsubscribe:
    'M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Zm-1 2 9 6 9-6M4 4l16 16',
  // Impresora: bandeja de papel arriba, cuerpo y la hoja que sale (imprimir el hilo).
  print:
    'M6 9V3h12v6M6 18H4a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2h-2M6 14h12v7H6z',
  // Embudo: «Opciones de búsqueda», el panel que acota la búsqueda por campos. Es el
  // `filter` de la barra del editor, el mismo dibujo para la misma idea.
  filter: 'M4 5h16l-6 6.5V19l-4-2v-5.5Z',
} as const;

/** Los nombres de Mail que dibuja el catálogo común. */
const DEL_COMUN = [
  'trash',
  'download',
  'bold',
  'italic',
  'underline',
  'strikethrough',
  'link',
  'quote',
  'table',
  'image',
  'undo',
  'redo',
  'clock',
  'tag',
] as const satisfies readonly IconName[];

type Propio = keyof typeof PATHS;
export type GlyphName = Propio | (typeof DEL_COMUN)[number];

function esPropio(name: GlyphName): name is Propio {
  return Object.hasOwn(PATHS, name);
}

export interface GlyphProps {
  name: GlyphName;
  /** Rellena la forma (estrella activa). */
  filled?: boolean;
  size?: number;
  class?: string;
}

/** Glifo decorativo (aria-hidden): el botón que lo contiene lleva el aria-label. */
export function Glyph({ name, filled = false, size = 16, class: cls }: GlyphProps): JSX.Element {
  const fill = filled ? 'currentColor' : undefined;
  return esPropio(name) ? (
    <Glifo d={PATHS[name]} size={size} class={cls} fill={fill} />
  ) : (
    <Icon name={name} size={size} class={cls} fill={fill} />
  );
}
